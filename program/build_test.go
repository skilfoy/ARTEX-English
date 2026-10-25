package program

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGroupsIndexAndIntake(t *testing.T) {
	host, err := CleanHost("https://Claims.Northline.lab/app")
	if err != nil || host != "claims.northline.lab" {
		t.Fatalf("clean host: %q %v", host, err)
	}
	if _, err := CleanHost("http://user:pw@claims.northline.lab/"); err == nil {
		t.Fatal("credentials accepted")
	}
	if _, err := CleanHost("127.0.0.1"); err == nil {
		t.Fatal("loopback accepted")
	}
	got, err := IntersectGroups([]HostGroup{
		{Name: "a", Hosts: []string{"claims.northline.lab", "other.northline.lab"}},
		{Name: "b", Hosts: []string{"https://claims.northline.lab/"}},
	})
	if err != nil || len(got) != 1 || got[0] != "claims.northline.lab" {
		t.Fatalf("intersect: %v %v", got, err)
	}

	index := ParseIndex("| name | summary |\n| --- | --- |\n| scopesentry | gate |\n| cve-poc-generator | no |\n- web-recon: read\n")
	if len(index) != 3 || !index[1].Skipped || index[0].Skipped || index[2].Name != "web-recon" {
		t.Fatalf("index: %+v", index)
	}

	scope, err := Parse([]byte(`{
		"habitat": "northline",
		"default": "deny",
		"allow": [{"cidr": "10.16.0.0/24"}],
		"names": [{"fqdn": "claims.northline.lab", "ip": "10.16.0.20"}],
		"deniedNames": [{"fqdn": "jump.northline.lab", "ip": "10.16.16.10"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	desk, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n, err := desk.IntakeScope(scope)
	if err != nil || n < 3 {
		t.Fatalf("intake %d %v", n, err)
	}
	again, err := desk.IntakeScope(scope)
	if err != nil || again != 0 {
		t.Fatalf("second intake added %d: %v", again, err)
	}
	if err := desk.AddNode("bad", "asset", "AKIAIOSFODNN7EXAMPLE", ""); err == nil {
		t.Fatal("secret label stored")
	}
	if err := desk.AddNode("plane-mcp", "plane", "mcp", ""); err != nil {
		t.Fatal(err)
	}
	if err := desk.NoteReview("../x.patch"); err == nil {
		t.Fatal("escaped review path")
	}
	if err := desk.NoteReview(".artex-patches/1-note.patch"); err != nil {
		t.Fatal(err)
	}
	if err := desk.Acquire("ada", time.Minute); err != nil {
		t.Fatal(err)
	}
	if desk.Allows("bea") {
		t.Fatal("other owner allowed")
	}
	if !desk.Allows("ada") {
		t.Fatal("owner refused")
	}
	if err := desk.Acquire("bea", time.Minute); err == nil {
		t.Fatal("second lease accepted")
	}
	if len(desk.Recent(5)) == 0 {
		t.Fatal("review was not logged")
	}
	if err := desk.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestIndexFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/INDEX.md", []byte("- scopesentry: gate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dir + "/INDEX.md")
	if err != nil {
		t.Fatal(err)
	}
	entries := ParseIndex(string(raw))
	if len(entries) != 1 || entries[0].Skipped || !strings.Contains(entries[0].Summary, "gate") {
		t.Fatalf("%+v", entries)
	}
}

func TestGraphBriefAndPatchCheck(t *testing.T) {
	graph := &CodeGraph{
		Nodes: []CodeNode{{Path: "portal/main.go", Kind: "file"}},
		Edges: []CodeEdge{{From: "portal/main.go", To: "external:fmt", Kind: "import"}},
	}
	InstallMountGraph(graph)
	t.Cleanup(func() { InstallMountGraph(nil) })
	note := CodeGraphNote()
	if !strings.Contains(note, "file portal/main.go") || !strings.Contains(note, "portal/main.go -> external:fmt") {
		t.Fatalf("brief: %s", note)
	}
	if strings.Contains(note, "package portal") || !strings.Contains(note, "CODE_GRAPH") {
		t.Fatalf("brief leaked a body or dropped the fence: %s", note)
	}

	root := t.TempDir()
	if err := os.MkdirAll(root+"/portal", 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package portal\nconst Version = \"1\"\n"
	if err := os.WriteFile(root+"/portal/main.go", []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "" +
		"--- a/portal/main.go\n" +
		"+++ b/portal/main.go\n" +
		"@@ -1,2 +1,2 @@\n" +
		" package portal\n" +
		"-const Version = \"1\"\n" +
		"+const Version = \"2\"\n"
	if err := os.MkdirAll(root+"/.artex-patches", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/.artex-patches/1-note.patch", []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}
	desk, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	report, err := desk.ReviewPatch(root, ".artex-patches/1-note.patch")
	if err != nil || !report.Match || report.Applied {
		t.Fatalf("review: %+v %v", report, err)
	}
	got, err := os.ReadFile(root + "/portal/main.go")
	if err != nil || string(got) != source {
		t.Fatalf("source changed: %q %v", got, err)
	}
	bad := "--- a/portal/main.go\n+++ b/portal/main.go\n@@ -1,2 +1,2 @@\n package portal\n-const Version = \"9\"\n+const Version = \"2\"\n"
	if err := os.WriteFile(root+"/.artex-patches/2-miss.patch", []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := desk.ReviewPatch(root, ".artex-patches/2-miss.patch"); err == nil {
		t.Fatal("mismatch accepted")
	}
	escape := "--- a/portal/main.go\n+++ b/../outside.go\n@@ -1 +1 @@\n-package portal\n+package portal\n"
	if err := os.WriteFile(root+"/.artex-patches/3-out.patch", []byte(escape), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckPatch(root, ".artex-patches/3-out.patch"); err == nil {
		t.Fatal("escaped target accepted")
	}
	if _, err := CheckPatch(root, "../1-note.patch"); err == nil {
		t.Fatal("escaped patch path accepted")
	}
	got, err = os.ReadFile(root + "/portal/main.go")
	if err != nil || string(got) != source {
		t.Fatalf("source changed after refusals: %q", got)
	}
}
