package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// weComMarkdownLimit It's a microbots. markdown content Hard limit (bytes, non-charts)).
// It's the tightest of all six channels. TruncateBytes Main reasons for existence.
const weComMarkdownLimit = 4096

// weComChannel Achieving the Enterprise Wisdom..
//
// Platform Features:
//   - Only through URL Top key Your Honor, you don't need to sign.——So webhook The address is all in itself..
//   - markdown content upper limit 4096 **Bytes**,The article was rejected (not cut). Chinese 3 Bytes/Words,
//     It means the body is written in more than a thousand words, and the client must be cut..
//   - Current limiting 20 strip/Minutes. Same client limit..
type weComChannel struct{}

func (weComChannel) Kind() string { return KindWeCom }

func (weComChannel) DefaultRatePerMin() int { return 20 }

// There's only one thing we can do. Webhook A certificate.(URL Top key),And it doesn't support signing.——
// The whole address is full of evidence. There's no other field to hide..
func (weComChannel) SecretKeys() []string { return []string{"webhook"} }

// Micro just Webhook A field, which is both a destination and evidence, and therefore no[We'll have to change our address.]Words.
func (weComChannel) DestinationKeys() []string { return []string{"webhook"} }

func (weComChannel) Validate(cfg map[string]any) error {
	hook := cfgString(cfg, "webhook")
	if hook == "" {
		return errors.New("Missing Webhook Address")
	}
	if err := validateHTTPURL(hook); err != nil {
		return fmt.Errorf("Webhook Address invalid: %w", err)
	}
	return nil
}

func (c weComChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	// The aggregate may be long.(50 strip × Every line + Prefix),4096 Bytes are easy to overwrite..
	// Interception is done here, not through the platform: rejection means total loss, while interruption reaches at least a few previous articles.
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
		return 0, fmt.Errorf("Failed to parse micro-credit response: %w (%s)", err, snippet(raw))
	}
	if res.ErrCode != 0 {
		// 45009 It's the interface calling beyond the limit.——The platform's restricted window will be rolling and retrying will work.,
		// So it's obvious that you can try again. Come here and show your client. rate_per_min It's too radical.,
		// It's just the bottom line. The real fix is to lower the channel. Value.
		if res.ErrCode == 45009 {
			return 0, fmt.Errorf("Enterprise micro-credit flow %d: %s", res.ErrCode, res.ErrMsg)
		}
		// 93000 Yes webhook key Invalid——It's a permanent failure..
		return 0, Permanent(fmt.Errorf("Enterprise Wireback Error %d: %s", res.ErrCode, res.ErrMsg))
	}
	return kept, nil
}
