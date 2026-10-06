package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/skilfoy/ARTEX-English/db"
	"github.com/skilfoy/ARTEX-English/notify"
)

// Global settings keys (stored in the settings table; no new table).
const (
	// settingNotifyEnabled is the master switch for notifications. It defaults on: a one-click
	// kill switch for maintenance, not the feature gate — the real gate is whether any channel is configured.
	settingNotifyEnabled = "notify_enabled"
	// settingNotifyPublicBaseURL is the external base URL used to build finding-detail links
	// (for example https://artex.example.com). Leave it empty and messages omit the link button.
	// Nothing else in the project provides a reusable external address, so this setting is new.
	settingNotifyPublicBaseURL = "notify_public_base_url"
	// settingNotifyDigestMinutes is the digest period, in minutes.
	settingNotifyDigestMinutes = "notify_digest_interval_min"
)

const (
	// notifyTick is the delivery engine's poll interval. Three seconds is its real-time ceiling,
	// and the main delay between "finding stored" and "message arrives in IM".
	notifyTick = 3 * time.Second
	// notifyLease is the lease taken when a delivery is claimed. It must be well above the worst
	// case for one send (the notify package's HTTP client times out at 15s), or the same row can
	// be delivered by two dispatchers at once.
	notifyLease = 3 * time.Minute
	// notifyFanOutPerTick caps how many events are dispatched per round, so the first time a
	// channel is enabled the historical backlog is not expanded into delivery jobs all at once.
	notifyFanOutPerTick = 200
	// notifyDefaultDigestMinutes is the default digest period.
	notifyDefaultDigestMinutes = 30
	// notifyUnlimitedBurstPerTick is the per-round send cap when a channel has no rate limit.
	// It stops "one unlimited channel plus a scan that emits thousands of findings" from
	// stalling the loop for a long time.
	notifyUnlimitedBurstPerTick = 50
	// notifyMaxSendsPerChannelPerTick is how many deliveries one channel may send per round.
	//
	// That cap is derived from the lease: claiming a row stamps a lease (notifyLease = 3 minutes).
	// If one round serially sends so many that the worst case exceeds the lease, later rows expire
	// before they are sent. Inside one process that does not matter (Run is a single goroutine and
	// a tick does not re-enter), but when two processes share one database the other side reclaims
	// expired rows, sends them again, double-increments attempts, and marks them failed while this process is still sending.
	//
	// Sizing: a 3-minute lease / 30-second send timeout = 6 uses the lease exactly, with no slack,
	// so it is not usable. 5 keeps the worst case at 150 seconds and leaves 30 seconds of slack. This
	// relationship is pinned by TestNotifyTickBudgetFitsWithinLease — changing notifyLease,
	// notifySendTimeout, or this value fails that assertion.
	notifyMaxSendsPerChannelPerTick = 5
	// notifySendTimeout is the timeout for one delivery. It also sets the constant above:
	// their product must not exceed notifyLease. See TestNotifyTickBudgetFitsWithinLease.
	notifySendTimeout = 30 * time.Second
)

// notifyBackoff is the failure-retry backoff; the index is how many attempts have already been made.
// Three chances (including the first try) match db.MaxNotifyAttempts; change them together.
var notifyBackoff = []time.Duration{
	time.Second,
	5 * time.Second,
	30 * time.Second,
}

// Notifier is the delivery engine for finding notifications.
//
// It runs as its own goroutine beside the Scheduler (see server.New). It deliberately does not
// share the Scheduler tick: push wants 3-second latency, a different cadence from triggers,
// and the two must not fail together — a stuck push must not block agent triggers.
type Notifier struct {
	s  *Server
	pg *db.DB

	// mu protects buckets. There are few channels and little contention, so one mutex is enough;
	// a finer structure is not worth it.
	mu      sync.Mutex
	buckets map[int64]*notifyBucket
}

