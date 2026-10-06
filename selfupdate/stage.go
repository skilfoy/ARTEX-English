package selfupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

// sumsAsset Yes release.yml Sum list generated, overwrite Release All of it. zip.
const sumsAsset = "SHA256SUMS"

// maxBinarySize To limit the volume of the binary from the depression and prevent malformations. zip Fill the disk..
const maxBinarySize = 512 << 20 // 512 MiB

// Phase It's a phase in the upgrade process and it's used directly. SSE From the event phase Field.
type Phase string

const (
	PhaseIdle     Phase = "idle"
	PhaseDownload Phase = "downloading"
	PhaseVerify   Phase = "verifying"
	PhaseExtract  Phase = "extracting"
	PhaseStaged   Phase = "staged"
	PhaseFailed   Phase = "failed"
)

// Progress By the caller to push progress to the front.pct It makes sense only at the download stage.(0-100),
// For the rest of the period -1.
type Progress func(ph Phase, pct int, msg string)

// Stage Download Assign Release Current platform release package, check and save the new binary as artex.new.
//
// It's complete. zip Not nudity binary for two reasons: existing Release of SHA256SUMS Yes.
// Overwrite Only zip,Go zip No need to change. CI,It's compatible with the historical version that's published.;zip It's still in there.
// skills/,Synchronization for the Future skill Keep your mouth shut. It's just a lot of downloads. skills The hundreds. KB.
//
// Function returns as the temporary saving is completed and the caller then closes with grace and ExitRestart Exit.
func Stage(ctx context.Context, c *http.Client, rel *Release, currentVersion string, prog Progress) error {
	if prog == nil {
		prog = func(Phase, int, string) {}
	}
	p, err := ResolvePaths()
	if err != nil {
		return err
	}
	if err := checkWritable(p.Dir); err != nil {
		return err
	}

	name := AssetName(rel.TagName, runtime.GOOS, runtime.GOARCH)
	asset, ok := rel.FindAsset(name)
	if !ok {
		return fmt.Errorf("This version is not available %s/%s release package (missing %s)", runtime.GOOS, runtime.GOARCH, name)
	}

	prog(PhaseDownload, 0, "Access checksum list…")
	sums, err := fetchSums(ctx, c, rel)
	if err != nil {
		return err
	}
	want, ok := sums[name]
	if !ok {
		return fmt.Errorf("%s Unrecorded %s,Refuse installation of uncertified binary", sumsAsset, name)
	}

	// The temporary files are all in the target directory. rename It's an atomic operation in the same file system.
	// (Cross-equipment rename It'll fail, and... /tmp Often standalone mount point).
	zipPath := p.New + ".zip.part"
	binPath := p.New + ".part"
	defer func() {
		_ = os.Remove(zipPath)
		_ = os.Remove(binPath)
	}()

	prog(PhaseDownload, 0, fmt.Sprintf("Download %s(%s)…", name, humanSize(asset.Size)))
	got, err := download(ctx, c, asset, zipPath, prog)
	if err != nil {
		return err
	}

	prog(PhaseVerify, -1, "Verification SHA256…")
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("SHA256 Not matching: expectations %s,Actual %s(Download corrupted or tampered)", short(want), short(got))
	}

	prog(PhaseExtract, -1, "Unpressure and smoke tests…")
	if err := extractBinary(zipPath, binPath); err != nil {
		return err
	}
	if err := smokeTest(binPath); err != nil {
		return fmt.Errorf("The new version cannot be run on the current system: %w", err)
	}

	// Save Yourself sha256 Save a single one: check again before next reloading starts.,
	// Prevent documents from being altered or broken during the period between temporary saving and restart.
	binSum, err := fileSHA256(binPath)
	if err != nil {
		return fmt.Errorf("Calculates the sum of the new binary: %w", err)
	}
	if err := os.WriteFile(p.Sum, []byte(binSum), 0o644); err != nil {
		return fmt.Errorf("Write Checksum: %w", err)
	}
	if err := os.Rename(binPath, p.New); err != nil {
		_ = os.Remove(p.Sum)
		return fmt.Errorf("Save New Version: %w", err)
	}

	if err := writeMarker(p.Marker, marker{
		From:     currentVersion,
		To:       strings.TrimPrefix(rel.TagName, "v"),
		StagedAt: time.Now().Unix(),
	}); err != nil {
		// The tag only affects the automatic rollback capacity, and the temporary deposit itself is in place and does not interrupt the upgrade.
		prog(PhaseStaged, -1, "Warning: Writing the upgrade tag failed and this upgrade will not have automatic rollback protection")
	}

	prog(PhaseStaged, 100, "New version ready, restarting.…")
	return nil
}

