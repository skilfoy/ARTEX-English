// Package selfupdate implements ARTEX . Page 1 update from: GitHub Release Draw new editions
// Binary, validation, temporary storage and atom replacement at next startup.
//
// Overall division of labour (see start.sh / start.bat):
//
//	Start Script  = Fools guard the loop, only responsible."After process exits, decide whether to pull again by exit code"
//	This bag.      = All error-prone logic (downloading) / SHA256 Verification / Smoke. / Change up / Failed Rollback)
//
// It's because of the change. Go Not in the script because... sha256 Checking and smoking tests. sh and bat on
// Two sets to write.(sha256sum / shasum / certutil),And that's exactly what makes the most of mistakes.——Change one.
// The binary that can't run, the dæmon will pull it up faithfully and repeatedly. Help!.
//
// One complete upgrade was initiated by three processes:
//
//	① Old version server Received /api/update/apply → Download Verification → Pending artex.new → exit 75
//	② Scripts to reboot old versions → Bootstrap Discover artex.new → Verification+Smoke. → Change up → exit 75
//	③ Scripts are back up. It's new. → Bootstrap One try. → Clear tag after start successful
//
// Any failure returns the old version.:② If you can't verify, delete the temporarys and keep running the old ones.;③ Continuous 3 He didn't make it.
// Clear the markers, and automatically put it. artex.old Change it back..
package selfupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ExitRestart Yes"Please protect me."Exit code(EX_TEMPFAIL).Start the script and see it.
// We'll run again, not counting the crash..0 This means that the user stops normally (script exits cycle), the rest is considered to be collapse.
const ExitRestart = 75

// maxAttempts is the number of start-up attempts allowed after reloading. The new edition counts every time it starts. +1,Live.
// settleDelay clears the markings; crashes maxAttempts The new edition doesn't come up..
const maxAttempts = 3

// Paths It's all the files involved in a single upgrade.**Directory of Executable Files**Down.
// I don't want to. CWD:The server runtime directory may be / Or any path, with CWD I'll let the temporary deposit come down.
// Somewhere else, the reloading logic simply lapses..
type Paths struct {
	Dir     string // Directory of Executable Files
	Current string // Current running binary        artex      / artex.exe
	New     string // Saved New Version            artex.new  / artex.new.exe
	Sum     string // New version sha256(hex)  artex.new.sha256 / artex.new.exe.sha256
	Old     string // Change old version of the backup before loading      artex.old  / artex.old.exe
	Marker  string // Upgrade Status Marker            artex.upgrade.json
}

// ResolvePaths Path all upgrades to the current executable.
//
// Windows on .new/.old It has to be. .exe The suffix, or the smoke test and the refitting will fail.,
// So take off the suffix and spell it. The name of the two platforms is symmetrical..
func ResolvePaths() (Paths, error) {
	exe, err := os.Executable()
	if err != nil {
		return Paths{}, fmt.Errorf("Position Executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	name := filepath.Base(exe)
	ext := filepath.Ext(name) // Windows Top ".exe",Unix It's usually empty.
	stem := strings.TrimSuffix(name, ext)

	join := func(suffix string) string { return filepath.Join(dir, stem+suffix+ext) }
	return Paths{
		Dir:     dir,
		Current: exe,
		New:     join(".new"),
		Sum:     join(".new") + ".sha256",
		Old:     join(".old"),
		Marker:  filepath.Join(dir, stem+".upgrade.json"),
	}, nil
}

// marker Record the progress of a change of clothes to trigger automatic rollback when the new edition does not come.
type marker struct {
	From     string `json:"from"`     // Pre-upgrade version
	To       string `json:"to"`       // Target Version
	Attempts int    `json:"attempts"` // Number of attempts to start after reloading
	StagedAt int64  `json:"staged_at"`
}

func readMarker(path string) (marker, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return marker{}, false
	}
	var m marker
	if json.Unmarshal(b, &m) != nil {
		return marker{}, false
	}
	return m, true
}

func writeMarker(path string, m marker) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// cleanStaged Other Organiser The reload was successful, the check failed, the user canceled it, and the rest was avoided.
// artex.new Try again at next startup.
func cleanStaged(p Paths) {
	_ = os.Remove(p.New)
	_ = os.Remove(p.Sum)
}

// CompareVersions Compare two versions, return -1/0/1(a<b / a==b / a>b).
// ok=false Means that at least one side is not a comparable version number (e.g. locally developed builder) "dev" or
// git describe Outputs "0.3.7-2-gabc1234-dirty"),The caller should disable one key update at this time,
// Or we'll put the building under development."Upgrade"Formalize, cover unsubmitted changes.
func CompareVersions(a, b string) (int, bool) {
	av, aok := parseVersion(a)
	bv, bok := parseVersion(b)
	if !aok || !bok {
		return 0, false
	}
	for i := range 3 {
		if av[i] != bv[i] {
			if av[i] < bv[i] {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

// parseVersion Analysis "v0.3.7" / "0.3.7" Version number in form [3]int.
//
// Only a pure three-part pattern is accepted:build.sh In Africa tag Use for construction git describe output
// "0.3.7-2-gabc1234" These suffixed versions must be judged non-comparison rather than as
// 0.3.7 —— Otherwise, the construction will be miscalculated."It's the latest."Or covered by official editions.
func parseVersion(s string) ([3]int, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return [3]int{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// InDocker Is the reporting process in the container?.Docker The next reload is written on the packaging.,
// `docker compose up -d` Rebuild the container will return the version of the mirror itself.——It's an expected act.
// (But the front end needs to be clear..
func InDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	b, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "docker") || strings.Contains(s, "containerd")
}
