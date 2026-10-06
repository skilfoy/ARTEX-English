// Package notify renders and delivers finding events through configured channels.
package notify

// Channel type identification. The value is the same. notification_channels.kind ♪ Legitimate collection by ♪ server Side
// White List Validation findings.status Same thing. No need. DB CHECK,Facilitate follow-up channels).
const (
	KindDingTalk = "dingtalk" // Nailed a self-defined robot.
	KindFeishu   = "feishu"   // Feishu(incl. Lark)Custom Robot
	KindWeCom    = "wecom"    // Enterprise Wisdom Robot
	KindWebhook  = "webhook"  // General Webhook:Custom Method/head/JSON Templates
	KindTelegram = "telegram" // Telegram Bot API
	KindEmail    = "email"    // SMTP Mail
)

// Event type, corresponding notification_events.kind.
const (
	EventFindingCreated       = "finding_created"
	EventFindingStatusChanged = "finding_status_changed"
)

// InitKind Yes config It's empty. kind Bottom value.
const InitKind = KindDingTalk

// severityRank Maps the gap level to a comparable sequence. Unknown level returns 0,♪ So any ♪
// min_severity The settings keep the unknown out.——Don't push when there's doubt, and don't miss the screen..
var severityRank = map[string]int{
	"low":      1,
	"medium":   2,
	"high":     3,
	"critical": 4,
}

// SeverityRank return order of level;unknown level return 0.
func SeverityRank(severity string) int { return severityRank[severity] }

// SeverityLabel Return Belt emoji Other Organiser.
// Unknown level resembling, no assumptions.
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

// StatusLabel Translation of disposal status into Chinese for status change messages.
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

// AtLeast Decision severity Achieved min threshold.min No threshold for empty expression, all through..
// Note unknown severity The number of the sequences is 0,♪ Will be anything empty ♪ min Rejected. severityRank Comment).
func AtLeast(severity, min string) bool {
	if min == "" {
		return true
	}
	return SeverityRank(severity) >= SeverityRank(min)
}
