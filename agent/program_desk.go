package agent

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/skilfoy/ARTEX-English/db"
	"github.com/skilfoy/ARTEX-English/program"
)

// noteProgramDesk records that a worker started, but only when a scope is
// installed for the process. Tasks without a scope file do not grow a desk.
func noteProgramDesk(workDir string, taskID int64, kind, name string) {
	if program.ProcessScope() == nil || workDir == "" || taskID <= 0 {
		return
	}
	dir := filepath.Join(workDir, "tasks", strconv.FormatInt(taskID, 10), "program")
	desk, err := program.Open(dir)
	if err != nil {
		return
	}
	if kind == "worker" {
		desk.SeatAdvisory(program.AdvisoryRoles)
		if graph := program.MountGraph(); graph != nil {
			desk.Append("mount", map[string]string{
				"files": strconv.Itoa(len(graph.Nodes)),
				"edges": strconv.Itoa(len(graph.Edges)),
			})
		}
	}
	desk.Append(kind, map[string]string{"name": name})
	_ = desk.Save()
}

// findingAllowed is fail-closed only when a scope is installed and the desk
// is in strict mode. Advisory teams, and tasks with no scope, still record.
func findingAllowed(workDir string, taskID int64) bool {
	if program.ProcessScope() == nil || workDir == "" || taskID <= 0 {
		return true
	}
	dir := filepath.Join(workDir, "tasks", strconv.FormatInt(taskID, 10), "program")
	desk, err := program.Open(dir)
	if err != nil {
		return false
	}
	return desk.MayWrite()
}

// classifyingRecorder labels a finding and writes that label to the desk.
// It does not change the finding the recorder stores.
type classifyingRecorder struct {
	inner   FindingRecorder
	workDir string
	taskID  int64
}

func (c classifyingRecorder) Record(ctx context.Context, in db.RecordFindingInput, refs []db.TrafficRef) (*db.RecordedFinding, error) {
	if c.inner == nil {
		return nil, errNoRecorder
	}
	if !findingAllowed(c.workDir, c.taskID) {
		return nil, errDeskUnconfirmed
	}
	category := program.Classify(in.Name, in.VulnClass)
	noteProgramDesk(c.workDir, c.taskID, "finding", category)
	return c.inner.Record(ctx, in, refs)
}
