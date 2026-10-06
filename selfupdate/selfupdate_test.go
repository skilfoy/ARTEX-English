package selfupdate

import (
	"archive/zip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testPaths Make a separate upgrade catalogue. It can't be used directly. ResolvePaths()——It'll point to the test.
// The binary itself, you run. go test The enforceable file has been renamed..
func testPaths(t *testing.T) Paths {
	t.Helper()
	dir := t.TempDir()
	return Paths{
		Dir:     dir,
		Current: filepath.Join(dir, "artex"),
		New:     filepath.Join(dir, "artex.new"),
		Sum:     filepath.Join(dir, "artex.new.sha256"),
		Old:     filepath.Join(dir, "artex.old"),
		Marker:  filepath.Join(dir, "artex.upgrade.json"),
	}
}

// fakeBin Write an enforceable shell script to pretend artex.smokeTest Just use it. -h Pull it up to see the exit code.,
// Scripts are perfect and faster than a true binary..
func fakeBin(t *testing.T, path, marker string, exitCode int) {
	t.Helper()
	script := "#!/bin/sh\necho " + marker + "\nexit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("Writing false binary %s: %v", path, err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	return string(rune('0' + n))
}

// stage handle bin Set"Saved pending replacement"Look: Write it. artex.new And check it out..
func stage(t *testing.T, p Paths, marker string, exitCode int) {
	t.Helper()
	fakeBin(t, p.New, marker, exitCode)
	sum, err := fileSHA256(p.New)
	if err != nil {
		t.Fatalf("Calculate checksum: %v", err)
	}
	if err := os.WriteFile(p.Sum, []byte(sum), 0o644); err != nil {
		t.Fatalf("Write Checksum: %v", err)
	}
}

func readAll(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Read %s: %v", path, err)
	}
	return string(b)
}

func requireUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("It's a fake binary. sh Script,Windows Can't get up.")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b       string
		want       int
		comparable bool
	}{
		{"0.3.7", "0.3.8", -1, true},
		{"0.3.8", "0.3.7", 1, true},
		{"0.3.7", "0.3.7", 0, true},
		{"v0.3.7", "0.3.8", -1, true}, // build.sh Get rid of it. v,tag With v,Both sides.
		{"0.3.7", "v0.3.7", 0, true},
		{"0.9.0", "0.10.0", -1, true}, // By Number rather than Dictionary
		{"1.0.0", "0.99.99", 1, true},
		// The development build must be classified as non-comparison, otherwise the unsubmitted changes will be covered by the official version.
		{"dev", "0.3.8", 0, false},
		{"0.3.7-2-gabc1234", "0.3.8", 0, false},
		{"0.3.7-dirty", "0.3.8", 0, false},
		{"0.3", "0.3.8", 0, false},
		{"", "0.3.8", 0, false},
	}
	for _, c := range cases {
		got, ok := CompareVersions(c.a, c.b)
		if ok != c.comparable {
			t.Errorf("CompareVersions(%q,%q) comparable=%v, Expectations %v", c.a, c.b, ok, c.comparable)
			continue
		}
		if ok && got != c.want {
			t.Errorf("CompareVersions(%q,%q)=%d, Expectations %d", c.a, c.b, got, c.want)
		}
	}
}

func TestResolvePathsNaming(t *testing.T) {
	p, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	// Key non-variant: All upgrade documents are in the same directory as implementable documents. Fall CWD It'll make service run.
	// (Work catalogue could be /)It's completely disabled..
	for name, path := range map[string]string{"New": p.New, "Sum": p.Sum, "Old": p.Old, "Marker": p.Marker} {
		if filepath.Dir(path) != p.Dir {
			t.Errorf("%s Not in the executable directory: %s (Expectations %s)", name, path, p.Dir)
		}
	}
	// Windows on .new/.old Must keep .exe,Otherwise, smoke tests and refitting will fail..
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(p.New, ".exe") || !strings.HasSuffix(p.Old, ".exe") {
			t.Errorf("Windows on .new/.old I have to. .exe End: new=%s old=%s", p.New, p.Old)
		}
	}
}

func TestVerifyStagedRejectsTamperedBinary(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	stage(t, p, "new", 0)

	// Check and write before changing files. Simulate download damage / It's switched..
	fakeBin(t, p.New, "tampered", 0)
	if err := verifyStaged(p); err == nil {
		t.Fatal("Expectations SHA256 No match was rejected, but passed.")
	}
}

func TestVerifyStagedRejectsUnrunnableBinary(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	stage(t, p, "broken", 1) // Can execute but quit. 0

	if err := verifyStaged(p); err == nil {
		t.Fatal("The expectation of a failed smoke test was rejected and passed.")
	}
}

