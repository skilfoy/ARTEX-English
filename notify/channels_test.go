package notify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// singleMsg Construct a single message with quotation marks and line breaks. Use it deliberately. `"` With `\n` Title/Abstract:
// It's just the illegality of a template plug. JSON Enter.
func singleMsg() Message {
	return Message{
		Items: []Item{{
			FindingID: 42,
			Name:      `Login "SQLInjection" Risk`,
			VulnClass: "SQLInjection",
			Severity:  "high",
			Summary:   "Parameter id\nUnfiltered leads to injection",
			Assets:    []string{"a.example.com", "b.example.com"},
			DetailURL: "https://artex.local/function/findings/detail?id=42",
		}},
	}
}

// batchMsg Construct a package message.
func batchMsg(n int) Message {
	m := Message{Batch: true, WindowMinutes: 30, HomeURL: "https://artex.local/function/findings"}
	for i := 0; i < n; i++ {
		m.Items = append(m.Items, Item{
			FindingID: int64(i + 1),
			Name:      "Vulnerability" + itoa(i+1),
			VulnClass: "XSS",
			Severity:  "medium",
			Summary:   "Reflective cross-station script",
			Assets:    []string{"target.example.com"},
		})
	}
	return m
}

// capturePost Start a false receiver, hand over the requested body to the asserted function.
func capturePost(t *testing.T, respBody string, assert func(t *testing.T, body map[string]any, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("The request is not legal. JSON: %v\nOriginal text: %s", err, raw)
			}
		}
		if assert != nil {
			assert(t, body, r)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDingTalkSendsActionCardWhenLinkPresent(t *testing.T) {
	srv := capturePost(t, `{"errcode":0,"errmsg":"ok"}`, func(t *testing.T, body map[string]any, _ *http.Request) {
		if body["msgtype"] != "actionCard" {
			t.Fatalf("When you have a return chain. actionCard,get %v", body["msgtype"])
		}
		card, _ := body["actionCard"].(map[string]any)
		if card["singleURL"] != "https://artex.local/function/findings/detail?id=42" {
			t.Errorf("Backlink lost.: %v", card["singleURL"])
		}
	})
	if _, err := (dingTalkChannel{}).Send(context.Background(), map[string]any{"webhook": srv.URL}, singleMsg()); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
}

func TestDingTalkFallsBackToMarkdownForBatch(t *testing.T) {
	srv := capturePost(t, `{"errcode":0,"errmsg":"ok"}`, func(t *testing.T, body map[string]any, _ *http.Request) {
		if body["msgtype"] != "markdown" {
			t.Fatalf("The summary message should be sent. markdown,get %v", body["msgtype"])
		}
		md, _ := body["markdown"].(map[string]any)
		if !strings.Contains(md["text"].(string), "past 30 minutes") {
			t.Errorf("Time window missing for summary body: %v", md["text"])
		}
	})
	if _, err := (dingTalkChannel{}).Send(context.Background(), map[string]any{"webhook": srv.URL}, batchMsg(3)); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
}

// TestDingTalkBusinessErrorIsPermanent Lock it.[HTTP 200 But... errcode Not 0]Decision.
// Do Not Check errcode You'll write down the delivery failure as a success.——It's all over the country. IM Platform common pits.
func TestDingTalkBusinessErrorIsPermanent(t *testing.T) {
	srv := capturePost(t, `{"errcode":310000,"errmsg":"keywords not in content"}`, nil)
	_, err := (dingTalkChannel{}).Send(context.Background(), map[string]any{"webhook": srv.URL}, singleMsg())
	if err == nil {
		t.Fatal("errcode Not 0 Wrong-doing.")
	}
	if !IsPermanent(err) {
		t.Fatalf("The mismatch is a configuration error and should be marked as a permanent failure and obtained %v", err)
	}
	if !strings.Contains(err.Error(), "310000") {
		t.Errorf("The error message should be carried with the platform code. %v", err)
	}
}

func TestWeComTruncatesCJKWithinByteLimit(t *testing.T) {
	var contentLen int
	srv := capturePost(t, `{"errcode":0,"errmsg":"ok"}`, func(t *testing.T, body map[string]any, _ *http.Request) {
		md, _ := body["markdown"].(map[string]any)
		content, _ := md["content"].(string)
		contentLen = len(content)
		if !utf8.ValidString(content) {
			t.Fatal("It's not legal to stop it. UTF-8——The entire Society refused to accept.")
		}
	})
	// Make enough Chinese aggregations to exceed. 4096 Bytes.
	m := batchMsg(200)
	if _, err := (weComChannel{}).Send(context.Background(), map[string]any{"webhook": srv.URL}, m); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
	if contentLen > weComMarkdownLimit {
		t.Fatalf("Text %d Bytes above micro cap %d", contentLen, weComMarkdownLimit)
	}
	if contentLen == 0 {
		t.Fatal("Text is empty")
	}
}

func TestWeComRateLimitIsRetryableButKeyErrorIsPermanent(t *testing.T) {
	limited := capturePost(t, `{"errcode":45009,"errmsg":"api freq out of limit"}`, nil)
	_, err := (weComChannel{}).Send(context.Background(), map[string]any{"webhook": limited.URL}, singleMsg())
	if err == nil || IsPermanent(err) {
		t.Fatalf("45009 It's a rolling window limit. You should try again. %v", err)
	}

	badKey := capturePost(t, `{"errcode":93000,"errmsg":"invalid webhook url"}`, nil)
	_, err = (weComChannel{}).Send(context.Background(), map[string]any{"webhook": badKey.URL}, singleMsg())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("93000 Yes key It's not working. It won't heal. %v", err)
	}
}

