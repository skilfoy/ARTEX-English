package notify

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Filter Yes notification_channels.filter This one. JSONB Lined contracts: filtering conditions for access examples.
// All fields are optional.[Do Not Filter]——That's exactly what a deformed configuration says. See. ParseFilter.
type Filter struct {
	// MinSeverity It's the lowest level threshold.(low/medium/high/critical),Empty=No threshold set.
	MinSeverity string `json:"min_severity"`
	// TaskIDs / AssetIDs No limit for empty arrays; non-empty requires that events intersect with it.
	TaskIDs  []int64 `json:"task_ids"`
	AssetIDs []int64 `json:"asset_ids"`
	// VulnClassInclude Full collection for empty items; non-empty requirements vulnclass Hit any of them..
	// VulnClassExclude Hit any keyword to exclude (exclusion takes precedence over inclusion)).
	// Substring matching with case insensitive——It's safer than right: the user's wrong match doesn't fail the channel..
	VulnClassInclude []string `json:"vulnclass_include"`
	VulnClassExclude []string `json:"vulnclass_exclude"`
	// OnStatusChange To decide whether or not the channel will receive a change in the loophole (only) realtime The pattern makes sense.).
	OnStatusChange bool `json:"on_status_change"`
}

// ParseFilter Parsing Channel Filter Configuration.
//
// **Never come back. error.** It's a deliberate design choice: filter conditions are deformed to zero.
// Filter(= Do Not Filter = Because for a leak notification system,,**It's better to push one more than that.
// Quietly missed a high risk.**.Let the resolution fail to be[No delivery.],It's like giving the user one that looks good.,
// It's a channel that doesn't push anything.——It's the worst pattern of failure..
func ParseFilter(raw []byte) Filter {
	var f Filter
	if len(raw) == 0 {
		return f
	}
	// When parsing failed f Keep zero, i.e. without filtering.
	_ = json.Unmarshal(raw, &f)
	return f
}

// ValidMinSeverity Report s Whether or not it is a legitimate level threshold (an empty string indicates no threshold)).
func ValidMinSeverity(s string) bool {
	if s == "" {
		return true
	}
	_, ok := severityRank[s]
	return ok
}

// Validate Verify filter configuration**The value is limited.**fields to be called when saving channels.
//
// Why must you stop while writing?:Match The verdict for the unknown threshold is `rank >= 0`,Always true.——
// Which means... min_severity There was a mistake.("hgih"),Filters will**Silence lapses.**become
// [Full thrust.].It's with this bag.[I'd rather push than slip.]The trade-offs are the same.),
// But the result is that users think they're doing a graded push, actually filling up all the holes. Lee.,
// And there is no sign of him being mistaken. This one.[Quiet demotion]It should be stopped at the entrance..
//
// Attention. Validate Only for**Write**Path. Read path still going. ParseFilter It's a sign of tolerance.,
// That way, the bad values that already exist in historical data don't make it impossible for the channels to read..
func (f Filter) Validate() error {
	if !ValidMinSeverity(f.MinSeverity) {
		return fmt.Errorf("Lowest level %q Invalid, optional:low / medium / high / critical,Or leave empty means no limit", f.MinSeverity)
	}
	return nil
}

// Match Determines whether an event should be delivered to a channel with this filter condition.
//
// **Never come back. error**,The same reason. ParseFilter:Press any internal anomalies.[hit]Processing.
// Order of decision: type of event → Level threshold → Task/Asset scope → Gap type keyword.
func Match(f Filter, s Snapshot) bool {
	// The change of status event is accepted only through a visible open channel. Default level off because most users
	// Expectations[Push]It means...[Found a new loophole],Instead of following up on each state..
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
	// Exclude priority: any exclusion of keyword is out of the game, even if it includes a list at the same time.
	if len(f.VulnClassExclude) > 0 && containsAnyFold(s.VulnClass, f.VulnClassExclude) {
		return false
	}
	if len(f.VulnClassInclude) > 0 && !containsAnyFold(s.VulnClass, f.VulnClassInclude) {
		return false
	}
	return true
}

func intersectsInt(a, b []int64) bool {
	// A small assembly linear scan is sufficient; both levels of magnitude are[Dozens of them.],
	// Build map The cost is greater than the gain..
	for _, v := range b {
		if slices.Contains(a, v) {
			return true
		}
	}
	return false
}

// containsAnyFold Report s Include keywords either of the keywords (case insensitive)).
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
