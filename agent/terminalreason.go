package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Autumn-27/norma/harness"
)

// runTrace retains the latest tool call so an interrupted run can identify it.
type runTrace struct {
	startedAt time.Time
	id        string
	name      string
	input     string
	at        time.Time
	pending   bool
}

func (t *runTrace) start(id, name, input string) {
	t.id, t.name, t.input, t.at, t.pending = id, name, input, time.Now(), true
}

func (t *runTrace) done(id string) {
	if id == t.id {
		t.pending = false
	}
}

var reasonHint = map[harness.TerminalReason]string{
	harness.ReasonCompleted:         "The model ended this turn. Recorded tool results contain its facts and assets.",
	harness.ReasonMaxTurns:          "The run reached its maximum turn count. Review recorded results before assigning more work.",
	harness.ReasonTimeout:           "The run reached its time limit. Recorded facts and assets remain available.",
	harness.ReasonModelError:        "The model request failed. Review the execution trace and model connection before retrying.",
	harness.ReasonBlockingLimit:     "The context reached a hard limit before the request could be sent.",
	harness.ReasonPromptTooLong:     "The prompt remained too long after context compression was attempted.",
	harness.ReasonImageError:        "The selected model could not process image content in this turn.",
	harness.ReasonStopHookPrevented: "A stop hook prevented this turn from ending. Review the configured guard rules.",
	harness.ReasonHookStopped:       "A tool or hook stopped the run. Review the most recent tool result.",
	harness.ReasonAbortedStreaming:  "The run was cancelled while the model was responding.",
	harness.ReasonAbortedTools:      "The run was cancelled while a tool was executing.",
}

// terminalText describes a run that ended without final model text.
func terminalText(ctx context.Context, term *harness.Terminal, tr *runTrace) (string, string) {
	reason := term.Reason
	aborted := reason == harness.ReasonAbortedStreaming || reason == harness.ReasonAbortedTools
	if reason == "" && ctx.Err() != nil {
		aborted = true
	}

	var summary string
	if aborted {
		_, short, _, ok := AbortReason(ctx)
		if !ok {
			short = "Cancellation cause unavailable"
		}
		stage := "execution"
		switch reason {
		case harness.ReasonAbortedStreaming:
			stage = "model response"
		case harness.ReasonAbortedTools:
			stage = "tool execution"
		}
		summary = "(Run interrupted: " + short + "; stage: " + stage + progressSuffix(term, tr) + "; incomplete)"
	} else if reason == harness.ReasonMaxTurns || reason == harness.ReasonTimeout {
		summary = "(Run budget reached: " + string(reason) + progressSuffix(term, tr) + "; no final summary)"
	} else {
		summary = "(No final text; " + terminalReasonLabel(reason) + ": " + firstLine(terminalReasonHint(reason), 80) + ")"
	}

	var detail strings.Builder
	detail.WriteString(summary)
	detail.WriteString("\n\n")
	fmt.Fprintf(&detail, "- **Final state**: `%s`. %s\n", terminalReasonLabel(reason), terminalReasonHint(reason))
	if aborted {
		code, _, explanation, ok := AbortReason(ctx)
		if ok {
			fmt.Fprintf(&detail, "- **Cancellation cause** (`%s`): %s\n", code, explanation)
		} else {
			detail.WriteString("- **Cancellation cause**: Unavailable.\n")
		}
	}
	if term.Err != nil {
		fmt.Fprintf(&detail, "- **Underlying error**: `%v`\n", term.Err)
	}
	if aborted && strings.TrimSpace(term.Text) != "" {
		detail.WriteString("- **Partial output before cancellation**:\n\n")
		detail.WriteString(term.Text)
		detail.WriteString("\n\n")
	}
	if term.Turns > 0 {
		fmt.Fprintf(&detail, "- **Model turns**: %d\n", term.Turns)
	}
	if !tr.startedAt.IsZero() {
		fmt.Fprintf(&detail, "- **Elapsed time**: %s\n", roundDur(time.Since(tr.startedAt)))
	}
	if usage := term.Usage; usage.InputTokens+usage.OutputTokens+usage.CacheReadTokens+usage.CacheWriteTokens > 0 {
		fmt.Fprintf(&detail, "- **Tokens**: input %d, output %d, cache read %d, cache write %d\n",
			usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheWriteTokens)
	}
	if tr.name == "" {
		detail.WriteString("- **Tool call**: None.\n")
	} else if tr.pending {
		fmt.Fprintf(&detail, "- **Pending tool**: `%s` (running for %s; no result received)\n\n  ```json\n  %s\n  ```\n",
			tr.name, roundDur(time.Since(tr.at)), firstLine(tr.input, 300))
	} else {
		fmt.Fprintf(&detail, "- **Last tool**: `%s` (returned normally)\n", tr.name)
	}
	return summary, detail.String()
}

func terminalReasonLabel(reason harness.TerminalReason) string {
	if reason == "" {
		return "context_canceled"
	}
	return string(reason)
}

func terminalReasonHint(reason harness.TerminalReason) string {
	if hint := reasonHint[reason]; hint != "" {
		return hint
	}
	if reason == "" {
		return "The context was cancelled without a terminal event."
	}
	return "An unrecognized terminal reason was reported."
}

func progressSuffix(term *harness.Terminal, tr *runTrace) string {
	var parts []string
	if term.Turns > 0 {
		parts = append(parts, fmt.Sprintf("%d turns", term.Turns))
	}
	if !tr.startedAt.IsZero() {
		parts = append(parts, roundDur(time.Since(tr.startedAt)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, ", ")
}

func roundDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return d.Round(100 * time.Millisecond).String()
	case d < time.Hour:
		return d.Round(time.Second).String()
	default:
		return d.Round(time.Minute).String()
	}
}