func TestFeishuCardStructureAndSign(t *testing.T) {
	const secret = "SECtest123"
	srv := capturePost(t, `{"code":0,"msg":"success"}`, func(t *testing.T, body map[string]any, _ *http.Request) {
		if body["msg_type"] != "interactive" {
			t.Fatalf("Should issue interactive cards, get %v", body["msg_type"])
		}
		card, _ := body["card"].(map[string]any)
		header, _ := card["header"].(map[string]any)
		if header["template"] != "orange" {
			t.Errorf("high The level should read orange Colors. Got them. %v", header["template"])
		}
		// Yeah. secret You'll have to bring the tags, or the flying book will be used to... 19021 Rejection.
		if body["sign"] == nil || body["timestamp"] == nil {
			t.Fatalf("Missing signature parameters: %v", body)
		}
		// There should be a button in the card element. url Pointing to gap details.
		elements, _ := card["elements"].([]any)
		foundButton := false
		for _, e := range elements {
			em, _ := e.(map[string]any)
			if em["tag"] != "action" {
				continue
			}
			actions, _ := em["actions"].([]any)
			for _, a := range actions {
				am, _ := a.(map[string]any)
				if am["url"] == "https://artex.local/function/findings/detail?id=42" {
					foundButton = true
				}
			}
		}
		if !foundButton {
			t.Fatal("There's no button in the card pointing to the details page.")
		}
	})
	cfg := map[string]any{"webhook": srv.URL, "secret": secret}
	if _, err := (feishuChannel{}).Send(context.Background(), cfg, singleMsg()); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
}

func TestFeishuWithoutSecretOmitsSign(t *testing.T) {
	srv := capturePost(t, `{"code":0,"msg":"success"}`, func(t *testing.T, body map[string]any, _ *http.Request) {
		if body["sign"] != nil || body["timestamp"] != nil {
			t.Fatalf("Not configured secret Do not carry signing parameters: %v", body)
		}
	})
	if _, err := (feishuChannel{}).Send(context.Background(), map[string]any{"webhook": srv.URL}, singleMsg()); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
}

