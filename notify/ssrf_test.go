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

// This document covers two related enhancements:
//   ① The delivery address must not be used as a springboard. Network / Cloud metadata(SSRF)
//   ② The error information from the address verification should not be taken out of the address.
//
// For the test environment: this package is used extensively 127.0.0.1 Top httptest Fake receiver. Guard will stop by default.
// They are. So... TestMain Unique Open AllowLocalTargetsEnv,And each below SSRF Example
// It's obvious.**Default Rejection**Conduct.

func TestMain(m *testing.M) {
	// Lets regular routines match local fake receptions. End;SSRF I'll use the regular meeting to empty myself..
	_ = os.Setenv(AllowLocalTargetsEnv, "1")
	os.Exit(m.Run())
}

// TestDialGuardRejectsLoopbackByDefault Yes SSRF The core of the defense.:
// Default configuration, drop to ring return address must be**Connect Layer**Reject.
func TestDialGuardRejectsLoopbackByDefault(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		_, _ = io.WriteString(w, `{"errcode":0}`)
	}))
	defer srv.Close()

	t.Setenv(AllowLocalTargetsEnv, "") // Turn off the escape. = Default Behaviour
	_, err := (dingTalkChannel{}).Send(context.Background(),
		map[string]any{"webhook": srv.URL + "/robot/send"}, Message{Items: []Item{{Severity: "high"}}})
	if err == nil {
		t.Fatal("Default should not allow delivery to ringback address")
	}
	if hit {
		t.Fatal("The request has reached the service.——The guards aren't working.")
	}
	// The error message should guide the user how to let it go. SMTP Relay is valid configuration).
	if !strings.Contains(err.Error(), AllowLocalTargetsEnv) {
		t.Errorf("The rejection message should show how to let it go.: %v", err)
	}
}

// TestDialGuardAllowsLoopbackWhenOptedIn Inverse use example: must be available after visible opening,
// Otherwise, it's ours. postfix / This kind of legitimate deployment will be scrapped..
func TestDialGuardAllowsLoopbackWhenOptedIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errcode":0,"errmsg":"ok"}`)
	}))
	defer srv.Close()

	t.Setenv(AllowLocalTargetsEnv, "1")
	if _, err := (dingTalkChannel{}).Send(context.Background(),
		map[string]any{"webhook": srv.URL + "/robot/send"}, Message{Items: []Item{{Severity: "high"}}}); err != nil {
		t.Fatalf("It's supposed to be ready for delivery when it's released.: %v", err)
	}
}

func TestIsBlockedDialIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.1.2.3", "::1",
		"169.254.169.254", // Cloud metadata endpoint——Main reason why this function exists
		"169.254.1.1", "fe80::1",
		"0.0.0.0", "::",
		"224.0.0.1", "ff02::1",
		"::ffff:127.0.0.1", // IPv4-mapped The form has to be restored or it's bypassed.
		"",
	}
	for _, s := range blocked {
		if !isBlockedDialIP(net.ParseIP(s)) {
			t.Errorf("%s It should be rejected.", s)
		}
	}
	// RFC1918 Private Internet**I'll let you go.**:Network built-in Mattermost / SMTP Relay is a common legal usage..
	// This claim fixes the deal.——If someone goes along with the Internet in the future, it will fail.,
	// So we can make a conscious decision.).
	allowed := []string{"10.0.0.5", "172.16.3.4", "192.168.1.10", "8.8.8.8", "2606:4700::1111"}
	for _, s := range allowed {
		if isBlockedDialIP(net.ParseIP(s)) {
			t.Errorf("%s Should be let go.)", s)
		}
	}
}

// TestValidateHTTPURLRejectsLiteralPrivateTargets Overwrite pretip for configuration:
// Literally IP It should be rejected while saving, not until the first delivery fails..
func TestValidateHTTPURLRejectsLiteralPrivateTargets(t *testing.T) {
	t.Setenv(AllowLocalTargetsEnv, "")
	for _, raw := range []string{
		"http://127.0.0.1:8080/hook",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:8080/hook",
	} {
		if err := validateHTTPURL(raw); err == nil {
			t.Errorf("%s Rejected at configuration stage", raw)
		}
	}
	// Public and private Internet addresses passed as they were.).
	for _, raw := range []string{"https://oapi.dingtalk.com/robot/send", "http://10.0.0.9/hook"} {
		if err := validateHTTPURL(raw); err != nil {
			t.Errorf("%s It should be verified.: %v", raw, err)
		}
	}
}

