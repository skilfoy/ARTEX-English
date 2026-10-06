package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// allowLocalTargets reports whether delivery to loopback and link-local
// addresses is allowed.
//
// The default is to refuse them. IM bots and public mail servers do not live
// on those ranges, and the things they can reach are sensitive: an admin
// port of another process on the same host, and the cloud metadata endpoint
// (169.254.169.254, which can yield instance credentials). The delivery
// address is set by an administrator, but a session borrowed through
// XSS/CSRF, or a second person sharing the same JWT, can change the config
// and read the response back. doJSON writes the first 200 bytes of a 4xx/5xx
// body into last_error, and the delivery-history API echoes that, which is
// a semi-blind read primitive.
//
// A local SMTP relay (postfix on 127.0.0.1:25) is a normal self-hosted mail
// setup, and a hard ban would block it. There is an explicit escape hatch
// instead of a hardcoded allow: set ARTEX_NOTIFY_ALLOW_LOCAL=1.
//
// AllowLocalTargetsEnv is exported so tests can turn the hatch on. This
// package and the server package use httptest receivers on 127.0.0.1, and
// without the hatch the guard would reject all of them.
const AllowLocalTargetsEnv = "ARTEX_NOTIFY_ALLOW_LOCAL"

func allowLocalTargets() bool {
	v := strings.TrimSpace(os.Getenv(AllowLocalTargetsEnv))
	return v == "1" || strings.EqualFold(v, "true")
}

// isBlockedDialIP reports whether ip is in a range that must not be dialed
// by default.
//
// Only loopback, link-local (including the cloud metadata address
// 169.254.169.254), unspecified, and multicast addresses are refused.
// RFC1918 private space is not refused: an internal Mattermost or SMTP relay
// is a common legitimate target, and blocking it would make the feature
// unusable in real deployments. The split is deliberate — stop the sensitive
// targets without breaking normal installs.
func isBlockedDialIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// IPv4-mapped IPv6 (::ffff:127.0.0.1) is unwrapped to IPv4 first.
	// Otherwise it slips past the check.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// blockInternalDial is the Control hook on the HTTP dialer. It checks the
// target address at connect time.
//
// The check belongs on the dial, not only when the config is saved, because
// this is the point that actually takes effect. It also covers two ways
// around a save-time check: DNS rebinding (a public IP at validation time, a
// private IP when the connection is made) and redirects (cross-host hops are
// already refused, but a same-host hop can still point the path somewhere
// else).
func blockInternalDial(_, address string, _ syscall.RawConn) error {
	if allowLocalTargets() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("cannot resolve target address %q", host)
	}
	if isBlockedDialIP(ip) {
		return fmt.Errorf("refusing to deliver to loopback or link-local address %s (set %s=1 to allow delivery to a local service)", ip, AllowLocalTargetsEnv)
	}
	return nil
}

// notifyTransport is the default transport plus one dial guard.
// Clone keeps the default tuning (pool, HTTP/2, timeouts, proxy, and so on)
// so the extra check does not change anything else.
var notifyTransport = func() *http.Transport {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	clone := t.Clone()
	clone.DialContext = (&net.Dialer{Timeout: 10 * time.Second, Control: blockInternalDial}).DialContext
	return clone
}()

// httpClient is the client shared by every channel delivery.
//
// It deliberately does not use the project's global egress proxy
// (GlobalProxy on the server side). That proxy is for traffic aimed at
// pentest targets and is often an unstable tunnel. Notification availability
// must not be tied to jitter on the target network. IM pushes go direct.
// The timeout is 15 seconds; a peer slower than that is already failing.
//
// Cross-host redirects are refused. Delivery targets here are a single fixed
// endpoint and do not normally redirect elsewhere. Credentials for these
// platforms (DingTalk access_token, the WeCom key, the Telegram bot token)
// sit in the URL, so following a cross-host hop would hand the credential to
// the redirect target. A same-host hop, such as a trailing slash, is still
// allowed.
var httpClient = &http.Client{
	Timeout:   15 * time.Second,
	Transport: notifyTransport,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("refusing cross-host redirect (%s → %s)", via[0].URL.Host, req.URL.Host)
		}
		return nil
	},
}

// respBodyLimit caps how much of a response body is read. A misbehaving peer
// can return a huge body, and all we need is the status and a short error
// description for the delivery history.
const respBodyLimit = 8 << 10

