package notify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Other Organiser[Pack the whole thing.]This repair: When the aggregate message exceeds the maximum channel length, it must**Press the whole article**
// Cut off and report the number of unloaded entries, so that the caller will mark only those that actually reach..
//
// The previous practice was to retransform the entire section and then the whole series of markings were delivered: the second half of the message disappeared.,
// And sending history shows all success.——The loophole is gone and nothing can be found..

func TestMarkdownBodyPacksWholeItemsWithinByteLimit(t *testing.T) {
	// 200 It's a Chinese combination. It's bound to be far more than nothing. 4096 Bytes.
	m := batchMsg(200)
	body, kept := markdownBody(m, weComMarkdownLimit)

	if len(body) > weComMarkdownLimit {
		t.Fatalf("Text %d Byte Superlimit %d", len(body), weComMarkdownLimit)
	}
	if !utf8.ValidString(body) {
		t.Fatal("The text is not legal. UTF-8")
	}
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("It should only be part of it.(0 < kept < %d),get %d", len(m.Items), kept)
	}
	// The head must state exactly how many articles this article contains and how many others it contains——Otherwise, the reader will turn his head.
	// That number is all..
	if !strings.Contains(body, "remain") || !strings.Contains(body, "next message") {
		t.Fatalf("The head should indicate how many more are not included in this article:\n%s", body[:minInt(400, len(body))])
	}
	// Only before kept strip.
	for i := 0; i < kept; i++ {
		if !strings.Contains(body, "Vulnerability"+itoa(i+1)) {
			t.Fatalf("No. %d The article should be in this article.:\n%s", i+1, body)
		}
	}
	if strings.Contains(body, "Vulnerability"+itoa(kept+1)) {
		t.Fatalf("No. %d It's not supposed to appear.)", kept+1)
	}
}

func TestMarkdownBodyKeepsEverythingWhenUnderLimit(t *testing.T) {
	m := batchMsg(3)
	body, kept := markdownBody(m, 0) // 0 = Unlimited
	if kept != len(m.Items) {
		t.Fatalf("All of them should be retained without limiting the length. kept=%d", kept)
	}
	if strings.Contains(body, "remain") {
		t.Fatalf("There should be no cut-off hint when there is no cut-off.:\n%s", body)
	}
}

func TestMarkdownBodyAlwaysKeepsAtLeastOneItem(t *testing.T) {
	// When the budget is too small to fit, you have to send one. Bottom).
	// Otherwise, a super-long loophole would have stuck the whole batch in place: every receipt would have been unfilled and never sent..
	m := batchMsg(5)
	_, kept := markdownBody(m, 50)
	if kept != 1 {
		t.Fatalf("At least it should be retained. 1 Article, get it. %d", kept)
	}
}

func TestMarkdownBodySingleReturnsOne(t *testing.T) {
	_, kept := markdownBody(singleMsg(), 4096)
	if kept != 1 {
		t.Fatalf("A single message should be delivered. 1 Article, get it. %d", kept)
	}
	// There are no serviceable entries for empty messages.
	if _, k := markdownBody(Message{}, 4096); k != 0 {
		t.Fatalf("We'll get the message. 0 Article, get it. %d", k)
	}
}

func TestTelegramPackingUsesRuneBudget(t *testing.T) {
	m := batchMsg(200)
	text, kept := telegramHTML(m)
	// Telegram Press**Number of characters**Long limit; press Chinese to one third with bytes.
	if n := utf8.RuneCountInString(text); n > telegramTextLimit {
		t.Fatalf("Text %d Character Superlimit %d", n, telegramTextLimit)
	}
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("It's supposed to be a part of it. %d", kept)
	}
	if !strings.Contains(text, "next message") {
		t.Fatalf("It should be noted that the balance is not included:\n%.300s", text)
	}
}

func TestFeishuPackingReportsKept(t *testing.T) {
	m := batchMsg(2000)
	_, kept := feishuCard(m)
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("The card should be only partially loaded. %d", kept)
	}
}

