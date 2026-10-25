package agent

import (
	"path/filepath"
	"testing"

	"github.com/skilfoy/ARTEX-English/program"
)

func TestFindingAllowedFollowsStrictMode(t *testing.T) {
	dir := t.TempDir()
	program.InstallProcessScope(&program.Scope{})
	defer program.InstallProcessScope(nil)

	noteProgramDesk(dir, 4, "worker", "web")
	if !findingAllowed(dir, 4) {
		t.Fatal("advisory team blocked a finding")
	}
	desk, err := program.Open(filepath.Join(dir, "tasks", "4", "program"))
	if err != nil {
		t.Fatal(err)
	}
	if len(desk.Team.Members) != len(program.AdvisoryRoles) {
		t.Fatalf("seats = %d", len(desk.Team.Members))
	}
	desk.SetStrict(true)
	if err := desk.Save(); err != nil {
		t.Fatal(err)
	}
	if findingAllowed(dir, 4) {
		t.Fatal("strict team allowed a finding")
	}
	if !findingAllowed("", 4) {
		t.Fatal("missing work dir should stay allowed")
	}
}
