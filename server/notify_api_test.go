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

// This document overwhelms end-to-end behaviour for the delivery function: loopholes Library → Event → Distribution → Really? HTTP.
//
// A safety alert: these examples**Do not call global Notifier.step()**,It's just something you created.
// Channel Call stepRealtime/stepDigest.Because... step() It'll be all over the library.——
// It's a real nail in one./Microbots are running tests on the developer. step It'll take the test.
// The holes created were pushed into those groups. Called on a channel-by-channel to limit impact to test self-made false receivers Move!.
//
// Clearance: elimination of events and channels arising from this example by the end of the case, without leaving a backlog of real channels.
//
// The caliber.:stepRealtime/stepDigest No return value, internal log, so here's the assertion:
// **Observable external behaviour**(What's received by the false receiver, what's the status of the delivery line, not the function?
// Return value——It's closer to a real call path than putting a pillar on a return value..

// notifyFixture It's a public device for example in this document..
type notifyFixture struct {
	s       *Server
	pg      *db.DB
	request func(method, path, body string) *httptest.ResponseRecorder
	n       *Notifier
	// Self-built. task/exploration:Take the example of a loophole here and isolate it from other examples..
	taskID int64
	expID  int64
	// cleanupMark The event that follows is removed during cleanup.
	cleanupMark int64
}

func newNotifyFixture(t *testing.T) *notifyFixture {
	t.Helper()
	// All false receivers of this document are running at 127.0.0.1 Up, and the delivery default refuses loop back to address
	// (Prevention SSRF Call the same machine service and cloud metadata. Test visible to open this switch.;
	// Guard![Default Rejection]by notify The bag. ssrf_test.go override.
	t.Setenv(notify.AllowLocalTargetsEnv, "1")
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.pg

	// Build your own task:Shared devices trafficEvidenceServer Made it. task I can't get it. exploration id,
	// And the record gap has to provide it..
	task, err := s.m.CreateTask("Notify transfer test", "Organisation", nil, 0, 0)
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
	// Let the device be self-contained: fixture One-time tag for events that existed before is assigned.
	//
	// Why do you have to?:FanOutPendingEvents Yes**Global**We'll get all the unassigned incidents in Curly.
	// Expand to all matching channels. And shared devices trafficEvidenceServer You'll remember a loophole.
	// (It's the first time it returns. finding),Other examples may also have residues. Without quarantine.,
	// These sprawling events will be assigned to the channels of this example.[Yes. N Bar delivery]Such claims
	// Right and wrong.——And the law of error depends on the order of execution, which is harder than a direct failure. Cha..
	if _, err := pg.Exec(`UPDATE notification_events SET fanned_out = true WHERE id <= $1 AND NOT fanned_out`, mark); err != nil {
		t.Fatal(err)
	}

	f := &notifyFixture{s: s, pg: pg, request: request, n: newNotifier(s), taskID: taskID, expID: task.ExpID, cleanupMark: mark}
	t.Cleanup(func() {
		if _, err := pg.Exec(`DELETE FROM notification_events WHERE id > $1`, f.cleanupMark); err != nil {
			t.Logf("Failed to clear notification event: %v", err)
		}
	})
	// The total switch must be on. It's...).
	if err := pg.SetBool(settingNotifyEnabled, true); err != nil {
		t.Fatal(err)
	}
	return f
}

// record Take the real evidence and write down a loophole. Go back. finding id.
// This way will be**Same business.**Lee registered the push event——It's the hanger of the function..
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
		t.Fatalf("Record loophole failed: %v", err)
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
		t.Fatalf("There's no way to get back.: %s (%v)", r.Body, err)
	}
	t.Cleanup(func() { f.pg.Exec(`DELETE FROM notification_channels WHERE id=$1`, res.ID) })
	return res.ID
}

// fakeWebhook It's a recorded receipt of the requested body. End.
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
		t.Fatalf("Fake receiver only received %d Request not available %d strip", len(f.bodies), i)
	}
	return f.bodies[i]
}

