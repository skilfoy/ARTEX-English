package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/skilfoy/ARTEX-English/db"
)

// Overview[Goal management]Manpower CRUD Interface. and agent Side. set_goals It's the same tool. goal node,
// But the entrance is human. UI Add & Delete;New/Modified Reuse[Resurrection mission]Logical(admitTask resume:
// Final state→running,Lift the pause and line up if necessary),Delete Not Again(By product).Every change handler Let's go.
// beginTaskOperation/decInflight,Avoids removing competition with the task(And intent. CRUD Consistent).

// listGoals Return all objectives of this task(text/vulnclass/state Open it.),For target management card rendering.
func (s *Server) listGoals(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	goals, err := t.Store.ListByKind(db.KindGoal, 10000)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"goals": goalDTOs(goals)})
}

// addGoal Add a target manually:Drop the library.(Synchronising folder spawns Down)→ Remember one.[New target.]Trigger awakening
// planner → Resurrection mission,Let the planners judge whether the new target has been met or not..
func (s *Server) addGoal(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, "Task is being deleted; goals cannot be changed")
		return
	}
	defer s.engine.decInflight(t.ID)

	var body struct {
		Text      string `json:"text"`
		VulnClass string `json:"vulnclass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, 400, "Target content cannot be empty")
		return
	}
	payload := map[string]any{"text": text}
	if vc := strings.TrimSpace(body.VulnClass); vc != "" {
		payload["vulnclass"] = vc
	}
	id, err := t.Store.AddGoal(payload, "human")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if of, _ := t.Store.OriginFactID(); of > 0 && id > 0 {
		_ = t.Store.Link(of, db.RelSpawns, id) // goal descends from the task root (origin fact)
	}
	t.NotifyGoal([]string{text}) // note[People have added targets.:…]Trigger and wake up planner
	s.reviveTask(t)              // Finish/Suspended task pulls back into operational mode and continues to run.
	node, _ := t.Store.GetNode(id)
	if node == nil {
		writeErr(w, 500, "Failed to read after target writing")
		return
	}
	writeJSON(w, 200, goalDTO(node))
}

// editGoal Manually modify a target text(and vulnclass):Change library → note[User modified target by old become new]
// Trigger awakening planner → Resurrection mission,Let the planners re-orient themselves to new targets..
func (s *Server) editGoal(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, "Task is being deleted; goals cannot be changed")
		return
	}
	defer s.engine.decInflight(t.ID)

	gid, err := strconv.ParseInt(r.PathValue("gid"), 10, 64)
	if err != nil || gid <= 0 {
		writeErr(w, 400, "bad goal id")
		return
	}
	var body struct {
		Text      string `json:"text"`
		VulnClass string `json:"vulnclass"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeErr(w, 400, "Target content cannot be empty")
		return
	}
	node, err := t.Store.GetNode(gid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if node == nil || node.Kind != db.KindGoal {
		writeErr(w, 404, "Target does not exist.")
		return
	}
	oldText := goalDTO(node).Text
	if err := t.Store.UpdateGoalPayload(gid, text, strings.TrimSpace(body.VulnClass)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	t.NotifyGoalEdited(oldText, text) // note[People modified the target by old become new]Trigger and wake up planner
	s.reviveTask(t)                   // Consistent with new:The resuscitation mission has been reset by new targets.
	updated, _ := t.Store.GetNode(gid)
	if updated == nil {
		writeErr(w, 500, "Failed to read after target update")
		return
	}
	writeJSON(w, 200, goalDTO(updated))
}

// deleteGoal Remove a target manually(Hard Delete,Declining cascades/anchor):Delete Library → note[User deleted the target X]
// Trigger awakening planner The remaining targets will be recast accordingly. By product,Delete[No]Resurrection mission.
func (s *Server) deleteGoal(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !s.engine.beginTaskOperation(t.ID) {
		writeErr(w, 409, "Task is being deleted; goals cannot be changed")
		return
	}
	defer s.engine.decInflight(t.ID)

	gid, err := strconv.ParseInt(r.PathValue("gid"), 10, 64)
	if err != nil || gid <= 0 {
		writeErr(w, 400, "bad goal id")
		return
	}
	node, err := t.Store.GetNode(gid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if node == nil || node.Kind != db.KindGoal {
		writeErr(w, 404, "Target does not exist.")
		return
	}
	text := goalDTO(node).Text
	if err := t.Store.DeleteGoal(gid); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	t.NotifyGoalDeleted(text) // note[People deleted the target.:…]Trigger and wake up planner(I'm not going back on a mission.)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
