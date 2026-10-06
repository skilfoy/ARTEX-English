package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/skilfoy/ARTEX-English/db"
	"github.com/skilfoy/ARTEX-English/notify"
)

// End-to-end coverage of finding notifications: a finding is stored → an event → fan-out → a real HTTP send.
//
// Safety: these tests do **not** call the global Notifier.step(). They call stepRealtime or stepDigest
// only on channels they created. step() walks every enabled channel in the database. On a dev database
// that already has a real DingTalk or WeCom robot, a global step would push findings created during the
// test into those rooms. Per-channel calls keep the effect on the test's own fake receiver.
//
// Cleanup deletes the events this test created (deliveries cascade) and the channels, so real channels
// are not left with a backlog.
//
// stepRealtime and stepDigest return nothing and only log, so assertions are on observable behavior
// (what the fake receiver got, and what state the delivery row landed in), not on a return value.
// That is closer to the real call path than stubbing the return.

// notifyFixture is the shared setup for tests in this file.
type notifyFixture struct {
	s       *Server
	pg      *db.DB
	request func(method, path, body string) *httptest.ResponseRecorder
	n       *Notifier
	// This test's own task and exploration, so findings stay isolated from other tests.
	taskID int64
	expID  int64
	// events created after cleanupMark are deleted during cleanup.
	cleanupMark int64
}

func newNotifyFixture(t *testing.T) *notifyFixture {
	t.Helper()
	// Every fake receiver in this file listens on 127.0.0.1. Delivery refuses loopback by default
	// (SSRF against a local service or cloud metadata). Tests set the allow-local switch explicitly.
	// The default-deny behavior is covered by notify/ssrf_test.go.
	t.Setenv(notify.AllowLocalTargetsEnv, "1")
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.pg

	// Own task: the shared trafficEvidenceServer task has no exploration id, and recording a finding requires one.
	task, err := s.m.CreateTask("notification delivery test", "verify delivery behavior", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := strconv.ParseInt(task.ID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pg.Exec(`DELETE FROM tasks WHERE id=$1`, taskID) })

	var mark int64
	if err := pg.QueryRow(`SELECT COALESCE(max(id),0) FROM notification_events`).Scan(&mark); err != nil {
		t.Fatal(err)
	}
	// Close the fixture over itself: mark every event that already existed as fanned out.
	//
	// FanOutPendingEvents is global. It expands every unfanned event onto every matching channel.
	// The shared trafficEvidenceServer records a finding of its own (the initial finding it returns),
	// and other tests may leave leftovers. Without this, those stray events land on this test's channel
	// and "expected N deliveries" passes or fails depending on test order, which is harder to debug than a hard failure.
	if _, err := pg.Exec(`UPDATE notification_events SET fanned_out = true WHERE id <= $1 AND NOT fanned_out`, mark); err != nil {
		t.Fatal(err)
	}

	f := &notifyFixture{s: s, pg: pg, request: request, n: newNotifier(s), taskID: taskID, expID: task.ExpID, cleanupMark: mark}
	t.Cleanup(func() {
		if _, err := pg.Exec(`DELETE FROM notification_events WHERE id > $1`, f.cleanupMark); err != nil {
			t.Logf("Failed to clear notification event: %v", err)
		}
	})
	// The global switch must be on (another test may have turned it off).
	if err := pg.SetBool(settingNotifyEnabled, true); err != nil {
		t.Fatal(err)
	}
	return f
}

// record writes a finding through the real evidence path and returns the finding id.
// That path records the notify event in the same transaction, which is the hook this feature uses.
func (f *notifyFixture) record(t *testing.T, vulnclass, severity string) int64 {
	t.Helper()
	out, err := f.s.evidenceStore().Record(context.Background(), db.RecordFindingInput{
		TaskID:        f.taskID,
		ExplorationID: f.expID,
		Worker:        "test",
		VulnClass:     vulnclass,
		Name:          vulnclass,
		Severity:      severity,
		Summary:       vulnclass + " Summary",
		Evidence:      "poc",
	}, nil)
	if err != nil {
		t.Fatalf("failed to record finding: %v", err)
	}
	return out.FindingID
}

// channel Read-back channel configuration (source-by-channel) stepX Use).
func (f *notifyFixture) channel(t *testing.T, id int64) *db.NotificationChannel {
	t.Helper()
	ch, err := f.pg.NotificationChannelByID(context.Background(), id)
	if err != nil {
		t.Fatalf("Reading Channel Failed: %v", err)
	}
	return ch
}

