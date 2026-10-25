package program

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PatchReport is a dry run. Applied is always false.
type PatchReport struct {
	Path    string   `json:"path"`
	Files   []string `json:"files"`
	Match   bool     `json:"match"`
	Applied bool     `json:"applied"`
}

// CheckPatch reads a unified diff under .artex-patches and compares it with
// the mount. It does not apply the diff, and it does not open a pull request.
func CheckPatch(root, rel string) (PatchReport, error) {
	report := PatchReport{Path: rel, Applied: false}
	if root == "" {
		return report, fmt.Errorf("mount: no tree")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return report, err
	}
	resolved, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return report, err
	}
	rel = filepath.ToSlash(rel)
	if strings.Contains(rel, "..") || !strings.HasPrefix(rel, ".artex-patches/") || !strings.HasSuffix(rel, ".patch") {
		return report, fmt.Errorf("mount: patch must stay under .artex-patches")
	}
	dest := filepath.Join(resolved, filepath.FromSlash(rel))
	inside, err := filepath.Rel(filepath.Join(resolved, ".artex-patches"), dest)
	if err != nil || strings.HasPrefix(inside, "..") || filepath.IsAbs(inside) {
		return report, fmt.Errorf("mount: patch path escaped")
	}
	before, err := snapshotTree(resolved)
	if err != nil {
		return report, err
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		return report, err
	}
	if len(raw) > 1<<20 {
		return report, fmt.Errorf("mount: patch is too large")
	}
	text := string(raw)
	if strings.Contains(text, "GIT binary patch") || strings.Contains(text, "AKIA") || strings.Contains(text, "BEGIN PRIVATE KEY") || strings.Contains(text, "BEGIN OPENSSH PRIVATE KEY") {
		return report, fmt.Errorf("mount: patch refused")
	}
	files, err := parseUnified(text)
	if err != nil {
		return report, err
	}
	if len(files) == 0 {
		return report, fmt.Errorf("mount: patch has no files")
	}
	match := true
	for _, file := range files {
		target := file.plus
		if target == "/dev/null" {
			target = file.minus
		}
		if target == "/dev/null" {
			return report, fmt.Errorf("mount: patch has no target")
		}
		clean, err := insideMount(resolved, target)
		if err != nil {
			return report, err
		}
		report.Files = append(report.Files, clean)
		ok, err := hunksMatch(resolved, clean, file)
		if err != nil {
			return report, err
		}
		if !ok {
			match = false
		}
	}
	after, err := snapshotTree(resolved)
	if err != nil {
		return report, err
	}
	if before != after {
		return report, fmt.Errorf("mount: patch check modified the tree")
	}
	report.Match = match
	report.Applied = false
	if !match {
		return report, fmt.Errorf("mount: patch does not match the tree")
	}
	return report, nil
}

// ReviewPatch checks a patch and records the result. A mismatch is logged
// and returned. The source tree is not modified.
func (d *Desk) ReviewPatch(root, rel string) (PatchReport, error) {
	report, err := CheckPatch(root, rel)
	if err != nil {
		if len(report.Files) > 0 {
			d.Append("review-refused", map[string]string{
				"path":    rel,
				"match":   "false",
				"applied": "false",
			})
		}
		return report, err
	}
	d.Append("review", map[string]string{
		"path":    rel,
		"match":   "true",
		"applied": "false",
	})
	return report, nil
}

func insideMount(root, rel string) (string, error) {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "/dev/null" || filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return "", fmt.Errorf("mount: patch target escaped")
	}
	if strings.HasPrefix(rel, ".artex-patches/") || rel == ".artex-patches" {
		return "", fmt.Errorf("mount: patch target is the patch store")
	}
	dest := filepath.Join(root, filepath.FromSlash(rel))
	got, err := filepath.Rel(root, dest)
	if err != nil || strings.HasPrefix(got, "..") || filepath.IsAbs(got) {
		return "", fmt.Errorf("mount: patch target escaped")
	}
	return filepath.ToSlash(got), nil
}

type diffFile struct {
	minus string
	plus  string
	hunks []diffHunk
}

type diffHunk struct {
	oldStart int
	lines    []string
}

func parseUnified(text string) ([]diffFile, error) {
	var files []diffFile
	var cur *diffFile
	var hunk *diffHunk
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "new file"), strings.HasPrefix(line, "deleted file"),
			strings.HasPrefix(line, "similarity "), strings.HasPrefix(line, "rename "),
			strings.HasPrefix(line, "old mode"), strings.HasPrefix(line, "new mode"):
			continue
		case strings.HasPrefix(line, "--- "):
			files = append(files, diffFile{minus: diffPath(line[4:])})
			cur = &files[len(files)-1]
			hunk = nil
		case strings.HasPrefix(line, "+++ "):
			if cur == nil {
				return nil, fmt.Errorf("mount: patch header is out of order")
			}
			cur.plus = diffPath(line[4:])
		case strings.HasPrefix(line, "@@"):
			if cur == nil {
				return nil, fmt.Errorf("mount: hunk without a file")
			}
			start, err := hunkOldStart(line)
			if err != nil {
				return nil, err
			}
			cur.hunks = append(cur.hunks, diffHunk{oldStart: start})
			hunk = &cur.hunks[len(cur.hunks)-1]
		default:
			if hunk == nil {
				continue
			}
			if line == "" || strings.HasPrefix(line, `\`) {
				continue
			}
			switch line[0] {
			case ' ', '+', '-':
				hunk.lines = append(hunk.lines, line)
			default:
				return nil, fmt.Errorf("mount: patch line is not a diff")
			}
		}
	}
	return files, sc.Err()
}

func diffPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '\t'); i >= 0 {
		raw = raw[:i]
	}
	if fields := strings.Fields(raw); len(fields) > 0 {
		raw = fields[0]
	}
	if raw == "/dev/null" {
		return raw
	}
	if strings.HasPrefix(raw, "a/") || strings.HasPrefix(raw, "b/") {
		raw = raw[2:]
	}
	return filepath.ToSlash(raw)
}

func hunkOldStart(header string) (int, error) {
	parts := strings.Fields(header)
	if len(parts) < 3 || !strings.HasPrefix(parts[1], "-") {
		return 0, fmt.Errorf("mount: bad hunk")
	}
	span := strings.TrimPrefix(parts[1], "-")
	if comma := strings.IndexByte(span, ','); comma >= 0 {
		span = span[:comma]
	}
	n, err := strconv.Atoi(span)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("mount: bad hunk")
	}
	return n, nil
}

func hunksMatch(root, rel string, file diffFile) (bool, error) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	raw, err := os.ReadFile(path)
	missing := os.IsNotExist(err)
	if err != nil && !missing {
		return false, err
	}
	if file.minus == "/dev/null" {
		if !missing {
			return false, nil
		}
		for _, hunk := range file.hunks {
			for _, line := range hunk.lines {
				if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "-") {
					return false, nil
				}
			}
		}
		return true, nil
	}
	if missing {
		return false, nil
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, hunk := range file.hunks {
		i := hunk.oldStart - 1
		if hunk.oldStart == 0 {
			i = 0
		}
		for _, line := range hunk.lines {
			if strings.HasPrefix(line, `\`) || strings.HasPrefix(line, "+") {
				continue
			}
			body := ""
			if len(line) > 1 {
				body = line[1:]
			}
			if i < 0 || i >= len(lines) || lines[i] != body {
				return false, nil
			}
			i++
		}
	}
	return true, nil
}

func snapshotTree(root string) (string, error) {
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d %d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return b.String(), err
}
