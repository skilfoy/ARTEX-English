package program

import (
	"fmt"
	"strings"
)

const (
	briefFiles = 80
	briefEdges = 80
)

// GraphBrief is paths and import edges. It does not include file contents.
func GraphBrief(graph *CodeGraph) string {
	if graph == nil {
		return ""
	}
	var b strings.Builder
	files := 0
	for _, node := range graph.Nodes {
		if node.Kind != "file" || strings.ContainsAny(node.Path, "\r\n") {
			continue
		}
		if files >= briefFiles {
			break
		}
		fmt.Fprintf(&b, "file %s\n", node.Path)
		files++
	}
	edges := 0
	for _, edge := range graph.Edges {
		if edge.Kind != "import" || strings.ContainsAny(edge.From+"\n"+edge.To, "\r") {
			continue
		}
		if strings.Contains(edge.From, "\n") || strings.Contains(edge.To, "\n") {
			continue
		}
		if edges >= briefEdges {
			break
		}
		fmt.Fprintf(&b, "%s -> %s\n", edge.From, edge.To)
		edges++
	}
	return b.String()
}

// CodeGraphNote fences the installed graph for a planner round.
// An empty graph returns an empty string.
func CodeGraphNote() string {
	brief := GraphBrief(MountGraph())
	if brief == "" {
		return ""
	}
	return WrapUntrusted(brief, "CODE_GRAPH")
}
