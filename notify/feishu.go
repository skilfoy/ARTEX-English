package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// feishuChannel Flying Book Lark)Custom robot, walk interactive card.
//
// Platform Features:
//   - Adding algorithms and nails**Different.**,Very easy to write. See. feishuSign Comment.
//   - Plug business mistakes like nails. HTTP 200 of body inside(code != 0).
//   - Card header Supports colour templates, horizontal mapping of colours, allowing for a glimpse of severity in the message list.
type feishuChannel struct{}

func (feishuChannel) Kind() string { return KindFeishu }

// Flying Book customizes robotics. 5 times/Seconds, match 100 times/min.
func (feishuChannel) DefaultRatePerMin() int { return 100 }

// Webhook At the end of the address, the only sign of the robot..
func (feishuChannel) SecretKeys() []string { return []string{"webhook", "secret"} }

// Same: modified Webhook Address must restate signature key for new address.
func (feishuChannel) DestinationKeys() []string { return []string{"webhook"} }

func (feishuChannel) Validate(cfg map[string]any) error {
	hook := cfgString(cfg, "webhook")
	if hook == "" {
		return errors.New("Missing Webhook Address")
	}
	if err := validateHTTPURL(hook); err != nil {
		return fmt.Errorf("Webhook Address invalid: %w", err)
	}
	return nil
}

func (c feishuChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	card, kept := feishuCard(m)
	payload := map[string]any{
		"msg_type": "interactive",
		"card":     card,
	}
	// Signing parameters are on the same level as messages and only configured secret It happens..
	if secret := cfgString(cfg, "secret"); secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		payload["timestamp"] = ts
		payload["sign"] = feishuSign(ts, secret)
	}
	raw, err := doJSON(ctx, "POST", cfgString(cfg, "webhook"), nil, payload)
	if err != nil {
		return 0, err
	}
	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		// Some of the flying books. hook Use this field name to be compatible.
		StatusCode    int    `json:"StatusCode"`
		StatusMessage string `json:"StatusMessage"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("Failed to parse flybook response: %w (%s)", err, snippet(raw))
	}
	if res.Code != 0 {
		return 0, Permanent(fmt.Errorf("Flying Book returned error %d: %s", res.Code, res.Msg))
	}
	if res.StatusCode != 0 {
		return 0, Permanent(fmt.Errorf("Flying Book returned error %d: %s", res.StatusCode, res.StatusMessage))
	}
	return kept, nil
}

// feishuSign I'm counting signatures according to the official rules of flying books..
//
// It's particularly easy to step on.
//
//	hmac.new(string_to_sign.encode(), digestmod=sha256)
//
// Which means... **key = timestamp + "\n" + secret,message Empty**,Not intuitive.
// [key=secret, message=stringToSign]——That's how nails work. Two-sided algorithms are the opposite.,
// It's a failure to verify the signature of the other family. 19021).
func feishuSign(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// feishuSeverityTemplate Map the gap level to the card header Colour Template.
// Unknown level grey——No need. blue,In case of peace. low Confusion.
func feishuSeverityTemplate(severity string) string {
	switch severity {
	case "critical":
		return "red"
	case "high":
		return "orange"
	case "medium":
		return "yellow"
	case "low":
		return "blue"
	default:
		return "grey"
	}
}

// feishuMaxCardBytes It's the conservative limit on the content of the card. Flying Book has a size limit on the card.;
// Take a value that is clearly below the official ceiling. JSON Packaging expenses are included..
const feishuMaxCardBytes = 24000

// feishuCard Construct interactive cards, return cards with**Number of entries actually written**.
// kept for the same purpose markdownBody:Only entries that actually enter the card should be marked as delivered.
func feishuCard(m Message) (map[string]any, int) {
	elements := []any{}
	kept := 0
	if m.Batch {
		// I'll type the whole thing and spell the head: the head.[The rest N The strip will continue with the next message.],
		// N Must come from the number of bars actually loaded..
		kept = packItemCount(m.Items, feishuMaxCardBytes, markdownReservedBytes, "", byteSize, func(it Item, idx int) string {
			return feishuBatchLine(it, idx+1)
		})
		items := m.Items[:kept]
		elements = append(elements, feishuMarkdownDiv(markdownBatchIntro(m, items, len(m.Items))))
		for i, it := range items {
			elements = append(elements, feishuMarkdownDiv(feishuBatchLine(it, i+1)))
		}
		if m.HomeURL != "" {
			elements = append(elements, feishuButton("View All on Platform", m.HomeURL))
		}
	} else if len(m.Items) > 0 {
		kept = 1
		it := m.Items[0]
		elements = append(elements, feishuMarkdownDiv(feishuItemLines(it)))
		if it.DetailURL != "" {
			elements = append(elements, feishuButton("View details", it.DetailURL))
		}
	}

	card := map[string]any{
		"config":   map[string]any{"wide_screen_mode": true},
		"header":   map[string]any{"title": map[string]any{"tag": "plain_text", "content": markdownTitle(m)}},
		"elements": elements,
	}
	if len(m.Items) > 0 {
		card["header"].(map[string]any)["template"] = feishuSeverityTemplate(m.Items[0].Severity)
	}
	return card, kept
}

func feishuMarkdownDiv(content string) map[string]any {
	return map[string]any{"tag": "div", "text": map[string]any{"tag": "lark_md", "content": content}}
}

func feishuButton(label, url string) map[string]any {
	return map[string]any{
		"tag": "action",
		"actions": []any{map[string]any{
			"tag":  "button",
			"text": map[string]any{"tag": "lark_md", "content": label},
			"url":  url,
			"type": "primary",
		}},
	}
}

// feishuItemLines Rendering individual loopholes lark_md Text.
//
// lark_md With markdown It's a text format for a homologue, which also analyzes links and highlights, so it's from outside.
// All fields passed. markdownText(Single Line + Conversion)——Otherwise a loophole header could be in
// Flying books become clickable outer chains..
func feishuItemLines(it Item) string {
	out := fmt.Sprintf("**%s · %s**", SeverityLabel(it.Severity), markdownText(it.Title(), 0))
	if it.IsStatusChange() {
		out += fmt.Sprintf("\n**Status change**:%s → %s",
			markdownText(StatusLabel(it.FromStatus), 0), markdownText(StatusLabel(it.ToStatus), 0))
	}
	if it.VulnClass != "" && it.VulnClass != it.Title() {
		out += fmt.Sprintf("\n**Type**:%s", markdownText(it.VulnClass, 0))
	}
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		out += fmt.Sprintf("\n**Assets**:%s", markdownText(a, 0))
	}
	if it.Summary != "" {
		if s := markdownText(it.Summary, maxSummaryRunes); s != "" {
			out += fmt.Sprintf("\n**Abstract**:%s", s)
		}
	}
	return out
}

// feishuBatchLine Render one of the summary cards..
func feishuBatchLine(it Item, index int) string {
	line := fmt.Sprintf("**%d. %s · %s**", index, SeverityLabel(it.Severity), markdownText(it.Title(), 0))
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		line += " — " + markdownText(a, 0)
	}
	return line
}
