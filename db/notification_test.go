package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/skilfoy/ARTEX-English/notify"
)

// It's gonna be a real story. PostgreSQL(Skip when there's no library. These. SQL I got it.
// FOR UPDATE SKIP LOCKED,make_interval,JSONB,Multiple IN(...) Placeholder Spelling,
// Both.[Compiled but possibly misdirected]In writing, you have to run to prove it. Pass..

func notifyTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestChannel Create a channel through which testing ends automatically to delete.
func newTestChannel(t *testing.T, d *DB, kind, mode string, filter string) *NotificationChannel {
	t.Helper()
	if filter == "" {
		filter = `{}`
	}
	ch := &NotificationChannel{
		Name:       "Test Channel-" + t.Name(),
		Kind:       kind,
		Mode:       mode,
		Config:     json.RawMessage(`{"webhook":"https://example.com/hook"}`),
		Filter:     json.RawMessage(filter),
		RatePerMin: 100,
	}
	id, err := d.SaveNotificationChannel(context.Background(), ch)
	if err != nil {
		t.Fatalf("Creating Channel failed: %v", err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM notification_channels WHERE id=$1`, id) })
	ch.ID = id
	return ch
}

// addTestEvent Write a direct event (without) finding),For testing assignment and delivery.
func addTestEvent(t *testing.T, d *DB, kind string, findingID int64, snap notify.Snapshot) int64 {
	t.Helper()
	snap.Kind = kind
	snap.FindingID = findingID
	id, err := d.AddNotificationEvent(context.Background(), kind, findingID, snap)
	if err != nil {
		t.Fatalf("Failed to write event: %v", err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM notification_events WHERE id=$1`, id) })
	return id
}

func TestNotificationAssetNamesResolvesAndPreservesOrder(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()

	// Each of the three types of asset has its own calibre: domain name,IP,URL.
	insertAsset := func(query, value string) int64 {
		t.Helper()
		var id int64
		if err := d.QueryRow(query, value).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	domID := insertAsset(`INSERT INTO assets(type, domain) VALUES('subdomain',$1) RETURNING id`, "a.example.com")
	ipID := insertAsset(`INSERT INTO assets(type, ip) VALUES('ip',$1) RETURNING id`, "10.1.2.3")
	svcID := insertAsset(`INSERT INTO assets(type, url) VALUES('service',$1) RETURNING id`, "https://a.example.com/admin")
	t.Cleanup(func() {
		d.Exec(`DELETE FROM assets WHERE id IN ($1,$2,$3)`, domID, ipID, svcID)
	})

	// The order of entry is random and contains a non-existent id.
	got, err := d.NotificationAssetNames(ctx, []int64{svcID, 999999999, domID, ipID, svcID})
	if err != nil {
		t.Fatalf("Failed to parse asset name: %v", err)
	}
	want := []string{"https://a.example.com/admin", "a.example.com", "10.1.2.3"}
	if len(got) != len(want) {
		t.Fatalf("Asset name does not match, expected %v get %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Order/It doesn't match. Expectations. %v get %v", want, got)
		}
	}
}

// TestRecordNotificationEventTxUnwindsOnFailure It is the core use of the depository mechanism:
// Let's go first in business. notification_events It is inevitable that writing will fail. false Constraints),
// The assertion. ① This function false ② No entry. aborted Status, follow-up statement still works.
//
// If there's no saving point,,PostgreSQL It's gonna make the whole thing go wrong.
// "current transaction is aborted" Failed——Exactly.[Problem with a notice form
// We can't get a hole in the library.]Other Organiser.
//
// It's for use here. **ROLLBACK End it instead. COMMIT**:ALTER TABLE at PG It's business.,
// Once submitted, the temporary restraint will remain in place forever. schema Lee, use all the following examples together. Hang up..
// Rollback automatically cancels. DDL,No manual cleaning is required. It's all you have to say.[Business is alive.],
// You don't have to do this..
func TestRecordNotificationEventTxUnwindsOnFailure(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()

	// Defensive clean-up: if history has left this constraint, remove it first..
	if _, err := d.Exec(`ALTER TABLE notification_events DROP CONSTRAINT IF EXISTS notify_test_never`); err != nil {
		t.Fatal(err)
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback() //nolint:errcheck // Withdraw provisional restraint, see function comments

	// NOT VALID:Only for subsequent entries without verifying historical events in the library
	// (Otherwise, stock violations will result in non-compliance.).
	if _, err := tx.ExecContext(ctx, `ALTER TABLE notification_events ADD CONSTRAINT notify_test_never CHECK (false) NOT VALID`); err != nil {
		t.Fatalf("Plus temporary restraint failed: %v", err)
	}
	if RecordNotificationEventTx(ctx, tx, notify.EventFindingCreated, 1, notify.Snapshot{Severity: "high"}) {
		t.Fatal("Still report success under the inevitable failure.")
	}
	// Critical assertion: Things work..
	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("Services contaminated (deposit point not effective)): %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	// Confirm DDL It's reversed with roll-back and no follow-up..
	var exists bool
	if err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='notify_test_never')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("The temporary restriction is not rolled back and will contaminate the subsequent use. Example")
	}
}

