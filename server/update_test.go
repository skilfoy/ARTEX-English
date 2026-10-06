package server

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/skilfoy/ARTEX-English/selfupdate"
)

// releaseCache protects the GitHub quota. Unauthenticated API calls are 60 per hour per IP,
// and the top-bar "update available" hint checks on every full page load. Once the cache misses,
// a few extra tabs exhaust the quota, and a real update check then fails.

func newTestCache(fetch func(context.Context, *http.Client) (*selfupdate.Release, error)) *releaseCache {
	return &releaseCache{fetch: fetch}
}

func TestReleaseCacheServesFromCache(t *testing.T) {
	calls := 0
	c := newTestCache(func(context.Context, *http.Client) (*selfupdate.Release, error) {
		calls++
		return &selfupdate.Release{TagName: "v0.3.8"}, nil
	})

	for range 5 {
		rel, err := c.get(t.Context(), nil, false)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if rel.TagName != "v0.3.8" {
			t.Fatalf("TagName = %q", rel.TagName)
		}
	}
	if calls != 1 {
		t.Errorf("5 lookups should hit the origin once, got %d", calls)
	}
}

func TestReleaseCacheForceBypasses(t *testing.T) {
	calls := 0
	c := newTestCache(func(context.Context, *http.Client) (*selfupdate.Release, error) {
		calls++
		return &selfupdate.Release{TagName: "v0.3.8"}, nil
	})

	if _, err := c.get(t.Context(), nil, false); err != nil {
		t.Fatal(err)
	}
	// Check for updates must bypass the cache, or a release that just shipped stays invisible until the TTL expires.
	if _, err := c.get(t.Context(), nil, true); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("force should bypass the cache: expected 2 origin calls, got %d", calls)
	}
}

func TestReleaseCacheExpiresAfterTTL(t *testing.T) {
	calls := 0
	c := newTestCache(func(context.Context, *http.Client) (*selfupdate.Release, error) {
		calls++
		return &selfupdate.Release{TagName: "v0.3.8"}, nil
	})

	if _, err := c.get(t.Context(), nil, false); err != nil {
		t.Fatal(err)
	}
	// Move the stored time to just past expiry to simulate the TTL elapsing.
	c.at = time.Now().Add(-releaseTTL - time.Second)
	if _, err := c.get(t.Context(), nil, false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("after the TTL expires it should hit the origin again: expected 2 calls, got %d", calls)
	}
}

func TestReleaseCacheUsesShorterTTLForErrors(t *testing.T) {
	calls := 0
	c := newTestCache(func(context.Context, *http.Client) (*selfupdate.Release, error) {
		calls++
		return nil, errors.New("github unreachable")
	})

	if _, err := c.get(t.Context(), nil, false); err == nil {
		t.Fatal("expected an error")
	}
	// Failures are cached briefly too. Otherwise every page load waits out a timeout while GitHub is unreachable.
	if _, err := c.get(t.Context(), nil, false); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("an error should be cached briefly: expected 1 origin call, got %d", calls)
	}

	// The error TTL must be clearly shorter than the success TTL so the cache heals soon after the network recovers.
	if releaseErrTTL >= releaseTTL {
		t.Fatalf("error TTL (%v) must be shorter than the success TTL (%v)", releaseErrTTL, releaseTTL)
	}
	c.at = time.Now().Add(-releaseErrTTL - time.Second)
	if _, err := c.get(t.Context(), nil, false); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 2 {
		t.Errorf("after the error TTL expires it should retry: expected 2 calls, got %d", calls)
	}
}

func TestReleaseCacheDoesNotPoisonOnCallerCancel(t *testing.T) {
	good := &selfupdate.Release{TagName: "v0.3.8"}
	c := newTestCache(func(ctx context.Context, _ *http.Client) (*selfupdate.Release, error) {
		return good, nil
	})
	if _, err := c.get(t.Context(), nil, false); err != nil {
		t.Fatal(err)
	}

	// A visitor closing the tab cancels the request. That is not a GitHub failure and must not be
	// written into the cache, or every visitor for the next 30 minutes would see a bogus error.
	c.fetch = func(ctx context.Context, _ *http.Client) (*selfupdate.Release, error) {
		return nil, ctx.Err()
	}
	c.at = time.Now().Add(-releaseTTL - time.Second) // Expire the cache so the next call has to hit the origin.

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.get(ctx, nil, false); err == nil {
		t.Fatal("a cancelled caller should see the cancellation error")
	}

	// The cancelled call must leave no trace: the cache must not store "cancelled",
	// and it must still hold the previous good result.
	if c.err != nil {
		t.Fatalf("a cancellation must not be written into the cache, got %v", c.err)
	}
	if c.rel == nil || c.rel.TagName != "v0.3.8" {
		t.Fatalf("the cache should keep the previous good result, got %+v", c.rel)
	}

	// That cancellation fetched nothing, so the next visitor should hit the origin again
	// and get a normal result, not inherit the previous cancellation.
	c.fetch = func(context.Context, *http.Client) (*selfupdate.Release, error) {
		return good, nil
	}
	rel, err := c.get(t.Context(), nil, false)
	if err != nil {
		t.Fatalf("a normal request after a cancellation should not fail: %v", err)
	}
	if rel == nil || rel.TagName != "v0.3.8" {
		t.Fatalf("expected a normal result, got %+v", rel)
	}
}