// notifyBucket is one channel's token bucket.
//
// A token bucket, not a "count for a minute, then reset" sliding window: the window's edge
// effect is bad. Filling 20 at the end of the window and 20 again a moment later is 40 in one
// second to the platform, and it gets rate-limited. A token bucket refills at a constant rate and avoids that burst.
type notifyBucket struct {
	tokens   float64
	lastFill time.Time
}

func newNotifier(s *Server) *Notifier {
	return &Notifier{s: s, pg: s.m.pg, buckets: map[int64]*notifyBucket{}}
}

// Run loops until ctx ends. server.New starts it once.
func (n *Notifier) Run(ctx context.Context) {
	if n.pg == nil {
		return
	}
	t := time.NewTicker(notifyTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n.step(ctx)
		}
	}
}

// step runs one round: dispatch new events, then deliver due jobs.
//
// A failed step is only logged and does not stop the loop — a notification failure must never
// become a process-level problem. Each tick is independent; the next round retries.
func (n *Notifier) step(ctx context.Context) {
	if !n.enabled() {
		return
	}
	if _, _, err := n.pg.FanOutPendingEvents(ctx, notifyFanOutPerTick); err != nil {
		log.Printf("[notify] failed to dispatch events: %v", err)
		return
	}
	channels, err := n.pg.ListNotificationChannels(ctx)
	if err != nil {
		log.Printf("[notify] failed to load channels: %v", err)
		return
	}
	baseURL := n.publicBaseURL()
	for _, ch := range channels {
		if !ch.IsEnabled() {
			continue
		}
		// The token bucket counts messages (HTTP requests), not findings.
		// In realtime mode they match (one finding, one message). In digest mode a whole batch of
		// findings becomes one message and costs one token.
		//
		// Both modes ask the token bucket first, then claim up to that budget. Do not reverse the order,
		// or a delivery blocked by the rate limit would already have spent a retry.
		now := time.Now()
		if ch.Mode == db.NotifyModeDigest {
			tokens, claimLimit := digestTickPlan()
			if n.takeTokens(ch.ID, ch.RatePerMin, tokens, now) <= 0 {
				continue
			}
			n.stepDigest(ctx, ch, claimLimit, baseURL)
			continue
		}
		allow := n.takeTokens(ch.ID, ch.RatePerMin, notifyMaxSendsPerChannelPerTick, now)
		if allow <= 0 {
			continue
		}
		n.stepRealtime(ctx, ch, allow, baseURL)
	}
}

// digestTickPlan returns this digest channel's token cost and the batch-size cap for this round.
//
// The two return values are different units, which is why this is its own function:
//
//   - tokens is the message count. A batch of findings is one message and one HTTP request, so it is always 1.
//     rate_per_min therefore still applies to digests (at most that many digest messages per minute).
//   - claimLimit is how many findings this batch may hold. It is bounded only by memory, not by the request budget.
//
// rate_per_min was once applied to digests by passing the per-round request budget
// (notifyMaxSendsPerChannelPerTick, derived from the lease) straight through as the batch size.
// A channel with rate_per_min=20 then gained only one token in a 3-second tick, so each digest
// message held one finding — digest degraded into "realtime push with digest wording", and readers
// saw a stream of "1 new finding in the last 30 minutes", while db.MaxDigestBatchSize was unreachable.
//
// End-to-end tests miss this easily (they pass a large enough limit into
// stepDigest and skip the budget math in step), so the decision lives here and is pinned by
// TestDigestTickPlanDecouplesBatchSizeFromSendBudget.
func digestTickPlan() (tokens, claimLimit int) {
	return 1, db.MaxDigestBatchSize
}

// stepRealtime claims and delivers one channel's realtime jobs: one finding, one message.
func (n *Notifier) stepRealtime(ctx context.Context, ch *db.NotificationChannel, allow int, baseURL string) {
	deliveries, err := n.pg.ClaimRealtimeDeliveries(ctx, ch.ID, allow, notifyLease)
	if err != nil {
		log.Printf("[notify] failed to claim realtime deliveries channel=%d: %v", ch.ID, err)
		return
	}
	if len(deliveries) == 0 {
		return
	}
	channel, cfg, ok := n.adapt(ch)
	if !ok {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), fmt.Sprintf("channel type %q is not registered", ch.Kind))
		return
	}
	for _, dl := range deliveries {
		msg, err := n.renderSingle(ctx, dl, baseURL)
		if err != nil {
			// A render failure is a local data problem; retrying will not fix it.
			_ = n.pg.FailDeliveries(ctx, []int64{dl.ID}, err.Error())
			continue
		}
		n.send(ctx, channel, cfg, msg, []*db.NotificationDelivery{dl})
	}
}

