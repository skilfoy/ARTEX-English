package notify

import (
	"strings"
	"unicode/utf8"
)

const ellipsis = "…"

// TruncateBytes handle s No more than that. max Bytes, make sure it's legal. UTF-8 without cutting off characters.
//
// Why do you have to cut by character? markdown Yes 4096 **Bytes**Hard ceiling (no)
// Number of characters) and one word in Chinese 3 bytes. Direct bytes cut a Han in half. It's illegal.
// UTF-8——The side of the platform either refuses to receive the whole piece or displays a piece of muddle. Here's the way to start with the budget position.
// Back to the nearest. rune Start bytes(utf8.RuneStart Deciding bytes 0b10xxxxxx).
//
// max<=0 Express unrestricted. Additional ellipses after break unless max Small enough to fit the ellipsis..
func TruncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	budget := max - len(ellipsis)
	suffix := ellipsis
	if budget < 0 {
		// max Shorter than the ellipsis: drop the ellipsis, cut it out, avoid the result being more than max.
		budget = max
		suffix = ""
	}
	cut := budget
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

// OneLine Thrust multi-line text into one line: fold all blanks, then cut by character number.
// for IM Message Title Line——There's always a change of line in the summary, and it's plugged into the form./The title will break the layout..
// max<=0 Means unlimited length.
func OneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	return TruncateRunes(s, max)
}

// TruncateRunes handle s No more than that. max Characters (rather than bytes), add ellipses when exceeded.
// max<=0 Means unlimited.
//
// With TruncateBytes The difference is the platform caliber: microbytes. Long,Telegram Long by character.
// Use the wrong caliber is not a miscalculation. 1 Words = 3 Bytes,
// Bytes 4096 There's only one thing left. 1365 word) so both functions must be preserved and selected by channel.
func TruncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + ellipsis
}

// TruncateHTML Interrupt by character number HTML Snippets and guarantees not to produce semi-labels.
//
// Right. HTML The character cut will cut out `<a href="htt` This missing label, platform solver or...
// Wrongly rejected the whole article or swallowed the subsequent text as an attribute. Here's the thing: cut it off by character.,
// Check if there's any unclosed tail. `<`,If you have one, back it up..
//
// No tag leveling (completion) </b> Or something.):Telegram of HTML The solver automatically closes unclosed labels,
// And to achieve self-balancing is to deal with quotes, notes, self-conclusion labels in properties that are disproportionate to the benefits..
func TruncateHTML(s string, max int) string {
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	cut := TruncateRunes(s, max)
	// If the tail is... `<` The beginning debris. `<` Not since. `>`),Return `<` Before.
	if lt := strings.LastIndex(cut, "<"); lt >= 0 && !strings.Contains(cut[lt:], ">") {
		cut = cut[:lt]
	}
	// If the tail is cut, HTML Entities (e.g., `&amp;` Cut. `&amp`),I'm going back too..
	// Physical debris may be in an entity-only solver**The whole message.**Rejected——One more.
	// It's not worth losing the entire notice..
	if amp := strings.LastIndex(cut, "&"); amp >= 0 && !strings.Contains(cut[amp:], ";") {
		cut = cut[:amp]
	}
	return cut
}

// packItemCount Calculate within budget**Complete**How many of them are down for wrapping up the whole message?.
//
// Why cut the whole article instead of rendering the whole text: Cutting it off will make the second half disappear.,
// And their delivery records will still be marked as delivered.——I can't tell from the news. I can't tell from the past.,
// The loophole is gone. When the whole box is packed, the unfilled entry remains next in the library. Batch,
// The caller got it. kept It's the number of articles that actually deliver this message..
//
// Parameter:maxSize<=0 Means unlimited;reserve It's for the head./The amount left in the tail;
// size Responsible for measurement (different platforms: micro-enterprises)/Nail by Bytes,Telegram By character——
// Use the wrong caliber is not a miscalculation.);
// render Put the no. idx The bar is rendered into its actual text——The length varies from content to content..
//
// Return at least 1(As long as there are entries. It's the one to send out when it's extremely long.
// Eventually cut the hole, or a super-long loophole will lock the whole batch in place..
func packItemCount(items []Item, maxSize, reserve int, footer string, size func(string) int, render func(Item, int) string) int {
	if maxSize <= 0 {
		return len(items)
	}
	budget := maxSize - reserve - size(footer)
	if budget < 0 {
		budget = 0
	}
	used := 0
	for i, it := range items {
		used += size(render(it, i))
		if used > budget && i > 0 {
			return i
		}
	}
	return len(items)
}

// byteSize / runeSize Yes packItemCount Two calibrations. Name them.
// Naked. func(s string) int Close it, or it's hard to see what caliber it uses..
func byteSize(s string) int { return len(s) }
func runeSize(s string) int { return utf8.RuneCountInString(s) }

// assetLine Render the asset list as a line of display text, more than limit Delete the rest and indicate the total.
// A loophole could anchor dozens of assets, all listed for breaking news..
func assetLine(assets []string, limit int) string {
	if len(assets) == 0 {
		return ""
	}
	if limit <= 0 || len(assets) <= limit {
		return strings.Join(assets, ",")
	}
	return strings.Join(assets[:limit], ",") + " etc. " + itoa(len(assets)) + " pieces"
}

// itoa Yes strconv.Itoa The short aliases are used only to spell out text and avoid going around. import strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