func TestTelegramEscapesHTMLInUntrustedContent(t *testing.T) {
	var text string
	srv := capturePost(t, `{"ok":true}`, func(t *testing.T, body map[string]any, _ *http.Request) {
		text, _ = body["text"].(string)
		if body["parse_mode"] != "HTML" {
			t.Fatalf("Should be used HTML Parsing mode, get %v", body["parse_mode"])
		}
	})
	m := Message{Items: []Item{{
		Severity: "high",
		// Title and summary from target detected/Model output, untrustworthy content.
		Name:    `<script>alert(1)</script>`,
		Summary: "a & b < c",
	}}}
	if _, err := (telegramChannel{}).Send(context.Background(),
		map[string]any{"bot_token": "tok", "chat_id": "1", "base_url": srv.URL}, m); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
	if strings.Contains(text, "<script>") {
		t.Fatalf("No conversion HTML,Existence of injection: %q", text)
	}
	if !strings.Contains(text, "&lt;script&gt;") {
		t.Fatalf("An entity that expects to be transformed, gets %q", text)
	}
	if !strings.Contains(text, "a &amp; b") {
		t.Fatalf("& Not converted. Got it. %q", text)
	}
}

func TestTelegramErrorClassification(t *testing.T) {
	rateLimited := capturePost(t, `{"ok":false,"error_code":429,"description":"Too Many Requests"}`, nil)
	_, err := (telegramChannel{}).Send(context.Background(),
		map[string]any{"bot_token": "tok", "chat_id": "1", "base_url": rateLimited.URL}, singleMsg())
	if err == nil || IsPermanent(err) {
		t.Fatalf("429 ♪ Should try again ♪ %v", err)
	}

	forbidden := capturePost(t, `{"ok":false,"error_code":403,"description":"bot was blocked by the user"}`, nil)
	_, err = (telegramChannel{}).Send(context.Background(),
		map[string]any{"bot_token": "tok", "chat_id": "1", "base_url": forbidden.URL}, singleMsg())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("403 It's the configuration. It's a permanent failure. %v", err)
	}
}

func TestWebhookDefaultTemplateProducesValidJSON(t *testing.T) {
	// This is the meaning of the default template: any simple in the title with quotation marks and line breaks
	// `"title": "{{.Title}}"` It's illegal. JSON.{{json .}} No, not at all..
	srv := capturePost(t, `{"ok":true}`, func(t *testing.T, body map[string]any, _ *http.Request) {
		if body["title"] != `[🟠 High] Login "SQLInjection" Risk` {
			t.Errorf("Title not correctly restored: %v", body["title"])
		}
		items, _ := body["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("items The quantity should read 1,get %d", len(items))
		}
		it, _ := items[0].(map[string]any)
		if it["summary"] != "Parameter id\nUnfiltered leads to injection" {
			t.Errorf("Summary not correctly restored: %v", it["summary"])
		}
		// The value must be JSON Numbers instead of Strings(json:"...,string" That kind of writing will step on the pit.).
		if _, ok := it["finding_id"].(float64); !ok {
			t.Errorf("finding_id Other Organiser %T", it["finding_id"])
		}
	})
	if _, err := (webhookChannel{}).Send(context.Background(), map[string]any{"url": srv.URL}, singleMsg()); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
}

func TestWebhookCustomTemplateAndHeaders(t *testing.T) {
	srv := capturePost(t, `{"ok":true}`, func(t *testing.T, body map[string]any, r *http.Request) {
		if r.Header.Get("X-Token") != "s3cret" {
			t.Errorf("Custom Head Missing: %v", r.Header)
		}
		if body["msg"] != "3 strip" {
			t.Errorf("Error Rendering Custom Template: %v", body["msg"])
		}
		if body["first"] != "Vulnerability1" {
			t.Errorf("range Wrong extraction.: %v", body["first"])
		}
	})
	cfg := map[string]any{
		"url":           srv.URL,
		"headers":       map[string]any{"X-Token": "s3cret"},
		"body_template": `{"msg": {{json (printf "%d strip" .Count)}}, "first": {{json (index .Items 0).Name}}}`,
	}
	if _, err := (webhookChannel{}).Send(context.Background(), cfg, batchMsg(3)); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
}

func TestWebhookRejectsNonJSONRenderResult(t *testing.T) {
	cfg := map[string]any{"url": "https://example.com/hook", "body_template": `not json at all`}
	_, err := (webhookChannel{}).Send(context.Background(), cfg, singleMsg())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("Rendering result is not JSON Should be a permanent failure (the template is wrong, the retry is useless) %v", err)
	}
}

