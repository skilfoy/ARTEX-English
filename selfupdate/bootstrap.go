package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// smokeEnv Let the subprocess pulled from the smoke test skip directly. Bootstrap.
//
// Strictly speaking, nothing happens: a little process. os.Executable() Yes artex.new,That's what I figured out.
// All paths. .new Prefix, no real upgrade file. But relying on such coincidences is too fragile.,
// It's a visible short circuit, and it's a useless disk detection..
const smokeEnv = "ARTEX_SELFUPDATE_SMOKE"

// Action Yes Bootstrap Give main Other Organiser.
type Action int

const (
	// Continue:Start as usual. server.
	Continue Action = iota
	// Restart:Now. ExitRestart Quit, let the guard script reboot..
	Restart
)

// State Describes the status of the upgrade at this start-up for /api/update/check Just tell the front end.
// "Did the last promotion succeed or got rolled back?".
type State struct {
	Pending     bool   // It's not stable yet.
	RolledBack  bool   // This launch has just been automatically rolled back
	FailedStage bool   // Validation of temporary memory/The smoke didn't pass, it was abandoned.
	Detail      string // A user-oriented statement
}

// Bootstrap at main run at the beginning of any listening port and open the database.
//
// Three scenarios.:
//
//	① Other Organiser artex.new  → Verification + Smoke, change through and ask to be restarted; abandon or continue running the old edition
//	② Only tag files left.          → It means you've just changed, you've made a cumulative attempt; you can roll back if you fail more than once.
//	③ Nothing.            → Normal startup
func Bootstrap() (Action, State) {
	if os.Getenv(smokeEnv) != "" {
		return Continue, State{}
	}
	p, err := ResolvePaths()
	if err != nil {
		log.Printf("[update] Skipping the ego.:%v", err)
		return Continue, State{}
	}

	if _, err := os.Stat(p.New); err == nil {
		return applyStaged(p)
	}

	m, ok := readMarker(p.Marker)
	if !ok {
		return Continue, State{}
	}
	return confirmOrRollback(p, m)
}

// applyStaged Processing"Other Organiser"Situation: Reload with verification and discard..
//
// This is the only place the whole upgrade chain will cover the enforceable documents, and the last gate.——Smoke test.
// Questions such as download damage, faulty structure, missing dynamic links. Once you let go of a binary that can't run,,
// Watching the script will pull it up again and again, without fatigue. Go The code doesn't have a chance to run, so there's no automatic rollback..
func applyStaged(p Paths) (Action, State) {
	m, _ := readMarker(p.Marker)

	if err := verifyStaged(p); err != nil {
		log.Printf("[update] Suspended new version not verified, discarded, continuing current version:%v", err)
		cleanStaged(p)
		_ = os.Remove(p.Marker)
		return Continue, State{FailedStage: true, Detail: "New version verification failed, discarded:" + err.Error()}
	}

	if err := swap(p); err != nil {
		log.Printf("[update] Reload failed, continuing current version:%v", err)
		cleanStaged(p)
		_ = os.Remove(p.Marker)
		return Continue, State{FailedStage: true, Detail: "Change failed:" + err.Error()}
	}

	// Changed. Keep the tags and give them to the next start..
	m.Attempts = 0
	if m.StagedAt == 0 {
		m.StagedAt = time.Now().Unix()
	}
	if err := writeMarker(p.Marker, m); err != nil {
		log.Printf("[update] failed to write upgrade marker; automatic rollback is unavailable:%v", err)
	}
	log.Printf("[update] Changed to %s,Exit to Restart(exit %d)", orUnknown(m.To), ExitRestart)
	return Restart, State{Pending: true}
}

// confirmOrRollback Processing"Start after reloading":The cumulative number of attempts, the limit is to replace the old version..
//
// Count only Go The code runs up and up, so it covers..."Can execute but crash at initialization"
// (Configuration incompatible, port occupied,DB It's not working.;"Not at all. exec" From before the reload.
// The smoke test stops. It's both complete..
func confirmOrRollback(p Paths, m marker) (Action, State) {
	m.Attempts++
	if m.Attempts > maxAttempts {
		if err := rollback(p); err != nil {
			// If the rollback fails, don't start again, or you'll be caught in an infinite reboot. Clear the mark.,
			// Let the process proceed as it is.——If you can't get up, the user can at least see the reason in the log..
			log.Printf("[update] New version continuous %d Starter failed and rollback failed:%v", maxAttempts, err)
			_ = os.Remove(p.Marker)
			return Continue, State{Detail: "New version failed to start and roll back failed:" + err.Error()}
		}
		log.Printf("[update] New version continuous %d Once started failed, rolling back to %s,Exit to Restart(exit %d)",
			maxAttempts, orUnknown(m.From), ExitRestart)
		_ = os.Remove(p.Marker)
		return Restart, State{RolledBack: true, Detail: fmt.Sprintf("New version failed to start, rolling back to %s", orUnknown(m.From))}
	}
	if err := writeMarker(p.Marker, m); err != nil {
		log.Printf("[update] failed to update upgrade marker:%v", err)
	}
	log.Printf("[update] New version under launch (No. %d/%d Stable operation will confirm upgrade",
		m.Attempts, maxAttempts)
	return Continue, State{Pending: true}
}

