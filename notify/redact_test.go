package notify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// This document is a non-variant test:**Any**You can't carry the wrong text from the channel. According to.
//
// Why pull one file alone: the original channel only covers successful paths and Platform business errors,
// There was no transmission failure at all. And it's just a transmission layer error./DNS Failed/The most dangerous.——
// http.Client.Do Returned *url.Error will**Complete URL** Type in the wrong text and this function
// These are the proofs. URL Lee. The evidence leads to four exits.:
//
//	notification_deliveries.last_error  → Clear File
//	GET /api/notify/deliveries Response     → Go around the mask of the channel configuration and replay it to the browser
//	Service End Log                          → They're often retired.
//	Test sent interface. 502 Response             → Right at the front end.
//
// So it's not just a function, it's a request to fail one way or another, claiming the wrong text.
// I can't find that..

// credentialCases Overwrite All[On the evidence. URL inside]the mode of channel:
// DingTalk/Small query,Flying Book at the end of the path.,Telegram In Path Segment.
var credentialCases = []struct {
	name   string
	ch     Channel
	cfg    map[string]any
	secret string
}{
	{
		name:   "DingTalk access_token at query",
		ch:     dingTalkChannel{},
		cfg:    map[string]any{"webhook": "http://127.0.0.1:1/robot/send?access_token=" + leakProbeToken},
		secret: leakProbeToken,
	},
	{
		name:   "Enterprise WeChat key at query",
		ch:     weComChannel{},
		cfg:    map[string]any{"webhook": "http://127.0.0.1:1/cgi-bin/webhook/send?key=" + leakProbeToken},
		secret: leakProbeToken,
	},
	{
		name:   "Feishu hook id At the end of the path",
		ch:     feishuChannel{},
		cfg:    map[string]any{"webhook": "http://127.0.0.1:1/open-apis/bot/v2/hook/" + leakProbeToken},
		secret: leakProbeToken,
	},
	{
		name:   "Telegram bot token In Path Segment",
		ch:     telegramChannel{},
		cfg:    map[string]any{"bot_token": leakProbeToken, "chat_id": "1", "base_url": "http://127.0.0.1:1"},
		secret: leakProbeToken,
	},
	{
		name:   "Staple with Sign Key",
		ch:     dingTalkChannel{},
		cfg:    map[string]any{"webhook": "http://127.0.0.1:1/robot/send", "secret": leakProbeToken},
		secret: leakProbeToken,
	},
}

// leakProbeToken It's a sentry that can never be a true document to search for it in the wrong text..
const leakProbeToken = "LEAKPROBE0123456789abcdef"

// TestChannelErrorsNeverLeakCredentials It's the core..
func TestChannelErrorsNeverLeakCredentials(t *testing.T) {
	for _, tc := range credentialCases {
		t.Run(tc.name, func(t *testing.T) {
			// A failed peer:127.0.0.1:1 No one's listening..
			_, err := tc.ch.Send(context.Background(), tc.cfg, Message{
				Items: []Item{{FindingID: 1, Severity: "high", Name: "The leak of the probe."}},
			})
			if err == nil {
				t.Fatal("Wrong with unreachable address.")
			}
			assertNoSecret(t, err.Error(), tc.secret)
		})
	}
}

// TestChannelErrorsNeverLeakCredentialsInPermanentPath Overwrite permanently failed branch:
// URL Failures to verify, errors in platform operations, etc. will also cause the error to be uploaded, and no proof of it..
func TestChannelErrorsNeverLeakCredentialsInPermanentPath(t *testing.T) {
	cases := []struct {
		name string
		ch   Channel
		cfg  map[string]any
	}{
		// It's not valid. → Trigger validateHTTPURL / url.Parse Branch.
		{"The nail address is illegal.", dingTalkChannel{}, map[string]any{"webhook": "file:///" + leakProbeToken}},
		{"Micro address is illegal", weComChannel{}, map[string]any{"webhook": "gopher://" + leakProbeToken}},
		{"Flying Book address is illegal", feishuChannel{}, map[string]any{"webhook": "ftp://" + leakProbeToken + "/hook"}},
		{"Telegram API Chile", telegramChannel{}, map[string]any{"bot_token": "tok", "chat_id": "1", "base_url": "file://" + leakProbeToken}},
		{"General Webhook Chile", webhookChannel{}, map[string]any{"url": "javascript:" + leakProbeToken}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.ch.Send(context.Background(), tc.cfg, Message{Items: []Item{{Severity: "high"}}})
			if err == nil {
				t.Fatal("Irregular Configuration Error")
			}
			assertNoSecret(t, err.Error(), leakProbeToken)
		})
	}
}

func assertNoSecret(t *testing.T, text, secret string) {
	t.Helper()
	if strings.Contains(text, secret) {
		t.Fatalf("Wrong text leaked evidence. %q:\n    %s", secret, text)
	}
}

