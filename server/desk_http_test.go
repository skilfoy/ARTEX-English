package server

import (
	"net/http"
	"net/http/httptest"
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
