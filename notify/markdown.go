package notify

import (
	"fmt"
	"strings"
)

// What is this document?[Markdown Yes]A common message from the channel..
// The flybook card. JSON,Telegram Use HTML,Mail HTML,They're in the adapter..

// maxAssetsShown It is up to a few assets listed in the news. A loophole could anchor dozens of assets.,
// The whole column will crush the message and have no information.——No. 4 After that, no one will know. IM Look inside..
const maxAssetsShown = 3

// maxSummaryRunes It's how many words the summary has been compressed. Arguments.IM The message is...[Tips for details],
// Not the body of the report. It's in the platform..
const maxSummaryRunes = 120

// markdownReservedBytes Preserve message header (summary line) + Level distribution + Possible Cuttips)
// With tail (platform link). When you pack the whole thing, you take that part off the budget, and you make sure it doesn't get cut off.——
// Once the end is cut, the readers will be[Which one is it? How many more are not shown?]I can't tell..
const markdownReservedBytes = 320

// markdownEscape Conversion markdown Character.
//
// Why do you have to do it: the headline, the summary, the type, the name of the asset is all from**Untrustable sources**——
// Titles and abstracts are derived from model outputs (models are read in response to detected targets), assets url Scan.
// Complete URL(contains a controllable query string for the target. If you don't, it's called
//
//	Login Port SQL Injection\n[Emergency: Click this to verify account number](http://attacker.tld)
//
// The leak will be nailed to the security engineer./It's in the flying book.**Clickable Extralinks**;And
// `![](http://attacker.tld/beacon)` It'll be pulled by the client during the rendering. It's like a notification.
// [This loophole has been seen.]And leak the reader. IP.Even if it's not malicious, it's in bold or
// The quote box also closes a serious loophole below..
//
// Conversion group overtook title/Link/Emphasizing/List/References/The strikeout lines change structures or create clickables.
// Characters of Elements.`\` We have to deal with it first, or we'll turn the back slash on the back..
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

// markdownText Compress untrustworthy texts into single lines and transpose them to markdown Text used.
// Single-line is the other half of the transposition: a line change itself can forge new list entries or quote blocks,
// And the transliteration won't stop it..
func markdownText(s string, maxRunes int) string {
	return markdownEscape(OneLine(s, maxRunes))
}

// markdownTitle Message Title(IM Platform Title Bar/The card title.**Original language not converted**.
//
// It's not meant to be transliterated here: the title is shared by the renderer of four languages.——markdown Text,Telegram of
// HTML,Flying Book Card. plain_text,and universal Webhook of JSON With the email theme. Every level.
// The rules of conversion are different.(markdown Transferred. HTML It'll leave a visible backslash. JSON It's polluting.
// Data), so transposition must be the responsibility of the respective output end. See writeItem / feishuItemLines /
// telegramEscape.Used to add in shared functions markdown Transfer, result. Telegram In the message
// There it is. `\(1\)` This visible backslash.
func markdownTitle(m Message) string {
	if m.Batch {
		return fmt.Sprintf("Summary of %d findings", len(m.Items))
	}
	if len(m.Items) == 0 {
		return "Hole notification"
	}
	it := m.Items[0]
	return fmt.Sprintf("[%s] %s", SeverityLabel(it.Severity), OneLine(it.Title(), 0))
}

// markdownBody Render message text, return text and**Number of entries actually written**.
//
// Return value kept It's the number of entries that actually reach this delivery. kept Mark Bar as
// Delivered——The entries that are blocked by the maximum channel length must be left in the next batch instead of being followed.
// Mark successful. Exactly.[Quietly lost.]Source: The message was blocked, but the delivery records show all deliveries.,
// There's no place to see the second half never coming out..
//
// maxBytes<=0 Means unlimited.
func markdownBody(m Message, maxBytes int) (string, int) {
	if !m.Batch {
		if len(m.Items) == 0 {
			return "", 0
		}
		var b strings.Builder
		writeItem(&b, m.Items[0], "", true)
		// Single message sent even if it is too long (by final cut-off): partial information from a loophole
		// It's better than nothing..
		return TruncateBytes(b.String(), maxBytes), 1
	}

	footer := ""
	if m.HomeURL != "" {
		footer = fmt.Sprintf("\n[View All on Platform](%s)\n", m.HomeURL)
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

// markdownBatchIntro Rendering the beginning of the summary message: Time window, number of bars and grade distribution.
// With this, the person receiving the package doesn't need to click on the platform to determine whether this needs to be handled immediately..
//
// items Yes**Actual Load**entry,total is the total amount due in this instalment. Both have to be clear.
// [How many more are there?]——Otherwise, the readers will think that the number in the message is all.,
// And the last ones that never came out did not exist on the interface..
func markdownBatchIntro(m Message, items []Item, total int) string {
	var b strings.Builder
	if m.WindowMinutes > 0 {
		fmt.Fprintf(&b, "**%d findings in the past %d minutes.**", total, m.WindowMinutes)
	} else {
		fmt.Fprintf(&b, "**%d new findings.**", total)
	}
	if extra := total - len(items); extra > 0 {
		fmt.Fprintf(&b, " (%d shown; %d remain for the next message.)", len(items), extra)
	}
	// Distribution by level gives readers a glimpse of any serious items. Statistics only**This article actually covers**of
	// Entries, promise.[Serious 3]It's consistent with the number of entries below..
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

// writeItem Render a single loop entry.
//
// prefix Serial number used to summarize the list;single=true Render full version of the time (comprising summary and backlink)),
// Only one line summary in summary list——Otherwise 50 It's a long document..
//
// All external content (title)/Type/Assets/All over. markdownText:
// Single Line + Conversion. The chain is configured by the administrator. public_base_url It's not untrustworthy.,
// And it has to be a pointable link, so it's the same output..
func writeItem(b *strings.Builder, it Item, prefix string, single bool) {
	line := fmt.Sprintf("%s**%s · %s**", prefix, SeverityLabel(it.Severity), markdownText(it.Title(), 0))
	if !single {
		// Summary pattern: single-line presentation followed by compression of assets and summary.
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
		fmt.Fprintf(b, "**Status change**:%s → %s\n",
			markdownText(StatusLabel(it.FromStatus), 0), markdownText(StatusLabel(it.ToStatus), 0))
	}
	if it.VulnClass != "" && it.VulnClass != it.Title() {
		fmt.Fprintf(b, "**Type**:%s\n", markdownText(it.VulnClass, 0))
	}
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		fmt.Fprintf(b, "**Assets**:%s\n", markdownText(a, 0))
	}
	if it.Summary != "" {
		if s := markdownText(it.Summary, maxSummaryRunes); s != "" {
			fmt.Fprintf(b, "**Abstract**:%s\n", s)
		}
	}
	if it.DetailURL != "" {
		fmt.Fprintf(b, "[View details](%s)\n", it.DetailURL)
	}
}
