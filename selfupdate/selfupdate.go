// Package selfupdate implements ARTEX's one-click in-app update: it pulls a new
// binary from a GitHub Release, verifies it, stages it, and swaps it in atomically
// on the next start.
//
// Division of work (see start.sh / start.bat):
//
//	start script = a dumb supervisor loop; after the process exits it only decides
//	               whether to relaunch, based on the exit code
//	this package = every fallible step (download / SHA256 check / smoke test /
//	               swap / rollback)
//
// The swap lives in Go rather than the scripts because the SHA256 check and the
// smoke test would otherwise have to be written twice (sha256sum / shasum /
// certutil), and that is the step that must not go wrong. If a binary that cannot
// run is installed, the supervisor will faithfully keep relaunching it, and the
// only recovery is a manual fix on the machine.
//
// One complete upgrade spans three process starts:
//
//	① the old server receives /api/update/apply → download and verify → stage artex.new → exit 75
//	② the script relaunches the old binary → Bootstrap finds artex.new → verify + smoke → swap → exit 75
//	③ the script relaunches, now the new binary → Bootstrap records one attempt → the marker is cleared after a successful start
//
// Any failure falls back to the old version: ② deletes the staged files and keeps
// running the old binary if verification fails; ③ automatically restores artex.old
// if the new binary dies 3 times in a row before the marker is cleared.
package selfupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ExitRestart is the "please relaunch me" exit code (EX_TEMPFAIL). The start
// script reruns immediately and does not count it as a crash. 0 means the user
// stopped normally (the script leaves its loop); anything else is a crash.
const ExitRestart = 75

// maxAttempts is how many starts are allowed after a swap. Each start of the new
// binary increments the count by 1. Surviving settleDelay clears the marker;
// crashing maxAttempts times means the new binary cannot stay up, so it is rolled back.
const maxAttempts = 3

// Paths is every file involved in one upgrade, all under the directory that
// contains the executable. The working directory is deliberately not used:
// a service may run with CWD set to / or any other path, and staging files
// there would make the swap logic miss them entirely.
type Paths struct {
	Dir     string // directory containing the executable
	Current string // binary that is running now          artex      / artex.exe
	New     string // staged new version                  artex.new  / artex.new.exe
	Sum     string // sha256 of the new version (hex)     artex.new.sha256 / artex.new.exe.sha256
	Old     string // previous version, backed up before the swap  artex.old  / artex.old.exe
	Marker  string // upgrade state marker                artex.upgrade.json
}

// ResolvePaths derives every upgrade path from the current executable.
//
// On Windows, .new and .old must keep the .exe suffix or the smoke test and the
// swapped binary both fail to run. The suffix is stripped and then reattached so
// the two platforms name files the same way.
func ResolvePaths() (Paths, error) {
	exe, err := os.Executable()
	if err != nil {
		return Paths{}, fmt.Errorf("locate executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	name := filepath.Base(exe)
	ext := filepath.Ext(name) // ".exe" on Windows; usually empty on Unix
	stem := strings.TrimSuffix(name, ext)

	join := func(suffix string) string { return filepath.Join(dir, stem+suffix+ext) }
	return Paths{
		Dir:     dir,
		Current: exe,
		New:     join(".new"),
		Sum:     join(".new") + ".sha256",
		Old:     join(".old"),
		Marker:  filepath.Join(dir, stem+".upgrade.json"),
	}, nil
}

// marker records how far a swap has got, so a new binary that never stays up
// can be rolled back automatically.
type marker struct {
	From     string `json:"from"`     // version before the upgrade
	To       string `json:"to"`       // target version
	Attempts int    `json:"attempts"` // starts attempted since the swap
	StagedAt int64  `json:"staged_at"`
}

func readMarker(path string) (marker, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return marker{}, false
	}
	var m marker
	if json.Unmarshal(b, &m) != nil {
		return marker{}, false
	}
	return m, true
}

func writeMarker(path string, m marker) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// cleanStaged removes staged files. A successful swap, a failed verification,
// and a user cancel all go through it, so a leftover artex.new is not retried
// on the next start.
func cleanStaged(p Paths) {
	_ = os.Remove(p.New)
	_ = os.Remove(p.Sum)
}

// CompareVersions compares two versions and returns -1/0/1 (a<b / a==b / a>b).
// ok=false means at least one side is not a comparable version (for example a
// local "dev" build, or git describe output such as "0.3.7-2-gabc1234-dirty").
// The caller should disable one-click update in that case. Otherwise a
// development build would be "upgraded" to a release and uncommitted changes
// would be overwritten.
func CompareVersions(a, b string) (int, bool) {
	av, aok := parseVersion(a)
	bv, bok := parseVersion(b)
	if !aok || !bok {
		return 0, false
	}
	for i := range 3 {
		if av[i] != bv[i] {
			if av[i] < bv[i] {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

// parseVersion parses a "v0.3.7" / "0.3.7" version into [3]int.
//
// Only a plain three-part version is accepted. Off a tag, build.sh uses git
// describe and produces suffixed versions such as "0.3.7-2-gabc1234". Those
// must be treated as incomparable, not as 0.3.7 — otherwise a development build
// is reported as "already up to date" or overwritten by a release.
func parseVersion(s string) ([3]int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return [3]int{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// InDocker reports whether the process is running in a container. A swap under
// Docker writes the container's writable layer; `docker compose up -d` recreates
// the container and returns to the image's own version. That is expected (the
// user is pulling a new image), but the frontend needs to be able to say so.
func InDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	b, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "docker") || strings.Contains(s, "containerd")
}
