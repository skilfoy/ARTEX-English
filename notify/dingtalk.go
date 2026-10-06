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

// dingTalkChannel Accomplishing nails to define robots..
//
// Platform characteristics.):
//   - Single robot limit. 20 strip/Minutes, super-hairs will be left silent.(HTTP Still possible. 200),
//     So restricted flow has to be done on the client side. DefaultRatePerMin.
//   - Security Settings Three or One: Add / Customized keywords / IP White list. Plus is the only non-dependent
//     The message content program, so we only support the signing. webhook).
//   - Success/Failures return HTTP 200,Shit. body inside errcode Distinction——Do Not Check errcode
//     You'll write down the delivery failure as a success..
type dingTalkChannel struct{}

func (dingTalkChannel) Kind() string { return KindDingTalk }

func (dingTalkChannel) DefaultRatePerMin() int { return 20 }

// Nailed it. Webhook In the address. access_token,It's the evidence itself, so it's the whole mask..
func (dingTalkChannel) SecretKeys() []string { return []string{"webhook", "secret"} }

// The target is nailed. Webhook address itself; changing address must be accompanied by a new signer key to the new address.
func (dingTalkChannel) DestinationKeys() []string { return []string{"webhook"} }

func (dingTalkChannel) Validate(cfg map[string]any) error {
	hook := cfgString(cfg, "webhook")
	if hook == "" {
		return errors.New("Missing Webhook Address")
	}
	if err := validateHTTPURL(hook); err != nil {
		return fmt.Errorf("Webhook Address invalid: %w", err)
	}
	return nil
}

// Send Send one message. When there's a return chain and it's single. ActionCard(with button) markdown.
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
	// DingTalk markdown There is no explicit byte limit in the body, but the upper limit is still protected to avoid abnormal expansion of the evidence field.
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
	// The nails hide business mistakes. 200 Response.
	var res struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("Parsing nails failed.: %w (%s)", err, snippet(raw))
	}
	if res.ErrCode != 0 {
		// 301000 Signature verification failed,310000 It doesn't match the keyword.——All configuration errors,
		// It won't heal..
		return 0, Permanent(fmt.Errorf("Nail back error %d: %s", res.ErrCode, res.ErrMsg))
	}
	return kept, nil
}

// dingTalkSignedURL In accordance with the official signing rules webhook Append timestamp With sign Parameter.
//
// Rule: To be signed = timestamp + "\n" + secret,HMAC-SHA256 of**And the key. secret**,
// Results base64 After URL Encode.timestamp milliseconds..secret Other Organiser,
// Support for unsigned machines People.
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
		// I don't know. err:url.Parse Cannot initialise Evolution's mail component. access_token).
		return "", fmt.Errorf("Analysis Webhook Chile: %s", redactRequestTarget(hook))
	}
	q := u.Query()
	q.Set("timestamp", ts)
	q.Set("sign", sign)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// validateHTTPURL Validation address available, protocol supported and literally IP Target's on the inside..
//
// Two o'clock.:
//
//  1. **Error messages must be dissensive.**.url.Parse I'm going back. *url.Error,It's... Error() With
//     **Full original address**,And this function is embedded in the addresses of these families. access_token,
//     Micro key,Telegram of bot token,Feishu hook id).It used to be straight here. `return err`,
//     So[Address Format Invalid]This mistake led the evidence out to the test interface. 400 Response,
//     Every time we drop the library last_error,Service-end log and delivery history interface.
//
//  2. **Literally IP Direct Internet**,Domain name reserved for dialup phase.(blockInternalDial It's the end.
//     It's effective. It's covered. DNS Rebound. One time here to get a hint when you save the configuration.,
//     Instead of waiting for the first delivery to fail..
//
// The restraining agreement is defensive.:file:///gopher:// And so on. http.Client Unexpected.
// Behaviour (although already scheme Cover the check, but there's no reason to let go of this side.).
func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("Address Could Not Parsing(%s)", redactRequestTarget(raw))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("Support only http/https,Received %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("Missing hostname")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && isBlockedDialIP(ip) && !allowLocalTargets() {
		return fmt.Errorf("Refuse delivery to this machine/Local address for links %s(If you really need to deliver to this service, settings %s=1)", ip, AllowLocalTargetsEnv)
	}
	return nil
}
