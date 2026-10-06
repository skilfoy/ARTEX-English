package notify

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// emailDialTimeout / emailSessionTimeout Separately bind the build-up and the whole SMTP session.
// net/smtp There's no time-out mechanism on its own. If we don't set up these two lines, a stuck-in-the-end will let
// Organisation goroutine Hang it there forever.——And dispatcher It's single. goroutine Serialized,
// It's equivalent to the entire notification system..
const (
	emailDialTimeout    = 10 * time.Second
	emailSessionTimeout = 45 * time.Second
)

// emailChannel Achieved SMTP Mail delivery.
type emailChannel struct{}

func (emailChannel) Kind() string { return KindEmail }

// Mail does not have platform limit, but should not use it to screen; give a relaxed default.
func (emailChannel) DefaultRatePerMin() int { return 60 }

// Cover password only.SMTP Hosts, accounts, recipients are not secrets. Boring..
func (emailChannel) SecretKeys() []string { return []string{"password"} }

// host/port I decided which server to give the password to.;tls Whether or not to encrypt the transfer. All three of them.
// Requesting a new password——Please.[Turn it off. TLS]It's a step that must be clearly documented, not changed..
func (emailChannel) DestinationKeys() []string { return []string{"host", "port", "tls"} }

func (emailChannel) Validate(cfg map[string]any) error {
	if cfgString(cfg, "host") == "" {
		return errors.New("Missing SMTP Chile")
	}
	port := cfgInt(cfg, "port")
	if port <= 0 || port > 65535 {
		return errors.New("SMTP Port is invalid (should read 1-65535)")
	}
	if cfgString(cfg, "from") == "" {
		return errors.New("Cannot initialise Evolution's mail component.")
	}
	if len(cfgStrings(cfg, "to")) == 0 {
		return errors.New("At least one recipient address is required.")
	}
	return nil
}

func (c emailChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	host := cfgString(cfg, "host")
	port := cfgInt(cfg, "port")
	from := cfgString(cfg, "from")
	to := cfgStrings(cfg, "to")
	username := cfgString(cfg, "username")
	password := cfgString(cfg, "password")
	implicitTLS := cfgBool(cfg, "tls")

	msg, err := buildEmailMessage(from, to, m)
	if err != nil {
		return 0, Permanent(err)
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := emailDial(ctx, addr, host, implicitTLS)
	if err != nil {
		return 0, err
	}
	defer client.Close()

	// STARTTLS:Peer support is upgraded. There is no proof of this under the express statement (see below). auth Instructions).
	if !implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				return 0, fmt.Errorf("STARTTLS Failed: %w", err)
			}
		}
	}
	if username != "" {
		if err := client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
			// smtp.PlainAuth You will refuse to send a certificate on an unencrypted connection (unless the target is localhost).
			// This is...**Correct.**Security behavior, not bypass, but need to be translated.——
			// Otherwise the user will only see[unencrypted connection]I don't know what to do..
			if strings.Contains(err.Error(), "unencrypted connection") {
				return 0, Permanent(fmt.Errorf("Refusal to issue documentation: the connection is not encrypted. Please enable TLS,Or change it. 465 Port(Implicit TLS),Or put[Enable TLS]Pick it up. (%w)", err))
			}
			return 0, Permanent(fmt.Errorf("SMTP Authentication failed: %w", err))
		}
	}
	if err := client.Mail(from); err != nil {
		return 0, smtpStageError(fmt.Sprintf("Sender %s Rejected", from), err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return 0, smtpStageError(fmt.Sprintf("Recipient %s Rejected", rcpt), err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return 0, fmt.Errorf("SMTP DATA Failed: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return 0, fmt.Errorf("could not write email body: %w", err)
	}
	if err := w.Close(); err != nil {
		return 0, fmt.Errorf("Failed to submit mail: %w", err)
	}
	// Quit Failure does not affect[Mail received by server]It's a fact that ignores its mistakes..
	_ = client.Quit()
	// The mail is open.(HTML It's all in the text..
	return len(m.Items), nil
}

// emailDial Create SMTP Connection.
//
// implicitTLS=true Go 465 This one.[It's connected. TLS]How;false Go 25/587 Once it's clearly established.
// STARTTLS.The two cannot be mixed: yes. 465 Port invention greeting They'll be cut off..
//
// Session term is in**Company**Set it up (rather than after), because net/smtp of Client Take the bottom.
// Connections are hidden in unexported fields and cannot be found outside; once the connection is handed over, it can only be preset deadline
// Go to the bottom. It's also covering the handshake..
// Control Hang up. blockInternalDial With HTTP It's a common source of security. If you don't hang up, SMTP
// It's the whole package. SSRF Deficiencies in protection:host Fill 169.254.169.254 or 127.0.0.1 It connects directly.,
// And smtp.NewClient When a handshake fails, the line to which the opposite side returns is wrong, wrong, wrong. last_error
// It's a semi-blind reading of the original language.;[Connection denied vs Timeout]The time-consuming difference still works.
// To detect ports. Dial-up phase is the final entry point and is also covered DNS Relock.
func emailDial(ctx context.Context, addr, host string, implicitTLS bool) (*smtp.Client, error) {
	d := &net.Dialer{Timeout: emailDialTimeout, Control: blockInternalDial}
	var conn net.Conn
	var err error
	if implicitTLS {
		conn, err = tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("Connection SMTP Server failed: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(emailSessionTimeout))
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("SMTP The handshake failed.: %w", err)
	}
	return client, nil
}

// smtpStageError Press SMTP The response code divides some of the failures.[Retry]With[Permanent Failure].
//
// Why must we distinguish?:SMTP of 4xx With 5xx The semantics are completely different.——
//   - 4xx(450 Grey List,451 Local Error,452 Other Organiser**Temporary**Reject,
//     The formal practice is to try again later; in particular, the grey list is encountered almost every time it is first delivered.
//   - 5xx(550 User does not exist,553 It's a permanent refusal. It's pointless to try again..
//
// If the sentence fails permanently, an ash list-enabled mail server will let**Every one.**It's been the first time.
// Try and fall failed——And this kind of failure is exactly what's needed to do it again..
// The first three numbers of the wrong text are taken from the response code; retry if not available (better try again),
// And don't try to kill a possible instant malfunction because you can't figure it out.).
func smtpStageError(what string, err error) error {
	code := smtpReplyCode(err.Error())
	if code >= 500 && code < 600 {
		return Permanent(fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// smtpReplyCode from SMTP Error text takes the leading three responses, no returns 0.
// net/smtp Do not export error code fields, only from text; format is[450 4.7.1 ...].
func smtpReplyCode(text string) int {
	if len(text) < 3 {
		return 0
	}
	n, err := strconv.Atoi(text[:3])
	if err != nil {
		return 0
	}
	return n
}

// buildEmailMessage Organise complete RFC 5322 Mail.
//
// Text base64 There are two reasons for coding: SMTP It's a single line. 1000 bytes, and HTML
// The text (especially the consolidated mail) can easily appear in long lines; base64 It's not natural. "." Start
// Save it. SMTP It's a question of changing points..
func buildEmailMessage(from string, to []string, m Message) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	// Chinese subject has to be done. RFC 2047 Encoding, otherwise the client will be shown as a spam.
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", htmlTitle(m)))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	// The mail does not have a hard limit on length, so the text is not cut off.
	b.WriteString("\r\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(htmlBody(m, 0)))
	// base64 Press 76 character wrap line, matching RFC 2045.
	for len(encoded) > 76 {
		b.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	b.WriteString(encoded + "\r\n")
	return b.String(), nil
}