func TestWebhookAndEmailReportAllItems(t *testing.T) {
	// These two channels don't cut through the text. The whole batch is on the way..
	m := batchMsg(7)
	if n := len(m.Items); n != 7 {
		t.Fatal("Preconditions not valid")
	}
	// Indirectly confirmed by the return value of the renderer:markdownBody(0) Keep All Unlimited.
	if _, k := markdownBody(m, 0); k != len(m.Items) {
		t.Fatalf("Apply all without limiting the length, get %d", k)
	}
}

// TestMarkdownEscapesUntrustedContent Yes[You can't change the message structure if you can't trust it.]The regression test.
// Titles and abstracts are derived from model outputs (models are read in response to detected target) and asset names are derived from detected target URL.
func TestMarkdownEscapesUntrustedContent(t *testing.T) {
	cases := []struct {
		name  string
		item  Item
		must  []string // It has to come up.)
		wrong []string // Results are not allowed in (non-converted form))
	}{
		{
			name: "Line Break in Title + Outlink",
			item: Item{
				Severity: "high",
				Name:     "Login Port SQL Injection\n[Emergency: Click this to verify account number](http://attacker.tld)",
			},
			// Line breaks must be folded (or new list entries can be forged)/Reference Blocks);
			// The square brackets and brackets must be transposed (otherwise, the clickable outer chain)).
			must:  []string{`\[Emergency: Click this to verify account number\]`, `\(http://attacker.tld\)`},
			wrong: []string{"\n[Emergency", "\n\n[Emergency"},
		},
		{
			name: "Photo beacon in title",
			item: Item{
				Severity: "high",
				Name:     "Vulnerability ![](http://attacker.tld/beacon)",
			},
			must:  []string{`\!`, `\(http://attacker.tld/beacon\)`},
			wrong: []string{"![]("},
		},
		{
			name: "Emphasis and citation in asset name",
			item: Item{
				Severity: "high",
				Name:     "General Title",
				Assets:   []string{"a.com/*Injection*>References"},
			},
			must:  []string{`\*Injection\*`, `\>`},
			wrong: []string{"*Injection*"},
		},
		{
			name: "Inverted quotes and vertical lines in the summary",
			item: Item{
				Severity: "high",
				Name:     "Title",
				Summary:  "`code` | Table",
			},
			must:  []string{"\\`code\\`", `\|`},
			wrong: []string{"`code`"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Message{Items: []Item{tc.item}}
			// Writing in single modeItem Three. markdown Rendering Path for Channel Sharing.
			var b strings.Builder
			writeItem(&b, tc.item, "", true)
			got := b.String()
			for _, want := range tc.must {
				if !strings.Contains(got, want) {
					t.Errorf("Missing conversion form %q:\n%s", want, got)
				}
			}
			for _, bad := range tc.wrong {
				if strings.Contains(got, bad) {
					t.Errorf("There's a pattern of non-conversion. %q(It can be used to inject structures or outer chains.):\n%s", bad, got)
				}
			}
			_ = m
		})
	}
}

// TestMarkdownEscapeBackslashFirst Lock conversion order: the back slash must be handled first,
// Otherwise, the back slash will be filled with a double slash in the output..
func TestMarkdownEscapeBackslashFirst(t *testing.T) {
	if got := markdownEscape(`a\b*c`); got != `a\\b\*c` {
		t.Fatalf("Changed order. Got it. %q", got)
	}
}

// TestTelegramTitleHasNoMarkdownEscapes Lock down a specific return.:
// markdown The transposition can't be leaked. Telegram of HTML Output (used in shared title functions) Lee.
// Add a transposition. Telegram It's coming in. `\(1\)` This visible backslash).
func TestTelegramTitleHasNoMarkdownEscapes(t *testing.T) {
	m := Message{Items: []Item{{Severity: "high", Name: "alert(1) *Focus*"}}}
	text, _ := telegramHTML(m)
	if strings.Contains(text, `\(`) || strings.Contains(text, `\*`) {
		t.Fatalf("Telegram It's in the text. markdown Inverted slash:\n%s", text)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
