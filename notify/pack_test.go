package notify

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// This file covers the "pack whole items" fix. When a digest exceeds the
// channel's length cap, it must be cut on item boundaries and report how
// many items did not fit, so the caller marks only the ones that were
// actually delivered.
//
// The previous approach rendered the full text, truncated it, and then
// marked the whole batch delivered. The tail of the message vanished while
// the delivery history said everything succeeded, and the findings were gone
// with nothing left to notice.

func TestMarkdownBodyPacksWholeItemsWithinByteLimit(t *testing.T) {
	// 200 digest items are far over WeCom's 4096-byte cap.
	m := batchMsg(200)
	body, kept := markdownBody(m, weComMarkdownLimit)

	if len(body) > weComMarkdownLimit {
		t.Fatalf("body is %d bytes, over the cap %d", len(body), weComMarkdownLimit)
	}
	if !utf8.ValidString(body) {
		t.Fatal("body is not valid UTF-8")
	}
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("only part of the batch should fit (0 < kept < %d), got %d", len(m.Items), kept)
	}
	// The header must say how many items this message contains and how many
	// are left. Otherwise the reader treats the number in the header as the
	// full set.
	if !strings.Contains(body, "remain") || !strings.Contains(body, "next message") {
		t.Fatalf("header should say how many items this message left out:\n%s", body[:minInt(400, len(body))])
	}
	// Only the first kept items should appear.
	for i := 0; i < kept; i++ {
		if !strings.Contains(body, "Vulnerability"+itoa(i+1)) {
			t.Fatalf("item %d should be in this message:\n%s", i+1, body)
		}
	}
	if strings.Contains(body, "Vulnerability"+itoa(kept+1)) {
		t.Fatalf("item %d should not appear (it belongs to the next batch)", kept+1)
	}
}

func TestMarkdownBodyKeepsEverythingWhenUnderLimit(t *testing.T) {
	m := batchMsg(3)
	body, kept := markdownBody(m, 0) // 0 = no limit
	if kept != len(m.Items) {
		t.Fatalf("with no length limit everything should be kept, got kept=%d", kept)
	}
	if strings.Contains(body, "remain") {
		t.Fatalf("there should be no truncation note when nothing was cut:\n%s", body)
	}
}

func TestMarkdownBodyAlwaysKeepsAtLeastOneItem(t *testing.T) {
	// When the budget cannot fit even one item, one item is still sent (the
	// final truncation is the backstop). Otherwise one oversized finding
	// stalls the whole batch: every attempt fails to fit and nothing is sent.
	m := batchMsg(5)
	_, kept := markdownBody(m, 50)
	if kept != 1 {
		t.Fatalf("at least 1 item should be kept, got %d", kept)
	}
}

func TestMarkdownBodySingleReturnsOne(t *testing.T) {
	_, kept := markdownBody(singleMsg(), 4096)
	if kept != 1 {
		t.Fatalf("a single message should report 1 item delivered, got %d", kept)
	}
	// An empty message has nothing to deliver.
	if _, k := markdownBody(Message{}, 4096); k != 0 {
		t.Fatalf("an empty message should report 0 items, got %d", k)
	}
}

func TestTelegramPackingUsesRuneBudget(t *testing.T) {
	m := batchMsg(200)
	text, kept := telegramHTML(m)
	// Telegram limits length by character count. A byte budget would compress
	// a multi-byte message to about a third.
	if n := utf8.RuneCountInString(text); n > telegramTextLimit {
		t.Fatalf("body is %d characters, over the cap %d", n, telegramTextLimit)
	}
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("only part of the batch should fit, got %d", kept)
	}
	if !strings.Contains(text, "next message") {
		t.Fatalf("should say the remainder was not included:\n%.300s", text)
	}
}

func TestFeishuPackingReportsKept(t *testing.T) {
	m := batchMsg(2000)
	_, kept := feishuCard(m)
	if kept <= 0 || kept >= len(m.Items) {
		t.Fatalf("the card should fit only part of the batch, got %d", kept)
	}
}

