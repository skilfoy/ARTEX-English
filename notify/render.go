package notify

import (
	"strings"
	"unicode/utf8"
)

const ellipsis = "…"

// TruncateBytes cuts s so it is at most max bytes, stays valid UTF-8, and
// does not split a character.
//
// The cut has to land on a character boundary. WeCom group-robot markdown
// has a hard cap of 4096 bytes, not characters, and one Chinese character is
// 3 bytes. Slicing on a raw byte splits a character in half and produces
// invalid UTF-8. The platform then either rejects the whole message or shows
// a replacement box. The cut walks back from the budget to the nearest rune
// start (utf8.RuneStart rejects continuation bytes, 0b10xxxxxx).
//
// max <= 0 means no limit. An ellipsis is appended after a cut, unless max
// is too small to hold the ellipsis.
func TruncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	budget := max - len(ellipsis)
	suffix := ellipsis
	if budget < 0 {
		// max is shorter than the ellipsis. Drop the ellipsis and cut
		// cleanly, so the result does not exceed max because of it.
		budget = max
		suffix = ""
	}
	cut := budget
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}

// OneLine collapses multiline text to a single line: all whitespace is
// folded, then the result is cut by character count. It is used for IM title
// lines. Summaries often contain newlines, and dropping those into a table
// or a title breaks the layout. max <= 0 means no length limit.
func OneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	return TruncateRunes(s, max)
}

// TruncateRunes cuts s to at most max characters (not bytes) and appends an
// ellipsis when it overflows. max <= 0 means no limit.
//
// The difference from TruncateBytes is the platform's unit. WeCom limits
// bytes; Telegram limits characters. Using the wrong unit does not error, it
// just cuts a message far shorter than intended (one Chinese character is 3
// bytes, so a 4096-byte cut leaves about 1365 characters). Both functions
// have to stay, and each channel picks the one that matches its cap.
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

// TruncateHTML cuts an HTML fragment by character count without leaving a
// half-open tag.
//
// Cutting HTML by characters can produce a fragment such as `<a href="htt`.
// The platform parser either rejects the whole message or swallows the rest
// of the body as an attribute value. The cut is by characters first, then if
// the tail has an unclosed `<` it backs up to just before that `<`.
//
// Tags are not rebalanced (no synthetic </b> and similar). Telegram's HTML
// parser closes unclosed tags itself. Doing the balancing here means handling
// quotes inside attributes, comments, and void tags, which costs more than
// it saves.
func TruncateHTML(s string, max int) string {
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	cut := TruncateRunes(s, max)
	// If the tail is a fragment that starts with `<` and never reaches `>`,
	// back up to before that `<`.
	if lt := strings.LastIndex(cut, "<"); lt >= 0 && !strings.Contains(cut[lt:], ">") {
		cut = cut[:lt]
	}
	// A cut HTML entity (for example `&` cut down to `&amp`) is backed
	// out the same way. A parser that only accepts entities may reject the
	// whole message because of the fragment. Digests that exceed the length
	// cap are common, and losing the entire notification over that is not
	// worth it.
	if amp := strings.LastIndex(cut, "&"); amp >= 0 && !strings.Contains(cut[amp:], ";") {
		cut = cut[:amp]
	}
	return cut
}

// packItemCount counts how many items fit completely inside the budget, so a
// digest is packed as whole items.
//
// Packing whole items, instead of rendering the full text and then cutting,
// matters because a cut makes the leftover items disappear while their
// delivery records are still marked delivered. They are not in the message
// and they are not visible as failures in the history, so the findings are
// simply gone. Whole-item packing leaves what did not fit for the next
// batch, and kept is how many this message actually delivered.
//
// maxSize <= 0 means no limit. reserve is set aside for the header and
// footer. size measures length, and the unit differs by platform: WeCom and
// DingTalk count bytes, Telegram counts characters. The wrong unit does not
// error; it just compresses Chinese text far below the real cap. render
// turns item idx into its actual text. Length depends on the content, so it
// cannot be estimated.
//
// At least 1 is returned when there are items. A single item that is
// extremely long is still sent, and the caller's final truncation is the
// backstop. Otherwise one oversized finding would stall the whole batch
// forever.
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

// byteSize and runeSize are the two units packItemCount can measure with.
// Naming them avoids a bare func(s string) int closure at the call site,
// where it is hard to see which unit is in use.
func byteSize(s string) int { return len(s) }
func runeSize(s string) int { return utf8.RuneCountInString(s) }

// assetLine renders an asset list as one line of display text. Past limit,
// the rest are omitted and the total count is noted. A finding can be
// anchored to dozens of assets, and listing all of them blows the message up.
func assetLine(assets []string, limit int) string {
	if len(assets) == 0 {
		return ""
	}
	if limit <= 0 || len(assets) <= limit {
		return strings.Join(assets, ", ")
	}
	return strings.Join(assets[:limit], ", ") + " (" + itoa(len(assets)) + " total)"
}

// itoa is a short alias for the decimal form of n, used only when building
// display text, so this file does not need to import strconv.
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