func TestWebhookValidateCatchesBadConfigEarly(t *testing.T) {
	bad := []map[string]any{
		{},
		{"url": "file:///etc/passwd"},
		{"url": "https://example.com", "method": "DELETE"},
		{"url": "https://example.com", "body_template": `{{.Items.`},
	}
	for i, cfg := range bad {
		if err := (webhookChannel{}).Validate(cfg); err == nil {
			t.Errorf("No. %d Group configuration should be rejected: %v", i, cfg)
		}
	}
}

func TestEmailMessageIsWellFormed(t *testing.T) {
	msg, err := buildEmailMessage("artex@example.com", []string{"a@example.com", "b@example.com"}, singleMsg())
	if err != nil {
		t.Fatalf("Failed to assemble mail: %v", err)
	}
	if !strings.HasPrefix(msg, "From: artex@example.com\r\n") {
		t.Fatalf("From Wrong head.:\n%s", msg)
	}
	if !strings.Contains(msg, "To: a@example.com, b@example.com\r\n") {
		t.Fatalf("To Wrong head.:\n%s", msg)
	}
	// Chinese subject must RFC 2047 Encoding, otherwise the client displays a bad code.
	if !strings.Contains(msg, "Subject: =?utf-8?") {
		t.Fatalf("Theme not done RFC 2047 Encode:\n%s", msg)
	}
	if dec, err := new(mime.WordDecoder).DecodeHeader(mustExtractHeader(t, msg, "Subject")); err != nil {
		t.Fatalf("Themes cannot be decoded: %v", err)
	} else if !strings.Contains(dec, "SQLInjection") {
		t.Fatalf("The subject was decoded wrongly.: %q", dec)
	}

	// The text is... base64,It's supposed to be legal. HTML.
	parts := strings.SplitN(msg, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatal("Mail Missing Header/Body Separation")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(parts[1]), "\r\n", ""))
	if err != nil {
		t.Fatalf("Text base64 Decoding failed: %v", err)
	}
	html := string(decoded)
	if !strings.HasPrefix(html, "<div") {
		t.Fatalf("Not the text. HTML: %.80s", html)
	}
	// Title appears as text position:HTML Double quotes in text content are valid characters and do not need to be transposed.
	// Here's the word.[Keep as it is.]It's to prevent any future error in adding a quote to the Chinese quote.
	// Show as &quot;.
	if !strings.Contains(html, `"SQLInjection"`) {
		t.Fatalf("The quotation marks in the title should be retained as they are in the text. ]: %.200s", html)
	}
}

// TestEmailEscapesStructuralInjection Covering the text of the mail really requires an injection.:
// The headlines and summaries of the holes are derived from the detected target and model output and are not credible. Text positions must be transposed
// & < >(Otherwise you can inject the label, and the attribute position must be transposed (or closed) href).
func TestEmailEscapesStructuralInjection(t *testing.T) {
	m := Message{
		Items: []Item{{
			Severity:  "high",
			Name:      `<script>alert(1)</script>`,
			Summary:   "a & b > c",
			DetailURL: `https://artex.local/x?a="onmouseover=alert(1)`,
		}},
	}
	html := htmlBody(m, 0)
	if strings.Contains(html, "<script>") {
		t.Fatalf("Title is not transposed and can be injected into the label: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("The entity expected to be converted: %s", html)
	}
	if !strings.Contains(html, "a &amp; b &gt; c") {
		t.Fatalf("& With > No conversion: %s", html)
	}
	// The chain of return is compatible with the administrator. public_base_url,It's more credible in itself, but the attribute position must remain.
	// Quote——Otherwise an address with quotation marks will close. href And inject the event processor..
	if strings.Contains(html, `onmouseover=alert(1)">`) {
		t.Fatalf("href Attribute is not correctly converted: %s", html)
	}
	if !strings.Contains(html, "&quot;") {
		t.Fatalf("The quotation marks of the attribute position should be converted: %s", html)
	}
}

func mustExtractHeader(t *testing.T, msg, name string) string {
	t.Helper()
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(line, name+": ") {
			return strings.TrimPrefix(line, name+": ")
		}
	}
	t.Fatalf("Not found %s head", name)
	return ""
}

