package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/skilfoy/ARTEX-English/selfupdate"
)

// Page One Update HTTP Noodles. Real Download/Verification/It's all about changing. selfupdate In the bag.,
// It's only about border control, cross-examination, broadcasting, and..."Time to quit."Tell main.
//
// Restart not completed by this process: save a new version after the process is completed selfupdate.ExitRestart Exit,
// By Guardian Script(start.sh / start.bat,Docker Here we go. ENTRYPOINT)Pull up again..

// restartCh Close after upgrade is ready or roll back,main After receipt ExitRestart Exit.
var (
	restartOnce sync.Once
	restartCh   = make(chan struct{})
)

// RestartRequested Return one in"Please quit and let the daemon pull me back."Closed on time channel.
func RestartRequested() <-chan struct{} { return restartCh }

func requestRestart() { restartOnce.Do(func() { close(restartCh) }) }

// bootState This is when it starts. selfupdate.Bootstrap (upgraded successfully) / Just roll back. /
// Other Organiser main Injecting, for /api/update/check Tell them what happened to the last upgrade..
var (
	bootStateMu sync.Mutex
	bootState   selfupdate.State
)

// SetBootUpdateState By main Call once on startup.
func SetBootUpdateState(st selfupdate.State) {
	bootStateMu.Lock()
	defer bootStateMu.Unlock()
	bootState = st
}

func bootUpdateState() selfupdate.State {
	bootStateMu.Lock()
	defer bootStateMu.Unlock()
	return bootState
}

// releaseCache Cache GitHub Other Organiser.
//
// top bar"There is a new version"The hint is checked once every full page load without authentication GitHub API Yes
// each IP Hourly 60 times——If you don't slow down, open more tabs or brush more pages and run out the quota.,
// It's hard to find out when I really want to update. User Visibility Point"Check for updates"Time is good. force Cache around.
type releaseCache struct {
	mu  sync.Mutex
	rel *selfupdate.Release
	err error
	at  time.Time
	// fetch is the numbering function, only the injection point left for the test; nil It's time to go real. GitHub Query.
	fetch func(context.Context, *http.Client) (*selfupdate.Release, error)
}

const (
	releaseTTL = 30 * time.Minute
	// If you fail, you'll have to wait a while. GitHub Every time you can't reach a page, you'll have to wait for a timeout.;
	// But... TTL Short. The network will be back on its own soon. Okay..
	releaseErrTTL = 2 * time.Minute
	// Timeout for queries.NewClient of 30 The minutes are for downloading the whole package. Long.
	releaseTimeout = 20 * time.Second
)

var relCache = &releaseCache{}

// get Return Update Release,Cache on impact does not access the network.
//
// Always holding locks during the countout: Queue requests will match the results of the same query, instead of fighting separately GitHub
// (Multiple tabs were checked when the page was loaded at the same time, which is the easiest time to trigger the limit.).
func (c *releaseCache) get(ctx context.Context, client *http.Client, force bool) (*selfupdate.Release, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !force {
		ttl := releaseTTL
		if c.err != nil {
			ttl = releaseErrTTL
		}
		if !c.at.IsZero() && time.Since(c.at) < ttl {
			return c.rel, c.err
		}
	}

	fetch := c.fetch
	if fetch == nil {
		fetch = selfupdate.FetchLatest
	}
	ctx, cancel := context.WithTimeout(ctx, releaseTimeout)
	defer cancel()
	rel, err := fetch(ctx, client)
	// Cancelled (user closes tab) does not represent GitHub There's a problem. Don't put it in the cache.,
	// Otherwise, the next visitor will get a weird one."Cancelled"Error.
	if err != nil && ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
		return c.rel, err
	}
	c.rel, c.err, c.at = rel, err, time.Now()
	return rel, err
}

// updateProgress It's a step forward..
type updateProgress struct {
	Phase   selfupdate.Phase `json:"phase"`
	Percent int              `json:"percent"` // Only download stages are meaningful; the rest are -1
	Message string           `json:"message"`
	Version string           `json:"version,omitempty"`
	Error   string           `json:"error,omitempty"`
}

// updateHub Holds an upgrade and broadcasts to SSE Subscriptions.
//
// running At the same time, acting as a mutually exclusive: again during promotion POST /api/update/apply Direct 409,
// Avoid two. goroutine At the same time. artex.new Write.
type updateHub struct {
	mu      sync.Mutex
	running bool
	cur     updateProgress
	subs    map[chan updateProgress]struct{}
}

var updHub = &updateHub{
	cur:  updateProgress{Phase: selfupdate.PhaseIdle, Percent: -1},
	subs: map[chan updateProgress]struct{}{},
}

// begin Seize upgrades, return in progress false.
func (h *updateHub) begin(version string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		return false
	}
	h.running = true
	h.cur = updateProgress{Phase: selfupdate.PhaseDownload, Percent: 0, Message: "Preparing…", Version: version}
	h.fanout(h.cur)
	return true
}

// finish End one upgrade.err for nil Express suspense successful until restart.
func (h *updateHub) finish(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = false
	if err != nil {
		h.cur = updateProgress{Phase: selfupdate.PhaseFailed, Percent: -1, Message: "Update failed", Error: err.Error(), Version: h.cur.Version}
	} else {
		h.cur = updateProgress{Phase: selfupdate.PhaseStaged, Percent: 100, Message: "New version ready, restarting.…", Version: h.cur.Version}
	}
	h.fanout(h.cur)
}

func (h *updateHub) publish(ph selfupdate.Phase, pct int, msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cur = updateProgress{Phase: ph, Percent: pct, Message: msg, Version: h.cur.Version}
	h.fanout(h.cur)
}

