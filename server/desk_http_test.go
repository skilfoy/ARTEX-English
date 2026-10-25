package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/skilfoy/ARTEX-English/program"
)

func TestDeskConfirmRoute(t *testing.T) {
	dir := t.TempDir()
	s := &Server{m: &Manager{dir: dir}}
	desk, err := s.openTaskDesk("7")
	if err != nil {
		t.Fatal(err)
	}
	if err := desk.AddRole("web", "status"); err != nil {
		t.Fatal(err)
	}
	desk.SetStrict(true)
	if err := desk.MarkDone("web"); err != nil {
		t.Fatal(err)
	}
	if err := desk.Collect(); err != nil {
		t.Fatal(err)
	}
	if err := desk.Save(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/7/desk/confirm", nil)
	req.SetPathValue("id", "7")
	rec := httptest.NewRecorder()
	s.taskDeskConfirm(rec, req)
	if rec.Code != 200 {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"mayWrite":true`) {
		t.Fatalf("body: %s", rec.Body.String())
	}

	bad := httptest.NewRequest(http.MethodGet, "/api/tasks/../desk", nil)
	bad.SetPathValue("id", "../x")
	rec = httptest.NewRecorder()
	s.taskDesk(rec, bad)
	if rec.Code != 400 {
		t.Fatalf("path: %d", rec.Code)
	}
}

func TestDeskFixtureNeedsScope(t *testing.T) {
	program.InstallProcessScope(nil)
	dir := t.TempDir()
	s := &Server{m: &Manager{dir: dir}}
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/3/desk/fixture", nil)
	req.SetPathValue("id", "3")
	rec := httptest.NewRecorder()
	s.taskDeskFixture(rec, req)
	if rec.Code != 409 {
		t.Fatalf("fixture without scope: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeskLeaseBlocksConfirm(t *testing.T) {
	dir := t.TempDir()
	s := &Server{m: &Manager{dir: dir}}
	desk, err := s.openTaskDesk("8")
	if err != nil {
		t.Fatal(err)
	}
	if err := desk.AddRole("web", "status"); err != nil {
		t.Fatal(err)
	}
	desk.SetStrict(true)
	if err := desk.MarkDone("web"); err != nil || desk.Collect() != nil || desk.Save() != nil {
		t.Fatal("setup")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/8/desk/lease", strings.NewReader(`{"owner":"ada","seconds":60}`))
	req.SetPathValue("id", "8")
	rec := httptest.NewRecorder()
	s.taskDeskLease(rec, req)
	if rec.Code != 200 {
		t.Fatalf("lease: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/tasks/8/desk/confirm", nil)
	req.SetPathValue("id", "8")
	rec = httptest.NewRecorder()
	s.taskDeskConfirm(rec, req)
	if rec.Code != 409 {
		t.Fatalf("confirm without owner: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/tasks/8/desk/confirm", nil)
	req.SetPathValue("id", "8")
	req.Header.Set("X-ARTEX-Owner", "ada")
	rec = httptest.NewRecorder()
	s.taskDeskConfirm(rec, req)
	if rec.Code != 200 {
		t.Fatalf("confirm as owner: %d %s", rec.Code, rec.Body.String())
	}
}

func TestProgramSkillsSkipsPayloadNames(t *testing.T) {
	root := t.TempDir()
	s := &Server{skillDir: root}
	if err := os.WriteFile(root+"/INDEX.md", []byte("- scopesentry: gate\n- cve-poc-generator: no\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/program/skills", nil)
	rec := httptest.NewRecorder()
	s.programSkills(rec, req)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "cve-poc") || !strings.Contains(rec.Body.String(), "scopesentry") {
		t.Fatalf("skills: %d %s", rec.Code, rec.Body.String())
	}
}