// stepDigest, when a batch is due, folds a channel's pending deliveries into one message and sends it.
func (n *Notifier) stepDigest(ctx context.Context, ch *db.NotificationChannel, allow int, baseURL string) {
	window := n.digestInterval()
	due, err := n.pg.DigestBatchDue(ctx, ch.ID, window)
	if err != nil {
		log.Printf("[notify] failed to check the digest batch channel=%d: %v", ch.ID, err)
		return
	}
	if !due {
		return
	}
	deliveries, err := n.pg.ClaimDigestBatch(ctx, ch.ID, allow, notifyLease)
	if err != nil {
		log.Printf("[notify] failed to claim the digest batch channel=%d: %v", ch.ID, err)
		return
	}
	if len(deliveries) == 0 {
		return
	}
	channel, cfg, ok := n.adapt(ch)
	if !ok {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), fmt.Sprintf("channel type %q is not registered", ch.Kind))
		return
	}
	msg, included, err := n.renderBatch(ctx, deliveries, baseURL, int(window.Minutes()))
	if err != nil {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), err.Error())
		return
	}
	// Deliveries whose snapshot is corrupt and never entered the message must be failed explicitly.
	// Otherwise they sit outside included, in neither the message nor the failure list. On success
	// the later bulk status update skips them, and they stay in sending until the lease expires and they are claimed again.
	if skipped := excludeDeliveries(deliveries, included); len(skipped) > 0 {
		reason := "event snapshot could not be parsed, so this finding cannot be rendered"
		if fErr := n.pg.FailDeliveries(ctx, deliveryIDs(skipped), reason); fErr != nil {
			log.Printf("[notify] failed to mark bad-snapshot deliveries channel=%s ids=%v: %v", ch.Kind, deliveryIDs(skipped), fErr)
		}
		log.Printf("[notify] skipped %d deliveries with unparseable snapshots channel=%d", len(skipped), ch.ID)
	}
	// Hand only the deliveries that entered the message to send: included[i] matches msg.Items[i],
	// and send uses that to apply "the channel fit the first K items" onto the right rows.
	n.send(ctx, channel, cfg, msg, included)
}