func TestApplyStagedHappyPath(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	fakeBin(t, p.Current, "old", 0)
	stage(t, p, "new", 0)
	if err := writeMarker(p.Marker, marker{From: "0.3.7", To: "0.3.8"}); err != nil {
		t.Fatalf("Write Tags: %v", err)
	}

	action, st := applyStaged(p)
	if action != Restart {
		t.Fatalf("Expectations Restart,get %v", action)
	}
	if !st.Pending {
		t.Error("After reloading state should read Pending")
	}
	if !strings.Contains(readAll(t, p.Current), "new") {
		t.Error("artex Should have been replaced by a new version")
	}
	if !strings.Contains(readAll(t, p.Old), "old") {
		t.Error("Old version should be backed up to artex.old")
	}
	if _, err := os.Stat(p.New); !os.IsNotExist(err) {
		t.Error("When you change. artex.new Should have disappeared.")
	}
	if _, err := os.Stat(p.Sum); !os.IsNotExist(err) {
		t.Error("Checksum files after reloading should be cleared")
	}
	// The tag must be kept, and the next launch will count and roll back if necessary..
	if _, ok := readMarker(p.Marker); !ok {
		t.Error("Upgrade mark after reloading should be kept")
	}
}

func TestApplyStagedKeepsCurrentWhenVerifyFails(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	fakeBin(t, p.Current, "old", 0)
	stage(t, p, "new", 0)
	fakeBin(t, p.New, "tampered", 0) // Disruption of checksum

	action, st := applyStaged(p)
	if action != Continue {
		t.Fatalf("Expectation when Verify Failed Continue,get %v", action)
	}
	if !st.FailedStage {
		t.Error("Status should be marked as FailedStage")
	}
	if !strings.Contains(readAll(t, p.Current), "old") {
		t.Fatal("The current version must not be used when verification failed")
	}
	if _, err := os.Stat(p.New); !os.IsNotExist(err) {
		t.Error("temporary update file should be removed")
	}
}

func TestSwapOverwritesPreviousBackup(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	fakeBin(t, p.Current, "v2", 0)
	fakeBin(t, p.Old, "v1", 0) // Backup from previous upgrades
	stage(t, p, "v3", 0)

	if err := swap(p); err != nil {
		t.Fatalf("swap: %v", err)
	}
	if !strings.Contains(readAll(t, p.Current), "v3") {
		t.Error("Should be replaced with v3")
	}
	if !strings.Contains(readAll(t, p.Old), "v2") {
		t.Error("Backup should be updated to just changed v2")
	}
}

func TestConfirmCountsAttemptsThenRollsBack(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	fakeBin(t, p.Current, "broken-new", 0)
	fakeBin(t, p.Old, "good-old", 0)
	m := marker{From: "0.3.7", To: "0.3.8"}

	// Previous maxAttempts The only cumulative number of startups gives the new edition a chance to stand on its own. Steady..
	for i := 1; i <= maxAttempts; i++ {
		action, st := confirmOrRollback(p, m)
		if action != Continue {
			t.Fatalf("No. %d A few attempts at expectations Continue,get %v", i, action)
		}
		if !st.Pending {
			t.Errorf("No. %d The second attempt should read Pending", i)
		}
		got, ok := readMarker(p.Marker)
		if !ok || got.Attempts != i {
			t.Fatalf("No. %d After a few attempts attempts=%d(ok=%v),Expectations %d", i, got.Attempts, ok, i)
		}
		m = got
	}

	// One more trip and you're out of bounds..
	action, st := confirmOrRollback(p, m)
	if action != Restart {
		t.Fatalf("Expectations when exceeded Restart,get %v", action)
	}
	if !st.RolledBack {
		t.Error("Status should be marked as RolledBack")
	}
	if !strings.Contains(readAll(t, p.Current), "good-old") {
		t.Fatal("It should be rolling back to the old version.")
	}
	if _, err := os.Stat(p.Marker); !os.IsNotExist(err) {
		t.Error("Back-rolling tags should be cleared, or they'll roll back indefinitely.")
	}
	// The version that doesn't get up is kept for a check, not deleted..
	if _, err := os.Stat(p.Current + ".failed"); err != nil {
		t.Error("The failed version should be retained as .failed For screening.")
	}
}

func TestManualRollbackIsReversible(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	fakeBin(t, p.Current, "v2", 0)
	fakeBin(t, p.Old, "v1", 0)

	// Rollback() Go ResolvePaths(),This is where the syntax of the bottom is directly measured..
	tmp := p.Current + ".swap"
	if err := os.Rename(p.Current, tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(p.Old, p.Current); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, p.Old); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readAll(t, p.Current), "v1") {
		t.Error("The current version after rolling back should read v1")
	}
	if !strings.Contains(readAll(t, p.Old), "v2") {
		t.Error("Backup should become v2,That way we can roll back.")
	}
}