// doJSON sends one request and returns the response body, already length-limited.
//
// A nil payload sends an empty body (GET, or a platform that wants no body).
// headers are attached as given, for a generic webhook's custom headers.
//
// Classifying the error is the point of this function. Transport failures
// and 5xx/408/429 are retryable. Other 4xx responses are permanent — retrying
// a 403 only writes the same error into the log three times.
func doJSON(ctx context.Context, method, url string, headers map[string]string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			// A marshal failure is a local bug (a config field has the wrong
			// type). Retrying will not fix it.
			return nil, Permanent(fmt.Errorf("failed to build request body: %w", err))
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		// A bad URL is usually a mistyped address, so it is permanent.
		// Do not pass err through: url.Parse's text contains the full address.
		return nil, Permanent(fmt.Errorf("invalid request URL: %s", redactRequestTarget(url)))
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Connection refused, DNS failure, timeout — usually transient, so
		// leave it for backoff retry.
		//
		// The error text must be redacted before it leaves this function.
		// http.Client.Do returns *url.Error, and Error() is
		// `Op "full URL": underlying error`. Credentials for these platforms
		// live in the URL (DingTalk access_token, WeCom key, Feishu hook id,
		// Telegram /bot<token>/). Without redaction the secret flows to four
		// places: notification_deliveries.last_error (stored in the clear),
		// the delivery-history API (which bypasses channel-config masking),
		// server logs, and the 502 text the test-send API returns to the
		// frontend.
		return nil, fmt.Errorf("request failed: %s", redactTransportError(err))
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, respBodyLimit))
	if readErr != nil {
		return nil, fmt.Errorf("failed to read response: %w", readErr)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, nil
	}
	// 429 (rate limit) and 408 (timeout) are worth retrying. Other 4xx
	// responses are config or permission problems; retrying them does nothing.
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout {
		return nil, fmt.Errorf("remote rate limit or timeout (HTTP %d): %s", resp.StatusCode, snippet(raw))
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("remote server error (HTTP %d): %s", resp.StatusCode, snippet(raw))
	}
	return nil, Permanent(fmt.Errorf("request rejected (HTTP %d): %s", resp.StatusCode, snippet(raw)))
}

// snippet collapses a response body to one short line for an error message.
// The body may contain newlines and runs of whitespace, and stuffing that
// into last_error wrecks the delivery-history page.
func snippet(raw []byte) string {
	return OneLine(string(raw), 200)
}

// redactRequestTarget reduces a delivery URL to "scheme://host/…" for error
// messages.
//
// This is the only address-redaction format in the package, and it is
// deliberately blunt: everything except scheme and host is dropped. There is
// no general, safe way to decide which part of a URL is the credential:
//
//	DingTalk   credential in the query        /robot/send?access_token=xxx
//	WeCom      credential in the query        /cgi-bin/webhook/send?key=xxx
//	Feishu     credential in the last path    /open-apis/bot/v2/hook/<hook_id>
//	Telegram   credential in the middle path  /bot<token>/sendMessage
//
// Keeping "only the useful part" would mean a per-channel patch, and missing
// one platform is a credential leak. The host is enough to diagnose DNS,
// connectivity, and certificate problems. Which bot it was is identified by
// the masked tail shown in the channel config.
//
// An unparseable input returns a fixed placeholder. The raw string is never
// echoed.
func redactRequestTarget(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(unparseable address)"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

// redactTransportError strips the address out of a transport error and keeps
// only the underlying cause.
//
// *url.Error is {Op, URL, Err}, and Error() prints the URL with it. Taking
// the Err field bypasses Error(). That is more reliable than a later string
// replace, which has to cope with every encoded or escaped variant of the URL
// and is easy to get wrong.
func redactTransportError(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		host := ""
		if u, parseErr := url.Parse(uerr.URL); parseErr == nil {
			host = u.Host
		}
		if uerr.Err != nil {
			return fmt.Sprintf("%s %s: %s", uerr.Op, host, uerr.Err)
		}
		return fmt.Sprintf("%s %s: unknown error", uerr.Op, host)
	}
	// Errors that are not *url.Error (for example from the redirect policy)
	// can still contain an address, so they go through the same redaction.
	return redactURLsInText(err.Error())
}

// redactURLsInText replaces http(s) URLs in a piece of text with the redacted
// form.
//
// This is the fallback for errors that do not expose a structured field
// (redirect-policy errors, custom errors from a third-party library). Only
// an http or https prefix is recognized, and the URL is cut at whitespace or
// a quote — an address does not contain either.
func redactURLsInText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		rest := s[i:]
		if strings.HasPrefix(rest, "http://") || strings.HasPrefix(rest, "https://") {
			end := len(rest)
			if j := strings.IndexAny(rest, " \t\n\"'"); j >= 0 {
				end = j
			}
			b.WriteString(redactRequestTarget(rest[:end]))
			i += end
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
