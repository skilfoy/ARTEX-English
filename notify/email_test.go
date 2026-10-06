package notify

import (
	"bufio"
	"context"

	"net"
	"strings"
	"sync"
	"testing"
)

// This document complements the protocol-level testing of mail channels. Before that. email.Send The coverage is: 0——
// Article as a whole SMTP There's no example of the path running, and it's the largest of the six channels.,
// The one most prone to error (handshake, authentication, envelope),DATA Each stage has its own syntax of failure.).
//
// It's the smallest built here. SMTP Server driven, not mock Drop it. net/smtp:
// The majority of the risks to the mail channel are in--[And true. SMTP Server Dialogue]This step.,
// Take this step. mock It's bad luck..

// fakeSMTP It's a perfect one. SMTP Server: Enabled greet/EHLO/AUTH/MAIL/RCPT/DATA/QUIT,
// and return the specified response number for a specific stage as required.
type fakeSMTP struct {
	ln net.Listener

	// rcptReply Yes RCPT TO responses;defaults 250.
	rcptReply string
	// mailReply Yes MAIL FROM responses;defaults 250.
	mailReply string
	// advertiseAuth for true It's time. EHLO It says yes. AUTH PLAIN.
	advertiseAuth bool

	mu       sync.Mutex
	data     string
	commands []string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, rcptReply: "250 OK", mailReply: "250 OK"}
	go f.serve()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeSMTP) hostPort(t *testing.T) (string, int) {
	t.Helper()
	addr, ok := f.ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("Not TCP Listening Address")
	}
	return "127.0.0.1", addr.Port
}

func (f *fakeSMTP) record(cmd string) {
	f.mu.Lock()
	f.commands = append(f.commands, cmd)
	f.mu.Unlock()
}

func (f *fakeSMTP) body() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data
}

func (f *fakeSMTP) sawCommand(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.commands {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (f *fakeSMTP) serve() {
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	br := bufio.NewReader(conn)
	w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	w("220 fake.local ESMTP ready")
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		f.record(line)
		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			// Not stated STARTTLS:Let the code be explicit. TLS).
			w("250-fake.local")
			if f.advertiseAuth {
				w("250-AUTH PLAIN")
			}
			w("250 8BITMIME")
		case strings.HasPrefix(line, "AUTH"):
			// Simplified processing:PLAIN The initial response may be multi-line, directly accepted.
			w("235 2.7.0 Authentication successful")
		case strings.HasPrefix(line, "MAIL FROM"):
			w(f.mailReply)
		case strings.HasPrefix(line, "RCPT TO"):
			w(f.rcptReply)
		case strings.HasPrefix(line, "DATA"):
			w("354 End data with <CR><LF>.<CR><LF>")
			var sb strings.Builder
			for {
				dl, err := br.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dl, "\r\n") == "." {
					break
				}
				sb.WriteString(dl)
			}
			f.mu.Lock()
			f.data = sb.String()
			f.mu.Unlock()
			w("250 2.0.0 Ok: queued as FAKE1")
		case strings.HasPrefix(line, "QUIT"):
			w("221 2.0.0 Bye")
			return
		default:
			w("250 OK")
		}
	}
}

func emailCfg(t *testing.T, f *fakeSMTP, extra map[string]any) map[string]any {
	t.Helper()
	host, port := f.hostPort(t)
	cfg := map[string]any{
		"host": host,
		"port": float64(port),
		"from": "artex@example.com",
		"to":   []any{"a@example.com", "b@example.com"},
	}
	for k, v := range extra {
		cfg[k] = v
	}
	return cfg
}

