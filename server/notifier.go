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

// Global Settings Key (Existing) settings Key Table, no watch).
const (
	// settingNotifyEnabled It's pushing the total switch. Default open: it's used to maintain a one-key stoppage.,
	// Instead of a function enabler——The real enabler condition is[Do you have access?].
	settingNotifyEnabled = "notify_enabled"
	// settingNotifyPublicBaseURL It's an external access address to generate a loophole backlink.
	// (As https://artex.example.com).Leave empty messages without chain buttons.
	// There is no reusable external address configuration in the project so add a new one here.
	settingNotifyPublicBaseURL = "notify_public_base_url"
	// settingNotifyDigestMinutes is the periodicity of the summary mode (minutes)).
	settingNotifyDigestMinutes = "notify_digest_interval_min"
)

const (
	// notifyTick It's the interrogation interval for the delivery engine..3 The second is the limit of the engine's real time.,
	// Yeah.[Hole Out]Arrived[Message has arrived. IM]Main sources of delay between.
	notifyTick = 3 * time.Second
	// notifyLease It is the length of the lease upon delivery. Must be significantly greater than the worst time of a single delivery
	// (notify The bag. HTTP Client timeout 15 (Secs) Otherwise there will be two in the same line.
	// dispatcher Simultaneous delivery.
	notifyLease = 3 * time.Minute
	// notifyFanOutPerTick Limit the number of events assigned per round to avoid first-time access
	// One-time roll-out of the historical backlog into a delivery mission.
	notifyFanOutPerTick = 200
	// notifyDefaultDigestMinutes is the default value for the summary cycle.
	notifyDefaultDigestMinutes = 30
	// notifyUnlimitedBurstPerTick is the maximum delivery per round when the channel is open..
	// The meaning of existence is prevention.[It's an open channel. + A thousand holes at once.]handle
	// Single cycle drags growth time blocking.
	notifyUnlimitedBurstPerTick = 50
	// notifyMaxSendsPerChannelPerTick It's a single channel with up to a few drops per round..
	//
	// This ceiling is by**Lease duration**Inverted: The lease is paid to the owner at the time of receipt(notifyLease = 3 min),
	// If the number of serials in the round exceeds the lease at its worst, the last few will expire before the lease is issued..
	// It doesn't matter in a single process.(Run It's single. goroutine Serial running,tick I'm not going back, but...**Two
	// Process with a library**, the end will retake and retransmit the expired line,
	// attempts Double increment, failed when the original process is still in delivery.
	//
	// Take Value:3 Minute lease / 30 Second single timeout = 6 Yes**Just in time for the lease.**,Zero,
	// not available;taken 5 Let the worst take time. 150 Hold on. 30 Second balance. This relationship...
	// TestNotifyTickBudgetFitsWithinLease Nail!——Change notifyLease,
	// notifySendTimeout Or any one of them will fail that claim..
	notifyMaxSendsPerChannelPerTick = 5
	// notifySendTimeout A single delivery timeout. It also determines the value of the last constant.,
	// We can't multiply the two. notifyLease,See TestNotifyTickBudgetFitsWithinLease.
	notifySendTimeout = 30 * time.Second
)

// notifyBackoff It's a runaway sequence that failed to try again..
// 3 Sub-opportunities (including initial) db.MaxNotifyAttempts We have to change both..
var notifyBackoff = []time.Duration{
	time.Second,
	5 * time.Second,
	30 * time.Second,
}

// Notifier It's a delivery engine from a leak..
//
// With Scheduler Parallel as Independence goroutine Run (see server.New).It's useless.
// Scheduler of tick:Real-time requirement for push(3 The operational tempo of the trigger is different.,
// And there's no connection between the two failures.——It doesn't matter if it's stuck. agent Trigger.
type Notifier struct {
	s  *Server
	pg *db.DB

	// mu Protection buckets.There's only a small number of channels and competition.,
	// It's not worth introducing a more nuanced structure..
	mu      sync.Mutex
	buckets map[int64]*notifyBucket
}

