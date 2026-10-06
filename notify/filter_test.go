package notify

import "testing"

func TestParseFilterMalformedFallsBackToMatchAll(t *testing.T) {
	// Malformed JSON,Empty input, invalid field——All must be degraded to zero. Filter,
	// i.e.[Do Not Filter].This is not a variable.[I'd rather push than slip.]Places:
	// Once this is changed to a false or semi-resolved character, the user dies silently with all the high-risk notifications..
	cases := []struct {
		name string
		raw  string
	}{
		{"Empty Input", ""},
		{"Illegal JSON", `{not json`},
		{"Cut. JSON", `{"min_severity":`},
		{"Type does not match", `{"min_severity": 123, "task_ids": "abc"}`},
		{"Top layer is array", `[1,2,3]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := ParseFilter([]byte(tc.raw))
			if f.MinSeverity != "" || len(f.TaskIDs) != 0 || len(f.AssetIDs) != 0 {
				t.Fatalf("Malformed configuration should be degraded to zero Filter,get %+v", f)
			}
			// zero value Filter Must hit anything..
			ev := Snapshot{Kind: EventFindingCreated, Severity: "low", VulnClass: "XSS"}
			if !Match(f, ev) {
				t.Fatal("zero value Filter We're gonna hit everything.")
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
		// Unknown serial number at level 0,Should be blocked by any non-empty threshold (without pushing in case of doubt)).
		{"low", "", false},
		{"low", "unknown", false},
		{"", "", true},
	}
	for _, tc := range cases {
		got := Match(Filter{MinSeverity: tc.min}, ev(tc.sev))
		if got != tc.expect {
			t.Errorf("min=%q sev=%q: Expectations %v get %v", tc.min, tc.sev, tc.expect, got)
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
		{"Empty range=No limit", Filter{}, true},
		{"Mission hit.", Filter{TaskIDs: []int64{7}}, true},
		{"Mission missed.", Filter{TaskIDs: []int64{8}}, false},
		{"Multiple hits.", Filter{TaskIDs: []int64{8, 7}}, true},
		{"Assets intersected", Filter{AssetIDs: []int64{20, 99}}, true},
		{"No intersection of assets", Filter{AssetIDs: []int64{99}}, false},
		{"Mission hit with asset.", Filter{TaskIDs: []int64{7}, AssetIDs: []int64{10}}, true},
		{"Mission hit but asset missed", Filter{TaskIDs: []int64{7}, AssetIDs: []int64{99}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Match(tc.filter, ev); got != tc.expect {
				t.Errorf("Expectations %v get %v", tc.expect, got)
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
		{"include Empty=All", Filter{}, "Any type", true},
		{"include hit", Filter{VulnClassInclude: []string{"SQL"}}, "SQLInjection", true},
		{"include miss", Filter{VulnClassInclude: []string{"Command execution"}}, "SQLInjection", false},
		{"include A multiple hit.", Filter{VulnClassInclude: []string{"Command execution", "SQL"}}, "SQLInjection", true},
		{"Case insensitive", Filter{VulnClassInclude: []string{"sql"}}, "SQLInjection", true},
		{"exclude It's off.", Filter{VulnClassExclude: []string{"Information leakage"}}, "Information leakage", false},
		{"exclude Let go if you don't hit.", Filter{VulnClassExclude: []string{"Information leakage"}}, "SQLInjection", true},
		// Exclusion over inclusion: should be out at the same time..
		{"Exclusion over Inclusion", Filter{
			VulnClassInclude: []string{"SQL"},
			VulnClassExclude: []string{"Injection"},
		}, "SQLInjection", false},
		// Purely blank keywords should be ignored, or they could degenerate into[Match all whitespaced strings].
		{"Empty keyword ignored", Filter{VulnClassInclude: []string{"", "  "}}, "SQLInjection", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Match(tc.filter, ev(tc.class)); got != tc.expect {
				t.Errorf("Expectations %v get %v", tc.expect, got)
			}
		})
	}
}

func TestMatchStatusChangeRequiresOptIn(t *testing.T) {
	ev := Snapshot{Kind: EventFindingStatusChanged, Severity: "critical", FromStatus: "pending", ToStatus: "fixed"}
	// Default level: most people say[Push the hole.]It means you've found a new loophole, not a state-of-the-art account..
	if Match(Filter{MinSeverity: "low"}, ev) {
		t.Fatal("Status Change Event Skipped without Open")
	}
	if !Match(Filter{OnStatusChange: true}, ev) {
		t.Fatal("Open on_status_change Post-state change event hit.")
	}
	// Other Organiser on_status_change Influence.
	created := Snapshot{Kind: EventFindingCreated, Severity: "critical"}
	if !Match(Filter{MinSeverity: "low"}, created) {
		t.Fatal("Create event should not depend on on_status_change")
	}
}