func TestParseSums(t *testing.T) {
	const (
		linuxSum = "1111111111111111111111111111111111111111111111111111111111111111"
		winSum   = "ABCDEF0000000000000000000000000000000000000000000000000000000000"
	)
	// sha256sum Output is bispaced;shasum -a 256 Filename given in binary mode Add *.
	raw := linuxSum + "  artex-0.3.8-linux-amd64.zip\n" +
		winSum + " *artex-0.3.8-windows-amd64.zip\n" +
		"\n" +
		"garbage line\n" + // Just two fields, but the first is not a summary
		"deadbeef  artex-0.3.8-darwin-arm64.zip\n" // Summary length is not correct

	out := parseSums(raw)
	if out["artex-0.3.8-linux-amd64.zip"] != linuxSum {
		t.Errorf("linux Entry parsing error: %v", out)
	}
	// Summarize lowercases, not miscalculated by case..
	if got := out["artex-0.3.8-windows-amd64.zip"]; got != strings.ToLower(winSum) {
		t.Errorf("windows Entry Error(* Prefix should be removed, summary should be downwritten): %q", got)
	}
	if len(out) != 2 {
		t.Errorf("Empty lines, non-summarized lines and lines of incorrect length should be ignored and obtained %v", out)
	}
}

func TestExtractBinaryFindsNestedEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("The name in the bag is Windows Top artex.exe,Use this example by Unix Named Structure")
	}
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "release.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	// Structure of the real release package:artex-<version>-<os>-<arch>/artex,Plus a few jamming files..
	for name, body := range map[string]string{
		"artex-0.3.8-linux-amd64/README.md":           "readme",
		"artex-0.3.8-linux-amd64/skills/a.md":         "skill",
		"artex-0.3.8-linux-amd64/artex":               "#!/bin/sh\nexit 0\n",
		"artex-0.3.8-linux-amd64/config.example.json": "{}",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dst := filepath.Join(dir, "out")
	if err := extractBinary(zipPath, dst); err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if got := readAll(t, dst); !strings.Contains(got, "exit 0") {
		t.Errorf("It's not a relief. artex Executable: %q", got)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("The depressed binary must be accompanied by an execution slot")
	}
}

func TestExtractBinaryMissingEntry(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "release.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("artex-0.3.8-linux-amd64/README.md")
	_, _ = w.Write([]byte("readme"))
	_ = zw.Close()
	f.Close()

	if err := extractBinary(zipPath, filepath.Join(dir, "out")); err == nil {
		t.Fatal("Mistake when there is no enforceable document in the bag")
	}
}

func TestCheckURLRejectsNonGitHub(t *testing.T) {
	bad := []string{
		"http://github.com/x",           // Not HTTPS
		"https://evil.com/artex.zip",    // Domain names are not on the white list
		"https://github.com.evil.com/x", // Post-fix disguise
		"https://raw.githubusercontent.com.evil.com/x",
	}
	for _, raw := range bad {
		u := mustParse(t, raw)
		if err := checkURL(u); err == nil {
			t.Errorf("checkURL(%q) It should be rejected.", raw)
		}
	}
	good := []string{
		"https://api.github.com/repos/x/releases/latest",
		"https://objects.githubusercontent.com/blah",
		"https://GitHub.com/x", // Domain Name Case Insensitive
	}
	for _, raw := range good {
		u := mustParse(t, raw)
		if err := checkURL(u); err != nil {
			t.Errorf("checkURL(%q) You should let go, but you're wrong.: %v", raw, err)
		}
	}
}

func TestAssetNameMatchesBuildScript(t *testing.T) {
	// build.sh of package_binary Yes. artex-<version>-<os>-<arch>.zip,and version number
	// It's gone. v Prefix. There's one character wrong here. All platform updates will never find an asset..
	if got := AssetName("v0.3.8", "linux", "amd64"); got != "artex-0.3.8-linux-amd64.zip" {
		t.Errorf("AssetName = %q", got)
	}
	if got := AssetName("0.3.8", "windows", "amd64"); got != "artex-0.3.8-windows-amd64.zip" {
		t.Errorf("AssetName = %q", got)
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("Analysis %q: %v", raw, err)
	}
	return u
}

func TestSettleClearsMarkerAndStopsRollback(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	fakeBin(t, p.Current, "new", 0)
	fakeBin(t, p.Old, "old", 0)
	if err := writeMarker(p.Marker, marker{From: "0.3.7", To: "0.3.8", Attempts: 2}); err != nil {
		t.Fatal(err)
	}

	settle(p)

	if _, err := os.Stat(p.Marker); !os.IsNotExist(err) {
		t.Fatal("Confirm that the post-stabilization upgrade mark must be removed.")
	}
	// The tags are gone, the next normal reboot will not accumulate and will not trigger the rollback by mistake..
	if _, ok := readMarker(p.Marker); ok {
		t.Error("Tag Read Should Failed")
	}
	// Keep the backup. Users can roll back manually..
	if _, err := os.Stat(p.Old); err != nil {
		t.Error("The previous version of the backup should be retained after stabilization")
	}
}

func TestSettleIsNoopWithoutMarker(t *testing.T) {
	requireUnix(t)
	p := testPaths(t)
	fakeBin(t, p.Current, "cur", 0)
	settle(p) // Normal start path, shouldn't. panic I shouldn't have moved any papers.
	if _, err := os.Stat(p.Current); err != nil {
		t.Error("When not marked settle No documents should be affected.")
	}
}
