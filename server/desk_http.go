package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/skilfoy/ARTEX-English/program"
)

var errBadDesk = errors.New("desk: bad task id")

var deskTaskID = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

func (s *Server) openTaskDesk(id string) (*program.Desk, error) {
	if !deskTaskID.MatchString(id) || s == nil || s.m == nil || s.m.dir == "" {
		return nil, errBadDesk
	}
	return program.Open(filepath.Join(s.m.dir, "tasks", id, "program"))
}

func deskView(desk *program.Desk) map[string]any {
	files, edges := 0, 0
	if graph := program.MountGraph(); graph != nil {
		files, edges = len(graph.Nodes), len(graph.Edges)
	}
	var record any
	if desk.Record != nil {
		record = desk.Record
	}
	return map[string]any{
		"team":     desk.Team,
		"mayWrite": desk.MayWrite(),
		"nodes":    len(desk.Graph.Nodes),
		"edges":    len(desk.Graph.Edges),
		"events":   len(desk.Log),
		"log":      desk.Recent(30),
		"record":   record,
		"mount":    map[string]any{"files": files, "edges": edges},
		"lease":    desk.Lease.Owner,
	}
}

func (s *Server) taskDesk(w http.ResponseWriter, r *http.Request) {
	desk, err := s.openTaskDesk(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "task id")
		return
	}
	writeJSON(w, 200, deskView(desk))
}

func (s *Server) taskDeskConfirm(w http.ResponseWriter, r *http.Request) {
	desk, err := s.openTaskDesk(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "task id")
		return
	}
	if !desk.Allows(r.Header.Get("X-ARTEX-Owner")) {
		writeErr(w, 409, "desk is leased")
		return
	}
	if err := desk.Confirm(); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if err := desk.Save(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, deskView(desk))
}

func (s *Server) taskDeskStrict(w http.ResponseWriter, r *http.Request) {
	desk, err := s.openTaskDesk(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "task id")
		return
	}
	var body struct {
		Strict bool `json:"strict"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeErr(w, 400, "strict must be a boolean")
		return
	}
	desk.SetStrict(body.Strict)
	if err := desk.Save(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, deskView(desk))
}

func (s *Server) taskDeskFixture(w http.ResponseWriter, r *http.Request) {
	desk, err := s.openTaskDesk(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "task id")
		return
	}
	scope := program.ProcessScope()
	if scope == nil {
		writeErr(w, 409, "no scope loaded")
		return
	}
	if err := desk.RecordFixture(scope.Hash()); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := desk.Save(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, deskView(desk))
}

func (s *Server) programMount(w http.ResponseWriter, r *http.Request) {
	graph := program.MountGraph()
	if graph == nil {
		writeJSON(w, 200, map[string]any{"loaded": false})
		return
	}
	paths := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if len(paths) >= 200 {
			break
		}
		paths = append(paths, node.Path)
	}
	writeJSON(w, 200, map[string]any{
		"loaded": true,
		"files":  len(graph.Nodes),
		"edges":  len(graph.Edges),
		"paths":  paths,
	})
}

func (s *Server) taskDeskIntake(w http.ResponseWriter, r *http.Request) {
	desk, err := s.openTaskDesk(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "task id")
		return
	}
	scope := program.ProcessScope()
	if scope == nil {
		writeErr(w, 409, "no scope loaded")
		return
	}
	if _, err := desk.IntakeScope(scope); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := desk.Save(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, deskView(desk))
}

func (s *Server) taskDeskReview(w http.ResponseWriter, r *http.Request) {
	desk, err := s.openTaskDesk(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "task id")
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		writeErr(w, 400, "path is required")
		return
	}
	if err := desk.NoteReview(body.Path); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := desk.Save(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, deskView(desk))
}

func (s *Server) taskDeskLease(w http.ResponseWriter, r *http.Request) {
	desk, err := s.openTaskDesk(r.PathValue("id"))
	if err != nil {
		writeErr(w, 400, "task id")
		return
	}
	var body struct {
		Owner   string `json:"owner"`
		Seconds int    `json:"seconds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeErr(w, 400, "owner is required")
		return
	}
	if body.Seconds <= 0 || body.Seconds > 3600 {
		body.Seconds = 300
	}
	if err := desk.Acquire(body.Owner, time.Duration(body.Seconds)*time.Second); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if err := desk.Save(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, deskView(desk))
}

func (s *Server) programSkills(w http.ResponseWriter, r *http.Request) {
	root := ""
	if s != nil {
		root = s.skillDir
	}
	if root == "" {
		writeJSON(w, 200, map[string]any{"skills": []string{}, "hardGate": 0, "index": []program.IndexEntry{}})
		return
	}
	skills, err := program.ScanSkills(root)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	var catalog []program.IndexEntry
	if root != "" {
		if raw, err := os.ReadFile(filepath.Join(root, "INDEX.md")); err == nil {
			for _, entry := range program.ParseIndex(string(raw)) {
				if !entry.Skipped {
					catalog = append(catalog, entry)
				}
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"skills":   names,
		"hardGate": program.HardGateCount(skills),
		"index":    catalog,
	})
}
