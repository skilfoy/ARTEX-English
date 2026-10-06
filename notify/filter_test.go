package notify

import "testing"

func TestParseFilterMalformedFallsBackToMatchAll(t *testing.T) {
	// Malformed JSON, empty input, and fields of the wrong type must all
	// fall back to the zero Filter, which means "do not filter". That
	// invariant is where "rather send extra than miss one" lands: if this
	// started returning an error or a partial parse, one bad character would
	// silently drop every critical notification.
	cases := []struct {
		name string
		raw  string
	}{
		{"empty input", ""},
		{"invalid JSON", `{not json`},
		{"truncated JSON", `{"min_severity":`},
		{"type mismatch", `{"min_severity": 123, "task_ids": "abc"}`},
		{"top-level array", `[1,2,3]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := ParseFilter([]byte(tc.raw))
			if f.MinSeverity != "" || len(f.TaskIDs) != 0 || len(f.AssetIDs) != 0 {
				t.Fatalf("malformed config should fall back to the zero Filter, got %+v", f)
			}
			// The zero Filter must match any event.
			ev := Snapshot{Kind: EventFindingCreated, Severity: "low", VulnClass: "XSS"}
			if !Match(f, ev) {
				t.Fatal("zero Filter should match every event")
			}
		})
	}
}

func TestMatchSeverityThreshold(t *testing.T) {
	ev := func(sev string) Snapshot {
		return Snapshot{Kind: EventFindingCreated, Severity: sev}
	}
	cases := []struct {
		min    string
		sev    string
		expect bool
	}{
		{"", "low", true},
		{"", "critical", true},
		{"high", "critical", true},
		{"high", "high", true},
		{"high", "medium", false},
		{"high", "low", false},
		{"critical", "high", false},
		{"critical", "critical", true},
		// Unknown severities have rank 0 and are blocked by any non-empty
		// threshold (do not push when in doubt).
		{"low", "", false},
		{"low", "unknown", false},
		{"", "", true},
	}
	for _, tc := range cases {
		got := Match(Filter{MinSeverity: tc.min}, ev(tc.sev))
		if got != tc.expect {
			t.Errorf("min=%q sev=%q: want %v, got %v", tc.min, tc.sev, tc.expect, got)
		}
	}
}

func TestMatchScopeRestrictions(t *testing.T) {
	ev := Snapshot{
		Kind:      EventFindingCreated,
		Severity:  "high",
		TaskID:    7,
		AssetIDs:  []int64{10, 20},
		VulnClass: "SQLInjection",
	}
	cases := []struct {
		name   string
		filter Filter
		expect bool
	}{
		{"empty scope means no limit", Filter{}, true},
		{"task matches", Filter{TaskIDs: []int64{7}}, true},
		{"task misses", Filter{TaskIDs: []int64{8}}, false},
		{"task list contains the match", Filter{TaskIDs: []int64{8, 7}}, true},
		{"assets intersect", Filter{AssetIDs: []int64{20, 99}}, true},
		{"assets do not intersect", Filter{AssetIDs: []int64{99}}, false},
		{"task and asset both match", Filter{TaskIDs: []int64{7}, AssetIDs: []int64{10}}, true},
		{"task matches but asset misses", Filter{TaskIDs: []int64{7}, AssetIDs: []int64{99}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Match(tc.filter, ev); got != tc.expect {
				t.Errorf("want %v, got %v", tc.expect, got)
			}
		})
	}
}

func TestMatchVulnClassKeywords(t *testing.T) {
	ev := func(class string) Snapshot {
		return Snapshot{Kind: EventFindingCreated, Severity: "high", VulnClass: class}
	}
	cases := []struct {
		name   string
		filter Filter
		class  string
		expect bool
	}{
		{"empty include accepts all", Filter{}, "any class", true},
		{"include matches", Filter{VulnClassInclude: []string{"SQL"}}, "SQLInjection", true},
		{"include misses", Filter{VulnClassInclude: []string{"command execution"}}, "SQLInjection", false},
		{"include matches any keyword", Filter{VulnClassInclude: []string{"command execution", "SQL"}}, "SQLInjection", true},
		{"case insensitive", Filter{VulnClassInclude: []string{"sql"}}, "SQLInjection", true},
		{"exclude match drops the event", Filter{VulnClassExclude: []string{"information leak"}}, "information leak", false},
		{"exclude miss allows the event", Filter{VulnClassExclude: []string{"information leak"}}, "SQLInjection", true},
		// Exclude wins over include: a hit on both drops the event.
		{"exclude wins over include", Filter{
			VulnClassInclude: []string{"SQL"},
			VulnClassExclude: []string{"Injection"},
		}, "SQLInjection", false},
		// Blank keywords are ignored. Otherwise they degenerate into
		// "match every string that contains a space".
		{"blank keywords are ignored", Filter{VulnClassInclude: []string{"", "  "}}, "SQLInjection", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Match(tc.filter, ev(tc.class)); got != tc.expect {
				t.Errorf("want %v, got %v", tc.expect, got)
			}
		})
	}
}

func TestMatchStatusChangeRequiresOptIn(t *testing.T) {
	ev := Snapshot{Kind: EventFindingStatusChanged, Severity: "critical", FromStatus: "pending", ToStatus: "fixed"}
	// Default is off: "push findings" means a new finding, not a running
	// log of status changes.
	if Match(Filter{MinSeverity: "low"}, ev) {
		t.Fatal("a status-change event should be skipped when not enabled")
	}
	if !Match(Filter{OnStatusChange: true}, ev) {
		t.Fatal("a status-change event should match once on_status_change is enabled")
	}
	// A create event is not affected by on_status_change.
	created := Snapshot{Kind: EventFindingCreated, Severity: "critical"}
	if !Match(Filter{MinSeverity: "low"}, created) {
		t.Fatal("a create event must not depend on on_status_change")
	}
}
