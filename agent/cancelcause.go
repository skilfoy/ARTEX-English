package agent

import (
	"context"
	"errors"
	"fmt"
)

// AbortCause records why an agent run was cancelled.
type AbortCause struct {
	Code  string
	Short string
	Text  string
}

func (c *AbortCause) Error() string { return c.Text }

func cause(code, short, text string) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: text}
}

// Causef builds a cancellation cause with runtime details.
func Causef(code, short, format string, args ...any) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: fmt.Sprintf(format, args...)}
}

var (
	AbortPausedByUser = cause("paused_by_user", "User Paused Task",
		"The user paused the task. Active planner and worker runs were cancelled. Unfinished intents return to the frontier for execution after the task resumes.")
	AbortPausedByOrchestrator = cause("paused_by_orchestrator", "Agent Paused Task",
		"An agent called pause_task. Active planner and worker runs were cancelled, and unfinished intents return to the frontier.")
	AbortTaskDeleted = cause("task_deleted", "Task Deleted",
		"The task was deleted. Active planner, worker, and interactive agent runs were cancelled, and their new results will be discarded.")
	AbortPausedOnReload = cause("paused_on_reload", "Task Pause Restored",
		"The backend restored the task's paused state during startup and cancelled work that should not continue while the task is paused.")
	AbortGoalMet = cause("goal_met", "Task Goal Met",
		"The planner marked the task goal as met. Active workers were stopped, and their intents will be recorded as stopped rather than failed.")
	AbortSettleDrainTimeout = cause("settle_drain_timeout", "Task Drain Timed Out",
		"The task reached its timeout. Workers did not finish within the drain period and were cancelled. Results recorded before cancellation remain available.")

	AbortKilledByPlanner = cause("killed_by_planner", "Planner Stopped Work",
		"The planner called kill_work to stop this intent. The intent will be marked as stopped after the worker exits.")
	AbortWorkPausedByUser = cause("work_paused_by_user", "User Paused Work",
		"The user paused this worker intent. Its run was cancelled, while recorded facts, findings, and activity remain available for a later resumption.")
	AbortWorkCancelledByUser = cause("work_cancelled_by_user", "User Cancelled Work",
		"The user cancelled this worker intent. After the worker exits, the service applies the selected deletion mode and retains or removes its records accordingly.")
	AbortWorkFinished = cause("work_finished", "Worker Finished",
		"The worker completed its intent and the engine released its context. A cancellation report here indicates a race with normal completion.")
	AbortPausedRaceGuard = cause("paused_race_guard", "Paused Task Guard",
		"The engine declined to start work while the task was paused. The claimed intent returns to the frontier.")

	AbortChatStoppedByUser = cause("chat_stopped_by_user", "User Stopped Chat",
		"The user stopped this interactive agent turn. Existing activity remains recorded, and a new message can be sent.")
	AbortChatPausedWithTask = cause("chat_paused_with_task", "Task Pause Stopped Chat",
		"The task pause also stopped the interactive agent turn. Recorded activity remains, and the turn will not replay automatically.")
	AbortChatTurnFinished = cause("chat_turn_finished", "Chat Turn Finished",
		"The interactive agent turn completed and its context was released. A cancellation report here indicates a race with normal completion.")

	AbortShutdown = cause("shutdown", "Backend Shutting Down",
		"The backend is stopping for shutdown, restart, or update. Active agent runs were cancelled, and unfinished intents can be resumed after startup.")
	AbortRunHardTimeout = cause("run_hard_timeout", "Run Timed Out",
		"A model request or tool exceeded the run's hard time limit. Review the last pending tool call and any recorded output.")
)

// AbortReason resolves the named cause attached to a cancelled run context.
func AbortReason(ctx context.Context) (code, short, text string, ok bool) {
	c := context.Cause(ctx)
	if c == nil {
		return "", "", "", false
	}
	var ac *AbortCause
	if errors.As(c, &ac) {
		return ac.Code, ac.Short, ac.Text, true
	}
	switch {
	case errors.Is(c, context.DeadlineExceeded):
		return "deadline_exceeded", "Upstream Deadline Exceeded",
			"The upstream context reached its deadline: " + c.Error(), true
	case errors.Is(c, context.Canceled):
		return "canceled_no_cause", "Cancelled Without Cause",
			"The upstream context was cancelled without a named cause. Register a cause at the cancellation site for a clearer activity record.", true
	default:
		return "other", firstLine(c.Error(), 80), c.Error(), true
	}
}
