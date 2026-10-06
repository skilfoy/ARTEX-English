package agent

import (
	"strings"

	"github.com/skilfoy/ARTEX-English/db"
)

// constraintBlock renders this task's operation constraints (task_constraints) as a
// high-priority block appended to the planner/worker system prompt. allow/deny are
// grouped; empty string when there are no constraints (or ts is nil). The framing
// deliberately puts these ABOVE the exploration/expansion heuristics so a declared
// boundary wins the tug-of-war against "chase another entry surface".
func constraintBlock(ts *db.ExplorationStore) string {
	if ts == nil {
		return ""
	}
	rows, err := ts.ListConstraints()
	if err != nil || len(rows) == 0 {
		return ""
	}
	var allow, deny []string
	for _, c := range rows {
		text := strings.TrimSpace(c.Text)
		if text == "" {
			continue
		}
		if c.Kind == "allow" {
			allow = append(allow, "- "+text)
		} else {
			deny = append(deny, "- "+text)
		}
	}
	if len(allow) == 0 && len(deny) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[Task operation constraints. Apply these to every proposed direction and action.]")
	if len(allow) > 0 {
		b.WriteString("\nAllowed Operations:\n")
		b.WriteString(strings.Join(allow, "\n"))
	}
	if len(deny) > 0 {
		b.WriteString("\nProhibited Operations:\n")
		b.WriteString(strings.Join(deny, "\n"))
	}
	b.WriteString("\nNewly discovered targets and ports remain outside the approved scope unless explicitly included above. Record them as out-of-scope observations without acting on them.")
	return b.String()
}