// fetchSums Download and parse SHA256SUMS,Return filename → Hexadecimal Summary.
func fetchSums(ctx context.Context, c *http.Client, rel *Release) (map[string]string, error) {
	asset, ok := rel.FindAsset(sumsAsset)
	if !ok {
		return nil, fmt.Errorf("The Release No %s,Could not verify completeness. Deny upgrade", sumsAsset)
	}
	body, err := get(ctx, c, asset.URL)
	if err != nil {
		return nil, fmt.Errorf("Download %s: %w", sumsAsset, err)
	}
	defer body.Close()

	raw, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("Read %s: %w", sumsAsset, err)
	}
	out := parseSums(string(raw))
	if len(out) == 0 {
		return nil, fmt.Errorf("%s Content is empty or format unrecognized", sumsAsset)
	}
	return out, nil
}

// parseSums Analysis sha256sum Style List, return file First Name → Hexadecimal Summary.
//
// The first field must be 64 Hexadecimal for recording. Press Only"Just two fields"It's not enough.——
// The description of any line or two words is used as a valid entry and the value of the trash is inserted in the summary table,
// Instead, real assets could match the wrong summary..
func parseSums(raw string) map[string]string {
	out := map[string]string{}
	for line := range strings.Lines(raw) {
		// Format As "<sha256>  <filename>"(sha256sum Double Space;shasum Binary
		// Modes add file names * Prefix).
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 || !isHexSHA256(fields[0]) {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == "" {
			continue
		}
		out[name] = strings.ToLower(fields[0])
	}
	return out
}

func isHexSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// download Write assets dst,Compute simultaneously SHA256 and press Content-Length Reporting on progress.
func download(ctx context.Context, c *http.Client, a Asset, dst string, prog Progress) (string, error) {
	body, err := get(ctx, c, a.URL)
	if err != nil {
		return "", fmt.Errorf("Download %s: %w", a.Name, err)
	}
	defer body.Close()

	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("Create temporary file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	pw := &progressWriter{total: a.Size, prog: prog, name: a.Name, last: time.Now()}
	if _, err := io.Copy(io.MultiWriter(f, h, pw), body); err != nil {
		return "", fmt.Errorf("Download aborted: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("Crash Failed: %w", err)
	}
	if a.Size > 0 && pw.written != a.Size {
		return "", fmt.Errorf("Download incomplete: expectation %d bytes, actual %d Bytes", a.Size, pw.written)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// get Start a white list. GET,Return Response.
func get(ctx context.Context, c *http.Client, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if err := checkURL(req.URL); err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "artex-selfupdate")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// extractBinary Remove from the release package artex Executable.
//
// The structure in the bag is... artex-<version>-<os>-<arch>/artex,But here.**Base Name**Match instead of spell-out
// Path: Version number appears once in the package name, the entire upgrade fails with a misspelled character, more durable by base name.
func extractBinary(zipPath, dst string) error {
	want := "artex"
	if runtime.GOOS == "windows" {
		want = "artex.exe"
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("Open Release Package: %w", err)
	}
	defer zr.Close()

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() || !strings.EqualFold(path.Base(entry.Name), want) {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return fmt.Errorf("Read %s: %w", entry.Name, err)
		}
		defer rc.Close()

		f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return fmt.Errorf("Write New Binary: %w", err)
		}
		defer f.Close()

		n, err := io.Copy(f, io.LimitReader(rc, maxBinarySize+1))
		if err != nil {
			return fmt.Errorf("Unzip %s: %w", entry.Name, err)
		}
		if n > maxBinarySize {
			return fmt.Errorf("Release the executable in the package more than %s,Deny depression.", humanSize(maxBinarySize))
		}
		if n == 0 {
			return fmt.Errorf("Release Package %s It's empty.", want)
		}
		return f.Sync()
	}
	return fmt.Errorf("It's not in the release bag. %s", want)
}

// checkWritable The catalogue is written in advance. There's no such thing as this. root Run, or binary is placed in the system
// The directories will be downloaded in dozens. MB And then I lost the moment I changed..
func checkWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".artex-update-probe-*")
	if err != nil {
		return fmt.Errorf("Program Directory %s Not written, cannot automatically update (check permissions or move to manual upgrade)): %w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// progressWriter Statistics written bytes and limited frequency reporting, avoiding each 32KiB I'll push one piece. SSE.
type progressWriter struct {
	total   int64
	written int64
	name    string
	prog    Progress
	last    time.Time
}

func (w *progressWriter) Write(b []byte) (int, error) {
	w.written += int64(len(b))
	if time.Since(w.last) < 300*time.Millisecond {
		return len(b), nil
	}
	w.last = time.Now()
	pct := -1
	if w.total > 0 {
		pct = int(w.written * 100 / w.total)
	}
	w.prog(PhaseDownload, pct, fmt.Sprintf("Downloading %s / %s", humanSize(w.written), humanSize(w.total)))
	return len(b), nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func short(sum string) string {
	if len(sum) > 12 {
		return sum[:12] + "…"
	}
	return sum
}
