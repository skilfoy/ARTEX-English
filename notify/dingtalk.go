package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"
)

// dingTalkChannel implements a DingTalk custom robot.
//
// Platform constraints that shape this implementation:
//   - One robot is limited to 20 messages per minute. Overflow is dropped
//     silently (HTTP may still be 200), so the client must rate-limit.
//     See DefaultRatePerMin.
//   - Security is one of three options: sign, custom keyword, or IP
//     allowlist. Signing is the only option that does not depend on message
//     content, so only signing is supported (plus a bare webhook with none
//     of the three enabled).
//   - Success and failure both return HTTP 200. The errcode in the body is
//     what distinguishes them. Skipping that check records a failed delivery
//     as a success.
type dingTalkChannel struct{}

func (dingTalkChannel) Kind() string { return KindDingTalk }

func (dingTalkChannel) DefaultRatePerMin() int { return 20 }

// The DingTalk webhook URL carries access_token, which is itself a
// credential, so the whole value is masked.
func (dingTalkChannel) SecretKeys() []string { return []string{"webhook", "secret"} }

// The destination is the webhook URL itself. Changing it requires stating
// the signing secret again for the new address.
func (dingTalkChannel) DestinationKeys() []string { return []string{"webhook"} }

func (dingTalkChannel) Validate(cfg map[string]any) error {
	hook := cfgString(cfg, "webhook")
	if hook == "" {
		return errors.New("missing Webhook address")
	}
	if err := validateHTTPURL(hook); err != nil {
		return fmt.Errorf("invalid Webhook address: %w", err)
	}
	return nil
}

// Send delivers one message. A single item with a detail link uses an
// ActionCard (with a button); everything else uses markdown.
func (c dingTalkChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	hook := cfgString(cfg, "webhook")
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	endpoint, err := dingTalkSignedURL(hook, cfgString(cfg, "secret"), time.Now())
	if err != nil {
		return 0, Permanent(err)
	}

	title := markdownTitle(m)
	// DingTalk markdown has no documented byte cap, but a cap is still
	// applied so an oversized evidence field cannot blow the body up.
	text, kept := markdownBody(m, 20000)

	var payload any
	if !m.Batch && len(m.Items) == 1 && m.Items[0].DetailURL != "" {
		payload = map[string]any{
			"msgtype": "actionCard",
			"actionCard": map[string]any{
				"title":          title,
				"text":           text,
				"btnOrientation": "0",
				"singleTitle":    "View details",
				"singleURL":      m.Items[0].DetailURL,
			},
		}
	} else {
		payload = map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]any{"title": title, "text": text},
		}
	}

	raw, err := doJSON(ctx, "POST", endpoint, nil, payload)
	if err != nil {
		return 0, err
	}
	// DingTalk hides business errors inside a 200 response.
	var res struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("failed to parse DingTalk response: %w (%s)", err, snippet(raw))
	}
	if res.ErrCode != 0 {
		// 301000 is a signature failure and 310000 is a keyword mismatch.
		// Both are configuration errors; retrying will not fix them.
		return 0, Permanent(fmt.Errorf("DingTalk returned error %d: %s", res.ErrCode, res.ErrMsg))
	}
	return kept, nil
}

// dingTalkSignedURL appends timestamp and sign to the webhook using the
// official signing rules.
//
// The string to sign is timestamp + "\n" + secret. HMAC-SHA256 uses secret
// as the key as well. The digest is base64-encoded and then URL-encoded.
// timestamp is milliseconds. An empty secret returns the URL unchanged, so
// robots that do not use signing still work.
func dingTalkSignedURL(hook, secret string, now time.Time) (string, error) {
	if secret == "" {
		return hook, nil
	}
	ts := strconv.FormatInt(now.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\n" + secret))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	u, err := url.Parse(hook)
	if err != nil {
		// Do not pass err through: url.Parse's text includes the full
		// address, and that address contains access_token.
		return "", fmt.Errorf("failed to parse Webhook URL: %s", redactRequestTarget(hook))
	}
	q := u.Query()
	q.Set("timestamp", ts)
	q.Set("sign", sign)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// validateHTTPURL checks that the address is usable and the scheme is
// supported, and rejects a literal IP that must not be dialed.
//
// Two constraints:
//
//  1. Error text must be redacted. url.Parse returns *url.Error, and Error()
//     includes the full original address. These platforms embed credentials
//     in that address (DingTalk access_token, WeCom key, Telegram bot token,
//     Feishu hook id). Returning err directly used to leak the credential
//     through the "invalid address" error into the test API's 400 response,
//     last_error on every delivery, server logs, and the delivery-history API.
//
//  2. A literal IP is judged here. Hostnames are left to dial time
//     (blockInternalDial is the check that actually takes effect, and it also
//     covers DNS rebinding). Checking here lets a bad address fail when the
//     config is saved, instead of on the first delivery.
//
// Restricting the scheme is defensive. file:// and gopher:// make
// http.Client do something unexpected. The scheme check already blocks them,
// and there is no reason to open that surface.
func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("address could not be parsed (%s)", redactRequestTarget(raw))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https is supported, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("missing hostname")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && isBlockedDialIP(ip) && !allowLocalTargets() {
		return fmt.Errorf("refusing to deliver to loopback or link-local address %s (set %s=1 to allow delivery to a local service)", ip, AllowLocalTargetsEnv)
	}
	return nil
}
