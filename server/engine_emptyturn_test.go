package server

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

// Empty turns (thinking only, no text and no tools): detection and continuation. See steerHooks.Stop.

func assistantThinking(text string) llm.Message {
	return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
		{Type: llm.BlockThinking, Thinking: text, Signature: "sig"},
	}}
}

func TestIsThinkingOnlyTurn(t *testing.T) {
	toolUse := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
		{Type: llm.BlockThinking, Thinking: "Sweep port first"},
		{Type: llm.BlockToolUse, ID: "t1", Name: "run_nuclei"},
	}}
	cases := []struct {
		name string
		msgs []llm.Message
		want bool
	}{
		{"Just thinking.", []llm.Message{llm.UserText("Start"), assistantThinking("Think about it.")}, true},
		{"Thinking+Tools", []llm.Message{llm.UserText("Start"), toolUse}, false},
		{"Thinking+Text", []llm.Message{assistantThinking("Think about it."), {
			Role:    llm.RoleAssistant,
			Content: []llm.ContentBlock{{Type: llm.BlockThinking, Thinking: "x"}, llm.TextBlock("Conclusion")},
		}}, false},
		{"Text only empty characters", []llm.Message{{
			Role:    llm.RoleAssistant,
			Content: []llm.ContentBlock{{Type: llm.BlockThinking, Thinking: "x"}, llm.TextBlock("  \n ")},
		}}, true},
		{"Totally empty. assistant Round", []llm.Message{{Role: llm.RoleAssistant}}, true},
		// A tool result is a user message. The check must walk back to the assistant message before it, not misjudge the nearest message.
		{"Last one's a tool result.", []llm.Message{toolUse, {
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{{Type: llm.BlockToolResult, ToolUseID: "t1"}},
		}}, false},
		{"No assistant Message", []llm.Message{llm.UserText("Start")}, false},
		{"Empty History", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isThinkingOnlyTurn(c.msgs); got != c.want {
				t.Fatalf("isThinkingOnlyTurn = %v, want %v", got, c.want)
			}
		})
	}
}

// fakeHooks is a programmable inner HookRunner, used to check that steerHooks respects the inner decision.
type fakeHooks struct {
	prevent  bool
	blocking []string
	msg      string
}

func (f fakeHooks) PreToolUse(context.Context, string, []byte) (bool, string, []byte) {
	return false, "", nil
}
func (f fakeHooks) PostToolUse(context.Context, string, []byte, []byte, bool) {}
func (f fakeHooks) Stop(context.Context, []llm.Message) (bool, []string, string) {
	return f.prevent, f.blocking, f.msg
}

func TestSteerHooksStopNudgesEmptyTurn(t *testing.T) {
	empty := []llm.Message{assistantThinking("I'm supposed to start with a list of the fields.")}

	t.Run("Empty turn to rerun command", func(t *testing.T) {
		h := steerHooks{nudges: &atomic.Int64{}, limit: defaultEmptyTurnNudges, label: "worker-1 · #1"}
		prevent, blocking, _ := h.Stop(context.Background(), empty)
		if prevent {
			t.Fatal("an empty turn should not hard-stop")
		}
		if len(blocking) != 1 || blocking[0] != emptyTurnNudge {
			t.Fatalf("blocking = %v, want [emptyTurnNudge]", blocking)
		}
	})

	t.Run("Do not intervene when there is a text or tool", func(t *testing.T) {
		h := steerHooks{nudges: &atomic.Int64{}, limit: defaultEmptyTurnNudges}
		normal := []llm.Message{{
			Role:    llm.RoleAssistant,
			Content: []llm.ContentBlock{llm.TextBlock("Scan completed, no open end found mouth")},
		}}
		if _, blocking, _ := h.Stop(context.Background(), normal); blocking != nil {
			t.Fatalf("The normal closure was miscalculated as an empty circuit.: %v", blocking)
		}
		if n := h.nudges.Load(); n != 0 {
			t.Fatalf("Not accrued without intervention, got %d", n)
		}
	})

	t.Run("Once the ceiling has been reached, let it go.", func(t *testing.T) {
		const limit = 5 // the user set empty-response retries to 5
		h := steerHooks{nudges: &atomic.Int64{}, limit: limit}
		for i := 1; i <= limit; i++ {
			if _, blocking, _ := h.Stop(context.Background(), empty); len(blocking) != 1 {
				t.Fatalf("No. %d The second should remain within the quota, blocking = %v", i, blocking)
			}
		}
		if _, blocking, _ := h.Stop(context.Background(), empty); blocking != nil {
			t.Fatalf("Over the ceiling is still injected.: %v", blocking)
		}
	})

	// Empty-response retries set to -1 disables this layer; emptyTurnNudgeLimit resolves that to 0.
	t.Run("Configure close without intervening", func(t *testing.T) {
		h := steerHooks{nudges: &atomic.Int64{}, limit: 0}
		if _, blocking, _ := h.Stop(context.Background(), empty); blocking != nil {
			t.Fatalf("Closed and still injected.: %v", blocking)
		}
	})

	t.Run("inner When deciding to stop hard, do not fold", func(t *testing.T) {
		h := steerHooks{inner: fakeHooks{prevent: true, msg: "guard refused to stop"}, nudges: &atomic.Int64{}, limit: defaultEmptyTurnNudges}
		prevent, blocking, msg := h.Stop(context.Background(), empty)
		if !prevent || msg != "guard refused to stop" || blocking != nil {
			t.Fatalf("inner hard-stop was rewritten: prevent=%v blocking=%v msg=%q", prevent, blocking, msg)
		}
		if n := h.nudges.Load(); n != 0 {
			t.Fatalf("yielding to inner should not consume the quota, got %d", n)
		}
	})

	t.Run("inner Do not fold when running again", func(t *testing.T) {
		h := steerHooks{inner: fakeHooks{blocking: []string{"guard continue reason"}}, nudges: &atomic.Int64{}, limit: defaultEmptyTurnNudges}
		_, blocking, _ := h.Stop(context.Background(), empty)
		if len(blocking) != 1 || blocking[0] != "guard continue reason" {
			t.Fatalf("inner continue message was rewritten: %v", blocking)
		}
	})

	t.Run("No change in behaviour when a counter is not installed", func(t *testing.T) {
		h := steerHooks{limit: defaultEmptyTurnNudges} // for example a future call site that forgets to pass nudges
		if _, blocking, _ := h.Stop(context.Background(), empty); blocking != nil {
			t.Fatalf("should not inject when there is no counter: %v", blocking)
		}
	})
}
