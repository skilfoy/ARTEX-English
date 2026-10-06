package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// weComMarkdownLimit is the hard cap on a WeCom group robot's markdown
// content, in bytes, not characters. It is the tightest limit of the six
// channels, and the main reason TruncateBytes exists.
const weComMarkdownLimit = 4096

// weComChannel implements a WeCom group robot.
//
// Platform constraints:
//   - Auth is only the key on the URL. There is no signature, so the webhook
//     address itself is the entire credential.
//   - markdown content is capped at 4096 bytes. Over that, the whole message
//     is rejected rather than truncated. Chinese is 3 bytes per character,
//     so the body holds a little over a thousand characters and the client
//     must truncate.
//   - The rate limit is 20 messages per minute, enforced on the client as well.
type weComChannel struct{}

func (weComChannel) Kind() string { return KindWeCom }

func (weComChannel) DefaultRatePerMin() int { return 20 }

// WeCom has a single credential: the key on the webhook URL. It does not
// support signing, so the whole address is the secret and nothing else needs
// to be masked.
func (weComChannel) SecretKeys() []string { return []string{"webhook"} }

// WeCom has only the webhook field. It is both the destination and the
// credential, so there is no leftover secret after the address changes.
func (weComChannel) DestinationKeys() []string { return []string{"webhook"} }

func (weComChannel) Validate(cfg map[string]any) error {
	hook := cfgString(cfg, "webhook")
	if hook == "" {
		return errors.New("missing Webhook address")
	}
	if err := validateHTTPURL(hook); err != nil {
		return fmt.Errorf("invalid Webhook address: %w", err)
	}
	return nil
}

func (c weComChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	// A digest can be long (50 items, one line each, plus a prefix) and
	// 4096 bytes is easy to exceed. Truncate here instead of waiting for the
	// platform to reject the message: a rejection drops the whole batch,
	// while truncation at least delivers the first items.
	content, kept := markdownBody(m, weComMarkdownLimit)
	payload := map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]any{"content": content},
	}
	raw, err := doJSON(ctx, "POST", cfgString(cfg, "webhook"), nil, payload)
	if err != nil {
		return 0, err
	}
	var res struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("failed to parse WeCom response: %w (%s)", err, snippet(raw))
	}
	if res.ErrCode != 0 {
		// 45009 means the API call exceeded the limit. The platform's rate
		// window rolls forward, so retrying after backoff works. It is
		// explicitly retryable. Reaching this point means the client's
		// rate_per_min is set too aggressively. Retry is only a backstop;
		// the real fix is to lower that channel's limit.
		if res.ErrCode == 45009 {
			return 0, fmt.Errorf("WeCom rate limited %d: %s", res.ErrCode, res.ErrMsg)
		}
		// 93000 means the webhook key is invalid. That is permanent; retry
		// will not fix it.
		return 0, Permanent(fmt.Errorf("WeCom returned error %d: %s", res.ErrCode, res.ErrMsg))
	}
	return kept, nil
}