func TestFanOutRoutesEventsByFilter(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()

	all := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{}`)
	onlyCritical := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{"min_severity":"critical"}`)
	sqlOnly := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{"vulnclass_include":["SQL"]}`)

	highSQL := addTestEvent(t, d, notify.EventFindingCreated, 1001, notify.Snapshot{Severity: "high", VulnClass: "SQLInjection"})
	lowXSS := addTestEvent(t, d, notify.EventFindingCreated, 1002, notify.Snapshot{Severity: "low", VulnClass: "XSS"})
	criticalXSS := addTestEvent(t, d, notify.EventFindingCreated, 1003, notify.Snapshot{Severity: "critical", VulnClass: "XSS"})

	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatalf("Assignment Failed: %v", err)
	}

	cases := []struct {
		name    string
		eventID int64
		channel int64
		want    bool
	}{
		{"Copy that. high", highSQL, all.ID, true},
		{"Copy that. low", lowXSS, all.ID, true},
		{"Only serious channels skip high", highSQL, onlyCritical.ID, false},
		{"Only serious channels. critical", criticalXSS, onlyCritical.ID, true},
		{"OnlySQLWe're on it. SQL", highSQL, sqlOnly.ID, true},
		{"OnlySQLChannel Skipping XSS", lowXSS, sqlOnly.ID, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var exists bool
			if err := d.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM notification_deliveries WHERE event_id=$1 AND channel_id=$2)`,
				tc.eventID, tc.channel).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if exists != tc.want {
				t.Fatalf("Does the delivery exist?: Expectations %v get %v", tc.want, exists)
			}
		})
	}

	// One more assignment should not result in duplicate delivery.(fanned_out Wait.).
	events, deliveries, err := d.FanOutPendingEvents(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if events != 0 || deliveries != 0 {
		t.Fatalf("Events assigned should not be re-processed and obtained events=%d deliveries=%d", events, deliveries)
	}
}

