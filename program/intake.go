package program

import (
	"fmt"
	"sort"
	"strings"
)

const maxIntakeNodes = 180

// IntakeScope copies the loaded scope into the engagement graph.
// Allowed names become asset nodes. Denied names become one count, not a
// list of targets. Nothing is contacted.
func (d *Desk) IntakeScope(s *Scope) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("desk: no scope")
	}
	added := 0
	hash := s.Hash()
	ref := hash
	if len(ref) > 12 {
		ref = ref[:12]
	}
	label := s.Habitat
	if label == "" {
		label = "scope"
	}
	created, err := d.addOnce("scope-allow", "scope-group", label, ref)
	if err != nil {
		return added, err
	}
	if created {
		added++
	}
	names := make([]string, 0, len(s.Names))
	for name := range s.Names {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if len(d.Graph.Nodes) >= maxIntakeNodes {
			break
		}
		id := nodeID("asset", name)
		ip := ""
		if rec := s.Names[name]; rec.IP != nil {
			ip = rec.IP.String()
		}
		created, err := d.addOnce(id, "asset", name, ip)
		if err != nil {
			return added, err
		}
		if !created {
			continue
		}
		if err := d.Link(id, "scope-allow", "in"); err != nil {
			return added, err
		}
		added++
	}
	created, err = d.addOnce("scope-denied", "scope-group", fmt.Sprintf("%d denied names", len(s.Denied)), "")
	if err != nil {
		return added, err
	}
	if created {
		added++
	}
	return added, nil
}

func (d *Desk) addOnce(id, kind, label, ref string) (bool, error) {
	err := d.AddNode(id, kind, label, ref)
	if err != nil && strings.Contains(err.Error(), "duplicate") {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func nodeID(prefix, name string) string {
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteByte('-')
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NoteReview records that a patch exists under .artex-patches. It does not
// apply the patch and it does not open a pull request.
func (d *Desk) NoteReview(relPath string) error {
	if strings.Contains(relPath, "..") || !strings.HasPrefix(relPath, ".artex-patches/") || !strings.HasSuffix(relPath, ".patch") {
		return fmt.Errorf("desk: review path must be a patch under .artex-patches")
	}
	d.Append("review", map[string]string{"path": relPath})
	return nil
}