func (f *fakeWebhook) last(t *testing.T) map[string]any {
	t.Helper()
	if f.count() == 0 {
		t.Fatal("No requests received from the false receiver")
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
		t.Fatalf("Should be sent 1 Message, Actual %d", hook.count())
	}
	text := markdownText(t, hook.last(t))
	for _, want := range []string{"SQLInjection", "High", "Abstract"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Message body missing %q:\n%s", want, text)
		}
	}
	// Organisation sent.
	var pending int
	if err := f.pg.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE channel_id=$1 AND state <> $2`,
		chID, db.NotifyStateSent).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("There's still after delivery. %d bar unmarked sent", pending)
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
		t.Fatalf("Column Channel Failed %d: %s", r.Code, r.Body)
	}
	if strings.Contains(r.Body.String(), "abc123456") || strings.Contains(r.Body.String(), "SECabcdef123456") {
		t.Fatalf("The interface leaked evidence.: %s", r.Body)
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
		t.Fatalf("Based field should be a mask value: %v", mine.Config)
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
		t.Fatalf("We've got cover.: %v", cfg["webhook"])
	}
	if cfg["secret"] != "SECabcdef123456" {
		t.Fatalf("Cover it up. secret It's covered.: %v", cfg["secret"])
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
		{"Type illegal", map[string]any{"name": "x", "kind": "nope", "config": map[string]any{}}, "Channel type invalid"},
		{"Missing Name", map[string]any{"kind": notify.KindDingTalk, "config": map[string]any{"webhook": "https://e.com/h"}}, "Missing channel name"},
		{"Missing webhook", map[string]any{"name": "x", "kind": notify.KindDingTalk, "config": map[string]any{}}, "Webhook"},
		{"webhook It's illegal.", map[string]any{"name": "x", "kind": notify.KindDingTalk, "config": map[string]any{"webhook": "file:///etc/passwd"}}, "Webhook Address invalid"},
		{"Pattern Illegal", map[string]any{"name": "x", "kind": notify.KindDingTalk, "mode": "sometimes", "config": map[string]any{"webhook": "https://e.com/h"}}, "Send mode invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.payload)
			r := f.request("POST", "/api/notify/channels", string(raw))
			if r.Code != 400 {
				t.Fatalf("Should return 400,get %d: %s", r.Code, r.Body)
			}
			if !strings.Contains(r.Body.String(), tc.wantSub) {
				t.Fatalf("Error message should be mentioned %q,get %s", tc.wantSub, r.Body)
			}
		})
	}
	if r := f.request("DELETE", "/api/notify/channels/99999999", ""); r.Code != 404 {
		t.Fatalf("Delete non-existent channels should 404,get %d", r.Code)
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
		t.Fatalf("A leak below the threshold should not create a delivery. %d strip", n)
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
		f.record(t, fmt.Sprintf("Summary loopholes%d", i+1), "high")
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
		t.Fatalf("The three should be consolidated in a single message. %d strip", got)
	}
	text := markdownText(t, hook.last(t))
	if !strings.Contains(text, "past 30 minutes") || !strings.Contains(text, "3 findings") {
		t.Fatalf("The sum message is missing the number of bars/Time Window:\n%s", text)
	}
	for i := 1; i <= 3; i++ {
		if !strings.Contains(text, fmt.Sprintf("Summary loopholes%d", i)) {
			t.Fatalf("The summary message is missing %d strip:\n%s", i, text)
		}
	}
	// The same number should be shared batch_id.
	var distinct, total int
	if err := f.pg.QueryRow(`SELECT count(DISTINCT batch_id), count(*) FROM notification_deliveries WHERE channel_id=$1`, chID).Scan(&distinct, &total); err != nil {
		t.Fatal(err)
	}
	if total != 3 || distinct != 1 {
		t.Fatalf("Three deliveries should be shared. batch_id,get distinct=%d total=%d", distinct, total)
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
		t.Fatalf("Discontinuation channels should not produce delivery. %d strip", n)
	}
}

func TestNotifyStatusChangeDelivery(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "Organisation",
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
		t.Fatalf("Not received[Status change → Fixed](total) %d strip)", hook.count())
	}
}

func TestNotifyStatusChangeSuppressedByDefault(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	chID := f.createChannel(t, map[string]any{
		"name":   "Can not open message",
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
		t.Fatalf("Unsubscribed change of status channel should not receive status change delivery, received %d strip", n)
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
		t.Fatalf("The false receiver should be received. 1 A test message. Got it. %d", hook.count())
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
	// Pointing to the address of the inevitable failure. failed Organisation.
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
		t.Fatalf("History should lead to a loophole. %q", hist.Deliveries[0].Title)
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
		t.Fatalf("meta All should be listed %d A channel. Got it. %d", len(notify.Kinds()), len(meta.Kinds))
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
			t.Errorf("%s Should return 400,get %d", body, r.Code)
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
	finding := f.record(t, "Bring back the loophole.", "high")
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
	f.record(t, "A loophole in the no-return chain", "high")
	f.deliver(t, chID, "")

	body := hook.last(t)
	if body["msgtype"] != "markdown" {
		t.Fatalf("Should be sent without an external address markdown,get %v", body["msgtype"])
	}
	if text := markdownText(t, body); strings.Contains(text, "View details") {
		t.Fatalf("No detail chain should appear without an external address Answer.:\n%s", text)
	}
}

// TestNotifyDigestSegmentsAndDefersRemainder Yes[Quietly lost.]End-to-end evidence of repair.
//
// Summarized messages are subject to the maximum length of the channel 4096 Bytes) , a batch must be filled**Press the whole article**Cut:
// The markings loaded into this article were delivered and the rest returned to the next line, etc. Once achieved, the whole lot was marked.
// Success——Those intercepted are neither in the message nor in the failed list, and the delivery history shows success.,
// The hole is gone..
//
// Four things.:① Only the actual number of bars is marked ② The rest is still pending. ③ Deferred entries
// **No retests consumed** ④ Another round will send the rest.).
func TestNotifyDigestSegmentsAndDefersRemainder(t *testing.T) {
	f := newNotifyFixture(t)
	hook := newFakeWebhook(t)
	// Use corporate Wisdom:markdown upper limit 4096 It's the tightest of the six channels..
	chID := f.createChannel(t, map[string]any{
		"name":   "Summary of subparagraphs",
		"kind":   notify.KindWeCom,
		"mode":   db.NotifyModeDigest,
		"config": map[string]any{"webhook": hook.URL},
	})
	const total = 60
	// Longer in the title, guaranteed. 60 It's far too far. 4096 bytes, necessary.
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
		t.Fatal("Other Organiser")
	}
	if pending == 0 {
		t.Fatalf("Groups %d It's impossible to load all the bars. 4096 Byte, left to be issued;sent=%d", total, sent)
	}
	if sent+pending != total {
		t.Fatalf("Inconsistent number of entries:sent=%d pending=%d total=%d(It's neither delivered nor prepared.=Lost)", sent, pending, total)
	}
	// How many more are not included in this article?.
	if text := markdownText(t, hook.last(t)); !strings.Contains(text, "remain") {
		t.Fatalf("The message should indicate that there are other entries not included in this article.:\n%.400s", text)
	}

	// Postponed entries may not consume the retest budget: when received attempts Optimistic. +1,We need to reduce it when we delay it..
	var maxAttempts int
	if err := f.pg.QueryRow(`SELECT COALESCE(max(attempts),0) FROM notification_deliveries