// TestFanOutMarksEventsWithNoMatchingChannel override[There's no way we're gonna get anything.]Situation.
// This type of event must be marked as assigned, or it will remain permanently in the assembly to be assigned. tick Reclean.
func TestFanOutMarksEventsWithNoMatchingChannel(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	pick := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{"vulnclass_include":["No match type"]}`)
	_ = pick

	ev := addTestEvent(t, d, notify.EventFindingCreated, 2001, notify.Snapshot{Severity: "high", VulnClass: "XSS"})
	_, deliveries, err := d.FanOutPendingEvents(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 {
		t.Fatalf("Shouldn't produce a delivery. Got it. %d", deliveries)
	}
	var fanned bool
	if err := d.QueryRowContext(ctx, `SELECT fanned_out FROM notification_events WHERE id=$1`, ev).Scan(&fanned); err != nil {
		t.Fatal(err)
	}
	if !fanned {
		t.Fatal("Incidents of unhit channels must also be marked as assigned, otherwise they will be resolved indefinitely.")
	}
}

func TestClaimRealtimeDeliveriesHonorsLeaseAndMode(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()

	realtime := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{}`)
	digest := newTestChannel(t, d, notify.KindDingTalk, NotifyModeDigest, `{}`)

	addTestEvent(t, d, notify.EventFindingCreated, 3001, notify.Snapshot{Severity: "high", VulnClass: "XSS"})
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}

	// Real time only. realtime You shouldn't move the channel. digest The channel..
	got, err := d.ClaimRealtimeDeliveries(ctx, realtime.ID, 10, time.Minute)
	if err != nil {
		t.Fatalf("Failed to collect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("You should get it. 1 Article, get it. %d", len(got))
	}
	if got[0].State != NotifyStateSending || got[0].Attempts != 1 {
		t.Fatalf("After receipt shall read sending and attempts=1,get state=%s attempts=%d", got[0].State, got[0].Attempts)
	}
	// Linked rendering must be fully contextd (channel configuration) + Eventshot + finding id).
	if got[0].Channel == nil || len(got[0].Channel.Config) == 0 {
		t.Fatal("Receiving results lacks channel configuration and rendering fails")
	}
	if got[0].FindingID != 3001 {
		t.Fatalf("finding id Not taken out of the incident. Got it. %d", got[0].FindingID)
	}

	// The lease is not due and the second payment should be empty——This is...[There's no two in the same line. dispatcher Simultaneous delivery]
	// Promises.
	again, err := d.ClaimRealtimeDeliveries(ctx, realtime.ID, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("No duplicates to be received during the lease period %d strip", len(again))
	}

	// digest The channel should not be accessed in real time..
	left, err := d.ClaimRealtimeDeliveries(ctx, digest.ID, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("Real-time receipts should not be received. digest Channel delivery, get %d strip", len(left))
	}
}

// TestClaimExpiredLeaseRecovers Covering crash self-healing: Process hangs on delivery. sending
// All right, the lease must be retaken when it's over, otherwise this delivery card will be forever. Stay..
func TestClaimExpiredLeaseRecovers(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	ch := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{}`)
	addTestEvent(t, d, notify.EventFindingCreated, 4001, notify.Snapshot{Severity: "high"})
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}
	first, err := d.ClaimRealtimeDeliveries(ctx, ch.ID, 10, time.Minute)
	if err != nil || len(first) != 1 {
		t.Fatalf("Failed to first collect: %v (%d strip)", err, len(first))
	}
	// Move the lease manually to the past.[The lease has expired].
	if _, err := d.Exec(`UPDATE notification_deliveries SET next_attempt_at = now() - interval '1 minute' WHERE id=$1`, first[0].ID); err != nil {
		t.Fatal(err)
	}
	second, err := d.ClaimRealtimeDeliveries(ctx, ch.ID, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("Leases expired sending Right to re-receipt, right? %d strip", len(second))
	}
	if second[0].Attempts != 2 {
		t.Fatalf("Re-receipt of cumulative attempts, received %d", second[0].Attempts)
	}
}

func TestClaimSkipsDisabledChannel(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	ch := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{}`)
	addTestEvent(t, d, notify.EventFindingCreated, 5001, notify.Snapshot{Severity: "high"})
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}
	// Disables the inventory to be shipped together as skipped.
	if err := d.SetNotificationChannelEnabled(ctx, ch.ID, false); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := d.QueryRow(`SELECT state FROM notification_deliveries WHERE channel_id=$1`, ch.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != NotifyStateSkipped {
		t.Fatalf("The inventory of disabled channels to be shipped should be marked as skipped,get %s", state)
	}
	got, err := d.ClaimRealtimeDeliveries(ctx, ch.ID, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("Disabled channels should not be available %d strip", len(got))
	}
}