// Settle Confirm that the new version has stabilized and clears the upgrade tag.
//
// By main at HTTP It sounds like a delay in calling: it's not counted until it's alive, otherwise the mark stays where it is.,
// Next start continues cumulatively until you trigger rollback.
func Settle() {
	p, err := ResolvePaths()
	if err != nil {
		return
	}
	settle(p)
}

func settle(p Paths) {
	if _, ok := readMarker(p.Marker); !ok {
		return // It's not an upgraded start. There's nothing to do.
	}
	if err := os.Remove(p.Marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("[update] failed to clear upgrade marker:%v", err)
		return
	}
	log.Printf("[update] New version running stable, upgrade completed (previous version retained as %s)", p.Old)
}

// SettleDelay It's a verdict."The new version survived."Time of running required.
const SettleDelay = 30 * time.Second

// verifyStaged Verify suspense: Match first SHA256,Pull it up and run again..
func verifyStaged(p Paths) error {
	want, err := os.ReadFile(p.Sum)
	if err != nil {
		return fmt.Errorf("Read checksum: %w", err)
	}
	got, err := fileSHA256(p.New)
	if err != nil {
		return fmt.Errorf("Calculate checksum: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(string(want)), got) {
		return errors.New("SHA256 Not matching (download damaged or tampered))")
	}
	return smokeTest(p.New)
}

// smokeTest Use -h Pull up the new binary to make sure it's actually operational on the current system..
// It'll block the download cut, the structure is wrong.(exec format error),Lack of dependence, etc..
func smokeTest(bin string) error {
	if err := os.Chmod(bin, 0o755); err != nil {
		return fmt.Errorf("Grant enforcement powers: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-h")
	cmd.Env = append(os.Environ(), smokeEnv+"=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return errors.New("Smoke test timed out. Response)")
	}
	if err != nil {
		snippet := strings.TrimSpace(string(out))
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		return fmt.Errorf("The smoke test failed.: %v: %s", err, snippet)
	}
	return nil
}

// swap Convert the current binary to a pending new version.
//
// Unix and Windows Allow rename A running executable(Windows What is prohibited is deletion and
// override,rename It's not in it, so there's no sub-platform here and no need to stop yourself..
func swap(p Paths) error {
	// Windows of rename It doesn't cover what already exists. .old We have to clear it first..
	if err := os.Remove(p.Old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Clear old backup %s: %w", p.Old, err)
	}
	if err := os.Rename(p.Current, p.Old); err != nil {
		return fmt.Errorf("Backup Current Version: %w", err)
	}
	if err := os.Rename(p.New, p.Current); err != nil {
		// The reload failed but the current version has been removed and must be put back as it is, otherwise there is no enforceable document for the next start.
		if rerr := os.Rename(p.Old, p.Current); rerr != nil {
			return fmt.Errorf("Failed to load new version(%v),Failed to restore current version: %w", err, rerr)
		}
		return fmt.Errorf("Load new version: %w", err)
	}
	_ = os.Remove(p.Sum)
	return nil
}

// rollback handle swap Change the old version back up..
func rollback(p Paths) error {
	if _, err := os.Stat(p.Old); err != nil {
		return fmt.Errorf("No backup to roll back %s: %w", p.Old, err)
	}
	// Move a new edition that doesn't work. .failed Save it for a check, not just delete it..
	failed := p.Current + ".failed"
	_ = os.Remove(failed)
	if err := os.Rename(p.Current, failed); err != nil {
		return fmt.Errorf("Remove failed version: %w", err)
	}
	if err := os.Rename(p.Old, p.Current); err != nil {
		return fmt.Errorf("Restore old version: %w", err)
	}
	return nil
}

// Rollback Yes /api/update/rollback Achieved: voluntary return of previous version.
// Just reload it and restart it to the Guardian Script. ExitRestart Exit).
func Rollback() error {
	p, err := ResolvePaths()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p.Old); err != nil {
		return errors.New("No previous version to roll back(" + p.Old + " does not exist)")
	}
	cleanStaged(p)
	if err := smokeTest(p.Old); err != nil {
		return fmt.Errorf("Could not execute previous version. Refuse rollback: %w", err)
	}
	// Exchange current and backup: roll back and roll back.
	tmp := p.Current + ".swap"
	_ = os.Remove(tmp)
	if err := os.Rename(p.Current, tmp); err != nil {
		return fmt.Errorf("Remove current version: %w", err)
	}
	if err := os.Rename(p.Old, p.Current); err != nil {
		_ = os.Rename(tmp, p.Current)
		return fmt.Errorf("Load previous version: %w", err)
	}
	if err := os.Rename(tmp, p.Old); err != nil {
		log.Printf("[update] failed to clean up the previous backup; the current process is unaffected:%v", err)
	}
	_ = os.Remove(p.Marker)
	return nil
}

// HasBackup Whether there is a previous version of the report that can be rolled back for the front to decide whether to show the rollback button.
func HasBackup() bool {
	p, err := ResolvePaths()
	if err != nil {
		return false
	}
	_, err = os.Stat(p.Old)
	return err == nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "Unknown version"
	}
	return s
}
