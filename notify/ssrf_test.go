package notify

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// This file covers two related hardenings:
//   - a delivery address must not use the server as a pivot into the internal
//     network or cloud metadata (SSRF)
//   - an address-validation error must not carry credentials that were in
//     the address
//
// Test environment: many cases in this package use an httptest receiver on
// 127.0.0.1, and the guard would reject them by default. TestMain turns
// AllowLocalTargetsEnv on for the package, and each SSRF case below clears
// it so the assertion is the default-deny behavior.

func TestMain(m *testing.M) {
	// Let ordinary cases reach a local fake receiver. SSRF cases clear the
	// variable themselves for the duration of the test.
	_ = os.Setenv(AllowLocalTargetsEnv, "1")
	os.Exit(m.Run())
}

// TestDialGuardRejectsLoopbackByDefault is the core SSRF assertion: with the
// default config, delivery to a loopback address must be refused at connect
// time.
func TestDialGuardRejectsLoopbackByDefault(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		_, _ = io.WriteString(w, `{"errcode":0}`)
	}))
	defer srv.Close()

	t.Setenv(AllowLocalTargetsEnv, "") // close the hatch = default behavior
	_, err := (dingTalkChannel{}).Send(context.Background(),
		map[string]any{"webhook": srv.URL + "/robot/send"}, Message{Items: []Item{{Severity: "high"}}})
	if err == nil {
		t.Fatal("delivery to a loopback address should not be allowed by default")
	}
	if hit {
		t.Fatal("the request reached a local service; the guard did not take effect")
	}
	// The error has to tell the user how to opt in. A local SMTP relay is a
	// legitimate configuration.
	if !strings.Contains(err.Error(), AllowLocalTargetsEnv) {
		t.Errorf("the refusal should say how to opt in explicitly: %v", err)
	}
}

// TestDialGuardAllowsLoopbackWhenOptedIn is the inverse: once the hatch is
// opened explicitly, delivery must work. Otherwise a local postfix or an
// internal relay is broken outright.
func TestDialGuardAllowsLoopbackWhenOptedIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errcode":0,"errmsg":"ok"}`)
	}))
	defer srv.Close()

	t.Setenv(AllowLocalTargetsEnv, "1")
	if _, err := (dingTalkChannel{}).Send(context.Background(),
		map[string]any{"webhook": srv.URL + "/robot/send"}, Message{Items: []Item{{Severity: "high"}}}); err != nil {
		t.Fatalf("delivery should succeed once explicitly allowed: %v", err)
	}
}

func TestIsBlockedDialIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.1.2.3", "::1",
		"169.254.169.254", // cloud metadata endpoint; the main reason this function exists
		"169.254.1.1", "fe80::1",
		"0.0.0.0", "::",
		"224.0.0.1", "ff02::1",
		"::ffff:127.0.0.1", // IPv4-mapped form must be unwrapped first, or it is a bypass
		"",
	}
	for _, s := range blocked {
		if !isBlockedDialIP(net.ParseIP(s)) {
			t.Errorf("%s should be refused", s)
		}
	}
	// RFC1918 private space is deliberately allowed. An internal Mattermost
	// or SMTP relay is a common legitimate target. This assertion pins that
	// choice: if someone later adds a private-network check, the test fails
	// and forces a conscious decision instead of silently breaking deploys.
	allowed := []string{"10.0.0.5", "172.16.3.4", "192.168.1.10", "8.8.8.8", "2606:4700::1111"}
	for _, s := range allowed {
		if isBlockedDialIP(net.ParseIP(s)) {
			t.Errorf("%s should be allowed (private networks are a common legitimate delivery target)", s)
		}
	}
}

// TestValidateHTTPURLRejectsLiteralPrivateTargets covers the hint at config
// time: a literal IP should be rejected when the config is saved, not on the
// first failed delivery.
func TestValidateHTTPURLRejectsLiteralPrivateTargets(t *testing.T) {
	t.Setenv(AllowLocalTargetsEnv, "")
	for _, raw := range []string{
		"http://127.0.0.1:8080/hook",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:8080/hook",
	} {
		if err := validateHTTPURL(raw); err == nil {
			t.Errorf("%s should be rejected at config time", raw)
		}
	}
	// Public addresses pass, and private addresses pass here too (private
	// space is left to dial time, which does not block it).
	for _, raw := range []string{"https://oapi.dingtalk.com/robot/send", "http://10.0.0.9/hook"} {
		if err := validateHTTPURL(raw); err != nil {
			t.Errorf("%s should pass validation: %v", raw, err)
		}
	}
}