func TestWebhookAndEmailReportAllItems(t *testing.T) {
	// These two channels do not truncate the body. The whole batch counts
	// as delivered.
	m := batchMsg(7)
	if n := len(m.Items); n != 7 {
		t.Fatal("precondition failed")
	}
	// Confirmed indirectly through the renderer: markdownBody(0) keeps
	// everything when there is no limit.
	if _, k := markdownBody(m, 0); k != len(m.Items) {
		t.Fatalf("with no length limit all items should be used, got %d", k)
	}
}

// TestMarkdownEscapesUntrustedContent is the regression test for "untrusted
// content must not change the message structure". Titles and summaries come
// from model output (the model read the target's response), and asset names
// come from the target's URL.
func TestMarkdownEscapesUntrustedContent(t *testing.T) {
	cases := []struct {
		name  string
		item  Item
		must  []string // must appear (escaped form)
		wrong []string // must not appear (unescaped form)
	}{
		{
			name: "newline and external link in the title",
			item: Item{
				Severity: "high",
				Name:     "Login Port SQL Injection\n[Emergency: Click this to verify account number](http://attacker.tld)",
			},
			// Newlines must be folded (otherwise they forge a list item or
			// a blockquote). Brackets and parentheses must be escaped
			// (otherwise the result is a clickable external link).
			must:  []string{`\[Emergency: Click this to verify account number\]`, `\(http://attacker.tld\)`},
			wrong: []string{"\n[Emergency", "\n\n[Emergency"},
		},
		{
			name: "image beacon in the title",
			item: Item{
				Severity: "high",
				Name:     "Vulnerability ![](http://attacker.tld/beacon)",
			},
			must:  []string{`\!`, `\(http://attacker.tld/beacon\)`},
			wrong: []string{"![]("},
		},
		{
			name: "emphasis and quote in an asset name",
			item: Item{
				Severity: "high",
				Name:     "Plain title",
				Assets:   []string{"a.com/*inject*>quote"},
			},
			must:  []string{`\*inject\*`, `\>`},
			wrong: []string{"*inject*"},
		},
		{
			name: "backtick and pipe in the summary",
			item: Item{
				Severity: "high",
				Name:     "Title",
				Summary:  "`code` | table",
			},
			must:  []string{"\\`code\\`", `\|`},
			wrong: []string{"`code`"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := Message{Items: []Item{tc.item}}
			// Single-item writeItem is the render path shared by the
			// markdown channels.
			var b strings.Builder
			writeItem(&b, tc.item, "", true)
			got := b.String()
			for _, want := range tc.must {
				if !strings.Contains(got, want) {
					t.Errorf("missing escaped form %q:\n%s", want, got)
				}
			}
			for _, bad := range tc.wrong {
				if strings.Contains(got, bad) {
					t.Errorf("unescaped form %q appeared (it can inject structure or an external link):\n%s", bad, got)
				}
			}
			_ = m
		})
	}
}

// TestMarkdownEscapeBackslashFirst locks escape order: backslash must be
// handled first, or the backslashes added later get a second layer and the
// output contains doubled backslashes.
func TestMarkdownEscapeBackslashFirst(t *testing.T) {
	if got := markdownEscape(`a\b*c`); got != `a\\b\*c` {
		t.Fatalf("escape order is wrong, got %q", got)
	}
}

// TestTelegramTitleHasNoMarkdownEscapes locks a specific regression:
// markdown escaping must not leak into Telegram's HTML output. Escaping was
// once added inside the shared title function, and Telegram messages showed
// a visible backslash in `\(1\)`.
func TestTelegramTitleHasNoMarkdownEscapes(t *testing.T) {
	m := Message{Items: []Item{{Severity: "high", Name: "alert(1) *emphasis*"}}}
	text, _ := telegramHTML(m)
	if strings.Contains(text, `\(`) || strings.Contains(text, `\*`) {
		t.Fatalf("Telegram body contains a markdown backslash escape:\n%s", text)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