// send delivers and advances status from the result.
//
// One batch (dozens of rows in digest mode) shares one send result: delivered, or the whole batch retries.
// There is no per-item retry — a digest is one message, and resending part of it would break that.
//
// The only exception is a split forced by the channel length limit: the channel reports that only
// the first K items fit, so item K+1 onward must wait for the next batch instead of being marked
// successful with the rest. Otherwise the truncated findings are in neither the message nor the failure list, and they vanish.
func (n *Notifier) send(ctx context.Context, channel notify.Channel, cfg map[string]any, msg notify.Message, deliveries []*db.NotificationDelivery) {
	// Cap one send so a stuck channel cannot hold up every other channel in this round.
	sendCtx, cancel := context.WithTimeout(ctx, notifySendTimeout)
	defer cancel()
	delivered, err := channel.Send(sendCtx, cfg, msg)
	if err == nil && delivered > 0 {
		if delivered > len(deliveries) {
			// A channel cannot report more delivered items than there are deliveries. If it does, the renderer
			// miscounted; treat them all as delivered and log it. That is better than corrupting the rows.
			log.Printf("[notify] channel reported %d delivered, above %d deliveries channel=%s; treating all as delivered",
				delivered, len(deliveries), channel.Kind())
			delivered = len(deliveries)
		}
		sent, rest := deliveries[:delivered], deliveries[delivered:]
		if err := n.pg.MarkDeliveriesSent(ctx, deliveryIDs(sent)); err != nil {
			log.Printf("[notify] failed to mark delivered channel=%s ids=%v: %v", channel.Kind(), deliveryIDs(sent), err)
		}
		if len(rest) > 0 {
			// This message hit the channel length limit: return the rest to the queue now, for the next tick.
			// Use DeferDeliveries, not RescheduleDeliveries — this is not a failure, so it must not spend
			// retry budget (claiming already added 1 optimistically, and that path subtracts it).
			if err := n.pg.DeferDeliveries(ctx, deliveryIDs(rest),
				fmt.Sprintf("message hit the channel length limit; only the first %d items were delivered, the rest wait for the next batch", delivered)); err != nil {
				log.Printf("[notify] failed to requeue the split remainder channel=%s ids=%v: %v", channel.Kind(), deliveryIDs(rest), err)
			}
		}
		return
	}
	if err == nil {
		// The channel neither returned an error nor said how many items were delivered. Treat it as a
		// failure (with backoff) so the delivery is not claimed forever without ever being marked done.
		err = fmt.Errorf("channel did not report how many items were delivered (delivered=%d)", delivered)
	}

	// Failures are decided per row, not from the highest attempt count in the batch.
	//
	// This used to be `if maxAttempts(deliveries) >= MaxNotifyAttempts`, failing the whole batch, but
	// attempt counts differ: an old delivery already retried twice (attempts=2) would drag a brand-new
	// delivery in the same batch (attempts=1) into failed — a new finding dropped forever without a
	// single retry, the opposite of "do not let an old row sink a new one".
	permanent := notify.IsPermanent(err)
	var failIDs, exhaustedIDs []int64
	byDelay := map[time.Duration][]int64{}
	for _, dl := range deliveries {
		switch {
		case permanent:
			failIDs = append(failIDs, dl.ID)
		case dl.Attempts >= db.MaxNotifyAttempts:
			exhaustedIDs = append(exhaustedIDs, dl.ID)
		default:
			delay := notifyBackoff[min(dl.Attempts, len(notifyBackoff)-1)]
			byDelay[delay] = append(byDelay[delay], dl.ID)
		}
	}

	if len(failIDs) > 0 {
		if fErr := n.pg.FailDeliveries(ctx, failIDs, err.Error()); fErr != nil {
			log.Printf("[notify] failed to mark failed channel=%s ids=%v: %v", channel.Kind(), failIDs, fErr)
		}
	}
	if len(exhaustedIDs) > 0 {
		reason := fmt.Sprintf("still failing after %d attempts: %s", db.MaxNotifyAttempts, err)
		if fErr := n.pg.FailDeliveries(ctx, exhaustedIDs, reason); fErr != nil {
			log.Printf("[notify] failed to mark exhausted channel=%s ids=%v: %v", channel.Kind(), exhaustedIDs, fErr)
		}
	}
	// Requeue grouped by delay: there are only 3 backoff slots, so the group count stays small and
	// there is no need for one UPDATE per row (a 500-row batch would otherwise be 500 round trips).
	for delay, group := range byDelay {
		if rErr := n.pg.RescheduleDeliveries(ctx, group, delay, err.Error()); rErr != nil {
			log.Printf("[notify] failed to reschedule deliveries channel=%s ids=%v: %v", channel.Kind(), group, rErr)
		}
	}
	if len(failIDs)+len(exhaustedIDs) > 0 {
		log.Printf("[notify] delivery failed channel=%d kind=%s permanent=%d exhausted=%d retrying=%d: %s",
			deliveries[0].ChannelID, channel.Kind(), len(failIDs), len(exhaustedIDs), len(byDelay), err)
	}
}

// excludeDeliveries returns the rows in all that are not in keep (compared by pointer identity).
// It finds deliveries that "never entered the message" — they must be handled explicitly, not left in limbo.
func excludeDeliveries(all, keep []*db.NotificationDelivery) []*db.NotificationDelivery {
	inKeep := make(map[*db.NotificationDelivery]bool, len(keep))
	for _, dl := range keep {
		inKeep[dl] = true
	}
	var out []*db.NotificationDelivery
	for _, dl := range all {
		if !inKeep[dl] {
			out = append(out, dl)
		}
	}
	return out
}