// TestValidateHTTPURLErrorNeverLeaksCredentials is the branch the audit said
// was missed last round.
//
// When url.Parse fails it returns *url.Error, and Error() contains the full
// original address. The previous round only redacted the error returned by
// http.Client.Do and missed this path. The "permanent failure" cases added
// then (file://, gopher://, ftp://) all parse successfully and take the
// scheme branch, so a green run did not prove this path was safe. It was a
// false assurance.
func TestValidateHTTPURLErrorNeverLeaksCredentials(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/%zz?access_token=" + leakProbeToken,         // illegal percent-escape
		"https://a.example.com:port/x?access_token=" + leakProbeToken, // port is not a number
		"http://[::1?access_token=" + leakProbeToken,                  // unmatched bracket
	}
	for _, raw := range cases {
		// First confirm this input really makes url.Parse fail. Without that
		// check the case can wander onto another branch without anyone
		// noticing, which is how the previous false assurance happened.
		if _, err := url.Parse(raw); err == nil {
			t.Errorf("%q should fail to parse, or this case does not cover the target branch", raw)
			continue
		}
		err := validateHTTPURL(raw)
		if err == nil {
			t.Errorf("%q should fail validation", raw)
			continue
		}
		assertNoSecret(t, err.Error(), leakProbeToken)
	}
	// The channel-level wrapper must not carry the address out either.
	t.Setenv(AllowLocalTargetsEnv, "")
	err := (dingTalkChannel{}).Validate(map[string]any{"webhook": cases[0]})
	if err == nil {
		t.Fatal("an invalid address should fail validation")
	}
	assertNoSecret(t, err.Error(), leakProbeToken)
}

// TestEmailDialGuardRejectsLoopbackByDefault covers the SMTP dial guard.
//
// The email channel used to dial with a bare net.Dialer, which was the one
// gap in the SSRF protection: a host of 169.254.169.254 or 127.0.0.1
// connected directly. When smtp.NewClient failed the handshake it wrapped
// the peer's reply line into the error, and last_error echoed that through
// the delivery-history API — the same semi-blind read the other channels
// had already closed. The timing difference between "connection refused" and
// "timeout" could also probe ports.
//
// TestMain turns AllowLocalTargetsEnv on for the package (many cases use a
// fake receiver on 127.0.0.1), so this case has to clear it itself.
// Otherwise the test passes whether or not the guard is installed, which is
// why the gap was not caught before.
func TestEmailDialGuardRejectsLoopbackByDefault(t *testing.T) {
	f := newFakeSMTP(t)
	cfg := emailCfg(t, f, nil)

	t.Setenv(AllowLocalTargetsEnv, "") // close the hatch = default behavior
	_, err := (emailChannel{}).Send(context.Background(), cfg, singleMsg())
	if err == nil {
		t.Fatal("mail should not be delivered to a loopback address by default")
	}
	// The connection should never be established. The guard stops it in the
	// Control hook, so EHLO is never sent.
	if f.sawCommand("EHLO") || f.sawCommand("HELO") {
		t.Fatal("an SMTP session was established; the guard did not take effect")
	}
	// The error has to tell the user how to opt in. A local postfix relay is
	// a legitimate configuration.
	if !strings.Contains(err.Error(), AllowLocalTargetsEnv) {
		t.Errorf("the refusal should say how to opt in explicitly: %v", err)
	}
}

// TestEmailDialGuardAllowsLoopbackWhenOptedIn is the paired inverse: once
// the hatch is opened, delivery must succeed. A self-hosted internal SMTP
// server or a local relay is a very common deployment, and the guard must
// not ban it outright.
func TestEmailDialGuardAllowsLoopbackWhenOptedIn(t *testing.T) {
	f := newFakeSMTP(t)
	cfg := emailCfg(t, f, nil)

	t.Setenv(AllowLocalTargetsEnv, "1")
	if _, err := (emailChannel{}).Send(context.Background(), cfg, singleMsg()); err != nil {
		t.Fatalf("local SMTP should be deliverable once explicitly allowed: %v", err)
	}
	if !f.sawCommand("EHLO") {
		t.Fatal("EHLO was not seen; the session was never established")
	}
}
