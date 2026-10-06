// Package notify is the adapter layer that pushes finding events to IM and email.
//
// Layering: this is a leaf package and depends only on the standard library.
// It knows nothing about the database or the server. Channel configuration
// arrives as map[string]any (the notification_channels.config JSONB column)
// and the payload arrives as a Message. Splitting it this way means signature
// calculation, UTF-8 truncation, and filter matching — the parts that are
// actually easy to get wrong — can be unit-tested without PostgreSQL. The
// host only has to orchestrate delivery on the server side.
//
// Concurrency: Channel implementations must be stateless. One Channel value
// is reused concurrently across many channel configs, including several bot
// instances of the same channel. Every credential comes in through the cfg
// argument. Do not cache a webhook URL, or anything like it, on the
// implementation itself.
package notify

// Channel kind identifiers. These values are also the allowed set of
// notification_channels.kind. The server-side whitelist checks them (same
// approach as findings.status: no DB CHECK, so a new channel does not need a
// migration).
const (
	KindDingTalk = "dingtalk" // DingTalk custom robot
	KindFeishu   = "feishu"   // Feishu (including Lark) custom robot
	KindWeCom    = "wecom"    // WeCom group robot
	KindWebhook  = "webhook"  // Generic webhook: custom method, headers, and JSON template
	KindTelegram = "telegram" // Telegram Bot API
	KindEmail    = "email"    // SMTP email
)

// Event kinds, stored as notification_events.kind.
const (
	EventFindingCreated       = "finding_created"
	EventFindingStatusChanged = "finding_status_changed"
)

// InitKind is the fallback when config leaves kind empty.
const InitKind = KindDingTalk

// severityRank maps a severity to a comparable rank. Unknown severities
// return 0, so any min_severity setting excludes them. When in doubt, do not
// push — that avoids flooding the channel with uncertain reports.
var severityRank = map[string]int{
	"low":      1,
	"medium":   2,
	"high":     3,
	"critical": 4,
}

// SeverityRank returns the rank of a severity. Unknown severities return 0.
func SeverityRank(severity string) int { return severityRank[severity] }

// SeverityLabel returns an emoji-prefixed severity label for message titles
// and card colors. An unknown severity is echoed unchanged; nothing is invented.
func SeverityLabel(severity string) string {
	switch severity {
	case "critical":
		return "🔴 Critical"
	case "high":
		return "🟠 High"
	case "medium":
		return "🟡 Medium"
	case "low":
		return "🔵 Low"
	default:
		return severity
	}
}

// StatusLabel returns a display label for a disposition status, used in
// status-change messages.
func StatusLabel(status string) string {
	switch status {
	case "pending":
		return "Pending"
	case "in_progress":
		return "Processing"
	case "confirmed":
		return "Confirmed"
	case "resolved":
		return "Resolved"
	case "fixed":
		return "Fixed"
	case "false_positive":
		return "False positive"
	case "ignored":
		return "Ignored"
	case "duplicate":
		return "Duplicate"
	case "risk_accepted":
		return "Risk accepted"
	default:
		return status
	}
}

// AtLeast reports whether severity meets the min threshold. An empty min
// means no threshold, so everything passes. An unknown severity has rank 0
// and is rejected by any non-empty min (see the severityRank comment).
func AtLeast(severity, min string) bool {
	if min == "" {
		return true
	}
	return SeverityRank(severity) >= SeverityRank(min)
}