// notifyBucket It's a single channel..
//
// It's not a jar.[Zero per minute.]The slide window is because the latter has a bad border effect.:
// Fill at the end of the window 20 The bar, the next moment. 20 It's a second for the platform. Internal 40 strip,
// They'll be restricted; the drums will be supplemented with constant rates, and they'll avoid such an outbreak..
type notifyBucket struct {
	tokens   float64
	lastFill time.Time
}

func newNotifier(s *Server) *Notifier {
	return &Notifier{s: s, pg: s.m.pg, buckets: map[int64]*notifyBucket{}}
}

// Run Loop until ctx Over. By server.New Start once..
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

// step Run round: assign a new event and then deliver an expired job.
//
// Any step that fails is only a log.——The failure of the notification system must not be a process-level problem..
// each tick They're all independent. The next round will try again..
func (n *Notifier) step(ctx context.Context) {
	if !n.enabled() {
		return
	}
	if _, _, err := n.pg.FanOutPendingEvents(ctx, notifyFanOutPerTick); err != nil {
		log.Printf("[notify] Failed to assign event: %v", err)
		return
	}
	channels, err := n.pg.ListNotificationChannels(ctx)
	if err != nil {
		log.Printf("[notify] Failed to read channel: %v", err)
		return
	}
	baseURL := n.publicBaseURL()
	for _, ch := range channels {
		if !ch.IsEnabled() {
			continue
		}
		// The unit of measure for the barrel is**Message Bar Number**(Equivalent HTTP (number of requests).
		// The two are the same in real-time mode (a gap message); the whole series of holes in aggregate mode is synthesized
		// One message, so only one token is spent..
		//
		// In both ways, ask for the barrel and collect it at a scale.——The order cannot be reversed or the flow is blocked.
		// The delivery has been overexhausted..
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

// digestTickPlan Returns the token consumption and batch size of the current round of aggregate channels.
//
// Two return values are**Two different scales.**,That's why it's a function.:
//
//   - tokens is the number of messages. A bunch of holes synthesized a message, sent it once. HTTP Request, so always 1.
//     rate_per_min So it's still true. digest Entry into force (up to so many summary messages per minute)).
//   - claimLimit It is this batch that contains up to a few holes. It's bound only by the upper memory, not the budget requested..
//
// ♪ Once for Jean ♪ rate_per_min Yes digest Entry into force, budget request per round
// (notifyMaxSendsPerChannelPerTick,It's going to pass directly to the size of the batch..
// The consequence is... rate_per_min=20 # The channel is # 3 Seconds tick It's only filled in. 1 A token, then a summary of each.
// Messages only 1 A loophole.——digest Degraded.[Real time delivery with summary files],Readers get a bunch of them.
// [near 30 min Add 1 A loophole.],And db.MaxDigestBatchSize Never..
//
// It's not easy to detect in end-to-end tests. limit Give
// stepDigest,It's bypassed. step The amount of money in it)
// TestDigestTickPlanDecouplesBatchSizeFromSendBudget Just nail it..
func digestTickPlan() (tokens, claimLimit int) {
	return 1, db.MaxDigestBatchSize
}

// stepRealtime Fetch and deliver a real-time task from a channel, a leaky message.
func (n *Notifier) stepRealtime(ctx context.Context, ch *db.NotificationChannel, allow int, baseURL string) {
	deliveries, err := n.pg.ClaimRealtimeDeliveries(ctx, ch.ID, allow, notifyLease)
	if err != nil {
		log.Printf("[notify] Fetching real time delivery failed channel=%d: %v", ch.ID, err)
		return
	}
	if len(deliveries) == 0 {
		return
	}
	channel, cfg, ok := n.adapt(ch)
	if !ok {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), fmt.Sprintf("Channel type %q Unregistered", ch.Kind))
		return
	}
	for _, dl := range deliveries {
		msg, err := n.renderSingle(ctx, dl, baseURL)
		if err != nil {
			// Rendering failure is a local data problem and retrying won't get better..
			_ = n.pg.FailDeliveries(ctx, []int64{dl.ID}, err.Error())
			continue
		}
		n.send(ctx, channel, cfg, msg, []*db.NotificationDelivery{dl})
	}
}