func TestDigestBatchDueAndStableBatchID(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	ch := newTestChannel(t, d, notify.KindDingTalk, NotifyModeDigest, `{}`)
	for i := 0; i < 3; i++ {
		addTestEvent(t, d, notify.EventFindingCreated, int64(6000+i), notify.Snapshot{Severity: "high"})
	}
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}

	// Batch newly built, age 0,30 The minutes are not due in the cycle.
	due, err := d.DigestBatchDue(ctx, ch.ID, 30*time.Minute)
	if err != nil {
		t.Fatalf("Failed to judge batch due: %v", err)
	}
	if due {
		t.Fatal("The batch just created does not expire immediately")
	}

	// Bringing the creation time of the three deliveries together to age and simulated a batch with sufficient frequency.
	if _, err := d.Exec(`UPDATE notification_deliveries SET created_at = now() - interval '40 minutes' WHERE channel_id=$1`, ch.ID); err != nil {
		t.Fatal(err)
	}
	due, err = d.DigestBatchDue(ctx, ch.ID, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !due {
		t.Fatal("Amounts exceeding the cycle shall be deemed to be due")
	}

	batch, err := d.ClaimDigestBatch(ctx, ch.ID, MaxDigestBatchSize, time.Minute)
	if err != nil {
		t.Fatalf("Failed to receive aggregate batch: %v", err)
	}
	if len(batch) != 3 {
		t.Fatalf("We'll take it all at once. 3 Article, get it. %d strip", len(batch))
	}
	if batch[0].BatchID == nil {
		t.Fatal("The sum batch must be written batch_id,Otherwise, history doesn't see them coming together.")
	}
	firstBatchID := *batch[0].BatchID
	for _, dl := range batch {
		if dl.BatchID == nil || *dl.BatchID != firstBatchID {
			t.Fatalf("The same number should be shared batch_id,get %v vs %d", dl.BatchID, firstBatchID)
		}
	}

	// Let this one go.**Overall**We'll wait until we lose.,batch_id The original value must be maintained(COALESCE Role):
	// Or you can try again.[It's all distributed together.]It's erased..
	//
	// We have to reorder the whole batch, not just one.——That's how you handle delivery engines when they send a summary.
	// (A piece of news represents a whole lot of success. If only one reorder, the rest will remain in the lease term.,
	// That's the only one you've got..
	allIDs := make([]int64, 0, len(batch))
	for _, dl := range batch {
		allIDs = append(allIDs, dl.ID)
	}
	if err := d.RescheduleDeliveries(ctx, allIDs, time.Second, "Simulation failed"); err != nil {
		t.Fatal(err)
	}
	// It's time for a model retreat..
	if _, err := d.Exec(`UPDATE notification_deliveries SET next_attempt_at = now() - interval '1 minute' WHERE channel_id=$1`, ch.ID); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := d.ClaimDigestBatch(ctx, ch.ID, MaxDigestBatchSize, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(reclaimed) != 3 {
		t.Fatalf("The reincarnation should get it all. 3 Article, get it. %d", len(reclaimed))
	}
	if reclaimed[0].BatchID == nil || *reclaimed[0].BatchID != firstBatchID {
		t.Fatalf("After retrying batch_id The original value should be maintained %d,get %v", firstBatchID, reclaimed[0].BatchID)
	}
}

func TestDeliveryStateTransitions(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	ch := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{}`)
	addTestEvent(t, d, notify.EventFindingCreated, 7001, notify.Snapshot{Severity: "high"})
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}
	got, err := d.ClaimRealtimeDeliveries(ctx, ch.ID, 10, time.Minute)
	if err != nil || len(got) != 1 {
		t.Fatalf("Failed to collect: %v (%d)", err, len(got))
	}
	id := got[0].ID

	if err := d.RescheduleDeliveries(ctx, []int64{id}, time.Second, "Network vibrating"); err != nil {
		t.Fatal(err)
	}
	var state, lastErr string
	if err := d.QueryRow(`SELECT state, last_error FROM notification_deliveries WHERE id=$1`, id).Scan(&state, &lastErr); err != nil {
		t.Fatal(err)
	}
	if state != NotifyStatePending || lastErr != "Network vibrating" {
		t.Fatalf("Renumber to read pending And record the reason, get state=%s err=%q", state, lastErr)
	}

	if err := d.FailDeliveries(ctx, []int64{id}, "It's exhausting."); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT state FROM notification_deliveries WHERE id=$1`, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != NotifyStateFailed {
		t.Fatalf("For failed,get %s", state)
	}

	// The manual re-run is to be counted in zero and immediately due, or the old budget fails..
	if err := d.RetryNotificationDelivery(ctx, id); err != nil {
		t.Fatalf("Resend failed: %v", err)
	}
	var attempts int
	var next time.Time
	if err := d.QueryRow(`SELECT state, attempts, next_attempt_at FROM notification_deliveries WHERE id=$1`, id).Scan(&state, &attempts, &next); err != nil {
		t.Fatal(err)
	}
	if state != NotifyStatePending || attempts != 0 {
		t.Fatalf("After reissue, insert pending and attempts=0,get state=%s attempts=%d", state, attempts)
	}
	if next.After(time.Now().Add(time.Second)) {
		t.Fatal("Reissued immediately available")
	}

	// Delivery delivered should not be reissued.
	if err := d.MarkDeliveriesSent(ctx, []int64{id}); err != nil {
		t.Fatal(err)
	}
	if err := d.RetryNotificationDelivery(ctx, id); err == nil {
		t.Fatal("Delivery delivered should not allow re-issuance")
	}
}

func TestListNotificationDeliveriesPagingAndFilter(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	ch := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{}`)
	for i := 0; i < 5; i++ {
		addTestEvent(t, d, notify.EventFindingCreated, int64(8000+i), notify.Snapshot{Severity: "high", Name: "Page Break Test"})
	}
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ClaimRealtimeDeliveries(ctx, ch.ID, 10, time.Minute); err != nil {
		t.Fatal(err)
	}

	page1, total, err := d.ListNotificationDeliveries(ctx, NotificationDeliveryFilter{ChannelID: ch.ID, State: NotifyStateSending}, 1, 2)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if total != 5 {
		t.Fatalf("The total should read 5,get %d", total)
	}
	if len(page1) != 2 {
		t.Fatalf("Every page 2 Article, get it. %d", len(page1))
	}
	// New front: first page of the article id Should be greater than the first article on page 2.
	page2, _, err := d.ListNotificationDeliveries(ctx, NotificationDeliveryFilter{ChannelID: ch.ID, State: NotifyStateSending}, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 || page2[0].ID >= page1[0].ID {
		t.Fatalf("Page break should be new before, get page1[0]=%d page2[0]=%d", page1[0].ID, page2[0].ID)
	}
	// Rendering context must return with history, otherwise the list cannot be displayed[What did you push?].
	if page1[0].ChannelName == "" || page1[0].FindingID == 0 {
		t.Fatalf("History entry missing display field: %+v", page1[0])
	}

	// Filter by status: none pending of.
	pending, totalPending, err := d.ListNotificationDeliveries(ctx, NotificationDeliveryFilter{ChannelID: ch.ID, State: NotifyStatePending}, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if totalPending != 0 || len(pending) != 0 {
		t.Fatalf("I shouldn't have. pending Delivery. Got it. %d strip (total=%d)", len(pending), totalPending)
	}
}

func TestSetFindingStatusWithNotifyOnlyEmitsOnRealChange(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()

	tk, err := d.CreateTask("Notification status change test", "Target", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(tk.ID)
	es := d.Exploration(tk.ExplorationID)
	f, err := es.RecordFinding(ctx, RecordFindingInput{
		TaskID: tk.ID, Worker: "test", VulnClass: "SQLInjection", Name: "Example of status change",
		Severity: "high", Summary: "Abstract",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM notification_events WHERE finding_id=$1`, f.FindingID) })

	// One was registered at the time we landed. finding_created Events. Count them as a baseline..
	var base int
	if err := d.QueryRow(`SELECT count(*) FROM notification_events WHERE finding_id=$1`, f.FindingID).Scan(&base); err != nil {
		t.Fatal(err)
	}
	if base < 1 {
		t.Fatal("The loophole should register a push-off in the same business.")
	}

	// Change to the same state: no event should occur (duplicate submission of brushing out noise)).
	from, found, notified, err := d.SetFindingStatusWithNotify(ctx, f.FindingID, "pending")
	if err != nil || !found {
		t.Fatalf("Status Settings Failed: found=%v err=%v", found, err)
	}
	if notified {
		t.Fatal("No push event should be registered if status remains unchanged")
	}
	if from != "pending" {
		t.Fatalf("Should return to pre-change state pending,get %q", from)
	}

	// Real change: event should be registered and recorded from/to.
	from, found, notified, err = d.SetFindingStatusWithNotify(ctx, f.FindingID, "fixed")
	if err != nil || !found {
		t.Fatalf("Status Settings Failed: found=%v err=%v", found, err)
	}
	if !notified {
		t.Fatal("The event should be registered for actual change of status")
	}
	if from != "pending" {
		t.Fatalf("from For pending,get %q", from)
	}
	var snapshot []byte
	if err := d.QueryRow(`SELECT snapshot FROM notification_events WHERE finding_id=$1 AND kind=$2`,
		f.FindingID, notify.EventFindingStatusChanged).Scan(&snapshot); err != nil {
		t.Fatalf("No status change event found: %v", err)
	}
	var snap notify.Snapshot
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.FromStatus != "pending" || snap.ToStatus != "fixed" {
		t.Fatalf("The situation in the snapshot is going wrong.: %s → %s", snap.FromStatus, snap.ToStatus)
	}
	// Quickshots should be retrofitted, otherwise the status change message will be empty..
	if snap.VulnClass != "SQLInjection" || snap.Severity != "high" || snap.Name != "Example of status change" {
		t.Fatalf("Scrap Missing Fields: %+v", snap)
	}
	var status string
	if err := d.QueryRow(`SELECT status FROM findings WHERE id=$1`, f.FindingID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "fixed" {
		t.Fatalf("Status should be updated to fixed,get %s", status)
	}

	// Other Organiser:found=false,No mistakes..
	if _, found, _, err := d.SetFindingStatusWithNotify(ctx, 999999999, "fixed"); err != nil || found {
		t.Fatalf("Other Organiser found=false ♪ And no mistake, get ♪ found=%v err=%v", found, err)
	}
}