// deliver Assign events and run a round of deliveries only for specified channels.
func (f *notifyFixture) deliver(t *testing.T, chID int64, baseURL string) {
	t.Helper()
	ctx := context.Background()
	if _, _, err := f.pg.FanOutPendingEvents(ctx, 500); err != nil {
		t.Fatalf("Assignment Failed: %v", err)
	}
	f.n.stepRealtime(ctx, f.channel(t, chID), 50, baseURL)
}

// createChannel Pass HTTP Interfaces build channels to cover the interface ' s own verification path.
func (f *notifyFixture) createChannel(t *testing.T, payload map[string]any) int64 {
	t.Helper()
	raw, _ := json.Marshal(payload)
	r := f.request("POST", "/api/notify/channels", string(raw))
	if r.Code != 200 {
		t.Fatalf("Creating Channel failed %d: %s", r.Code, r.Body)
	}
	var res struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &res); err != nil || res.ID == 0 {
		t.Fatalf("channel create returned an unexpected body: %s (%v)", r.Body, err)
	}
	t.Cleanup(func() { f.pg.Exec(`DELETE FROM notification_channels WHERE id=$1`, res.ID) })
	return res.ID
}

// fakeWebhook records the request bodies the fake receiver got.
type fakeWebhook struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
}

func newFakeWebhook(t *testing.T) *fakeWebhook {
	t.Helper()
	f := &fakeWebhook{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"errcode":0,"errmsg":"ok"}`)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeWebhook) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func (f *fakeWebhook) body(t *testing.T, i int) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.bodies) {
		t.Fatalf("fake receiver only has %d requests; cannot read request %d", len(f.bodies), i)
	}
	return f.bodies[i]
}

func (f *fakeWebhook) last(t *testing.T) map[string]any {
	t.Helper()
	if f.count() == 0 {
		t.Fatal("fake receiver received no requests")
	}
	return f.body(t, f.count()-1)
}

// markdownText Take the body of the request and match the names of the fields.:
// DingTalk markdown Use `text`,ActionCard Use `text`,Enterprise WeChat markdown Use `content`.
func markdownText(t *testing.T, body map[string]any) string {
	t.Helper()
	for _, key := range []string{"markdown", "actionCard"} {
		section, ok := body[key].(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"text", "content"} {
			if s, ok := section[field].(string); ok && s != "" {
				return s
			}
		}
	}
	t.Fatalf("There is no identifiable body in the request.: %v", body)
	return ""
}

// agePendingBatch The channel is being sent on time to test the expiry of the batch..
func (f *notifyFixture) agePendingBatch(t *testing.T, chID int64) {
	t.Helper()
	if _, err := f.pg.Exec(`UPDATE notification_deliveries SET created_at = now() - interval '2 hours'
WHERE channel_id=$1 AND state=$2`, chID, db.NotifyStatePending); err != nil {
		t.Fatal(err)
	}
}

func TestNotifyEndToEndRealtimeDelivery(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "Real time delivery",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": hook.URL},
	})
	f.record(t, "SQLInjection", "high")
	f.deliver(t, chID, "")

	if hook.count() != 1 {
		t.Fatalf("expected 1 message, got %d", hook.count())
	}
	text := markdownText(t, hook.last(t))
	for _, want := range []string{"SQLInjection", "High", "Abstract"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Message body missing %q:\n%s", want, text)
		}
	}
	// The delivery should now be sent.
	var pending int
	if err := f.pg.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE channel_id=$1 AND state <> $2`,
		chID, db.NotifyStateSent).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("after delivery, %d entries are still not marked sent", pending)
	}
}