// stepDigest Combine the pending delivery of a channel into a message when the batch expires.
func (n *Notifier) stepDigest(ctx context.Context, ch *db.NotificationChannel, allow int, baseURL string) {
	window := n.digestInterval()
	due, err := n.pg.DigestBatchDue(ctx, ch.ID, window)
	if err != nil {
		log.Printf("[notify] failed to count pending batch channel=%d: %v", ch.ID, err)
		return
	}
	if !due {
		return
	}
	deliveries, err := n.pg.ClaimDigestBatch(ctx, ch.ID, allow, notifyLease)
	if err != nil {
		log.Printf("[notify] failed to claim digest batch channel=%d: %v", ch.ID, err)
		return
	}
	if len(deliveries) == 0 {
		return
	}
	channel, cfg, ok := n.adapt(ch)
	if !ok {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), fmt.Sprintf("Channel type %q Unregistered", ch.Kind))
		return
	}
	msg, included, err := n.renderBatch(ctx, deliveries, baseURL, int(window.Minutes()))
	if err != nil {
		_ = n.pg.FailDeliveries(ctx, deliveryIDs(deliveries), err.Error())
		return
	}
	// Those who have broken the flashlight and failed to get in the news will fail in the obvious. If they don't, they'll stay.
	// included Outside, neither messages nor failed lists——When it's successful, their status will be...
	// The subsequent batch mark is missing and will remain permanently. sending Until the lease expired and received repeatedly.
	if skipped := excludeDeliveries(deliveries, included); len(skipped) > 0 {
		reason := "Event snapshot could not be solved, this loophole could not render the message"
		if fErr := n.pg.FailDeliveries(ctx, deliveryIDs(skipped), reason); fErr != nil {
			log.Printf("[notify] could not mark skipped deliveries for channel %s, IDs %v: %v", ch.Kind, deliveryIDs(skipped), fErr)
		}
		log.Printf("[notify] skipped %d invalid deliveries channel=%d", len(skipped), ch.ID)
	}
	// Just the ones that got the message. send:included[i] With msg.Items[i] Strictly matching.,
	// send We rely on this correspondence.[The channel's in return. K strip]Fall to the right delivery line.
	n.send(ctx, channel, cfg, msg, included)
}

