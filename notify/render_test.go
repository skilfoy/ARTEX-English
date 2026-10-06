package notify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateBytesKeepsValidUTF8(t *testing.T) {
	// This is the most important variable of this package. Micropress**Bytes**Long, Chinese 3 Bytes/Words,
	// Any byte hard-cut realization will turn Han into half a piece. UTF-8 They were rejected by the platform..
	// Use multiple Chinese-British rows of length to hit every possible cut..
	inputs := []string{
		"Chinese Test Contents",
		"Mixed mixed Content content",
		"aΩbMancMeasuredTrye",
		"🔴🟠🟡🔵", // 4 Bytes emoji,The error is more obvious.
		strings.Repeat("Vulnerability", 100),
	}
	for _, in := range inputs {
		for max := 1; max <= len(in)+2; max++ {
			got := TruncateBytes(in, max)
			if !utf8.ValidString(got) {
				t.Fatalf("Input %q max=%d: Illegal output UTF-8 %q", in, max, got)
			}
			if len(got) > max {
				t.Fatalf("Input %q max=%d: Results %d Bytes in excess of ceiling", in, max, len(got))
			}
			// No change in content if not interrupted.
			if len(in) <= max && got != in {
				t.Fatalf("Input %q max=%d: I changed the content without exceeding the limit. -> %q", in, max, got)
			}
		}
	}
}

func TestTruncateBytesZeroMeansUnlimited(t *testing.T) {
	long := strings.Repeat("x", 10000)
	if got := TruncateBytes(long, 0); got != long {
		t.Fatal("max=0 It should be unrestricted.")
	}
	if got := TruncateBytes(long, -5); got != long {
		t.Fatal("max<0 It should be unrestricted.")
	}
}

func TestTruncateBytesEllipsisBudget(t *testing.T) {
	// max When less than the ellipsis itself, the additional ellipsis cannot be exceeded.
	got := TruncateBytes("abcdefgh", 1)
	if len(got) > 1 {
		t.Fatalf("max=1 Time result %q Length %d Overlimit", got, len(got))
	}
	// Normality should have ellipses..
	if got := TruncateBytes("abcdefgh", 5); !strings.HasSuffix(got, ellipsis) {
		t.Fatalf("Expecting ellipsis, get it. %q", got)
	}
}

func TestTruncateRunesCountsCharactersNotBytes(t *testing.T) {
	// With TruncateBytes The difference in calibre must be maintained:Telegram Long by Character,
	// With a byte caliber, the Chinese message is only a third..
	s := "One, two, three, four, five, six, seven, eight, ninety."
	got := TruncateRunes(s, 5)
	if n := utf8.RuneCountInString(got); n != 5 {
		t.Fatalf("Expectations 5 Character, get %d pieces (%q)", n, got)
	}
	// The same string should be significantly shorter by bytes.
	if utf8.RuneCountInString(TruncateBytes(s, 5)) >= 5 {
		t.Fatal("Byte caliber should not produce the same number of characters as the caliber")
	}
}

func TestOneLineCollapsesWhitespace(t *testing.T) {
	got := OneLine("First line\n\nSecond line\tTables with multispaces", 0)
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("Should fold all the blanks, get %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Fatalf("Should not keep continuous spaces, get %q", got)
	}
	// It's still readable and legal..
	got = OneLine("One, two, three, four, five, six, seven, eight, ninety.", 4)
	if n := utf8.RuneCountInString(got); n != 4 {
		t.Fatalf("Expectations 4 Character, get %d (%q)", n, got)
	}
}

func TestTruncateHTMLNeverCutsTagInHalf(t *testing.T) {
	// Direct Cut HTML It'll cut out. `<a href="htt` This kind of thing, the platform won't get the whole message..
	s := `<b>Title</b>Body text<a href="https://example.com/very/long/path">View details</a>`
	for max := 1; max <= utf8.RuneCountInString(s)+2; max++ {
		got := TruncateHTML(s, max)
		if n := utf8.RuneCountInString(got); max > 0 && n > max {
			t.Fatalf("max=%d: Results %d Character limit", max, n)
		}
		// There can't be any loose ends. `<`(In the last paragraph. `<` But none. `>`).
		if lt := strings.LastIndex(got, "<"); lt >= 0 && !strings.Contains(got[lt:], ">") {
			t.Fatalf("max=%d: The tail tag is cut. -> %q", max, got)
		}
	}
}

func TestAssetLineOmitsExcess(t *testing.T) {
	if got := assetLine(nil, 3); got != "" {
		t.Fatalf("No assets should be returned to empty strings. %q", got)
	}
	if got := assetLine([]string{"a", "b"}, 3); got != "a,b" {
		t.Fatalf("Not exceeding full line. %q", got)
	}
	// If the ceiling is exceeded, the total must be indicated, otherwise the reader does not know how many assets remain unlisted..
	got := assetLine([]string{"a", "b", "c", "d", "e"}, 2)
	if !strings.Contains(got, "etc. 5 pieces") {
		t.Fatalf("Total number to be indicated 5,get %q", got)
	}
}

func TestSeverityAndStatusLabels(t *testing.T) {
	if AtLeast("", "low") {
		t.Fatal("Empty Order is 0,Should be blocked by any threshold.")
	}
	if !AtLeast("critical", "") {
		t.Fatal("The air threshold should be released.")
	}
	if got := StatusLabel("fixed"); got != "Fixed" {
		t.Fatalf("Unknown status map, received %q", got)
	}
	// Unknown state resembling as it is, no labels assumed.
	if got := StatusLabel("weird_status"); got != "weird_status" {
		t.Fatalf("Unknown state should be re-stated. %q", got)
	}
}

// TestTruncateHTMLNeverCutsEntity Override one of the omissions noted by the audit: cut-off is not only avoided
// It's a semi-label. HTML Entities.
//
// `&amp;` Cut. `&amp` After that, an entity-only solver might refuse to accept it.**Article as a whole**Message——
// And it's common to aggregate information for too long..
func TestTruncateHTMLNeverCutsEntity(t *testing.T) {
	s := "aaaa&amp;bbbb&lt;cccc&quot;dddd"
	for max := 1; max <= utf8.RuneCountInString(s)+2; max++ {
		got := TruncateHTML(s, max)
		// No tails.[Yes & But it doesn't match. ;]It's a physical piece..
		if amp := strings.LastIndex(got, "&"); amp >= 0 && !strings.Contains(got[amp:], ";") {
			t.Fatalf("max=%d: The tail leaves a physical piece. %q", max, got[amp:])
		}
		if strings.Contains(got, "&amp\x00") {
			t.Fatalf("max=%d: Deformed entity", max)
		}
	}
}