func TestNotifyChannelAPIMasksSecretsAndPreservesOnUpdate(t *testing.T) {
	f := newNotifyFixture(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "Example of mask",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": "https://oapi.dingtalk.com/robot/send?access_token=abc123456", "secret": "SECabcdef123456"},
	})

	r := f.request("GET", "/api/notify/channels", "")
	if r.Code != 200 {
		t.Fatalf("listing channels failed %d: %s", r.Code, r.Body)
	}
	if strings.Contains(r.Body.String(), "abc123456") || strings.Contains(r.Body.String(), "SECabcdef123456") {
		t.Fatalf("the API leaked a secret: %s", r.Body)
	}
	var listed struct {
		Channels []struct {
			ID         int64          `json:"id"`
			Config     map[string]any `json:"config"`
			SecretKeys []string       `json:"secret_keys"`
		} `json:"channels"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var mine *struct {
		ID         int64          `json:"id"`
		Config     map[string]any `json:"config"`
		SecretKeys []string       `json:"secret_keys"`
	}
	for i := range listed.Channels {
		if listed.Channels[i].ID == chID {
			mine = &listed.Channels[i]
		}
	}
	if mine == nil {
		t.Fatal("New Channel does not appear in list")
	}
	if !notify.IsMasked(fmt.Sprint(mine.Config["webhook"])) || !notify.IsMasked(fmt.Sprint(mine.Config["secret"])) {
		t.Fatalf("credential fields should be masked: %v", mine.Config)
	}
	if len(mine.SecretKeys) == 0 {
		t.Fatal("The interface should inform the front end which fields are supported")
	}

	// PATCH Change name only + Back-to-back mask: the genuine document must be retained as it is..
	body, _ := json.Marshal(map[string]any{
		"name":   "After the change of name",
		"config": map[string]any{"webhook": fmt.Sprint(mine.Config["webhook"]), "secret": fmt.Sprint(mine.Config["secret"])},
	})
	if r := f.request("PATCH", fmt.Sprintf("/api/notify/channels/%d", chID), string(body)); r.Code != 200 {
		t.Fatalf("Update failed %d: %s", r.Code, r.Body)
	}
	cfg := f.channelConfig(t, chID)
	if cfg["webhook"] != "https://oapi.dingtalk.com/robot/send?access_token=abc123456" {
		t.Fatalf("sending the masked value back overwrote the real webhook: %v", cfg["webhook"])
	}
	if cfg["secret"] != "SECabcdef123456" {
		t.Fatalf("sending the masked value back overwrote secret: %v", cfg["secret"])
	}
	if f.channel(t, chID).Name != "After the change of name" {
		t.Fatal("Name not updated")
	}

	// Visible Clear secret Should enter into force (as distinct from[Send back the mask.=No change]).
	body, _ = json.Marshal(map[string]any{"config": map[string]any{"secret": ""}})
	if r := f.request("PATCH", fmt.Sprintf("/api/notify/channels/%d", chID), string(body)); r.Code != 200 {
		t.Fatalf("Clear secret Failed %d: %s", r.Code, r.Body)
	}
	if _, still := f.channelConfig(t, chID)["secret"]; still {
		t.Fatal("Empty string to empty secret")
	}
}

func (f *notifyFixture) channelConfig(t *testing.T, id int64) map[string]any {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal(f.channel(t, id).Config, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestNotifyChannelAPICreateValidation(t *testing.T) {
	f := newNotifyFixture(t)
	cases := []struct {
		name    string
		payload map[string]any
		wantSub string
	}{
		{"invalid type", map[string]any{"name": "x", "kind": "nope", "config": map[string]any{}}, "Invalid channel type"},
		{"missing name", map[string]any{"kind": notify.KindDingTalk, "config": map[string]any{"webhook": "https://e.com/h"}}, "Missing channel name"},
		{"missing webhook", map[string]any{"name": "x", "kind": notify.KindDingTalk, "config": map[string]any{}}, "Webhook"},
		{"illegal webhook scheme", map[string]any{"name": "x", "kind": notify.KindDingTalk, "config": map[string]any{"webhook": "file:///etc/passwd"}}, "invalid Webhook address"},
		{"invalid mode", map[string]any{"name": "x", "kind": notify.KindDingTalk, "mode": "sometimes", "config": map[string]any{"webhook": "https://e.com/h"}}, "Invalid delivery mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.payload)
			r := f.request("POST", "/api/notify/channels", string(raw))
			if r.Code != 400 {
				t.Fatalf("expected 400, got %d: %s", r.Code, r.Body)
			}
			if !strings.Contains(r.Body.String(), tc.wantSub) {
				t.Fatalf("Error message should be mentioned %q,get %s", tc.wantSub, r.Body)
			}
		})
	}
	if r := f.request("DELETE", "/api/notify/channels/99999999", ""); r.Code != 404 {
		t.Fatalf("deleting a missing channel should be 404, got %d", r.Code)
	}
}

func TestNotifyFilterBlocksBelowThreshold(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "Severe only",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": hook.URL},
		"filter": map[string]any{"min_severity": "critical"},
	})
	f.record(t, "Low risk", "low")
	if _, _, err := f.pg.FanOutPendingEvents(context.Background(), 500); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pg.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE channel_id=$1`, chID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a finding below the threshold must not create a delivery, got %d", n)
	}
	f.n.stepRealtime(context.Background(), f.channel(t, chID), 50, "")
	if hook.count() != 0 {
		t.Fatal("The filtered bug should not send a message.")
	}
}