// send Organisation.
//
// Share one delivery result with the same delivery (possibly dozens under aggregation mode): deliver it or try again in bulk.
// Do not try again article by article——Summarizing the message is one thing, and re-transmitting part of it would be confusing..
//
// The only exception is...**Divisions due to maximum channel length**:The channel returns are actually only in the front. K strip,
// Then... K+1 The bars must be kept in the next batch instead of being marked with success. Or they're cut off.
// Those holes are not in the news, they're not in the failure list. They're gone..
func (n *Notifier) send(ctx context.Context, channel notify.Channel, cfg map[string]any, msg notify.Message, deliveries []*db.NotificationDelivery) {
	// One drop cap to avoid a channel stuck to the rest of this round. Hold on..
	sendCtx, cancel := context.WithTimeout(ctx, notifySendTimeout)
	defer cancel()
	delivered, err := channel.Send(sendCtx, cfg, msg)
	if err == nil && delivered > 0 {
		if delivered > len(deliveries) {
			// The number of bars returned from the channel is unlikely to exceed the number delivered; indeed, there was an error in accounting for the rendering layer,
			// It's better to press everything and write down the problem than to mess up the record..
			log.Printf("[notify] Number of reports received %d More than delivered %d channel=%s,By all service",
				delivered, len(deliveries), channel.Kind())
			delivered = len(deliveries)
		}
		sent, rest := deliveries[:delivered], deliveries[delivered:]
		if err := n.pg.MarkDeliveriesSent(ctx, deliveryIDs(sent)); err != nil {
			log.Printf("[notify] Tag service failed channel=%s ids=%v: %v", channel.Kind(), deliveryIDs(sent), err)
		}
		if len(rest) > 0 {
			// This message reaches the limit of the channel length: the rest will return immediately to the next one. tick Continuation.
			// Use DeferDeliveries instead of RescheduleDeliveries —— It's not a failure.,
			// Shouldn't have consumed the trial budget. +1 That's it. It'll go back.).
			if err := n.pg.DeferDeliveries(ctx, deliveryIDs(rest),
				fmt.Sprintf("The channel message reached its length limit. %d findings were sent; the remainder will be sent later", delivered)); err != nil {
				log.Printf("[notify] Queuing failed channel=%s ids=%v: %v", channel.Kind(), deliveryIDs(rest), err)
			}
		}
		return
	}
	if err == nil {
		// The channels were neither misreported nor given much service. By failure),
		// Before this delivery gets picked up over and over again, it never gets marked..
		err = fmt.Errorf("Failure to report service(delivered=%d)", delivered)
	}

	// Failed to dispose**Article by article**Decide, not judge by the maximum number of attempts in the whole batch..
	//
	// Was. `if maxAttempts(deliveries) >= MaxNotifyAttempts` He's been sentenced to death for the whole time.
	// The number of attempts is not the same: an old delivery that has tried twice(attempts=2)They'll take the same group.
	// New delivery.(attempts=1)Let's drag in. failed——A new loophole can't be tried again and it's gone forever.,
	// With[Don't let the old man drag you down.]It was the opposite..
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
			log.Printf("[notify] Error marking failure channel=%s ids=%v: %v", channel.Kind(), failIDs, fErr)
		}
	}
	if len(exhaustedIDs) > 0 {
		reason := fmt.Sprintf("Retry %d After a while, I failed.: %s", db.MaxNotifyAttempts, err)
		if fErr := n.pg.FailDeliveries(ctx, exhaustedIDs, reason); fErr != nil {
			log.Printf("[notify] Error marking failure channel=%s ids=%v: %v", channel.Kind(), exhaustedIDs, fErr)
		}
	}
	// Regroup by Delay: Only 3 It's a very small group, so it doesn't have to be sent for each one.
	// UPDATE(That'll get one. 500 Batch generation of articles 500 Second round trip).
	for delay, group := range byDelay {
		if rErr := n.pg.RescheduleDeliveries(ctx, group, delay, err.Error()); rErr != nil {
			log.Printf("[notify] Redo delivery failed channel=%s ids=%v: %v", channel.Kind(), group, rErr)
		}
	}
	if len(failIDs)+len(exhaustedIDs) > 0 {
		log.Printf("[notify] delivery failed channel=%d kind=%s permanent=%d exhausted=%d retry_groups=%d: %s",
			deliveries[0].ChannelID, channel.Kind(), len(failIDs), len(exhaustedIDs), len(byDelay), err)
	}
}

// excludeDeliveries Return all Not in keep The ones in there.).
// For finding[I didn't get a message.]Delivery——They have to be clearly disposed of, not left in the grey zone..
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

