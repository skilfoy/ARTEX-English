package notify

import (
	"fmt"
	"strings"
)

// This file is the shared message renderer for the markdown-family channels
// (DingTalk and WeCom). Feishu renders a card as JSON, Telegram renders HTML,
// and email renders HTML, each inside its own adapter.

// maxAssetsShown is how many assets a message lists. A finding can be
// anchored to dozens of assets. Listing all of them blows the message up and
// adds nothing: nobody reads the fourth domain in an IM client.
const maxAssetsShown = 3

// maxSummaryRunes is how far a summary is compressed, in characters. An IM
// message is a prompt to open the detail, not the report itself. The full
// text lives on the platform.
const maxSummaryRunes = 120

// markdownReservedBytes is reserved for the header (digest line, severity
// breakdown, and a possible truncation note) and the footer (the platform
// link). Whole-item packing subtracts this from the budget so the head and
// tail are not the part that gets cut. If they are cut, the reader cannot
// tell which batch this is or how many items were left out.
const markdownReservedBytes = 320

// markdownEscape escapes markdown metacharacters.
//
// This is required because titles, summaries, vuln classes, and asset names
// all come from untrusted input. Titles and summaries come from model output
// (the model read the target's response), and an asset URL is the full URL
// the scan observed, including a query string the target controls. Without
// escaping, a title of
//
//	login SQL injection\n[urgent: verify your account](http://attacker.tld)
//
// renders as a clickable external link in a security engineer's DingTalk or
// Feishu chat. `![](http://attacker.tld/beacon)` is fetched by the client
// while rendering, which reports that the finding was opened and leaks the
// reader's IP. Even without malice, injected bold or a blockquote can push
// a serious finding below the fold.
//
// The set covers characters that change structure or create something
// clickable: headings, links, emphasis, lists, quotes, and strikethrough.
// Backslash has to be handled first, or the backslashes added later are
// escaped again.
func markdownEscape(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"`", "\\`",
		"*", `\*`,
		"_", `\_`,
		"[", `\[`,
		"]", `\]`,
		"(", `\(`,
		")", `\)`,
		"!", `\!`,
		"#", `\#`,
		">", `\>`,
		"|", `\|`,
		"~", `\~`,
	)
	return replacer.Replace(s)
}

// markdownText collapses untrusted text to one line and escapes it, for use
// in a markdown body. Collapsing is the other half of escaping: a newline
// by itself can forge a new list item or blockquote, and character escaping
// does not stop that.
func markdownText(s string, maxRunes int) string {
	return markdownEscape(OneLine(s, maxRunes))
}

// markdownTitle returns the message title (the IM title bar or card title)
// as unescaped source text.
//
// Escaping is deliberately not done here. Four renderers share this title:
// markdown bodies, Telegram HTML, Feishu card plain_text, and the generic
// webhook's JSON plus the email subject. Each context has its own escape
// rules. Markdown escapes left in HTML show up as visible backslashes, and
// the same escapes stuffed into JSON corrupt the data, so each output path
// escapes for itself. See writeItem, feishuItemLines, and telegramEscape.
// Escaping inside this shared function once produced visible `\(1\)` in
// Telegram messages.
func markdownTitle(m Message) string {
	if m.Batch {
		return fmt.Sprintf("Summary of %d findings", len(m.Items))
	}
	if len(m.Items) == 0 {
		return "Finding notification"
	}
	it := m.Items[0]
	return fmt.Sprintf("[%s] %s", SeverityLabel(it.Severity), OneLine(it.Title(), 0))
}