func TestNotifyDigestBatchesMultipleFindingsIntoOneMessage(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "Summary transfer",
		"kind":   notify.KindDingTalk,
		"mode":   db.NotifyModeDigest,
		"config": map[string]any{"webhook": hook.URL},
	})
	for i := 0; i < 3; i++ {
		f.record(t, fmt.Sprintf("digest-finding-%d", i+1), "high")
	}
	ctx := context.Background()
	if _, _, err := f.pg.FanOutPendingEvents(ctx, 500); err != nil {
		t.Fatal(err)
	}
	ch := f.channel(t, chID)

	// Not due: not issued.
	f.n.stepDigest(ctx, ch, 50, "")
	if hook.count() != 0 {
		t.Fatal("The aggregate batch was sent before the due date.")
	}

	// Following the old rule: 3 synthesizing a message.
	f.agePendingBatch(t, chID)
	f.n.stepDigest(ctx, ch, 50, "")
	if got := hook.count(); got != 1 {
		t.Fatalf("three findings should be one message, sent %d", got)
	}
	text := markdownText(t, hook.last(t))
	if !strings.Contains(text, "past 30 minutes") || !strings.Contains(text, "3 findings") {
		t.Fatalf("summary is missing the count or the time window:\n%s", text)
	}
	for i := 1; i <= 3; i++ {
		if !strings.Contains(text, fmt.Sprintf("digest-finding-%d", i)) {
			t.Fatalf("summary is missing item %d:\n%s", i, text)
		}
	}
	// The same number should be shared batch_id.
	var distinct, total int
	if err := f.pg.QueryRow(`SELECT count(DISTINCT batch_id), count(*) FROM notification_deliveries WHERE channel_id=$1`, chID).Scan(&distinct, &total); err != nil {
		t.Fatal(err)
	}
	if total != 3 || distinct != 1 {
		t.Fatalf("three deliveries should share one batch_id, got distinct=%d total=%d", distinct, total)
	}
}

func TestNotifyDisabledChannelDoesNotSend(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":    "Disable channel",
		"kind":    notify.KindDingTalk,
		"enabled": false,
		"config":  map[string]any{"webhook": hook.URL},
	})
	f.record(t, "Gaps during Disable", "critical")
	if _, _, err := f.pg.FanOutPendingEvents(context.Background(), 500); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pg.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE channel_id=$1`, chID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a disabled channel must not create a delivery, got %d", n)
	}
}

func TestNotifyStatusChangeDelivery(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "status changes",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": hook.URL},
		"filter": map[string]any{"on_status_change": true},
	})
	finding := f.record(t, "Example of status change", "high")
	r := f.request("PATCH", fmt.Sprintf("/api/exploration/findings/%d", finding), `{"status":"fixed"}`)
	if r.Code != 200 {
		t.Fatalf("Change failed %d: %s", r.Code, r.Body)
	}
	f.deliver(t, chID, "")

	// There should be two.:fixed That's a change of status.;finding_created That one could be sent in the same round..
	// The change in status is actually created later, but it doesn't depend on order..
	found := false
	for i := 0; i < hook.count(); i++ {
		text := markdownText(t, hook.body(t, i))
		if strings.Contains(text, "Status change") && strings.Contains(text, "Fixed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("did not receive a message containing Status change → Fixed (%d messages)", hook.count())
	}
}

func TestNotifyStatusChangeSuppressedByDefault(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "status changes off",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": hook.URL},
	})
	finding := f.record(t, "Do not subscribe to changes", "high")
	if r := f.request("PATCH", fmt.Sprintf("/api/exploration/findings/%d", finding), `{"status":"false_positive"}`); r.Code != 200 {
		t.Fatalf("Change failed %d: %s", r.Code, r.Body)
	}
	if _, _, err := f.pg.FanOutPendingEvents(context.Background(), 500); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pg.QueryRow(`SELECT count(*) FROM notification_deliveries d