func TestChannelValidateReportsMissingFields(t *testing.T) {
	// The error will be presented directly to the configuration, and it must be clear what is missing, not general.[Configuration Invalid].
	cases := []struct {
		kind   string
		cfg    map[string]any
		substr string
	}{
		{KindDingTalk, map[string]any{}, "Webhook"},
		{KindFeishu, map[string]any{}, "Webhook"},
		{KindWeCom, map[string]any{}, "Webhook"},
		{KindTelegram, map[string]any{}, "Bot Token"},
		{KindTelegram, map[string]any{"bot_token": "t"}, "Chat ID"},
		{KindEmail, map[string]any{}, "SMTP"},
		{KindEmail, map[string]any{"host": "h"}, "Port"},
		{KindEmail, map[string]any{"host": "h", "port": 587, "from": "f"}, "recipient"},
	}
	for _, tc := range cases {
		ch, ok := Get(tc.kind)
		if !ok {
			t.Fatalf("Channel %s Unregistered", tc.kind)
		}
		err := ch.Validate(tc.cfg)
		if err == nil {
			t.Errorf("%s Configuration %v Failed to verify", tc.kind, tc.cfg)
			continue
		}
		if !strings.Contains(err.Error(), tc.substr) {
			t.Errorf("%s Other Organiser %q,get %q", tc.kind, tc.substr, err.Error())
		}
	}
}

// TestEmailSMTPErrorClassification Lock it. SMTP 4xx/5xx Semantic distinction.
// If you... 4xx It's a permanent failure. A blacklist-enabled mail server makes every delivery the first.
// Try and fall failed —— And the grey list is exactly what it's supposed to be..
func TestEmailSMTPErrorClassification(t *testing.T) {
	cases := []struct {
		reply     string
		permanent bool
	}{
		{"450 4.7.1 Greylisting in action, please come back later", false},
		{"451 4.3.0 Temporary system failure", false},
		{"452 4.2.2 Mailbox full", false},
		{"550 5.1.1 User unknown", true},
		{"553 5.1.3 Bad address syntax", true},
		{"554 5.7.1 Relay access denied", true},
		// Press when no answer is reached[Retry]Deal with: I'd rather try it once more than the moment of possibility.
		// Fragmentation..
		{"unexpected EOF", false},
		{"", false},
	}
	for _, tc := range cases {
		err := smtpStageError("Recipient rejected", errors.New(tc.reply))
		if got := IsPermanent(err); got != tc.permanent {
			t.Errorf("Response %q: Expectations permanent=%v get %v", tc.reply, tc.permanent, got)
		}
		// Whatever the classification, the originals are kept for user checking..
		if tc.reply != "" && !strings.Contains(err.Error(), tc.reply) {
			t.Errorf("Response %q The original was abandoned.: %v", tc.reply, err)
		}
	}
}

func TestRegistryCoversAllKinds(t *testing.T) {
	// Six channels.——One less. UI Larry disappeared..
	want := []string{KindDingTalk, KindEmail, KindFeishu, KindTelegram, KindWebhook, KindWeCom}
	got := Kinds()
	if len(got) != len(want) {
		t.Fatalf("The number of channels should read %d,get %d: %v", len(want), len(got), got)
	}
	for _, k := range want {
		if !ValidKind(k) {
			t.Errorf("Channel %s Unregistered", k)
		}
		if ch, ok := Get(k); !ok || ch.Kind() != k {
			t.Errorf("Channel %s of Kind() Inconsistent with registration key", k)
		}
	}
	if ValidKind("nope") {
		t.Error("Unregistered types should not be verified")
	}
}

func TestPermanentErrorUnwrap(t *testing.T) {
	base := &permanentSentinel{}
	err := Permanent(base)
	if !IsPermanent(err) {
		t.Fatal("Should be identified as a permanent failure")
	}
	if !strings.Contains(err.Error(), "sentinel") {
		t.Fatalf("Error message should pass through the bottom: %v", err)
	}
	if Permanent(nil) != nil {
		t.Fatal("Permanent(nil) We have to go back. nil")
	}
	if IsPermanent(nil) {
		t.Fatal("nil Not permanent failure.")
	}
}

type permanentSentinel struct{}

func (*permanentSentinel) Error() string { return "sentinel" }
