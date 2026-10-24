package program

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Mount is a source tree the operator owns. The code graph is files and
// import edges inside that tree. WritePatch stores a diff beside the tree.
// It does not apply the diff and it does not modify the source files.
type Mount struct {
	Root string
}

// CodeGraph is a structural view of a mount. External imports are stubs.
type CodeGraph struct {
	Nodes []CodeNode `json:"nodes"`
	Edges []CodeEdge `json:"edges"`
}

// CodeNode is one file or one external import stub.
type CodeNode struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// CodeEdge links a file to another file or to an external stub.
type CodeEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

const maxCodeFiles = 2000

// OpenMount checks that root is a directory. Symlinks are resolved so a
// later write cannot be aimed outside the tree by a link.
func OpenMount(root string) (*Mount, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("mount: %s is not a directory", resolved)
	}
	return &Mount{Root: resolved}, nil
}

// Graph walks the mount and records files plus import edges. It skips
// dependency and VCS directories. It does not execute anything it reads.
func (m *Mount) Graph() (*CodeGraph, error) {
	graph := &CodeGraph{}
	files := 0
	err := filepath.WalkDir(m.Root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "dist" || name == ".artex-patches" {
				return filepath.SkipDir
			}
			return nil
		}
		if files >= maxCodeFiles {
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(m.Root, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil
		}
		files++
		graph.Nodes = append(graph.Nodes, CodeNode{Path: rel, Kind: "file"})
		edges, stubs := importsOf(m.Root, rel, path)
		graph.Edges = append(graph.Edges, edges...)
		graph.Nodes = append(graph.Nodes, stubs...)
		return nil
	})
	return graph, err
}

// WritePatch stores a diff under .artex-patches inside the mount. rel is
// ignored except as a label; the bytes are not written into the source tree.
func (m *Mount) WritePatch(label, diff string) (string, error) {
	if strings.TrimSpace(diff) == "" {
		return "", fmt.Errorf("mount: empty patch")
	}
	label = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return -1
	}, label)
	if label == "" {
		label = "patch"
	}
	dir := filepath.Join(m.Root, ".artex-patches")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + label + ".patch"
	dest := filepath.Join(dir, name)
	rel, err := filepath.Rel(m.Root, dest)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("mount: patch path escaped the mount")
	}
	if err := os.WriteFile(dest, []byte(diff), 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

var (
	reGoImport = regexp.MustCompile(`(?m)^\s*(?:import\s+)?"([^"]+)"\s*$`)
	reJSImport = regexp.MustCompile(`(?m)(?:from|import|require\()\s*['"]([^'"]+)['"]`)
)

func importsOf(root, rel, path string) ([]CodeEdge, []CodeNode) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".go" && ext != ".js" && ext != ".mjs" && ext != ".ts" && ext != ".tsx" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 1<<20 {
		return nil, nil
	}
	var specs []string
	if ext == ".go" {
		for _, group := range reGoImport.FindAllStringSubmatch(string(raw), -1) {
			specs = append(specs, group[1])
		}
	} else {
		for _, group := range reJSImport.FindAllStringSubmatch(string(raw), -1) {
			specs = append(specs, group[1])
		}
	}
	var edges []CodeEdge
	var stubs []CodeNode
	seen := map[string]bool{}
	for _, spec := range specs {
		target, kind := resolveImport(root, rel, spec)
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		edges = append(edges, CodeEdge{From: rel, To: target, Kind: "import"})
		if kind == "external" {
			stubs = append(stubs, CodeNode{Path: target, Kind: "external"})
		}
	}
	return edges, stubs
}

func resolveImport(root, from, spec string) (string, string) {
	if strings.HasPrefix(spec, ".") {
		dir := filepath.Dir(filepath.Join(root, from))
		dest := filepath.Clean(filepath.Join(dir, spec))
		rel, err := filepath.Rel(root, dest)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return "", ""
		}
		return rel, "file"
	}
	if spec == "" || strings.Contains(spec, "..") {
		return "", ""
	}
	return "external:" + spec, "external"
}