// markdownBody renders the message body and returns it with the number of
// items actually written.
//
// kept is how many items this delivery really sent. The caller marks only
// the first kept items as delivered. Items that did not fit under the
// channel's length cap must wait for the next batch instead of being marked
// successful along with the rest. That is where silent loss comes from: the
// message was truncated, the delivery record says everything was sent, and
// nothing shows that the rest never went out.
//
// maxBytes <= 0 means no limit.
func markdownBody(m Message, maxBytes int) (string, int) {
	if !m.Batch {
		if len(m.Items) == 0 {
			return "", 0
		}
		var b strings.Builder
		writeItem(&b, m.Items[0], "", true)
		// A single message is still sent when it is too long (the final
		// truncation is the backstop). Part of one finding is better than
		// sending nothing.
		return TruncateBytes(b.String(), maxBytes), 1
	}

	footer := ""
	if m.HomeURL != "" {
		footer = fmt.Sprintf("\n[View all on platform](%s)\n", m.HomeURL)
	}
	kept := packItemCount(m.Items, maxBytes, markdownReservedBytes, footer, byteSize, func(it Item, idx int) string {
		var b strings.Builder
		writeItem(&b, it, fmt.Sprintf("%d. ", idx+1), false)
		return b.String()
	})

	items := m.Items[:kept]
	var b strings.Builder
	b.WriteString(markdownBatchIntro(m, items, len(m.Items)))
	for i, it := range items {
		writeItem(&b, it, fmt.Sprintf("%d. ", i+1), false)
	}
	b.WriteString(footer)
	return TruncateBytes(b.String(), maxBytes), kept
}

// markdownBatchIntro renders the start of a digest: time window, count, and
// severity breakdown. With that, the reader can decide whether the batch
// needs attention without opening the platform.
//
// items are the items that actually fit. total is how many the batch should
// have contained. When they differ, the text must say how many continue in
// the next message. Otherwise the reader treats the number in the header as
// the full set, and the items that were never sent do not exist anywhere in
// the UI.
func markdownBatchIntro(m Message, items []Item, total int) string {
	var b strings.Builder
	if m.WindowMinutes > 0 {
		fmt.Fprintf(&b, "**%d findings in the past %d minutes**", total, m.WindowMinutes)
	} else {
		fmt.Fprintf(&b, "**%d new findings**", total)
	}
	if extra := total - len(items); extra > 0 {
		fmt.Fprintf(&b, " (%d shown; %d remain for the next message)", len(items), extra)
	}
	// The breakdown lets the reader see whether anything serious is in this
	// message. Only items actually included here are counted, so "critical 3"
	// matches the lines that follow.
	counts := map[string]int{}
	for _, it := range items {
		counts[it.Severity]++
	}
	var parts []string
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		if n := counts[sev]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", SeverityLabel(sev), n))
		}
	}
	if len(parts) > 0 {
		b.WriteString("\n" + strings.Join(parts, " · "))
	}
	b.WriteString("\n\n")
	return b.String()
}

// writeItem renders one finding.
//
// prefix is the digest index. single renders the full form (summary and
// detail link). A digest list is one line per item; otherwise 50 items
// become a long document.
//
// Every field that came from outside (title, class, assets, summary) goes
// through markdownText: one line, then escaped. The detail link is built
// from the administrator's public_base_url, so it is not untrusted content,
// and it has to stay a clickable link, so it is written as-is.
func writeItem(b *strings.Builder, it Item, prefix string, single bool) {
	line := fmt.Sprintf("%s**%s · %s**", prefix, SeverityLabel(it.Severity), markdownText(it.Title(), 0))
	if !single {
		// Digest mode: one line, with assets and summary compressed onto it.
		var extras []string
		if a := assetLine(it.Assets, maxAssetsShown); a != "" {
			extras = append(extras, markdownText(a, 0))
		}
		if it.Summary != "" {
			extras = append(extras, markdownText(it.Summary, 60))
		}
		if len(extras) > 0 {
			line += " — " + strings.Join(extras, " · ")
		}
		b.WriteString(line + "\n")
		return
	}
	b.WriteString(line + "\n")
	if it.IsStatusChange() {
		fmt.Fprintf(b, "**Status change**: %s → %s\n",
			markdownText(StatusLabel(it.FromStatus), 0), markdownText(StatusLabel(it.ToStatus), 0))
	}
	if it.VulnClass != "" && it.VulnClass != it.Title() {
		fmt.Fprintf(b, "**Type**: %s\n", markdownText(it.VulnClass, 0))
	}
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		fmt.Fprintf(b, "**Assets**: %s\n", markdownText(a, 0))
	}
	if it.Summary != "" {
		if s := markdownText(it.Summary, maxSummaryRunes); s != "" {
			fmt.Fprintf(b, "**Summary**: %s\n", s)
		}
	}
	if it.DetailURL != "" {
		fmt.Fprintf(b, "[View details](%s)\n", it.DetailURL)
	}
}
