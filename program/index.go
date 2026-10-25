package program

import "strings"

// IndexEntry is one row of a skill catalog. Skipped entries are names this
// process will not load: generators, payloads, and social-engineering packs.
type IndexEntry struct {
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
}

// ParseIndex reads a markdown list or table. It does not read skill bodies.
func ParseIndex(text string) []IndexEntry {
	var out []IndexEntry
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		name, summary := indexFields(line)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, IndexEntry{Name: name, Summary: summary, Skipped: skippedSkill(name)})
	}
	return out
}

func indexFields(line string) (string, string) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "|") {
		parts := strings.Split(line, "|")
		cells := make([]string, 0, len(parts))
		for _, part := range parts {
			cell := strings.TrimSpace(part)
			if cell != "" {
				cells = append(cells, cell)
			}
		}
		if len(cells) < 2 {
			return "", ""
		}
		name := stripLink(cells[0])
		if name == "" || strings.EqualFold(name, "name") || strings.EqualFold(name, "skill") || strings.Contains(name, "---") {
			return "", ""
		}
		return name, cells[1]
	}
	if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") {
		return "", ""
	}
	line = strings.TrimSpace(line[1:])
	name, summary, _ := strings.Cut(line, ":")
	name = stripLink(strings.TrimSpace(name))
	summary = strings.TrimSpace(summary)
	if name == "" || strings.Contains(name, " ") {
		return "", ""
	}
	return name, summary
}

func stripLink(cell string) string {
	if strings.HasPrefix(cell, "[") {
		if end := strings.IndexByte(cell, ']'); end > 1 {
			return cell[1:end]
		}
	}
	return strings.Trim(cell, "` ")
}

func skippedSkill(name string) bool {
	n := strings.ToLower(name)
	for _, word := range []string{"poc", "exploit", "payload", "social-engineering", "jailbreak", "ransomware", "crypter"} {
		if strings.Contains(n, word) {
			return true
		}
	}
	return false
}