JOIN notification_events e ON e.id = d.event_id
WHERE d.channel_id=$1 AND e.kind=$2`, chID, notify.EventFindingStatusChanged).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("a channel that does not subscribe to status changes must not receive one, got %d", n)
	}
}

func TestNotifyTestMessageEndpoint(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "Test Send",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": hook.URL},
	})
	if r := f.request("POST", fmt.Sprintf("/api/notify/channels/%d/test", chID), ""); r.Code != 200 {
		t.Fatalf("Test Sender Failed %d: %s", r.Code, r.Body)
	}
	if hook.count() != 1 {
		t.Fatalf("the fake receiver should get 1 test message, got %d", hook.count())
	}
	// The test message must be able to tell at first sight that it's a test. Hole.
	if text := markdownText(t, hook.last(t)); !strings.Contains(text, "Test") {
		t.Fatalf("Test messages should be marked as tests.: %s", text)
	}
	// When the configuration is broken, the original error of the channel is returned to the user..
	badID := f.createChannel(t, map[string]any{
		"name":   "Bad address",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": "http://127.0.0.1:1/hook"},
	})
	if r := f.request("POST", fmt.Sprintf("/api/notify/channels/%d/test", badID), ""); r.Code != 502 {
		t.Fatalf("expected HTTP 502, got %d: %s", r.Code, r.Body)
	}
}

func TestNotifyDeliveriesHistoryAndRetry(t *testing.T) {
	f := newNotifyFixture(t)
	// Point at an address that always fails, to create a failed delivery.
	chID := f.createChannel(t, map[string]any{
		"name":   "Failed to try again",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": "http://127.0.0.1:1/hook"},
	})
	f.record(t, "It'll fail.", "high")
	ctx := context.Background()
	if _, _, err := f.pg.FanOutPendingEvents(ctx, 500); err != nil {
		t.Fatal(err)
	}
	ch := f.channel(t, chID)
	// I've spent all my time trying..
	for i := 0; i < db.MaxNotifyAttempts; i++ {
		f.n.stepRealtime(ctx, ch, 50, "")
		if _, err := f.pg.Exec(`UPDATE notification_deliveries SET next_attempt_at = now() - interval '1 minute' WHERE channel_id=$1`, chID); err != nil {
			t.Fatal(err)
		}
	}
	var state string
	if err := f.pg.QueryRow(`SELECT state FROM notification_deliveries WHERE channel_id=$1`, chID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != db.NotifyStateFailed {
		t.Fatalf("After re-drive, read failed,get %s", state)
	}

	r := f.request("GET", fmt.Sprintf("/api/notify/deliveries?channel_id=%d&state=failed", chID), "")
	if r.Code != 200 {
		t.Fatalf("History failed. %d: %s", r.Code, r.Body)
	}
	var hist struct {
		Deliveries []struct {
			ID        int64  `json:"id"`
			State     string `json:"state"`
			LastError string `json:"last_error"`
			Attempts  int    `json:"attempts"`
			Title     string `json:"title"`
		} `json:"deliveries"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &hist); err != nil {
		t.Fatal(err)
	}
	if hist.Total != 1 || len(hist.Deliveries) != 1 {
		t.Fatalf("I think we got it. 1 Failed delivery, got total=%d len=%d", hist.Total, len(hist.Deliveries))
	}
	if hist.Deliveries[0].LastError == "" {
		t.Fatal("History should include the reason for the failure, otherwise the user cannot check.")
	}
	if hist.Deliveries[0].Attempts < db.MaxNotifyAttempts {
		t.Fatalf("The number of attempts should be recorded. %d", hist.Deliveries[0].Attempts)
	}
	if hist.Deliveries[0].Title != "It'll fail." {
		t.Fatalf("history should include the finding title, got %q", hist.Deliveries[0].Title)
	}

	// Re-activate manually: should return pending And count to zero..
	if r := f.request("POST", fmt.Sprintf("/api/notify/deliveries/%d/retry", hist.Deliveries[0].ID), ""); r.Code != 200 {
		t.Fatalf("Resend failed %d: %s", r.Code, r.Body)
	}
	var attempts int
	if err := f.pg.QueryRow(`SELECT state, attempts FROM notification_deliveries WHERE id=$1`, hist.Deliveries[0].ID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != db.NotifyStatePending || attempts != 0 {
		t.Fatalf("After reissue, insert pending and attempts=0,get %s/%d", state, attempts)
	}
}