WHERE channel_id=$1 AND state=$2`, chID, db.NotifyStatePending).Scan(&maxAttempts); err != nil {
		t.Fatal(err)
	}
	if maxAttempts > 0 {
		t.Fatalf("Postponed entries should not consume the number of re-tests (other than a few that will fail), received attempts=%d", maxAttempts)
	}

	// Run over and over until it's condensed. The assertion is...**It's all there.**And there were multiple rounds.——
	// This is more than[Second round.]Stronger: it proves it won't get stuck, it won't get rid of the rest..
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
			t.Fatalf("Divisional delivery is not subdued: %d The wheel is still there. %d Article outstanding", rounds, undelivered)
		}
		before := hook.count()
		f.n.stepDigest(ctx, ch, 50, "")
		if hook.count() == before {
			t.Fatalf("No. %d There's no progress on the wheel. %d It'll be stuck forever.", rounds, undelivered)
		}
	}
	if rounds < 2 {
		t.Fatalf("One. 4096 I can't load the bytes. %d Long title gaps, should be distributed in multiple rounds, actually only used %d wheel", total, rounds)
	}
	// Every round after the first round should be**Pure renewal.**,No entries rejected by channel.
	var failed int
	if err := f.pg.QueryRow(`SELECT count(*) FROM notification_deliveries WHERE channel_id=$1 AND state=$2`,
		chID, db.NotifyStateFailed).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if failed != 0 {
		t.Fatalf("The false receiver always returns a successful entry. %d", failed)
	}
}

// TestNotifyBackoffTableMatchesAttemptBudget It's a drift-proof assertion..
//
// Retry budget(db.MaxNotifyAttempts)Sequence table with retreat(notifyBackoff)Two bags separated.:
// The former is the strategy of the status machine and the latter is the executive beat of the engine. If you change one of them,——For example, mention the budget. 5
// And then I forgot to drop out.——The code doesn't go wrong. It's just for the next one. 4,5 Try again the last slot interval,
// As in[Try again to slow down for no reason.],It's hard to think of it when you're checking..
// It's the same length. CI It's exposed..
func TestNotifyBackoffTableMatchesAttemptBudget(t *testing.T) {
	if len(notifyBackoff) != db.MaxNotifyAttempts {
		t.Fatalf("Number of exit slots(%d)with maximum number of attempts(%d)Inconsistencies——One has to be the other.",
			len(notifyBackoff), db.MaxNotifyAttempts)
	}
	// The distance between retreats must be kept intact, otherwise the re-test will increase the speed of the trials and the flow limits..
	for i := 1; i < len(notifyBackoff); i++ {
		if notifyBackoff[i] < notifyBackoff[i-1] {
			t.Fatalf("The distance between retreats must be kept in order: %d Trail %v < No. %d Trail %v",
				i, notifyBackoff[i], i-1, notifyBackoff[i-1])
		}
	}
}

// TestNotifyRateLimitDoesNotConsumeRetryBudget Lock it.[Take the token and get it.]Order.
// If it's the other way around, it's already been counted once. attempts,
// The budget will be drained by pure waiting, and finally it will enter. failed.
func TestNotifyRateLimitDoesNotConsumeRetryBudget(t *testing.T) {
	// Only the barrel itself, no need. Server(I shouldn't have built one for it.).
	n := &Notifier{buckets: map[int64]*notifyBucket{}}
	now := time.Now()
	// Every minute 1 Article: Most when full 1 strip.
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now); got != 1 {
		t.Fatalf("Every minute when the bucket is full 1 Articles to be drawn 1 A token. Got it. %d", got)
	}
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now.Add(time.Millisecond)); got != 0 {
		t.Fatalf("Return as soon as the token is exhausted 0,get %d", got)
	}
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now.Add(30*time.Second)); got != 0 {
		t.Fatalf("Halfway should not be filled with a token. %d", got)
	}
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now.Add(time.Minute)); got != 1 {
		t.Fatalf("Recoverable after one cycle 1 A token. Got it. %d", got)
	}
	// Limited ceilings for open channels to avoid an unlimited backlog of single rounds Hold on..
	if got := n.takeTokens(2, 0, notifyUnlimitedBurstPerTick+10, now); got != notifyUnlimitedBurstPerTick {
		t.Fatalf("Unlimited flow should return the maximum per round %d,get %d", notifyUnlimitedBurstPerTick, got)
	}
	// The barrel between channels is independent of each other..
	if got := n.takeTokens(1, 1, notifyMaxSendsPerChannelPerTick, now.Add(time.Millisecond)); got != 0 {
		t.Fatalf("Channel 1 The buckets should still be empty, get %d", got)
	}
}

// TestNotifyTakeTokensKeepsUnusedTokens Lock it.[Only want pieces]Semantic.
//
// Once achieved, the barrel was empty before it was cut off by the caller. rate=100/min The channels are full of buckets.,
// One round only. 5 Bar, left 95 A token is dropped; the channel is not subject to the same button as the delivery.
// The result is what the note says.[It'll be one-off when there's a backlog. rate_per_min strip]Not under any circumstances..
func TestNotifyTakeTokensKeepsUnusedTokens(t *testing.T) {
	n := &Notifier{buckets: map[int64]*notifyBucket{}}
	now := time.Now()
	// The barrel is initially full.(100),It's only for this round. 5 pieces.
	if got := n.takeTokens(1, 100, 5, now); got != 5 {
		t.Fatalf("want=5 It should be right there. 5 A token. Got it. %d", got)
	}
	// Critical assertion: the rest 95 The one must still be in the bucket, not be left empty..
	// Without advance time, ensure that only stocks, not supplements, are available.
	if got := n.takeTokens(1, 100, 95, now); got != 95 {
		t.Fatalf("The remaining tokens should remain valid (expected) 95),get %d——The barrel is empty.", got)
	}
	if got := n.takeTokens(1, 100, 1, now); got != 0 {
		t.Fatalf("The buckets are out. We should go back. 0,get %d", got)
	}
	// want<=0 No token should be deducted.).
	n2 := &Notifier{buckets: map[int64]*notifyBucket{}}
	if got := n2.takeTokens(1, 20, 0, now); got != 0 {
		t.Fatalf("want=0 Should return 0,get %d", got)
	}
	if got := n2.takeTokens(1, 20, 20, now); got != 20 {
		t.Fatalf("want=0 It's not supposed to take away the cards. It should still be full. 20,get %d", got)
	}
}

// TestDigestTickPlanDecouplesBatchSizeFromSendBudget Collapse the two scales of aggregation..
//
// Once the size of the aggregate batch is on the budget per round,rate_per_min=20 There's only one channel.
// 3 second tick Add to 1 A token, so each aggregate message is only loaded. 1 A loophole.——Functionally no.
// It's a summary, and the message says,[near 30 min Add 1 A loophole.].This degradation doesn't make a mistake.,
// Existing end-to-end examples are not visible either. Here. stepDigest One big enough. limit,
// It's bypassed. step So here's the decision-making itself..
func TestDigestTickPlanDecouplesBatchSizeFromSendBudget(t *testing.T) {
	tokens, claimLimit := digestTickPlan()
	// Groups = A message. = One request. = A token. The token is for information, not a loophole..
	if tokens != 1 {
		t.Fatalf("One message from the group, which should be consumed. 1 A token. Got it. %d", tokens)
	}
	if claimLimit != db.MaxDigestBatchSize {
		t.Fatalf("The sum batch size should be memory upper db.MaxDigestBatchSize=%d,get %d",
			db.MaxDigestBatchSize, claimLimit)
	}
	// Key relationships: Batch size must be much larger than the budget requested per round. Once they're equal,,
	// It's another one.[Send a few messages.]and[A bunch of holes.]It's a number..
	if claimLimit <= notifyMaxSendsPerChannelPerTick {
		t.Fatalf("Summarize Batch Size %d Budget not subject to per round of requests %d Constraints——"+
			"The request for the budget was reversed by the lease.[How many requests?],With[A bunch of holes.]It's two scales.",
			claimLimit, notifyMaxSendsPerChannelPerTick)
	}
}

// TestNotifyTickBudgetFitsWithinLease It's another drift-proof assertion..
//
// Maximum number of deliverers per round for single channel(notifyMaxSendsPerChannelPerTick)It's from the lease.:
// The worst part of the series must take time. < Leases. Otherwise, the last few will expire before the lease is issued.,
// Multiple instances are retaken and duplicated when deployed. These three constants are in different places.,
// Any change could break the relationship without a report. Wrong.——That's why I nailed it here..
func TestNotifyTickBudgetFitsWithinLease(t *testing.T) {
	worst := time.Duration(notifyMaxSendsPerChannelPerTick) * notifySendTimeout
	if worst >= notifyLease {
		t.Fatalf("One-channel round takes the worst time. %v No lease should be reached or exceeded %v"+
			"(notifyMaxSendsPerChannelPerTick=%d × notifySendTimeout=%v)——"+
			"Change any of these three constants and check the other two simultaneously.",
			worst, notifyLease, notifyMaxSendsPerChannelPerTick, notifySendTimeout)
	}
}