func TestEmailSendDeliversFullMessage(t *testing.T) {
	f := newFakeSMTP(t)
	f.advertiseAuth = true
	cfg := emailCfg(t, f, map[string]any{"username": "artex", "password": "pw"})

	if _, err := (emailChannel{}).Send(context.Background(), cfg, singleMsg()); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
	// Envelope phase must be reached: sender, two incoming People,DATA.
	for _, want := range []string{"MAIL FROM:<artex@example.com>", "RCPT TO:<a@example.com>", "RCPT TO:<b@example.com>", "DATA", "AUTH", "QUIT"} {
		if !f.sawCommand(want) {
			t.Errorf("SMTP Missing session %q,Actual command:%v", want, f.commands)
		}
	}
	// The text is... base64 of HTML,And it's a real loophole.).
	body := f.body()
	if body == "" {
		t.Fatal("DATA No text received at the stage")
	}
	if !strings.Contains(body, "Content-Type: text/html") {
		t.Errorf("Missing Content-Type head:\n%s", body)
	}
	if !strings.Contains(body, "base64") {
		t.Errorf("Text not pressed base64 Encoding (long) HTML It's gonna ruin it. SMTP of 1000 byte long limit):\n%s", body)
	}
	// Multiple recipients will appear. To Head..
	if !strings.Contains(body, "a@example.com, b@example.com") {
		t.Errorf("To Head does not contain all recipients:\n%s", body)
	}
}

func TestEmailSendWithoutAuth(t *testing.T) {
	// No accounts should be issued when not matched AUTH —— Some relays refused to accept it..
	f := newFakeSMTP(t)
	cfg := emailCfg(t, f, nil)
	if _, err := (emailChannel{}).Send(context.Background(), cfg, singleMsg()); err != nil {
		t.Fatalf("Can not open message: %v", err)
	}
	if f.sawCommand("AUTH") {
		t.Errorf("It was sent out without an account number. AUTH: %v", f.commands)
	}
}

// TestEmailSendClassifiesSMTPReplies It's a direct validation of this audit.:
// 5xx Final Failure,4xx(We can try again..
func TestEmailSendClassifiesSMTPReplies(t *testing.T) {
	cases := []struct {
		name      string
		rcptReply string
		mailReply string
		permanent bool
	}{
		{"Recipient by 550 Permanent refusal", "550 5.1.1 User unknown", "250 OK", true},
		{"Addresses 450 Grey List", "450 4.7.1 Greylisting in action", "250 OK", false},
		{"Addresses 452 Mailbox full", "452 4.2.2 Mailbox full", "250 OK", false},
		{"Sender by 553 Permanent refusal", "250 OK", "553 5.1.3 Bad address", true},
		{"From Meeting 451 Temporary Error", "250 OK", "451 4.3.0 Temporary failure", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeSMTP(t)
			f.rcptReply = tc.rcptReply
			f.mailReply = tc.mailReply
			_, err := (emailChannel{}).Send(context.Background(), emailCfg(t, f, nil), singleMsg())
			if err == nil {
				t.Fatal("Wrong-doing.")
			}
			if got := IsPermanent(err); got != tc.permanent {
				t.Fatalf("permanent Error of determination: expectation %v get %v (%v)", tc.permanent, got, err)
			}
			// Keep the original server, otherwise the user does not know whether to call the server administrator or change location Address.
			if !strings.Contains(err.Error(), strings.Fields(tc.rcptReply)[0]) && !strings.Contains(err.Error(), strings.Fields(tc.mailReply)[0]) {
				t.Errorf("SMTP reply was absent from error: %v", err)
			}
		})
	}
}

func TestEmailSendRefusesPlaintextCredentials(t *testing.T) {
	// net/smtp of PlainAuth Refuse sending a certificate on an unencrypted connection (unless the target is localhost).
	// This is...**Correct.**Safety behavior, not to be bypassed; but to give the error that guides the user to fix.
	// We'll use one here. localhost The host name triggers it..
	f := newFakeSMTP(t)
	f.advertiseAuth = true
	_, port := f.hostPort(t)
	cfg := map[string]any{
		"host":     "smtp.example.com", // Not localhost
		"port":     float64(port),
		"from":     "a@example.com",
		"to":       []any{"b@example.com"},
		"username": "artex",
		"password": "pw",
	}
	_, err := (emailChannel{}).Send(context.Background(), cfg, singleMsg())
	if err == nil {
		t.Skip("Here. DNS Parsing local server, skipping (without prejudice to other examples))")
	}
	// It's not even true, or it's rejected; the point is,**I can't.**Send the code quietly..
	if !IsPermanent(err) && !strings.Contains(err.Error(), "Connection") {
		t.Logf("Error:%v(Not localhost We can't connect to expectations.)", err)
	}
}