// adapt Access channels to achieve and interpret their configuration.
// Return ok=false Indicates that the type is not registered and the delivery is subject to a direct judgement failure rather than an unlimited retry.
func (n *Notifier) adapt(ch *db.NotificationChannel) (notify.Channel, map[string]any, bool) {
	channel, ok := notify.Get(ch.Kind)
	if !ok {
		return nil, nil, false
	}
	var cfg map[string]any
	if len(ch.Config) > 0 {
		// Give empty when configuration failed map:The channel itself. Validate It'll come out.[Which field is missing],
		// That mistake. JSON Parsing error will guide user fixes.
		_ = json.Unmarshal(ch.Config, &cfg)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return channel, cfg, true
}

// renderSingle Render a single loophole message.
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

// renderBatch Render summary messages. Article by article——It's broken. Just skip that one.,
// Don't let him drag the whole batch..
//
// Return value included With msg.Items **It's the exact opposite.**(No. i A delivery. ↔ No. i Entry).
// This correspondence is hard: caller press[The channel's in return. K strip]Before you decide. K A delivery.
// Marks delivered. If we skip a bad snapshot and don't deliver the jump from here, included Remove,
// The subscript is wrong.——Bad entries that should have failed will be marked as delivered and good entries miscalculated as not delivered Da..
// Failed to break the caller's visible tags. See stepDigest.
func (n *Notifier) renderBatch(ctx context.Context, deliveries []*db.NotificationDelivery, baseURL string, windowMinutes int) (notify.Message, []*db.NotificationDelivery, error) {
	items := make([]notify.Item, 0, len(deliveries))
	included := make([]*db.NotificationDelivery, 0, len(deliveries))
	for _, dl := range deliveries {
		snap, err := parseSnapshot(dl)
		if err != nil {
			// Bad snapshots don't get in. included——It's handled by the caller.
			// (Visible tag failed, not mixed[Delivered]Limon passed through.).
			log.Printf("[notify] Skip unresolved snapshots in group batch delivery=%d: %v", dl.ID, err)
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
		return notify.Message{}, nil, fmt.Errorf("none of the %d deliveries could be rendered for the digest", len(deliveries))
	}
	return notify.Message{
		Items:         items,
		Batch:         true,
		WindowMinutes: windowMinutes,
		HomeURL:       baseURL,
	}, included, nil
}

// itemFor Render event snapshots as pending entry, deciphering asset name and detail back.
func (n *Notifier) itemFor(ctx context.Context, snap notify.Snapshot, baseURL string) (notify.Item, error) {
	assets, err := n.pg.NotificationAssetNames(ctx, snap.AssetIDs)
	if err != nil {
		// Failure to resolve the asset name should not prevent the transfer: failure to read a name is much less than failure to receive notice,
		// It's just a line of assets..
		log.Printf("[notify] Failed to parse asset name finding=%d: %v", snap.FindingID, err)
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
		// For details, see the route. web/src/app/(main)/function/findings/detail/page.tsx,
		// It's from query Parameter id Read Hole id.
		item.DetailURL = fmt.Sprintf("%s/function/findings/detail?id=%d", baseURL, snap.FindingID)
	}
	return item, nil
}

// takeTokens Take it from the channel.**Max want pieces**token, return the actual number.
//
// A token. = One message (one time) HTTP Please. We need a few calls in real time mode.;
// The next series of leaks in the aggregation mode only sends one message. 1.
//
// The barrel capacity is the maximum per minute of the channel, supplemented at constant rate.ratePerMin<=0 Means no limit,
// Return a limited but sufficiently large value to prevent an unlimited backlog of single-cycle cycles Drag Stay..
//
// want The limit is necessary: without it, the barrel will be empty, and the caller has a ceiling per round.,
// More tokens won't be needed and will disappear before the next update.——It's never gonna be able to save up.,
// Company[There's nothing to deliver this round.]They'll take a cut..
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
	// By real time, the rate is... ratePerMin/60 Per second.
	if elapsed := now.Sub(b.lastFill).Seconds(); elapsed > 0 {
		b.tokens = minF(float64(ratePerMin), b.tokens+elapsed*float64(ratePerMin)/60)
		b.lastFill = now
	}
	// Add a tiny one. epsilon Reset: The number of tokens is added to the floating point, filled in two parts. Time
	// 0.5 + 0.5 Maybe. 0.9999999999,Direct int() It'll be cut off. 0——
	// I can't get a license for a full bucket in math..1e-9 It's much smaller than a token. It won't let go of the real debt..
	take := min(int(b.tokens+1e-9), want)
	if take <= 0 {
		return 0
	}
	b.tokens -= float64(take)
	return take
}

// enabled Read Total Switches.
func (n *Notifier) enabled() bool {
	return n.pg.GetBool(settingNotifyEnabled, true)
}

// publicBaseURL Return to the outside address of the chain and remove the tail slash.
func (n *Notifier) publicBaseURL() string {
	v, ok, err := n.pg.GetSetting(settingNotifyPublicBaseURL)
	if err != nil || !ok {
		return ""
	}
	return trimTrailingSlash(v)
}

// digestInterval Returns the grouping cycle and returns to the default value when illegal or unconfigured.
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

// parseSnapshot Parsing snapshot of the corresponding event.
func parseSnapshot(dl *db.NotificationDelivery) (notify.Snapshot, error) {
	var snap notify.Snapshot
	if len(dl.Snapshot) == 0 {
		return snap, fmt.Errorf("Organisation %d The event snapshot is empty.", dl.ID)
	}
	if err := json.Unmarshal(dl.Snapshot, &snap); err != nil {
		return snap, fmt.Errorf("Parsing delivery %d Event snapshot failed: %w", dl.ID, err)
	}
	if snap.Kind == "" {
		// The type of event is behavioral. The one in the snapshot may be written in the old version. Pass..
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
