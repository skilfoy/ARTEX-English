package notify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateBytesKeepsValidUTF8(t *testing.T) {
	// This is the package's most important invariant. WeCom limits length in
	// bytes, and a non-ASCII character can be several bytes. Any
	// implementation that slices on a raw byte splits a character in half,
	// emits invalid UTF-8, and the platform rejects the message. Mixed
	// inputs of coprime lengths are used so every possible cut is hit.
	// Euro sign is a 3-byte rune, standing in for the multi-byte case
	// without using Han characters.
	inputs := []string{
		"€€€€ test",
		"mixed € content",
		"a€b€c€d€e",
		"🔴🟠🟡🔵", // 4-byte emoji; a bad cut is even more obvious
		strings.Repeat("€v", 100),
	}
	for _, in := range inputs {
		for max := 1; max <= len(in)+2; max++ {
			got := TruncateBytes(in, max)
			if !utf8.ValidString(got) {
				t.Fatalf("input %q max=%d: invalid UTF-8 %q", in, max, got)
			}
			if len(got) > max {
				t.Fatalf("input %q max=%d: result is %d bytes, over the cap", in, max, len(got))
			}
			// Content must not change when it was not truncated.
			if len(in) <= max && got != in {
				t.Fatalf("input %q max=%d: under the cap but content changed -> %q", in, max, got)
			}
		}
	}
}

func TestTruncateBytesZeroMeansUnlimited(t *testing.T) {
	long := strings.Repeat("x", 10000)
	if got := TruncateBytes(long, 0); got != long {
		t.Fatal("max=0 should mean no limit")
	}
	if got := TruncateBytes(long, -5); got != long {
		t.Fatal("max<0 should mean no limit")
	}
}

func TestTruncateBytesEllipsisBudget(t *testing.T) {
	// When max is shorter than the ellipsis itself, appending the ellipsis
	// must not push the result back over the cap.
	got := TruncateBytes("abcdefgh", 1)
	if len(got) > 1 {
		t.Fatalf("max=1: result %q has length %d, over the cap", got, len(got))
	}
	// The normal case should carry the ellipsis.
	if got := TruncateBytes("abcdefgh", 5); !strings.HasSuffix(got, ellipsis) {
		t.Fatalf("expected an ellipsis, got %q", got)
	}
}

func TestTruncateRunesCountsCharactersNotBytes(t *testing.T) {
	// The unit difference from TruncateBytes has to stay: Telegram limits
	// characters, and a byte limit would cut a multi-byte message down to
	// about a third.
	s := strings.Repeat("€", 10)
	got := TruncateRunes(s, 5)
	if n := utf8.RuneCountInString(got); n != 5 {
		t.Fatalf("want 5 characters, got %d (%q)", n, got)
	}
	// The same string measured in bytes should be clearly shorter.
	if utf8.RuneCountInString(TruncateBytes(s, 5)) >= 5 {
		t.Fatal("the byte unit must not yield the same character count as the rune unit")
	}
}

func TestOneLineCollapsesWhitespace(t *testing.T) {
	got := OneLine("first line\n\nsecond line\twith tabs   and spaces", 0)
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("should fold all whitespace, got %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Fatalf("should not keep runs of spaces, got %q", got)
	}
	// After truncation it must still be readable and valid.
	got = OneLine(strings.Repeat("€", 10), 4)
	if n := utf8.RuneCountInString(got); n != 4 {
		t.Fatalf("want 4 characters, got %d (%q)", n, got)
	}
}

func TestTruncateHTMLNeverCutsTagInHalf(t *testing.T) {
	// Cutting HTML directly can produce a fragment such as `<a href="htt`,
	// and the platform rejects the whole message.
	s := `<b>Title</b>body text<a href="https://example.com/very/long/path">View details</a>`
	for max := 1; max <= utf8.RuneCountInString(s)+2; max++ {
		got := TruncateHTML(s, max)
		if n := utf8.RuneCountInString(got); max > 0 && n > max {
			t.Fatalf("max=%d: result is %d characters, over the cap", max, n)
		}
		// The tail must not contain an unclosed `<` (a `<` in the last
		// stretch with no matching `>`).
		if lt := strings.LastIndex(got, "<"); lt >= 0 && !strings.Contains(got[lt:], ">") {
			t.Fatalf("max=%d: tail tag was cut -> %q", max, got)
		}
	}
}

func TestAssetLineOmitsExcess(t *testing.T) {
	if got := assetLine(nil, 3); got != "" {
		t.Fatalf("no assets should return an empty string, got %q", got)
	}
	if got := assetLine([]string{"a", "b"}, 3); got != "a, b" {
		t.Fatalf("under the limit should list everything, got %q", got)
	}
	// Past the limit the total must be stated, or the reader cannot tell how
	// many assets were left out.
	got := assetLine([]string{"a", "b", "c", "d", "e"}, 2)
	if !strings.Contains(got, "5 total") {
		t.Fatalf("should note the total of 5, got %q", got)
	}
}

func TestSeverityAndStatusLabels(t *testing.T) {
	if AtLeast("", "low") {
		t.Fatal("an empty severity has rank 0 and should be blocked by any threshold")
	}
	if !AtLeast("critical", "") {
		t.Fatal("an empty threshold should allow the event")
	}
	if got := StatusLabel("fixed"); got != "Fixed" {
		t.Fatalf("unexpected status label, got %q", got)
	}
	// An unknown status is echoed unchanged; no label is invented.
	if got := StatusLabel("weird_status"); got != "weird_status" {
		t.Fatalf("unknown status should be echoed unchanged, got %q", got)
	}
}

// TestTruncateHTMLNeverCutsEntity covers a gap the audit called out:
// truncation must avoid a cut HTML entity, not only a half-open tag.
//
// After `&` is cut down to `&amp`, a parser that only accepts entities
// may reject the whole message. Over-long digests are common, and the cost
// of dropping the entire notification is too high.
func TestTruncateHTMLNeverCutsEntity(t *testing.T) {
	s := "aaaa&amp;bbbb&lt;cccc&quot;dddd"
	for max := 1; max <= utf8.RuneCountInString(s)+2; max++ {
		got := TruncateHTML(s, max)
		// The tail must not be an entity fragment: an `&` with no matching `;`.
		if amp := strings.LastIndex(got, "&"); amp >= 0 && !strings.Contains(got[amp:], ";") {
			t.Fatalf("max=%d: tail left an entity fragment %q", max, got[amp:])
		}
		if strings.Contains(got, "&amp\x00") {
			t.Fatalf("max=%d: malformed entity", max)
		}
	}
}
