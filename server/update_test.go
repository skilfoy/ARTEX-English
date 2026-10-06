package server

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/skilfoy/ARTEX-English/selfupdate"
)

// releaseCache It's protection. GitHub Level of quota: uncertified API Only 60 times/hours/IP,
// And the top bar."There is a new version"The prompt is checked every time the entire page is loaded. Once the cache fails, the user opens more.
// The tab will run out of quotas, and then I can't find it..

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
		t.Errorf("5 SubQuery should only be returned to source 1 Second, actual %d times", calls)
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
	// User Point[Check for updates]We have to get real-time results, or we'll have to wait for the release to expire..
	if _, err := c.get(t.Context(), nil, true); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("force Should be bypassing the cache, expecting a return. 2 Second, actual %d times", calls)
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
	// Putting down the library until it just expired. Simulation. TTL To the point..
	c.at = time.Now().Add(-releaseTTL - time.Second)
	if _, err := c.get(t.Context(), nil, false); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("TTL It should be returned after expiry, with expectations 2 Second, actual %d times", calls)
	}
}

func TestReleaseCacheUsesShorterTTLForErrors(t *testing.T) {
	calls := 0
	c := newTestCache(func(context.Context, *http.Client) (*selfupdate.Release, error) {
		calls++
		return nil, errors.New("github Unreachable")
	})

	if _, err := c.get(t.Context(), nil, false); err == nil {
		t.Fatal("Expect to return error")
	}
	// If you fail, you'll have to wait. GitHub Every time you can't reach it, every page load will wait for a timeout..
	if _, err := c.get(t.Context(), nil, false); err == nil {
		t.Fatal("Expect to return error")
	}
	if calls != 1 {
		t.Errorf("Errors should cache short and expect returns 1 Second, actual %d times", calls)
	}

	// But it was wrong. TTL It has to be significantly shorter than success..
	if releaseErrTTL >= releaseTTL {
		t.Fatalf("Error TTL(%v) It has to be shorter than success. TTL(%v)", releaseErrTTL, releaseTTL)
	}
	c.at = time.Now().Add(-releaseErrTTL - time.Second)
	if _, err := c.get(t.Context(), nil, false); err == nil {
		t.Fatal("Expect to return error")
	}
	if calls != 2 {
		t.Errorf("Error TTL It should be repeated after expiry, with expectations 2 Second, actual %d times", calls)
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

	// Visitors close tabs and cancel requests. That doesn't mean... GitHub There's a problem."Cancelled"
	// Write Cache——Otherwise, 30 Every visitor in a minute will get an incredible mistake..
	c.fetch = func(ctx context.Context, _ *http.Client) (*selfupdate.Release, error) {
		return nil, ctx.Err()
	}
	c.at = time.Now().Add(-releaseTTL - time.Second) // Let the cache expire and force it back.

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.get(ctx, nil, false); err == nil {
		t.Fatal("If the caller is cancelled, pass it through.")
	}

	// Key non-variant: No trace from the time it was cancelled——Not in the cache."Cancelled"This mistake.,
	// And the last good results..
	if c.err != nil {
		t.Fatalf("Undo errors should not be written into the cache, get %v", c.err)
	}
	if c.rel == nil || c.rel.TagName != "v0.3.8" {
		t.Fatalf("Cache should keep the last good result. %+v", c.rel)
	}

	// That cancellation didn't bring in any new data, so the next visitor was supposed to start over. Back Source——And it's normal to get results.,
	// It won't get into trouble with the last cancellation..
	c.fetch = func(context.Context, *http.Client) (*selfupdate.Release, error) {
		return good, nil
	}
	rel, err := c.get(t.Context(), nil, false)
	if err != nil {
		t.Fatalf("Regular requests after cancellation should not be misreported.: %v", err)
	}
	if rel == nil || rel.TagName != "v0.3.8" {
		t.Fatalf("We should get normal results. %+v", rel)
	}
}