func TestNotifyMetaAndSettingsRoundTrip(t *testing.T) {
	f := newNotifyFixture(t)
	r := f.request("GET", "/api/notify/meta", "")
	if r.Code != 200 {
		t.Fatalf("meta Failed: %s", r.Body)
	}
	var meta struct {
		Kinds []struct {
			Kind       string   `json:"kind"`
			SecretKeys []string `json:"secret_keys"`
		} `json:"kinds"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Kinds) != len(notify.Kinds()) {
		t.Fatalf("meta should list all %d channel types, got %d", len(notify.Kinds()), len(meta.Kinds))
	}
	for _, k := range meta.Kinds {
		if len(k.SecretKeys) == 0 {
			t.Errorf("Channel %s Documented fields not reported", k.Kind)
		}
	}

	// Three global set round trip. The tail slash should be standardized or the chain will spell out. "//function/...".
	if r := f.request("PUT", "/api/settings", `{"notify_public_base_url":"https://artex.example.com/","notify_digest_interval_min":15,"notify_enabled":true}`); r.Code != 200 {
		t.Fatalf("Writing Settings Failed %d: %s", r.Code, r.Body)
	}
	t.Cleanup(func() {
		f.pg.Exec(`DELETE FROM settings WHERE key IN ($1,$2)`, settingNotifyPublicBaseURL, settingNotifyDigestMinutes)
	})
	payload := f.s.settingsPayload()
	if payload["notify_public_base_url"] != "https://artex.example.com" {
		t.Fatalf("Backlink address not standardized: %v", payload["notify_public_base_url"])
	}
	if payload["notify_digest_interval_min"] != 15 {
		t.Fatalf("Summary cycle not effective: %v", payload["notify_digest_interval_min"])
	}

	// Illegal value should be rejected.
	for _, body := range []string{
		`{"notify_public_base_url":"ftp://x"}`,
		`{"notify_digest_interval_min":0}`,
		`{"notify_digest_interval_min":99999}`,
	} {
		if r := f.request("PUT", "/api/settings", body); r.Code != 400 {
			t.Errorf("%s should return 400, got %d", body, r.Code)
		}
	}
}

// TestNotifyDeepLinkUsesPublicBaseURL Override chain fusion: paired public_base_url hour
// The message must be buttoned. ActionCard,And the link points to the gap details page.
func TestNotifyDeepLinkUsesPublicBaseURL(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "Back chain.",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": hook.URL},
	})
	finding := f.record(t, "finding with a deep link", "high")
	f.deliver(t, chID, "https://artex.example.com")

	body := hook.last(t)
	card, _ := body["actionCard"].(map[string]any)
	if card == nil {
		t.Fatalf("Apply when chain returns ActionCard,get msgtype=%v", body["msgtype"])
	}
	want := fmt.Sprintf("https://artex.example.com/function/findings/detail?id=%d", finding)
	if card["singleURL"] != want {
		t.Fatalf("Wrong chain.\nExpectations %s\nget %v", want, card["singleURL"])
	}
}

// TestNotifyNoDeepLinkWithoutBaseURL Inverse Overwrite: No bad chain should occur without external address Answer.
// (Like pointing. localhost (or relative path) markdown.
func TestNotifyNoDeepLinkWithoutBaseURL(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "No Return Chain",
		"kind":   notify.KindDingTalk,
		"config": map[string]any{"webhook": hook.URL},
	})
	f.record(t, "finding with no deep link", "high")
	f.deliver(t, chID, "")

	body := hook.last(t)
	if body["msgtype"] != "markdown" {
		t.Fatalf("without an external base URL the message should be markdown, got %v", body["msgtype"])
	}
	if text := markdownText(t, body); strings.Contains(text, "View details") {
		t.Fatalf("with no external base URL the message must not contain a detail link:\n%s", text)
	}
}

// TestNotifyDigestSegmentsAndDefersRemainder is the end-to-end proof of the silent-loss fix.
//
// A digest is capped by the channel length (WeCom markdown is 4096 bytes). When a batch does not
// fit, it must be split on whole findings: the ones that fit are marked sent, and the rest go back
// on the queue for the next message. The old code marked the whole batch sent, so truncated findings
// were in neither the message nor the failure list, history said success, and the finding vanished.
//
// Assert four things: (1) only the findings that fit are marked sent, (2) the rest stay pending,
// (3) deferred rows do not consume a retry, (4) another round sends the rest and does not stall.
func TestNotifyDigestSegmentsAndDefersRemainder(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	// WeCom: markdown limit is 4096 bytes, the tightest of the six channels.
	chID := f.createChannel(t, map[string]any{
		"name":   "Summary of subparagraphs",
		"kind":   notify.KindWeCom,
		"mode":   db.NotifyModeDigest,
		"config": map[string]any{"webhook": hook.URL},
	})
	const total = 60
	// A long title so 60 findings are far past 4096 bytes and must be split.
	longName := strings.Repeat("Overlong Hole Name", 6)
	for i := 0; i < total; i++ {
		f.record(t, longName+strconv.Itoa(i+1), "high")
	}
	ctx := context.Background()
	if _, _, err := f.pg.FanOutPendingEvents(ctx, 500); err != nil {
		t.Fatal(err)
	}
	f.agePendingBatch(t, chID)
	ch := f.channel(t, chID)

	f.n.stepDigest(ctx, ch, 50, "")
	if hook.count() != 1 {
		t.Fatalf("Should send only one message. %d", hook.count())
	}

	var sent, pending int
	if err := f.pg.QueryRow(`SELECT
    count(*) FILTER (WHERE state=$2),
    count(*) FILTER (WHERE state=$3)
  FROM notification_deliveries WHERE channel_id=$1`, chID, db.NotifyStateSent, db.NotifyStatePending).
		Scan(&sent, &pending); err != nil {
		t.Fatal(err)
	}
	if sent == 0 {
		t.Fatal("expected some entries to be marked sent")
	}
	if pending == 0 {
		t.Fatalf("a batch of %d cannot fit in 4096 bytes; some must stay pending; sent=%d", total, sent)
	}
	if sent+pending != total {
		t.Fatalf("delivery counts do not add up: sent=%d pending=%d total=%d (neither sent nor pending means lost)", sent, pending, total)
	}
	// How many more are not included in this article?.
	if text := markdownText(t, hook.last(t)); !strings.Contains(text, "remain") {
		t.Fatalf("the message should say some entries were left out:\n%.400s", text)
	}

	// Postponed entries may not consume the retest budget: when received attempts Optimistic. +1,We need to reduce it when we delay it..
	var maxAttempts int
	if err := f.pg.QueryRow(`SELECT COALESCE(max(attempts),0) FROM notification_deliveries
WHERE channel_id=$1 AND state=$2`, chID, db.NotifyStatePending).Scan(&maxAttempts); err != nil {
		t.Fatal(err)
	}
	if maxAttempts > 0 {
		t.Fatalf("postponed entries must not consume retries (otherwise a few delays mark them failed), got attempts=%d", maxAttempts)
	}

	// Keep going until everything is delivered, and assert that it really took more than one round.
	// Stronger than "the second round finished": segmentation must not stall and must not drop the remainder.
	rounds := 0
	for {
		var undelivered int
		if err := f.pg.QueryRow(`SELECT count(*) FROM notification_deliveries
WHERE channel_id=$1 AND state <> $2 AND state <> $3`, chID, db.NotifyStateSent, db.NotifyStateFailed).
			Scan(&undelivered); err != nil {
			t.Fatal(err)
		}
		if undelivered == 0 {
			break
		}
		rounds++
		if rounds > total+5 {
			t.Fatalf("segmented delivery did not converge: %d rounds and %d still outstanding", rounds, undelivered)
		}
		before := hook.count()
		f.n.stepDigest(ctx, ch, 50, "")
		if hook.count() == before {
			t.Fatalf("round %d made no progress; %d would stay stuck forever", rounds, undelivered)
		}
	}
	if rounds < 2 {
		t.Fatalf("one 4096-byte message cannot hold %d long-title findings; expected several rounds, got %d", total, rounds)
	}
	// Every round after the first should be a pure continuation. The fake receiver never rejects, so nothing should be failed.
	var failed int
	if err := f.pg.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE channel_id=$1 AND state=$2`,
		chID, db.NotifyStateFailed).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if failed != 0 {
		t.Fatalf("the fake receiver always succeeds, so nothing should be failed, got %d", failed)
	}
}