func TestNotificationStatsSnapshot(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	ch := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{}`)
	addTestEvent(t, d, notify.EventFindingCreated, 9001, notify.Snapshot{Severity: "high"})
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}
	stats, err := d.NotificationStatsSnapshot(ctx)
	if err != nil {
		t.Fatalf("Statistics Failed: %v", err)
	}
	if stats.Channels < 1 || stats.ChannelsOn < 1 {
		t.Fatalf("It's wrong to count.: %+v", stats)
	}
	if stats.Pending < 1 {
		t.Fatalf("Should be counted to be delivered: %+v", stats)
	}
	// The newly built backlog should be close 0,Not negative numbers or huge values..
	if stats.BacklogAgeMS < 0 || stats.BacklogAgeMS > int64(time.Hour/time.Millisecond) {
		t.Fatalf("The backlog is illegal.: %d ms", stats.BacklogAgeMS)
	}
	_ = ch
}

func TestNotificationChannelCRUDRoundTrip(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()

	ch := &NotificationChannel{
		Name:       "CRUD Round trip",
		Kind:       notify.KindEmail,
		Mode:       NotifyModeDigest,
		Config:     json.RawMessage(`{"host":"smtp.example.com","port":587,"from":"a@b.c","to":["x@y.z"]}`),
		Filter:     json.RawMessage(`{"min_severity":"medium","on_status_change":true}`),
		RatePerMin: 42,
	}
	id, err := d.SaveNotificationChannel(ctx, ch)
	if err != nil {
		t.Fatalf("New Failed: %v", err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM notification_channels WHERE id=$1`, id) })

	got, err := d.NotificationChannelByID(ctx, id)
	if err != nil {
		t.Fatalf("Reading Failed: %v", err)
	}
	if got.Mode != NotifyModeDigest || got.RatePerMin != 42 || got.Name != "CRUD Round trip" {
		t.Fatalf("Inconsistent round-trip fields: %+v", got)
	}
	if !got.IsEnabled() {
		t.Fatal("Default should be enabled")
	}
	var cfg map[string]any
	if err := json.Unmarshal(got.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["host"] != "smtp.example.com" {
		t.Fatalf("Configure not correctly stored: %v", cfg)
	}
	var filter notify.Filter
	if err := json.Unmarshal(got.Filter, &filter); err != nil {
		t.Fatal(err)
	}
	if filter.MinSeverity != "medium" || !filter.OnStatusChange {
		t.Fatalf("Filter condition not correctly stored: %+v", filter)
	}

	// Read after updating.
	got.Name = "Change of name."
	off := false
	got.Enabled = &off
	if _, err := d.SaveNotificationChannel(ctx, got); err != nil {
		t.Fatal(err)
	}
	after, err := d.NotificationChannelByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.Name != "Change of name." || after.IsEnabled() {
		t.Fatalf("Update not effective: %+v", after)
	}

	// Retrieving after deleting[does not exist]Not a silent success..
	if err := d.DeleteNotificationChannel(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := d.NotificationChannelByID(ctx, id); err != ErrNotificationChannelNotFound {
		t.Fatalf("Expectations ErrNotificationChannelNotFound,get %v", err)
	}
	if err := d.DeleteNotificationChannel(ctx, id); err != ErrNotificationChannelNotFound {
		t.Fatalf("Repeat delete report does not exist, get %v", err)
	}
}

// TestSaveNotificationChannelKeepsExplicitZeroRate Locked a place that was wrong.:
// **0 It's a legitimate configuration.[No current limit],Can't be. db Like...[Not specified]Overwrite as Default**.
//
// History bug:SaveNotificationChannel It says. `if RatePerMin <= 0 { Take Default }`,
// So the document,UI Tips,takeTokens Press both[0=No current limit]The only way to explain it is to change the level of the only library.
// 20(DingTalk/Micro/Telegram)or 100(Feishu)——The operator thinks it's free, it's actually stuck.,
// And there's no hint..[Not specified]With[Visible 0]The difference is only physical.,
// So the default value is server Layer Fill (see notifyCreateChannel),db Layers only exist..
func TestSaveNotificationChannelKeepsExplicitZeroRate(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()

	// Visible 0(Unlimited: must remain as it is..
	unlimited := &NotificationChannel{
		Name: "No current limit", Kind: notify.KindDingTalk, RatePerMin: 0,
		Config: json.RawMessage(`{"webhook":"https://example.com/h"}`),
	}
	id, err := d.SaveNotificationChannel(ctx, unlimited)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM notification_channels WHERE id=$1`, id) })
	got, err := d.NotificationChannelByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.RatePerMin != 0 {
		t.Fatalf("Visible 0 This means that it's endless. It has to be saved as it is. %d", got.RatePerMin)
	}
	if got.Mode != NotifyModeRealtime {
		t.Fatalf("Default mode should read realtime,get %s", got.Mode)
	}

	// Negative value is illegal and should be rejected instead of quietly changing to another value.
	bad := &NotificationChannel{
		Name: "Negative flow", Kind: notify.KindDingTalk, RatePerMin: -1,
		Config: json.RawMessage(`{"webhook":"https://example.com/h"}`),
	}
	if _, err := d.SaveNotificationChannel(ctx, bad); err == nil {
		t.Fatal("Negative flow should be rejected.")
	}
}

// TestDeleteChannelCascadesDeliveries Lock out external key behaviour: the channel disappears after its delivery history is removed
// (No configuration, no history, but the event itself.——It may be quoted from another source..
func TestDeleteChannelCascadesDeliveries(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	ch := newTestChannel(t, d, notify.KindDingTalk, NotifyModeRealtime, `{}`)
	ev := addTestEvent(t, d, notify.EventFindingCreated, 9101, notify.Snapshot{Severity: "high"})
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := d.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE channel_id=$1`, ch.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before == 0 {
		t.Fatal("Precondition not valid: no delivery generated")
	}
	if err := d.DeleteNotificationChannel(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := d.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE channel_id=$1`, ch.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("The channel is deleted and its delivery cascades are deleted. %d strip", after)
	}
	var evExists bool
	if err := d.QueryRow(`SELECT EXISTS(SELECT 1 FROM notification_events WHERE id=$1)`, ev).Scan(&evExists); err != nil {
		t.Fatal(err)
	}
	if !evExists {
		t.Fatal("The channel should not be associated with the event itself.")
	}
}

// TestClaimDigestBatchHonorsCallerLimit Override one of the words identified in the audit:
// The aggregating channel was completely bypassed by the barrel.——allow Being takeTokens It's not used.,
// rate_per_min Yes digest The model is useless. Now. limit And he's bound..
func TestClaimDigestBatchHonorsCallerLimit(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()
	ch := newTestChannel(t, d, notify.KindDingTalk, NotifyModeDigest, `{}`)
	for i := 0; i < 10; i++ {
		addTestEvent(t, d, notify.EventFindingCreated, int64(7000+i), notify.Snapshot{Severity: "high"})
	}
	if _, _, err := d.FanOutPendingEvents(ctx, 100); err != nil {
		t.Fatal(err)
	}
	// take limit=3:I can only get it. 3 The rest will be left in the Courge..
	got, err := d.ClaimDigestBatch(ctx, ch.ID, 3, time.Minute)
	if err != nil {
		t.Fatalf("Failed to collect: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("The caller limit should be applied only 3 Article, get it. %d", len(got))
	}
	// limit=0 Means that the current round has been exhausted: none of them should be taken nor wrongly reported.
	if got, err := d.ClaimDigestBatch(ctx, ch.ID, 0, time.Minute); err != nil || len(got) != 0 {
		t.Fatalf("Amount 0 ♪ Should be ♪ 0 I don't give a shit. I get it. %d strip err=%v", len(got), err)
	}
}

// TestFinishFindingRetestEmitsStatusChange Override an completeness gap identified by the audit:
// Reconciling conclusion:[Fixed]The state did change, but the one. UPDATE It's for the library.,
// I've bypassed the version with the notice.——That's how it works. on_status_change The channel that flows through this state.
// We can't get a delivery. We've changed the situation on the interface..
//
// This one's locked.[All re-routings are registered for the change of status event.].
func TestFinishFindingRetestEmitsStatusChange(t *testing.T) {
	d := notifyTestDB(t)
	ctx := context.Background()

	tk, err := d.CreateTask("Retrometric transfer test", "Target", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(tk.ID)
	es := d.Exploration(tk.ExplorationID)
	f, err := es.RecordFinding(ctx, RecordFindingInput{
		TaskID: tk.ID, Worker: "test", VulnClass: "SQLInjection", Name: "Repeating target.",
		Severity: "high", Summary: "Abstract",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM notification_events WHERE finding_id=$1`, f.FindingID) })

	// Build a double-check record and push it straight to completion State.
	rt, _, _, err := d.CreateFindingRetest(ctx, f.FindingID, "Review")
	if err != nil {
		t.Fatal(err)
	}
	if rt.ConversationID == nil {
		t.Fatal("Reconsideration should relate to a session")
	}
	// We have to go in first. running In order to reach a conclusion (consistent with true processes)).
	if ok, err := d.StartFindingRetest(ctx, rt.ID); err != nil || !ok {
		t.Fatalf("Failed to initiate repeat detection: ok=%v err=%v", ok, err)
	}
	if err := d.RecordFindingRetestResult(ctx, *rt.ConversationID, "fixed", "Fixed", "Evidence"); err != nil {
		t.Fatal(err)
	}
	if err := d.FinishFindingRetest(rt.ID, "completed", ""); err != nil {
		t.Fatalf("Ending Retrometry Failed: %v", err)
	}

	var status string
	if err := d.QueryRow(`SELECT status FROM findings WHERE id=$1`, f.FindingID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != FindingFixed {
		t.Fatalf("The restored state should read fixed,get %s", status)
	}

	// Critical assertion: there must be a change of status event and from/to Correct..
	var snapshot []byte
	err = d.QueryRow(`SELECT snapshot FROM notification_events WHERE finding_id=$1 AND kind=$2 ORDER BY id DESC LIMIT 1`,
		f.FindingID, notify.EventFindingStatusChanged).Scan(&snapshot)
	if err != nil {
		t.Fatalf("Reconsideration repaired registration of a change of status event (otherwise assigned) on_status_change I can't get it.): %v", err)
	}
	var snap notify.Snapshot
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.FromStatus != "pending" || snap.ToStatus != FindingFixed {
		t.Fatalf("The snapshot's running wrong.: %s → %s", snap.FromStatus, snap.ToStatus)
	}
	// Quick-actioned fields, or empty shells..
	if snap.Name != "Repeating target." || snap.Severity != "high" {
		t.Fatalf("Scrap Missing Fields: %+v", snap)
	}
}
