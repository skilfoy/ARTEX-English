package server

import (
	"time"

	"github.com/skilfoy/ARTEX-English/agent"
	"github.com/skilfoy/ARTEX-English/db"
)

// Server-side resolution of the retry policy; see the LLM retry design notes. Of the five layers:
//   - connect, empty-response, and same-provider safe window follow the endpoint. Each LLM
//     profile may override the global default (a blank field inherits global; if global is also
//     unset, the built-in default applies);
//   - circuit-breaking and intent replay are process-wide; there is only one global copy.
//
// The global policy is a single settings row, read on cold paths (building a provider, finishing
// work, saving config), so another cache is not worth it. Breaker parameters are the exception — they are read on every failure — so applyRetryPolicy pushes them into the Registry.

// retryPolicy reads the global policy; a nil DB yields the zero policy (all
// layers on their built-in defaults).
func (s *Server) retryPolicy() db.LLMRetryPolicy {
	if s.m == nil || s.m.pg == nil {
		return db.LLMRetryPolicy{}
	}
	return s.m.pg.LLMRetryPolicy()
}

// resolveRetry layers one profile's override on top of the global policy and
// converts the result into the form agent.Config carries. Rules combine field by
// field, so a profile that only pins an interval still inherits the global count.
func resolveRetry(o db.RetryOverride, pol db.LLMRetryPolicy) agent.RetryConfig {
	connect := o.Connect.Or(pol.Connect)
	empty := o.Empty.Or(pol.Empty)
	stream := o.Stream.Or(pol.Stream)
	return agent.RetryConfig{
		// Keep the raw "0 = default / negative = disabled" semantics here: the SDK's MaxRetries /
		// EmptyResponseRetries use the same shape, so leave the parsing to them.
		ConnectAttempts: connect.Attempts, ConnectInterval: connect.Interval(),
		EmptyAttempts: empty.Attempts, EmptyInterval: empty.Interval(),
		StreamAttempts: stream.Attempts, StreamInterval: stream.Interval(),
	}
}

// applyProfileRetry fills cfg.Retry for a profile read from the DB.
func (s *Server) applyProfileRetry(cfg *agent.Config, p *db.LLMProfile) {
	if p == nil {
		return
	}
	cfg.Retry = resolveRetry(p.Retry, s.retryPolicy())
}

// Circuit-breaker (poll cooldown) defaults match llmpool's built-ins and are overridden here
// only when the user set a value. Intent-replay defaults are modelErrorRetries / modelErrorRetryBackoff in engine.go.

// applyRetryPolicy pushes the process-wide layers of the policy into the objects
// that consume them on a hot path: the circuit-breaker registry. Called at
// startup and whenever the policy is saved.
func (s *Server) applyRetryPolicy() {
	pol := s.retryPolicy()
	if s.llmHealth != nil {
		s.llmHealth.SetPolicy(pol.Breaker.Attempts, pol.Breaker.Interval())
	}
}

// modelErrorRetryPolicy resolves the intent-level replay knobs (layer ⑤): how
// many times a model_error work is re-run and how long to back off between runs.
func (e *Engine) modelErrorRetryPolicy() (retries int, backoff time.Duration) {
	retries, backoff = modelErrorRetries, modelErrorRetryBackoff
	if e == nil || e.m == nil || e.m.pg == nil {
		return retries, backoff
	}
	rule := e.m.pg.LLMRetryPolicy().Intent
	if rule.Attempts != 0 {
		retries = max(rule.Attempts, 0)
	}
	if d := rule.Interval(); d > 0 {
		backoff = d
	}
	return retries, backoff
}

// emptyTurnNudgeLimit resolves how many empty-turn continuations one work may
// inject (see steerHooks.Stop). It deliberately reuses layer ②'s empty-response retry
// count: they are one idea with two mechanisms. The SDK handles "no content block at all"
// by resending the same request. This layer handles "thinking only, no text and no tools"
// by appending an instruction so the model continues from thinking it already produced
// (resending unchanged cannot fix an empty turn caused by the shape of the context). The
// emptiness checks differ because the SDK keys off whether any event was yielded, and a
// thinking delta is itself an event — but "retry empty responses N times" means "try again
// if the model produced nothing substantive", so both layers share one count.
//
// Read the global policy, not a profile override: a run may fail over to another profile,
// and this cap covers the whole intent, so it must not change with the endpoint. Same shape as the SDK emptyRetries(): 0 = defaultEmptyTurnNudges; -1 = disable empty-turn continuation; >0 = use that value.
func (e *Engine) emptyTurnNudgeLimit() int {
	if e == nil || e.m == nil || e.m.pg == nil {
		return defaultEmptyTurnNudges
	}
	switch n := e.m.pg.LLMRetryPolicy().Empty.Attempts; {
	case n == 0:
		return defaultEmptyTurnNudges
	case n < 0:
		return 0
	default:
		return n
	}
}