// TestNotifyBackoffTableMatchesAttemptBudget guards against the two constants drifting apart.
//
// The retry budget (db.MaxNotifyAttempts) and the backoff table (notifyBackoff) live in different
// packages: one is the state machine's policy, the other is the engine's timing. Changing only one
// — for example raising the budget to 5 and forgetting a backoff slot — does not fail to compile.
// Retries 4 and 5 would just reuse the last interval, so retries mysteriously slow down.
// Asserting equal length makes that drift fail in CI.
func TestNotifyBackoffTableMatchesAttemptBudget(t *testing.T) {
	if len(notifyBackoff) != db.MaxNotifyAttempts {
		t.Fatalf("backoff slots (%d) and max attempts (%d) disagree — changing one requires changing the other",
			len(notifyBackoff), db.MaxNotifyAttempts)
	}
	// Backoff intervals must be non-decreasing. If they shrink, retries get more
	// aggressive and make rate limiting worse.
	for i := 1; i < len(notifyBackoff); i++ {
		if notifyBackoff[i] < notifyBackoff[i-1] {
			t.Fatalf("backoff intervals must be non-decreasing: slot %d %v < slot %d %v",
				i, notifyBackoff[i], i-1, notifyBackoff[i-1])
		}
	}
}

// TestNotifyRateLimitDoesNotConsumeRetryBudget locks the order: take a token, then claim the delivery.
// If that were reversed, a delivery blocked by the rate limit would already have counted one attempt.
// Pure waiting would drain the retry budget and the delivery would end in failed.
func TestNotifyRateLimitDoesNotConsumeRetryBudget(t *testing.T) {
	// Exercise the token bucket only. Do not build a Server for it.
	n := &Notifier{buckets: map[int64]*notifyBucket{}}
	now := time.Now()
	// One per minute: a full bucket yields at most one.
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now); got != 1 {
		t.Fatalf("a full bucket at 1/min should yield 1 token, got %d", got)
	}
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now.Add(time.Millisecond)); got != 0 {
		t.Fatalf("an empty bucket should return 0 immediately, got %d", got)
	}
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now.Add(30*time.Second)); got != 0 {
		t.Fatalf("half a period must not refill a whole token, got %d", got)
	}
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now.Add(time.Minute)); got != 1 {
		t.Fatalf("one full period should refill 1 token, got %d", got)
	}
	// An unlimited channel still has a per-round cap so one tick cannot be stuck on an infinite backlog.
	if got := n.takeTokens(2, 0, notifyUnlimitedBurstPerTick+10, now); got != notifyUnlimitedBurstPerTick {
		t.Fatalf("unlimited rate should return the per-round cap %d, got %d", notifyUnlimitedBurstPerTick, got)
	}
	// Token buckets are independent per channel.
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now.Add(time.Millisecond)); got != 0 {
		t.Fatalf("channel 1's bucket should still be empty, got %d", got)
	}
}