// adapt loads the channel implementation and parses its config.
// ok=false means the type is not registered; the delivery should fail immediately rather than retry forever.
func (n *Notifier) adapt(ch *db.NotificationChannel) (notify.Channel, map[string]any, bool) {
	channel, ok := notify.Get(ch.Kind)
	if !ok {
		return nil, nil, false
	}
	var cfg map[string]any
	if len(ch.Config) > 0 {
		// A config that fails to parse becomes an empty map: the channel's own Validate then reports which
		// field is missing, which tells the user more than a JSON parse error.
		_ = json.Unmarshal(ch.Config, &cfg)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return channel, cfg, true
}

// renderSingle renders one finding message.
func (n *Notifier) renderSingle(ctx context.Context, dl *db.NotificationDelivery, baseURL string) (notify.Message, error) {
	snap, err := parseSnapshot(dl)
	if err != nil {
		return notify.Message{}, err
	}
	item, err := n.itemFor(ctx, snap, baseURL)
	if err != nil {
		return notify.Message{}, err
	}
	return notify.Message{Items: []notify.Item{item}, HomeURL: baseURL}, nil
}

// renderBatch renders a digest. Snapshots are parsed one by one — one bad snapshot skips only
// that row, and does not drop the whole digest.
//
// The returned included lines up exactly with msg.Items (delivery i ↔ item i).
// That alignment is required: the caller treats "the channel fit the first K items" as "the first
// K deliveries are delivered". If a bad snapshot is skipped here but left in included, the indexes
// shift — a bad entry that should fail is marked delivered, and a good one is marked not delivered.
// The caller marks the bad ones failed explicitly; see stepDigest.
func (n *Notifier) renderBatch(ctx context.Context, deliveries []*db.NotificationDelivery, baseURL string, windowMinutes int) (notify.Message, []*db.NotificationDelivery, error) {
	items := make([]notify.Item, 0, len(deliveries))
	included := make([]*db.NotificationDelivery, 0, len(deliveries))
	for _, dl := range deliveries {
		snap, err := parseSnapshot(dl)
		if err != nil {
			// A bad snapshot enters neither the message nor included. The caller handles it
			// (mark it failed explicitly; do not let it pass as delivered).
			log.Printf("[notify] skipped an unparseable snapshot in a digest batch delivery=%d: %v", dl.ID, err)
			continue
		}
		item, err := n.itemFor(ctx, snap, baseURL)
		if err != nil {
			return notify.Message{}, nil, err
		}
		items = append(items, item)
		included = append(included, dl)
	}
	if len(items) == 0 {
		return notify.Message{}, nil, fmt.Errorf("all %d deliveries in the digest batch failed to parse", len(deliveries))
	}
	return notify.Message{
		Items:         items,
		Batch:         true,
		WindowMinutes: windowMinutes,
		HomeURL:       baseURL,
	}, included, nil
}

// itemFor renders an event snapshot into an item to push, and resolves the asset name and detail link.
func (n *Notifier) itemFor(ctx context.Context, snap notify.Snapshot, baseURL string) (notify.Item, error) {
	assets, err := n.pg.NotificationAssetNames(ctx, snap.AssetIDs)
	if err != nil {
		// Failing to resolve an asset name must not block the push: a missing name is much cheaper than
		// a missing notification. The message is just short one asset line.
		log.Printf("[notify] failed to resolve asset name finding=%d: %v", snap.FindingID, err)
	}
	item := notify.Item{
		FindingID:  snap.FindingID,
		Name:       snap.Name,
		VulnClass:  snap.VulnClass,
		Severity:   snap.Severity,
		Summary:    snap.Summary,
		Assets:     assets,
		FromStatus: snap.FromStatus,
		ToStatus:   snap.ToStatus,
	}
	if baseURL != "" {
		// Detail route: web/src/app/(main)/function/findings/detail/page.tsx,
		// which reads the finding id from the query parameter id.
		item.DetailURL = fmt.Sprintf("%s/function/findings/detail?id=%d", baseURL, snap.FindingID)
	}
	return item, nil
}

// takeTokens removes at most want tokens from the channel bucket and returns how many it took.
//
// One token = one message (one HTTP request). Realtime callers pass how many they want;
// a digest sends one message for the whole batch, so it passes 1.
//
// Bucket capacity is the channel's per-minute limit, refilled at a constant rate. ratePerMin<=0
// means unlimited: return a finite but large value so one round cannot be stuck on an infinite backlog.
//
// The want cap is required. Without it the only option is to drain the bucket, but the caller also
// has a per-round cap, so extra tokens are unused and vanish before the next refill — saved burst
// capacity is unreachable, and even "nothing to send this round" would still spend tokens.
func (n *Notifier) takeTokens(channelID int64, ratePerMin, want int, now time.Time) int {
	if want <= 0 {
		return 0
	}
	if ratePerMin <= 0 {
		return min(want, notifyUnlimitedBurstPerTick)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	b := n.buckets[channelID]
	if b == nil {
		b = &notifyBucket{tokens: float64(ratePerMin), lastFill: now}
		n.buckets[channelID] = b
	}
	// Refill from real elapsed time, at ratePerMin/60 tokens per second.
	if elapsed := now.Sub(b.lastFill).Seconds(); elapsed > 0 {
		b.tokens = minF(float64(ratePerMin), b.tokens+elapsed*float64(ratePerMin)/60)
		b.lastFill = now
	}
	// Add a tiny epsilon before truncating: the token count is a float sum, and filling in two steps
	// can turn 0.5+0.5 into 0.9999999999, which int() truncates to 0 — a mathematically full bucket
	// that yields no token. 1e-9 is far smaller than one token and does not forgive a real shortfall.
	take := min(int(b.tokens+1e-9), want)
	if take <= 0 {
		return 0
	}
	b.tokens -= float64(take)
	return take
}

// enabled reads the master switch.
func (n *Notifier) enabled() bool {
	return n.pg.GetBool(settingNotifyEnabled, true)
}

// publicBaseURL returns the external base URL for links, with a trailing slash removed.
func (n *Notifier) publicBaseURL() string {
	v, ok, err := n.pg.GetSetting(settingNotifyPublicBaseURL)
	if err != nil || !ok {
		return ""
	}
	return trimTrailingSlash(v)
}

// digestInterval returns the digest period, falling back to the default when unset or invalid.
func (n *Notifier) digestInterval() time.Duration {
	v, ok, err := n.pg.GetSetting(settingNotifyDigestMinutes)
	if err != nil || !ok {
		return time.Duration(notifyDefaultDigestMinutes) * time.Minute
	}
	m := 0
	if _, err := fmt.Sscanf(v, "%d", &m); err != nil || m <= 0 {
		return time.Duration(notifyDefaultDigestMinutes) * time.Minute
	}
	return time.Duration(m) * time.Minute
}

// parseSnapshot parses the snapshot of the event behind a delivery.
func parseSnapshot(dl *db.NotificationDelivery) (notify.Snapshot, error) {
	var snap notify.Snapshot
	if len(dl.Snapshot) == 0 {
		return snap, fmt.Errorf("delivery %d has an empty event snapshot", dl.ID)
	}
	if err := json.Unmarshal(dl.Snapshot, &snap); err != nil {
		return snap, fmt.Errorf("failed to parse the event snapshot for delivery %d: %w", dl.ID, err)
	}
	if snap.Kind == "" {
		// The event type follows the event row; the copy inside the snapshot may have been written by an older version.
		snap.Kind = dl.EventKind
	}
	return snap, nil
}

func deliveryIDs(deliveries []*db.NotificationDelivery) []int64 {
	out := make([]int64, 0, len(deliveries))
	for _, dl := range deliveries {
		out = append(out, dl.ID)
	}
	return out
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
