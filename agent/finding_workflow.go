package agent

import (
	"encoding/json"
	"fmt"

	actool "github.com/Autumn-27/norma/tool"
	"github.com/skilfoy/ARTEX-English/db"
)

const findingIDGuidance = "\n\n**Finding identifiers:** finding_id identifies a confirmed finding; finding_node_id identifies an exploration node. Read each identifier from list_findings, list_task_findings, node_detail, or get_task_node_detail. Use finding_id with get_finding_traffic and bind_finding_traffic. Use finding_node_id with update_finding_report. Do not infer one identifier from the other."

// The server supplies the persisted setting. A missing setting/host is off.
// Consulted at assembly and again on writes so an already-running session
// cannot keep binding after the user switches the feature off.
var FindingTrafficBindingEnabled func() bool

func findingTrafficBindingEnabled() bool {
	return FindingTrafficBindingEnabled != nil && FindingTrafficBindingEnabled()
}

// Applied after ToolResolve: user descriptions and prompts remain intact, while
// all actual reporters (including Planner and custom chat agents) see the same
// API contract. Disabled/unbound tools are never reintroduced here.
func findingWorkflowTools(agentKey string, tools []actool.CoreTool) ([]actool.CoreTool, string) {
	if !findingTrafficBindingEnabled() {
		out := make([]actool.CoreTool, 0, len(tools))
		for _, tool := range tools {
			if tool.Name() == "bind_finding_traffic" {
				continue
			}
			if agentKey == "reporter" && (tool.Name() == "traffic_search" || tool.Name() == "traffic_get" || tool.Name() == "traffic_blob") {
				continue
			}
			switch tool.Name() {
			case "report_finding", "add_hint", "add_task_hint":
				// Work on a copy: toggling back on must restore the original schema.
				raw, _ := json.Marshal(tool.InputSchema())
				var schema map[string]any
				if json.Unmarshal(raw, &schema) == nil {
					stripTrafficParameters(schema)
					tool = DecorateTool(tool, tool.Description(), schema)
				}
			}
			out = append(out, tool)
		}
		return out, ""
	}
	out := append([]actool.CoreTool(nil), tools...)
	has := map[string]bool{}
	for i, tool := range out {
		has[tool.Name()] = true
		note := ""
		switch tool.Name() {
		case "report_finding":
			note = "\nReport verified findings with their supporting evidence. Include actual traffic_refs or evidence_hint_id only when the corresponding request and response have been checked. The reporter can bind traffic before writing its report. Finding IDs and exploration node IDs are distinct."
		case "add_hint", "add_task_hint":
			note = "\nPreserve verified traffic_refs in the hint that describes the finding. Include the relevant flow ID, its purpose, and the observed sequence. Do not present unverified candidates as evidence."
		case "get_finding_traffic", "bind_finding_traffic", "list_findings", "list_task_findings", "node_detail", "get_task_node_detail", "update_finding_report":
			note = findingIDGuidance
		}
		if note != "" {
			out[i] = DecorateTool(tool, tool.Description()+note, tool.InputSchema())
		}
	}
	guidance := ""
	if has["report_finding"] || has["add_task_hint"] || has["add_hint"] {
		guidance = "\n\n**Traffic evidence:** Preserve actual flow IDs and their purpose alongside command output and other verified evidence. Use traffic_refs in add_hint or add_task_hint for handoff. report_finding also accepts traffic_refs or evidence_hint_id. Confirm each reference before reporting. For TCP findings without matching HTTP traffic, preserve other verified evidence."
		if has["add_task_hint"] && !has["add_hint"] {
			guidance += "\nUse add_task_hint to hand findings to an agent in the existing task, then verify the result with list_task_findings."
		}
		if has["prove_goal"] || has["goal_met"] {
			guidance += "\nComplete the evidence handoff before marking the goal complete."
		}
	}
	if has["update_finding_report"] && has["bind_finding_traffic"] && has["get_finding_traffic"] {
		guidance += "\n\n**Report preparation:** Read the finding and its recorded evidence. For HTTP findings, search for candidate flows and inspect each request and response. Bind only flows that substantiate the finding, in sequence, with a clear role and purpose. Read get_finding_traffic after binding and pass its evidence_version to update_finding_report. Reuse the existing finding. Where no matching flow is available, explain the evidence used and why traffic could not be bound."
	}
	if guidance != "" || has["get_finding_traffic"] || has["update_finding_report"] {
		guidance += findingIDGuidance
	}
	return out, guidance
}

func stripTrafficParameters(schema map[string]any) {
	props, _ := schema["properties"].(map[string]any)
	delete(props, "traffic_refs")
	delete(props, "evidence_hint_id")
	if required, ok := schema["required"].([]any); ok {
		kept := required[:0]
		for _, key := range required {
			if key != "traffic_refs" && key != "evidence_hint_id" {
				kept = append(kept, key)
			}
		}
		schema["required"] = kept
	}
	if hints, ok := props["hints"].(map[string]any); ok {
		if items, ok := hints["items"].(map[string]any); ok {
			stripTrafficParameters(items)
		}
	}
}

// HintTrafficSchema is shared by the task-local and cross-task hint tools.
func HintTrafficSchema() map[string]any {
	return map[string]any{"type": "array", "description": "Optional verified traffic flow references for this finding, in sequence. The receiving agent can pass the hint ID as evidence_hint_id to report_finding.", "items": obj(map[string]any{"traffic_id": str("Real flow ID"), "role": str("baseline / proof / verification / supporting"), "note": str("What does the flow support?")}, "traffic_id")}
}

func (t *ToolSet) findingRefsFromHint(hintID int64, explicit []db.TrafficRef) ([]db.TrafficRef, error) {
	if hintID <= 0 {
		return db.NormalizeTrafficRefs(explicit)
	}
	n, err := t.ts.GetNode(hintID) // local store only: inherited hints cannot supply evidence
	if err != nil {
		return nil, err
	}
	if n == nil || n.Kind != db.KindHint {
		return nil, fmt.Errorf("evidence_hint_id=%d must identify a hint in this task; inherited hints cannot supply binding evidence", hintID)
	}
	var payload struct {
		Refs []db.TrafficRef `json:"traffic_refs"`
	}
	if err := json.Unmarshal(n.Payload, &payload); err != nil {
		return nil, err
	}
	return db.NormalizeTrafficRefs(append(append([]db.TrafficRef{}, explicit...), payload.Refs...))
}
