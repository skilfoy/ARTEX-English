package server

import (
	"time"

	"github.com/skilfoy/ARTEX-English/agent"
	"github.com/skilfoy/ARTEX-English/db"
)

// Retest the service end resolution. See docs/LLMRetry Design.md.Five floors.:
//   - Jianlian / Empty response / Same provider Safe window. Yes.[Follow the endpoint]Every one. LLM Configure to Overwrite
//     Global Default(profile If you leave any space, you'll inherit the whole picture.);
//   - Melting / I'm trying to run again..
//
// Global strategy read it once. DB One line. settings,Call points are on low frequency path (build) provider,work End,
// Save Configuration) is not worth adding another cache; the melting parameter is the exception——It has to read every time it fails, so...
// By applyRetryPolicy Push. Registry Save.

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
		// Keep the number here.[0=Default / Negative=Close]Original semantics:SDK of MaxRetries /
		// EmptyResponseRetries It's identical.,Just leave it to itself..
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

// Melting(Query cooling)Default value,With llmpool Embedded Consistency —— It's just here.[User with value]Other Organiser.
// See default value for attempted runback engine.go of modelErrorRetries / modelErrorRetryBackoff.

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
// inject (see steerHooks.Stop). It deliberately reuses layer ②'s knob —— [Empty response
// Number of retries]:They're the same two ways..SDK The tube.[Not a single piece.],The means are to...
// Reissuance of the same request;Here.[Just think, have no text or tools.],The way to do this is to add an order to
// The model goes on with the thought.(Re-issuance does not make sense for such an empty rotation determined by context shapes).Declining calibration
// It's different because SDK With[Did you? yield Events]That's right. Thinking about incremental is an event.——But users match
// [We'll try again.]The point is,[If the model doesn't produce the substance, do it again.],One on both floors.
// That's why you're right..
//
// Read a global strategy instead of one. profile Overwrite:one run It's probably a bad move. profile,And this...
// It's the whole intended volume gate. It's not supposed to change with the other end. Semantics and SDK of emptyRetries() Compositing:
// 0 = Default defaultEmptyTurnNudges;-1(Negative) = Turn off the air and run.;>0 = Use this value.
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