// TestNotifyTakeTokensKeepsUnusedTokens locks the "take only want" behavior.
//
// An older implementation drained the whole bucket and let the caller truncate.
// A channel at rate=100/min with a full bucket that only needed 5 this round
// discarded the other 95 tokens. A round with nothing waiting was charged the same way.
// The comment's promise, "a backlog can flush rate_per_min items at once", was then impossible.
func TestNotifyTakeTokensKeepsUnusedTokens(t *testing.T) {
	n := &Notifier{buckets: map[int64]*notifyBucket{}}
	now := time.Now()
	// The bucket starts full (100). This round wants 5.
	if got := n.takeTokens(1, 100, 5, now); got != 5 {
		t.Fatalf("want=5 should take exactly 5 tokens, got %d", got)
	}
	// The remaining 95 must still be in the bucket, not discarded.
	// Do not advance time, so a later take can only come from stock, not a refill.
	if got := n.takeTokens(1, 100, 95, now); got != 95 {
		t.Fatalf("the remaining tokens should still be available (want 95), got %d — the bucket was drained", got)
	}
	if got := n.takeTokens(1, 100, 1, now); got != 0 {
		t.Fatalf("the bucket is empty and should return 0, got %d", got)
	}
	// want<=0 must not spend any tokens (an empty round is free).
	n2 := &Notifier{buckets: map[int64]*notifyBucket{}}
	if got := n2.takeTokens(1, 20, 0, now); got != 0 {
		t.Fatalf("want=0 should return 0, got %d", got)
	}
	if got := n2.takeTokens(1, 20, 20, now); got != 20 {
		t.Fatalf("the want=0 call must not have spent tokens; a full 20 should still be available, got %d", got)
	}
}

// TestDigestTickPlanDecouplesBatchSizeFromSendBudget pins the two different units of digest mode.
//
// If the digest batch size were tied to the per-tick request budget, a channel with rate_per_min=20
// would earn one token per 3-second tick and each digest would hold one finding. That is not a digest,
// and the header would still say "1 finding in the past 30 minutes". The bug does not error.
// Existing end-to-end tests pass stepDigest a large limit and skip the budget math inside step,
// so this test asserts the decision itself.
func TestDigestTickPlanDecouplesBatchSizeFromSendBudget(t *testing.T) {
	tokens, claimLimit := digestTickPlan()
	// One batch = one message = one HTTP request = one token. Tokens count messages, not findings.
	if tokens != 1 {
		t.Fatalf("one digest batch is one message and must cost exactly 1 token, got %d", tokens)
	}
	if claimLimit != db.MaxDigestBatchSize {
		t.Fatalf("digest batch size should be the in-memory cap db.MaxDigestBatchSize=%d, got %d",
			db.MaxDigestBatchSize, claimLimit)
	}
	// The batch size must be much larger than the per-tick request budget. If they are the same
	// magnitude, "how many messages to send" and "how many findings in a batch" have been collapsed into one number.
	if claimLimit <= notifyMaxSendsPerChannelPerTick {
		t.Fatalf("digest batch size %d must not be capped by the per-tick request budget %d — "+
			"the request budget is how many HTTP calls fit in the lease, and how many findings a batch holds is a different unit",
			claimLimit, notifyMaxSendsPerChannelPerTick)
	}
}

// TestNotifyTickBudgetFitsWithinLease is another drift guard.
//
// notifyMaxSendsPerChannelPerTick is derived from the lease: the worst-case time of the serial
// sends in one round must stay under the lease. Otherwise the last sends outlive the lease, and
// another instance reclaims and resends them. The three constants live in different places, and
// changing any one can break the relationship without a compiler error, so pin it here.
func TestNotifyTickBudgetFitsWithinLease(t *testing.T) {
	worst := time.Duration(notifyMaxSendsPerChannelPerTick) * notifySendTimeout
	if worst >= notifyLease {
		t.Fatalf("worst-case one-channel round %v must stay under the lease %v "+
			"(notifyMaxSendsPerChannelPerTick=%d × notifySendTimeout=%v)——"+
			"Change any of these three constants and check the other two simultaneously.",
			worst, notifyLease, notifyMaxSendsPerChannelPerTick, notifySendTimeout)
	}
}
