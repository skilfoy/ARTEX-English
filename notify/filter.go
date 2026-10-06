package notify

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Filter is the contract for the notification_channels.filter JSONB column:
// the filter on one channel instance. Every field is optional. Omitted
// fields mean "do not filter", which is also the fallback for a malformed
// config. See ParseFilter.
type Filter struct {
	// MinSeverity is the minimum severity (low, medium, high, or critical).
	// Empty means no threshold.
	MinSeverity string `json:"min_severity"`
	// Empty TaskIDs or AssetIDs means no restriction. A non-empty list
	// requires the event to intersect it.
	TaskIDs  []int64 `json:"task_ids"`
	AssetIDs []int64 `json:"asset_ids"`
	// An empty VulnClassInclude accepts every class. Otherwise vulnclass
	// must contain one of the keywords.
	// VulnClassExclude drops the event when any keyword matches. Exclude
	// wins over include.
	// Matching is a case-insensitive substring, which is safer than a
	// regexp: a bad pattern cannot silently disable the channel.
	VulnClassInclude []string `json:"vulnclass_include"`
	VulnClassExclude []string `json:"vulnclass_exclude"`
	// OnStatusChange selects whether this channel receives finding
	// status-change events. It only matters in realtime mode.
	OnStatusChange bool `json:"on_status_change"`
}

// ParseFilter parses a channel filter.
//
// It never returns an error. That is deliberate: a malformed filter falls
// back to the zero Filter, which filters nothing and therefore matches
// everything. For a finding-notification system, sending one extra message
// is far better than silently dropping a critical one. Treating a parse
// failure as "do not send" would leave the user with a channel that looks
// configured and pushes nothing — the worst failure mode.
func ParseFilter(raw []byte) Filter {
	var f Filter
	if len(raw) == 0 {
		return f
	}
	// On parse failure f stays at zero, which means no filtering.
	_ = json.Unmarshal(raw, &f)
	return f
}

// ValidMinSeverity reports whether s is a legal severity threshold.
// An empty string means no threshold.
func ValidMinSeverity(s string) bool {
	if s == "" {
		return true
	}
	_, ok := severityRank[s]
	return ok
}

// Validate checks the fields whose values are constrained, and is called
// when a channel is saved.
//
// It has to run on write because Match treats an unknown threshold as
// `rank >= 0`, which is always true. A one-character typo in min_severity
// ("hgih") would silently disable the filter and push everything. That
// matches this package's "rather send extra than miss one" choice (nothing
// is dropped), but the user would believe they had severity routing while
// every finding landed in the chat, with no sign that the config was wrong.
// That kind of silent downgrade should be rejected at the door.
//
// Validate is only for the write path. Reads still use ParseFilter's
// tolerant behavior, so a bad value already stored in history does not make
// the whole channel unreadable.
func (f Filter) Validate() error {
	if !ValidMinSeverity(f.MinSeverity) {
		return fmt.Errorf("minimum severity %q is invalid; allowed: low / medium / high / critical, or leave empty for no limit", f.MinSeverity)
	}
	return nil
}

// Match reports whether an event should be delivered to a channel with this
// filter.
//
// It never returns an error, for the same reason as ParseFilter: any
// internal problem is treated as a match. Order: event kind, severity
// threshold, task/asset scope, then vulnclass keywords.
func Match(f Filter, s Snapshot) bool {
	// Status changes are delivered only when the channel opts in. The
	// default is off because most people mean "a new finding" by "push",
	// not a running log of every status transition.
	if s.Kind == EventFindingStatusChanged && !f.OnStatusChange {
		return false
	}
	if !AtLeast(s.Severity, f.MinSeverity) {
		return false
	}
	if len(f.TaskIDs) > 0 && !slices.Contains(f.TaskIDs, s.TaskID) {
		return false
	}
	if len(f.AssetIDs) > 0 && !intersectsInt(f.AssetIDs, s.AssetIDs) {
		return false
	}
	// Exclude wins: any excluded keyword drops the event, even if an
	// include keyword also matches.
	if len(f.VulnClassExclude) > 0 && containsAnyFold(s.VulnClass, f.VulnClassExclude) {
		return false
	}
	if len(f.VulnClassInclude) > 0 && !containsAnyFold(s.VulnClass, f.VulnClassInclude) {
		return false
	}
	return true
}

func intersectsInt(a, b []int64) bool {
	// A linear scan is enough. Both sides are a few dozen ids someone
	// checked by hand, and building a map would cost more than it saves.
	for _, v := range b {
		if slices.Contains(a, v) {
			return true
		}
	}
	return false
}

// containsAnyFold reports whether s contains any keyword, case-insensitively.
func containsAnyFold(s string, keywords []string) bool {
	lower := strings.ToLower(s)
	for _, kw := range keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw != "" && strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
