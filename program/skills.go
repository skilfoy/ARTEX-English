package program

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Skill is one directory under the skill root. HardGate means the skill is
// not optional guidance: scopesentry is the scope gate, and the guard is what
// enforces it.
type Skill struct {
	Name     string
	Path     string
	Summary  string
	HardGate bool
}

// ScanSkills reads each immediate subdirectory that contains a SKILL.md.
// A missing root returns an empty list, not an error, so a fresh checkout
// still boots.
func ScanSkills(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "SKILL.md")
		summary, err := firstLine(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		out = append(out, Skill{
			Name:     entry.Name(),
			Path:     path,
			Summary:  summary,
			HardGate: entry.Name() == "scopesentry",
		})
	}
	return out, nil
}

// HardGateCount is how many scanned skills are gates rather than notes.
func HardGateCount(skills []Skill) int {
	n := 0
	for _, skill := range skills {
		if skill.HardGate {
			n++
		}
	}
	return n
}

func firstLine(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", scanner.Err()
}