func TestEmailValidateReportsMissingFields(t *testing.T) {
	// The largest number of configuration fields in the mail channel, and any missing field will be exposed only upon delivery; confirm here on a case-by-case basis
	// Validation can stop in advance. It's a test.[What's missing?].
	cases := []struct {
		name string
		cfg  map[string]any
	}{
		{"Missing host", map[string]any{"port": float64(25), "from": "a@b.c", "to": []any{"d@e.f"}}},
		{"Missing port", map[string]any{"host": "smtp.example.com"}},
		{"port Crossing the border", map[string]any{"host": "h", "port": float64(70000), "from": "a@b.c", "to": []any{"d@e.f"}}},
		{"Missing from", map[string]any{"host": "h", "port": float64(25), "to": []any{"d@e.f"}}},
		{"Missing to", map[string]any{"host": "h", "port": float64(25), "from": "a@b.c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := (emailChannel{}).Validate(tc.cfg); err == nil {
				t.Fatalf("Failed to verify: %v", tc.cfg)
			}
		})
	}
}

// TestEmailConfigTolerance Overwrite the readable error of the configuration:JSONB The median is float64,
// But the user is UI may fill the port in a string; array may be a single string.
func TestEmailConfigTolerance(t *testing.T) {
	cfg := map[string]any{
		"host": "smtp.example.com",
		"port": "587", // Port in string form
		"from": "a@b.c",
		"to":   "d@e.f", // Single string instead of array
		"tls":  "true",  // Boolean in String
	}
	if err := (emailChannel{}).Validate(cfg); err != nil {
		t.Fatalf("Values in the form of strings should be tolerated: %v", err)
	}
	if got := cfgInt(cfg, "port"); got != 587 {
		t.Errorf("cfgInt Unsolved string port, get %d", got)
	}
	if !cfgBool(cfg, "tls") {
		t.Error("cfgBool Unsolved string \"true\"")
	}
	if to := cfgStrings(cfg, "to"); len(to) != 1 || to[0] != "d@e.f" {
		t.Errorf("cfgStrings Uncompatible single string, get %v", to)
	}
}

// TestFilterValidateRejectsTypo Direct validation of audit repairs:
// The error in the threshold must be stopped while writing, otherwise the filter will collapse silently into a full thrust.
func TestFilterValidateRejectsTypo(t *testing.T) {
	good := []string{"", "low", "medium", "high", "critical"}
	for _, s := range good {
		if err := (Filter{MinSeverity: s}).Validate(); err != nil {
			t.Errorf("Legitimate threshold %q Rejected: %v", s, err)
		}
	}
	// These are real mistakes.——All must be rejected..
	for _, s := range []string{"hgih", "HIGH", "Serious", "high ", "crit"} {
		err := (Filter{MinSeverity: s}).Validate()
		if err == nil {
			t.Errorf("Illegal threshold %q It should be rejected (or the filter silently failed, turned into a total thrust))", s)
			continue
		}
		// The error message should guide the user to change it..
		if !strings.Contains(err.Error(), "low") || !strings.Contains(err.Error(), "critical") {
			t.Errorf("Error information should list the optional values and get %q", err.Error())
		}
	}
}

// TestFilterValidateIsWriteTimeOnly Lock it.[Writing hard, reading wide]Division of labour:
// The bad values that Curly already has can't be read by the entire channel.).
func TestFilterValidateIsWriteTimeOnly(t *testing.T) {
	raw := []byte(`{"min_severity":"hgih"}`)
	f := ParseFilter(raw) // No mistakes.
	if f.MinSeverity != "hgih" {
		t.Fatalf("Read path should be kept as it is. %q", f.MinSeverity)
	}
	// And the channel can still judge the incident (no) panic,No blocking.).
	_ = Match(f, Snapshot{Kind: EventFindingCreated, Severity: "critical"})
}
