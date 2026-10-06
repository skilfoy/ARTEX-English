package intercept

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVerdict(t *testing.T) {
	for _, action := range []string{"allow", "ask", "deny"} {
		t.Run(action, func(t *testing.T) {
			reason := "Actual operation: write a report containing ALLOW, DENY, and ASK; Consequence if successful: save the report text; Applicable rule: A5"
			raw, _ := json.Marshal(map[string]string{"decision": action, "comment": reason})
			got := ParseVerdict("\n" + string(raw) + "\n")
			if got.Action != action || got.Reason != reason {
				t.Fatalf("lost verdict or explanation: %+v", got)
			}
		})
	}
}

func TestParseVerdictEnglishContract(t *testing.T) {
	comment := "Actual operation: delete a business order; Consequence if successful: remove the record; Applicable rule: D4"
	raw, _ := json.Marshal(map[string]string{"decision": "deny", "comment": comment})
	got := ParseVerdict(string(raw))
	if got.Action != "deny" || got.Reason != comment {
		t.Fatalf("lost English verdict: %+v", got)
	}
	for _, invalid := range []string{
		strings.Replace(comment, "Actual operation: delete a business order", "Actual operation:", 1),
		strings.Replace(comment, "Consequence if successful: remove the record", "Consequence if successful:", 1),
		strings.Replace(comment, "Applicable rule: D4", "Applicable rule:", 1),
	} {
		raw, _ := json.Marshal(map[string]string{"decision": "deny", "comment": invalid})
		if got := ParseVerdict(string(raw)); got.Action != "" {
			t.Fatalf("accepted incomplete English verdict: %+v", got)
		}
	}
}

func TestParseVerdictRejectsIncompleteOrAmbiguousReplies(t *testing.T) {
	valid := `{"decision":"allow","comment":"Actual operation: read a document; Consequence if successful: return its content; Applicable rule: A5"}`
	for _, reply := range []string{
		"", "ALLOW", "DENY:hitD4", "Release:ALLOW", "ASK:Unknown",
		`{"decision":"allow"}`, `{"decision":"approve","comment":"Practical: read; post-success consequences: return content; hit rule:A5"}`,
		`{"decision":"allow","comment":null}`, `{"decision":"allow","comment":123}`,
		strings.Replace(valid, "Actual operation: read a document", "Actual operation:", 1),
		strings.Replace(valid, "Consequence if successful: return its content", "Consequence if successful:", 1),
		strings.Replace(valid, "Applicable rule: A5", "Applicable rule:", 1),
		strings.Replace(valid, "; Applicable rule: A5", "", 1),
		strings.Replace(valid, `"decision":"allow"`, `"decision":"deny","decision":"allow"`, 1),
		strings.Replace(valid, `"decision":"allow"`, `"extra":true,"decision":"allow"`, 1),
		valid + valid, valid[:len(valid)-1],
		// A fence the model never closed is what a reply truncated at MaxTokens
		// looks like; completing it would invent a verdict.
		"```json\n" + valid[:len(valid)-1],
		"```json\n" + valid + "\n```\nIn addition, I recommend a manual review..",
		"My ruling is...:\n" + valid,
	} {
		if got := ParseVerdict(reply); got.Action != "" {
			t.Errorf("accepted incomplete/ambiguous verdict: %q => %+v", reply, got)
		}
	}
}

// Wrapping JSON in markdown is the one deviation models make routinely. Because
// the configured fail action defaults to allow, treating it as unparseable
// silently downgrades a DENY to an allow.
func TestParseVerdictUnwrapsCodeFence(t *testing.T) {
	deny := `{"decision":"deny","comment":"Actual operation: delete production documents; Consequence if successful: remove business data; Applicable rule: D4"}`
	for _, reply := range []string{
		"```json\n" + deny + "\n```",
		"```JSON\n" + deny + "\n```",
		"```\n" + deny + "\n```",
		"  ```json\n" + deny + "\n```  ",
	} {
		got := ParseVerdict(reply)
		if got.Action != "deny" || !strings.HasSuffix(got.Reason, "Applicable rule: D4") {
			t.Errorf("fenced verdict lost: %q => %+v", reply, got)
		}
	}
}

func TestParseVerdictKeepsCompleteEnglishExplanation(t *testing.T) {
	reason := "Actual operation: " + strings.Repeat("write report ", 30) + "; Consequence if successful: save the report; Applicable rule: A2"
	raw, _ := json.Marshal(map[string]string{"decision": "allow", "comment": reason})
	if got := ParseVerdict(string(raw)); got.Reason != reason {
		t.Fatal("explanation was truncated or lost its rule")
	}
	raw, _ = json.Marshal(map[string]string{"decision": "allow", "comment": strings.Repeat("Ω", 2401)})
	if got := ParseVerdict(string(raw)); got.Action != "" {
		t.Fatal("accepted unbounded explanation")
	}
}
