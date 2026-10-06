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

// smokeEnv makes the subprocess started by the smoke test skip Bootstrap.
//
// Strictly speaking it would still be safe without this: the child's
// os.Executable() is artex.new, so every derived path carries a .new prefix and
// cannot touch the real upgrade files. Relying on that coincidence is fragile.
// An explicit short-circuit is obvious, and it skips a useless disk probe.
const smokeEnv = "ARTEX_SELFUPDATE_SMOKE"

// Action is the instruction Bootstrap returns to main.
type Action int

const (
	// Continue starts the server as usual.
	Continue Action = iota
	// Restart exits immediately with ExitRestart so the supervisor relaunches.
	Restart
)

// State describes the upgrade status at this start, so /api/update/check can
// tell the frontend whether the last upgrade succeeded or was rolled back.
type State struct {
	Pending     bool   // swapped, but not yet confirmed stable
	RolledBack  bool   // this start just performed an automatic rollback
	FailedStage bool   // staged file failed verification or the smoke test and was discarded
	Detail      string // one sentence for the user
}

// Bootstrap runs at the very start of main, before any port is bound or any database is opened.
//
// Three situations:
//
//	① a staged artex.new exists → verify + smoke; swap and request a restart on success, otherwise discard it and keep the old binary
//	② only the marker file remains → a swap just happened; count one attempt, and roll back after enough consecutive failures
//	③ neither exists → normal start
func Bootstrap() (Action, State) {
	if os.Getenv(smokeEnv) != "" {
		return Continue, State{}
	}
	p, err := ResolvePaths()
	if err != nil {
		log.Printf("[update] skipping bootstrap: %v", err)
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

// applyStaged handles "a staged file exists": swap if verification passes, otherwise discard it.
//
// This is the only place in the upgrade path that overwrites the executable, and
// the last gate. The smoke test catches a truncated download, the wrong
// architecture, or a missing dynamic library. Once a binary that cannot run is
// installed, the supervisor relaunches it forever, the Go code never gets a
// chance to run, and automatic rollback is impossible.
func applyStaged(p Paths) (Action, State) {
	m, _ := readMarker(p.Marker)

	if err := verifyStaged(p); err != nil {
		log.Printf("[update] staged new version failed verification and was discarded; continuing the current version: %v", err)
		cleanStaged(p)
		_ = os.Remove(p.Marker)
		return Continue, State{FailedStage: true, Detail: "new version failed verification and was discarded: " + err.Error()}
	}

	if err := swap(p); err != nil {
		log.Printf("[update] swap failed; continuing the current version: %v", err)
		cleanStaged(p)
		_ = os.Remove(p.Marker)
		return Continue, State{FailedStage: true, Detail: "swap failed: " + err.Error()}
	}

	// Swap succeeded. Keep the marker for the next start (which runs the new binary) to confirm stability.
	m.Attempts = 0
	if m.StagedAt == 0 {
		m.StagedAt = time.Now().Unix()
	}
	if err := writeMarker(p.Marker, m); err != nil {
		log.Printf("[update] failed to write upgrade marker; automatic rollback is unavailable: %v", err)
	}
	log.Printf("[update] swapped to %s; exiting to restart (exit %d)", orUnknown(m.To), ExitRestart)
	return Restart, State{Pending: true}
}

// confirmOrRollback handles a start after a swap: count the attempt, and put the old binary back if the limit is exceeded.
//
// The count only increases once Go code is running, so it covers "executable,
// but crashes during init" (incompatible config, port in use, database migration
// failure). "Cannot exec at all" is stopped by the smoke test before the swap.
// The two together are the complete check.
func confirmOrRollback(p Paths, m marker) (Action, State) {
	m.Attempts++
	if m.Attempts > maxAttempts {
		if err := rollback(p); err != nil {
			// Do not restart again if rollback itself failed, or the process loops forever.
			// Clear the marker and continue as-is — if it still cannot start, the user at least sees why in the log.
			log.Printf("[update] new version failed to start %d times in a row, and rollback failed: %v", maxAttempts, err)
			_ = os.Remove(p.Marker)
			return Continue, State{Detail: "new version failed to start and rollback failed: " + err.Error()}
		}
		log.Printf("[update] new version failed to start %d times in a row; rolled back to %s; exiting to restart (exit %d)",
			maxAttempts, orUnknown(m.From), ExitRestart)
		_ = os.Remove(p.Marker)
		return Restart, State{RolledBack: true, Detail: fmt.Sprintf("new version failed to start; rolled back to %s", orUnknown(m.From))}
	}
	if err := writeMarker(p.Marker, m); err != nil {
		log.Printf("[update] failed to update upgrade marker: %v", err)
	}
	log.Printf("[update] new version starting (attempt %d/%d); the upgrade is confirmed once it stays up",
		m.Attempts, maxAttempts)
	return Continue, State{Pending: true}
}

// Settle confirms that the new version has stayed up and clears the upgrade marker.
//
// main calls this after a delay once HTTP is listening: only surviving that
// long counts. Otherwise the marker stays, and the next start keeps counting
// attempts until rollback.
func Settle() {
	p, err := ResolvePaths()
	if err != nil {
		return
	}
	settle(p)
}

func settle(p Paths) {
	if _, ok := readMarker(p.Marker); !ok {
		return // not a post-upgrade start; nothing to do
	}
	if err := os.Remove(p.Marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("[update] failed to clear upgrade marker: %v", err)
		return
	}
	log.Printf("[update] new version is stable; upgrade complete (previous version kept as %s)", p.Old)
}

// SettleDelay is how long the new version must keep running before it counts as having survived.
const SettleDelay = 30 * time.Second

// verifyStaged checks a staged file: compare SHA256, then actually run it.
func verifyStaged(p Paths) error {
	want, err := os.ReadFile(p.Sum)
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	got, err := fileSHA256(p.New)
	if err != nil {
		return fmt.Errorf("compute checksum: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(string(want)), got) {
		return errors.New("SHA256 mismatch (download damaged or tampered with)")
	}
	return smokeTest(p.New)
}

// smokeTest starts the new binary with -h to confirm it can actually execute on this system.
// That catches a truncated download, the wrong architecture (exec format error), missing libraries, and similar failures.
func smokeTest(bin string) error {
	if err := os.Chmod(bin, 0o755); err != nil {
		return fmt.Errorf("grant execute permission: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-h")
	cmd.Env = append(os.Environ(), smokeEnv+"=1")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return errors.New("smoke test timed out (new binary did not respond)")
	}
	if err != nil {
		snippet := strings.TrimSpace(string(out))
		if len(snippet) > 300 {
			snippet = snippet[:300] + "…"
		}
		return fmt.Errorf("smoke test failed: %v: %s", err, snippet)
	}
	return nil
}

// swap replaces the current binary with the staged new version.
//
// Both Unix and Windows allow rename of a running executable (Windows forbids
// deleting or overwriting one, but rename is allowed), so this does not need a
// per-platform path and does not need to stop itself first.
func swap(p Paths) error {
	// Windows rename does not overwrite an existing target, so a .old left by the previous upgrade must be removed first.
	if err := os.Remove(p.Old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old backup %s: %w", p.Old, err)
	}
	if err := os.Rename(p.Current, p.Old); err != nil {
		return fmt.Errorf("back up current version: %w", err)
	}
	if err := os.Rename(p.New, p.Current); err != nil {
		// The swap failed after the current version was moved aside. Put it back, or the next start has no executable.
		if rerr := os.Rename(p.Old, p.Current); rerr != nil {
			return fmt.Errorf("install new version failed (%v), and restoring the current version failed: %w", err, rerr)
		}
		return fmt.Errorf("install new version: %w", err)
	}
	_ = os.Remove(p.Sum)
	return nil
}

// rollback puts back the old version that swap backed up.
func rollback(p Paths) error {
	if _, err := os.Stat(p.Old); err != nil {
		return fmt.Errorf("no backup to roll back to %s: %w", p.Old, err)
	}
	// Move the binary that will not start to .failed for inspection instead of deleting it.
	failed := p.Current + ".failed"
	_ = os.Remove(failed)
	if err := os.Rename(p.Current, failed); err != nil {
		return fmt.Errorf("move failed version aside: %w", err)
	}
	if err := os.Rename(p.Old, p.Current); err != nil {
		return fmt.Errorf("restore old version: %w", err)
	}
	return nil
}

// Rollback implements /api/update/rollback: voluntarily return to the previous version.
// It only swaps files. The caller then exits with ExitRestart and lets the supervisor relaunch.
func Rollback() error {
	p, err := ResolvePaths()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p.Old); err != nil {
		return errors.New("no previous version to roll back to (" + p.Old + " does not exist)")
	}
	cleanStaged(p)
	if err := smokeTest(p.Old); err != nil {
		return fmt.Errorf("previous version cannot execute; refusing rollback: %w", err)
	}
	// Swap current and backup so a rollback can itself be rolled back.
	tmp := p.Current + ".swap"
	_ = os.Remove(tmp)
	if err := os.Rename(p.Current, tmp); err != nil {
		return fmt.Errorf("move current version aside: %w", err)
	}
	if err := os.Rename(p.Old, p.Current); err != nil {
		_ = os.Rename(tmp, p.Current)
		return fmt.Errorf("install previous version: %w", err)
	}
	if err := os.Rename(tmp, p.Old); err != nil {
		log.Printf("[update] failed to tidy the backup after rollback; the running process is unaffected: %v", err)
	}
	_ = os.Remove(p.Marker)
	return nil
}

// HasBackup reports whether a previous version can be rolled back to, so the frontend can show the rollback button.
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
		return "unknown version"
	}
	return s
}
