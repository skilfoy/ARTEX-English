package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// telegramTextLimit Yes Telegram sendMessage of text Field cap (number of characters)).
const telegramTextLimit = 4096

// telegramChannel Achieved Telegram Bot API.
//
// Platform Features:
//   - All rights are vested in you. URL path inside(/bot<token>/sendMessage),No need to sign.
//   - Use HTML Parsing mode instead of MarkdownV2:MarkdownV2 Request for conversion. `_*[]()~`>#+-=|{}.!`
//     Total 18 One character, one missing, the whole message is rejected.;HTML Just a transfer. & < > Three..
//   - Business mistakes are also hidden. HTTP 200 Shit. ok Field judgement.
type telegramChannel struct{}

func (telegramChannel) Kind() string { return KindTelegram }

// Telegram We'll talk. 1 strip/seconds, groups 20 strip/Minutes. Take Conservative Value.
func (telegramChannel) DefaultRatePerMin() int { return 20 }

// Bot Token It's complete evidence.;chat_id It's not a secret. Token I can't send a message.).
func (telegramChannel) SecretKeys() []string { return []string{"bot_token"} }

// base_url Decision Token To whom? API Endpoints (e.g., self-constructing) where change must be made Token.
func (telegramChannel) DestinationKeys() []string { return []string{"base_url"} }

func (telegramChannel) Validate(cfg map[string]any) error {
	if cfgString(cfg, "bot_token") == "" {
		return errors.New("Missing Bot Token")
	}
	if cfgString(cfg, "chat_id") == "" {
		return errors.New("Missing Chat ID")
	}
	if base := cfgString(cfg, "base_url"); base != "" {
		if err := validateHTTPURL(base); err != nil {
			return fmt.Errorf("API Address invalid: %w", err)
		}
	}
	return nil
}

func (c telegramChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	endpoint, err := telegramEndpoint(cfg)
	if err != nil {
		return 0, Permanent(err)
	}
	text, kept := telegramHTML(m)
	payload := map[string]any{
		"chat_id":                  cfgString(cfg, "chat_id"),
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": false,
	}
	raw, err := doJSON(ctx, "POST", endpoint, nil, payload)
	if err != nil {
		return 0, err
	}
	var res struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("Analysis Telegram Failed to respond: %w (%s)", err, snippet(raw))
	}
	if res.OK {
		return kept, nil
	}
	// 429 It is restricted, and it is tried again; the rest(400 Error Parameter,401 token Wrong.,403 ♪ To be taken ♪,
	// 404 chat It doesn't exist. It's all about configuration. It's not gonna heal..
	if res.ErrorCode == 429 {
		return 0, fmt.Errorf("Telegram Current limiting: %s", res.Description)
	}
	return 0, Permanent(fmt.Errorf("Telegram Return error %d: %s", res.ErrorCode, res.Description))
}

// telegramEndpoint Spell sendMessage Address.base_url It's official. API,
// Non-empty time for self-building Bot API Inverse (common demand under domestic networks)).
func telegramEndpoint(cfg map[string]any) (string, error) {
	base := cfgString(cfg, "base_url")
	if base == "" {
		base = "https://api.telegram.org"
	}
	base = strings.TrimSuffix(base, "/")
	token := cfgString(cfg, "bot_token")
	raw := base + "/bot" + token + "/sendMessage"
	u, err := url.Parse(raw)
	if err != nil {
		// I don't know. err:Chile Bot Token,And at this point, addr I shouldn't have..
		return "", fmt.Errorf("Collapse API Chile(API Address:%s)", redactRequestTarget(base))
	}
	return u.String(), nil
}

// telegramHTML Rendering HTML Text, return text and actual entries (see Channel.Send).
func telegramHTML(m Message) (string, int) {
	var b strings.Builder
	b.WriteString("<b>" + telegramEscape(markdownTitle(m)) + "</b>\n")
	if m.Batch {
		// Telegram The limit is...**Number of characters**,So packing is also measured by characters.(runeSize).
		footer := ""
		if m.HomeURL != "" {
			footer = fmt.Sprintf("\n\n<a href=\"%s\">View All on Platform</a>", telegramEscapeAttr(m.HomeURL))
		}
		kept := packItemCount(m.Items, telegramTextLimit, telegramReservedRunes, footer, runeSize, func(it Item, idx int) string {
			return telegramBatchLine(it, idx+1)
		})
		items := m.Items[:kept]
		b.Reset()
		b.WriteString("<b>" + telegramEscape(telegramBatchTitle(m, items, len(m.Items))) + "</b>")
		for i, it := range items {
			b.WriteString("\n" + telegramEscape(telegramBatchLine(it, i+1)))
		}
		b.WriteString(footer)
		return TruncateHTML(b.String(), telegramTextLimit), kept
	}
	if len(m.Items) == 0 {
		return b.String(), 0
	}
	it := m.Items[0]
	if it.IsStatusChange() {
		b.WriteString(fmt.Sprintf("\n<b>Status change</b>:%s → %s",
			telegramEscape(StatusLabel(it.FromStatus)), telegramEscape(StatusLabel(it.ToStatus))))
	}
	if it.VulnClass != "" && it.VulnClass != it.Title() {
		b.WriteString("\n<b>Type</b>:" + telegramEscape(it.VulnClass))
	}
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		b.WriteString("\n<b>Assets</b>:" + telegramEscape(a))
	}
	if s := OneLine(it.Summary, maxSummaryRunes); s != "" {
		b.WriteString("\n<b>Abstract</b>:" + telegramEscape(s))
	}
	if it.DetailURL != "" {
		b.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">View details</a>", telegramEscapeAttr(it.DetailURL)))
	}
	return TruncateHTML(b.String(), telegramTextLimit), 1
}

// telegramReservedRunes Save message headers and possible cut-off tips (charts)).
const telegramReservedRunes = 160

// telegramBatchLine One of the rendering combinations (not converted, unified by caller)).
func telegramBatchLine(it Item, idx int) string {
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		return fmt.Sprintf("%d. %s · %s — %s", idx, SeverityLabel(it.Severity), it.Title(), a)
	}
	return fmt.Sprintf("%d. %s · %s", idx, SeverityLabel(it.Severity), it.Title())
}

// telegramBatchTitle Renders the title line of the summary message. The number of bars is...**This article actually covers**number of items,
// Not the total number of instalments——Otherwise, readers think the numbers in the headline are all..
func telegramBatchTitle(m Message, items []Item, total int) string {
	title := fmt.Sprintf("%d findings", total)
	if extra := total - len(items); extra > 0 {
		title += fmt.Sprintf(" (%d shown; %d remain for the next message.)", len(items), extra)
	}
	if m.WindowMinutes > 0 {
		title = fmt.Sprintf("Past %d minutes · %s", m.WindowMinutes, title)
	}
	return title
}

// telegramEscape Conversion HTML Text Contents.
// Telegram Only these three entities. &amp; An existing entity like that is subject to secondary conversion.——Exactly.
// Expectations: We're going to show original characters, not infusion. HTML.
func telegramEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// telegramEscapeAttr Conversion HTML attribute value. We have to deal with quotation marks in addition to text transposition.——
// URL The quotes will close early. href Properties, turn the rest into an injection point..
func telegramEscapeAttr(s string) string {
	s = telegramEscape(s)
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