// fanout Must hold h.mu . Subscriptions channel There's a buffer. If it's full, throw it away.——
// Progress is a momentary information that can be discarded. SSE Connection block upgrade itself.
func (h *updateHub) fanout(p updateProgress) {
	for ch := range h.subs {
		select {
		case ch <- p:
		default:
		}
	}
}

func (h *updateHub) snapshot() (updateProgress, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cur, h.running
}

func (h *updateHub) subscribe() (<-chan updateProgress, func()) {
	ch := make(chan updateProgress, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
}

// updateCheck Query GitHub the most recent official version and comparison with the current version.
//
// It'll be straight in front. api.github.com(GitHub of CORS Yes *),But...**Based on this interface**:
// Downloads are done at the back end. Only the back end can access them. GitHub The browser can't connect to the server.
// It's very common (server inside, or proxy on browsers only), when a point update is bound to fail.,
// Why don't you tell the truth about this?.
func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	current := BuildVersion
	mode := "binary"
	if selfupdate.InDocker() {
		mode = "docker"
	}
	boot := bootUpdateState()
	out := map[string]any{
		"current":     current,
		"mode":        mode,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"has_backup":  selfupdate.HasBackup(),
		"repo":        selfupdate.Repo,
		"boot_notice": boot.Detail,
		"rolled_back": boot.RolledBack,
	}

	// Topbar Cache (default); user points"Check for updates"Timeband force=1 Forced return to source.
	force := r.URL.Query().Get("force") != ""
	client := selfupdate.NewClient(s.m.GlobalProxy())
	rel, err := relCache.get(r.Context(), client, force)
	if err != nil {
		out["error"] = err.Error()
		writeJSON(w, 200, out)
		return
	}

	latest := rel.TagName
	out["latest"] = latest
	out["notes"] = rel.Body
	out["html_url"] = rel.HTMLURL
	if !rel.PublishedAt.IsZero() {
		out["published_at"] = rel.PublishedAt.Format(time.RFC3339)
	}

	asset := selfupdate.AssetName(latest, runtime.GOOS, runtime.GOARCH)
	out["asset"] = asset
	if a, ok := rel.FindAsset(asset); ok {
		out["asset_available"] = true
		out["size"] = a.Size
	} else {
		out["asset_available"] = false
	}

	cmp, comparable := selfupdate.CompareVersions(current, latest)
	out["comparable"] = comparable
	out["has_update"] = comparable && cmp < 0
	if !comparable {
		// Develop Build(dev / git describe No comparable version number. Just let go.
		// Override local debugging binaries with an official version, so do not update them directly.
		out["reason"] = fmt.Sprintf("Current version %q Not officially released, one key update disabled", current)
	}
	writeJSON(w, 200, out)
}

// updateApply Download and save a new version, and then get the process out to the Guardian script to restart.
//
// Return immediately. 202,The actual work is backstage. goroutine Up and running: The whole package may take a few minutes to download.,
// Hanging on a request can be cut off by an inverse timeout. Let's go. /api/update/stream.
func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	current := BuildVersion

	// Cache: Make sure it's the version the user saw and confirmed on the interface..
	client := selfupdate.NewClient(s.m.GlobalProxy())
	rel, err := relCache.get(r.Context(), client, false)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	cmp, comparable := selfupdate.CompareVersions(current, rel.TagName)
	if !comparable {
		writeErr(w, 400, fmt.Sprintf("Current version %q Not officially released, one key update disabled", current))
		return
	}
	if cmp >= 0 {
		writeErr(w, 400, fmt.Sprintf("This is the latest version %s", current))
		return
	}
	if !updHub.begin(rel.TagName) {
		writeErr(w, 409, "An update is in progress")
		return
	}

	go func() {
		// Use it deliberately. s.ctx Not the request. ctx:HTTP It's over as soon as we get back.,
		// If you hang on to it, the download will be canceled immediately..
		err := selfupdate.Stage(s.ctx, client, rel, current, func(ph selfupdate.Phase, pct int, msg string) {
			updHub.publish(ph, pct, msg)
		})
		updHub.finish(err)
		if err != nil {
			log.Printf("[update] Update failed:%v", err)
			return
		}
		log.Printf("[update] %s → %s Saved pending exit to complete replacement", current, rel.TagName)
		// Leave some time to push the last progress to the front and trigger the exit..
		time.Sleep(1500 * time.Millisecond)
		requestRestart()
	}()

	writeJSON(w, 202, map[string]any{"ok": true, "target": rel.TagName})
}

// updateRollback Actively return the previous version (backup before reloading) artex.old).
func (s *Server) updateRollback(w http.ResponseWriter, r *http.Request) {
	if _, running := updHub.snapshot(); running {
		writeErr(w, 409, "Update is ongoing and cannot roll back")
		return
	}
	if err := selfupdate.Rollback(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	log.Printf("[update] Manually roll back to the previous version, about to exit to complete the switch")
	writeJSON(w, 202, map[string]any{"ok": true})
	go func() {
		time.Sleep(500 * time.Millisecond)
		requestRestart()
	}()
}

// updateStream With SSE Send Update Progress.
func (s *Server) updateStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := updHub.subscribe()
	defer unsub()

	send := func(p updateProgress) {
		b, _ := json.Marshal(p)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}
	// Add a current status and you can see an ongoing upgrade as soon as the page is refreshed..
	cur, _ := updHub.snapshot()
	send(cur)

	ctx := r.Context()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case p, ok := <-ch:
			if !ok {
				return
			}
			send(p)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
