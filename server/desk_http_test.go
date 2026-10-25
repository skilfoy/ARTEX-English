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

func TestDeskReviewChecksPatch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/portal", 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package portal\nconst Version = \"1\"\n"
	if err := os.WriteFile(root+"/portal/main.go", []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "--- a/portal/main.go\n+++ b/portal/main.go\n@@ -1,2 +1,2 @@\n package portal\n-const Version = \"1\"\n+const Version = \"2\"\n"
	if err := os.MkdirAll(root+"/.artex-patches", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/.artex-patches/1-note.patch", []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	program.InstallMountRoot(root)
	t.Cleanup(func() { program.InstallMountRoot("") })
	s := &Server{m: &Manager{dir: t.TempDir()}}
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/9/desk/review", strings.NewReader(`{"path":".artex-patches/1-note.patch"}`))
	req.SetPathValue("id", "9")
	rec := httptest.NewRecorder()
	s.taskDeskReview(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"applied":false`) {
		t.Fatalf("review: %d %s", rec.Code, rec.Body.String())
	}
	got, err := os.ReadFile(root + "/portal/main.go")
	if err != nil || string(got) != source {
		t.Fatalf("source changed: %q", got)
	}
}