// TestValidateHTTPURLErrorNeverLeaksCredentials It's the branch that the audit pointed out I missed in the last round..
//
// url.Parse **Failed**Back in time. *url.Error,Other Error() Contains the full original address. Last round I just...
// I'm allergic. http.Client.Do The return error left it here; it was filled.[Path to permanent failure]Example
// (file://,gopher://,ftp://)It's possible. url.Parse It's a success. scheme Branch,
// So all green can't prove this path is safe.——It's a false promise..
func TestValidateHTTPURLErrorNeverLeaksCredentials(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/%zz?access_token=" + leakProbeToken,         // Illegal percentage conversion
		"https://a.example.com:port/x?access_token=" + leakProbeToken, // Port Non-number
		"http://[::1?access_token=" + leakProbeToken,                  // Brackets don't match
	}
	for _, raw := range cases {
		// Check this input first.**Indeed.**Let url.Parse Failure. If we don't do this, we might use the example.
		// I went to another branch without knowing.).
		if _, err := url.Parse(raw); err == nil {
			t.Errorf("%q Should have parsed failed, otherwise this example did not cover the target branch", raw)
			continue
		}
		err := validateHTTPURL(raw)
		if err == nil {
			t.Errorf("%q Failed to verify", raw)
			continue
		}
		assertNoSecret(t, err.Error(), leakProbeToken)
	}
	// Make sure the package on the channel level doesn't get the address out..
	t.Setenv(AllowLocalTargetsEnv, "")
	err := (dingTalkChannel{}).Validate(map[string]any{"webhook": cases[0]})
	if err == nil {
		t.Fatal("Invalid address verified failed")
	}
	assertNoSecret(t, err.Error(), leakProbeToken)
}

// TestEmailDialGuardRejectsLoopbackByDefault override SMTP The dial-up guards at the channel..
//
// The mail channels used to be naked. net.Dialer,It's full. SSRF The only gap in protection.:host Fill
// 169.254.169.254 or 127.0.0.1 It connects directly, and... smtp.NewClient When a handshake fails...
// The line returned from the opposite end was wrongly packaged, passed last_error From the delivery history interface——It's just another channel.
// It's a half-blind reading.;[Connection denied vs Timeout]Time-consuming differences can also be used to detect the end. mouth.
//
// This bag. TestMain It's open. AllowLocalTargetsEnv(Extensive use of standard 127.0.0.1 Top
// So this example has to be cleaned up.——Otherwise, the guards will pass in or out.,
// That's why the gap wasn't detected by any tests..
func TestEmailDialGuardRejectsLoopbackByDefault(t *testing.T) {
	f := newFakeSMTP(t)
	cfg := emailCfg(t, f, nil)

	t.Setenv(AllowLocalTargetsEnv, "") // Turn off the escape. = Default Behaviour
	_, err := (emailChannel{}).Send(context.Background(), cfg, singleMsg())
	if err == nil {
		t.Fatal("Default should not allow mail delivery to ringback address")
	}
	// The connection should never have been created: the guards are Control Stop in the hook.,EHLO It's never coming out..
	if f.sawCommand("EHLO") || f.sawCommand("HELO") {
		t.Fatal("SMTP Session created——The guards aren't working.")
	}
	// The error message should guide the user how to let it go. postfix Relay is valid configuration).
	if !strings.Contains(err.Error(), AllowLocalTargetsEnv) {
		t.Errorf("The rejection message should show how to let it go.: %v", err)
	}
}

// TestEmailDialGuardAllowsLoopbackWhenOptedIn It's a paired inverted example: after visible opening
// We must be able to deliver normally. Network built-in SMTP / This relay is a very common deployment. The guards can't do anything..
func TestEmailDialGuardAllowsLoopbackWhenOptedIn(t *testing.T) {
	f := newFakeSMTP(t)
	cfg := emailCfg(t, f, nil)

	t.Setenv(AllowLocalTargetsEnv, "1")
	if _, err := (emailChannel{}).Send(context.Background(), cfg, singleMsg()); err != nil {
		t.Fatalf("Let go of the machine. SMTP Should be capable of delivery: %v", err)
	}
	if !f.sawCommand("EHLO") {
		t.Fatal("Not seen EHLO——Session not really established")
	}
}