func TestRedactRequestTargetKeepsOnlySchemeAndHost(t *testing.T) {
	cases := map[string]string{
		"https://oapi.dingtalk.com/robot/send?access_token=S1":    "https://oapi.dingtalk.com/…",
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=S2": "https://qyapi.weixin.qq.com/…",
		"https://open.feishu.cn/open-apis/bot/v2/hook/S3":         "https://open.feishu.cn/…",
		"https://api.telegram.org/botS4/sendMessage":              "https://api.telegram.org/…",
		"http://10.0.0.5:8080/hook":                               "http://10.0.0.5:8080/…",
	}
	for in, want := range cases {
		got := redactRequestTarget(in)
		if got != want {
			t.Errorf("redactRequestTarget(%q) = %q,Expectations %q", in, got, want)
		}
		// The dissensitisation itself must no longer contain any path to the original address./Search Snippet.
		if parts := strings.SplitN(in, "://", 2); len(parts) == 2 {
			if hostAndRest := strings.SplitN(parts[1], "/", 2); len(hostAndRest) == 2 && hostAndRest[1] != "" {
				if strings.Contains(got, hostAndRest[1]) {
					t.Errorf("It still has path after dissensitisation./Search Snippet %q: %q", hostAndRest[1], got)
				}
			}
		}
	}
	// Unresolved input never returns the original string.
	for _, bad := range []string{"", "://", "not a url", "http://"} {
		if got := redactRequestTarget(bad); strings.Contains(got, bad) && bad != "" {
			t.Errorf("Can not parse input %q It's recognizable. %q", bad, got)
		}
	}
}

// TestRedactTransportErrorStripsURL Keep an eye on it. *url.Error This particular type.:
// It is. http.Client.Do The type of return that was the first scene of the leak..
func TestRedactTransportErrorStripsURL(t *testing.T) {
	inner := errors.New("dial tcp 127.0.0.1:1: connect: connection refused")
	uerr := &url.Error{
		Op:  "Post",
		URL: "https://api.telegram.org/bot" + leakProbeToken + "/sendMessage",
		Err: inner,
	}
	got := redactTransportError(uerr)
	assertNoSecret(t, got, leakProbeToken)
	if !strings.Contains(got, "api.telegram.org") {
		t.Errorf("Should be retained host To check, get %q", got)
	}
	if !strings.Contains(got, "connection refused") {
		t.Errorf("The bottom causes should be retained for screening. %q", got)
	}
	// Op And keep it.(POST Still? GET It makes sense to check.).
	if !strings.Contains(got, "Post") {
		t.Errorf("Should retain the operational name and get %q", got)
	}
}

// TestRedactURLsInTextHandlesFallback Bottom path: Not *url.Error Custom error
// (The address in the re-direction policy is also removed..
func TestRedactURLsInTextHandlesFallback(t *testing.T) {
	in := fmt.Sprintf("Deny cross-host reorientation(a.example → http://b.example/bot%s/send)", leakProbeToken)
	got := redactURLsInText(in)
	assertNoSecret(t, got, leakProbeToken)
	if !strings.Contains(got, "http://b.example/…") {
		t.Errorf("Should replace the address with a dissensitized form. %q", got)
	}
	// Keep text without address as it is.
	if plain := "dial tcp: connection refused"; redactURLsInText(plain) != plain {
		t.Error("Text without address should not be altered")
	}
}

// TestCrossHostRedirectRefused override[On the evidence. URL inside + Following Trans-Key = Give me the papers.].
// httptest Two services listening. 127.0.0.1 Different ports, different ports. Host Different.,
// It's exactly what constitutes a cross-host jump..
func TestCrossHostRedirectRefused(t *testing.T) {
	var hit bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		_, _ = io.WriteString(w, `{"errcode":0,"errmsg":"ok"}`)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/robot/send?access_token="+leakProbeToken, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	_, err := (dingTalkChannel{}).Send(context.Background(),
		map[string]any{"webhook": redirector.URL + "/robot/send?access_token=" + leakProbeToken},
		Message{Items: []Item{{Severity: "high"}}})
	if err == nil {
		t.Fatal("Cross-host reorientation should be rejected")
	}
	if hit {
		t.Fatal("The jumper was interviewed.——The evidence is leaking with re-direction.")
	}
	assertNoSecret(t, err.Error(), leakProbeToken)
}

// TestSameHostRedirectAllowed Example: Jump with host (e.g. end slash) must remain available,
// Otherwise, we'll block the normal flow..
func TestSameHostRedirectAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robot/send" {
			// Jumping with Host, Same Port.
			http.Redirect(w, r, "/robot/send/", http.StatusTemporaryRedirect)
			return
		}
		_, _ = io.WriteString(w, `{"errcode":0,"errmsg":"ok"}`)
	}))
	defer srv.Close()

	if _, err := (dingTalkChannel{}).Send(context.Background(),
		map[string]any{"webhook": srv.URL + "/robot/send"},
		Message{Items: []Item{{Severity: "high"}}}); err != nil {
		t.Fatalf("Redirection with the host should not be rejected: %v", err)
	}
}
