package client

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/TestGorilla-BV/terraform-provider-vanta/internal/mockserver"
)

// TestRequestsPerSecondPacesConcurrentRequests verifies that the shared rate
// gate spaces requests by minInterval even when many goroutines fire at once —
// the behavior that keeps a bulk apply/import under Vanta's rate limit. It
// asserts only a lower bound on elapsed time, so a slow CI host can't flake it.
func TestRequestsPerSecondPacesConcurrentRequests(t *testing.T) {
	srv := mockserver.New()
	defer srv.Close()
	srv.SeedVendor(map[string]any{"name": "Acme Inc", "status": "MANAGED"})

	const interval = 20 * time.Millisecond
	c, err := New(Options{
		ClientID:          "test-id",
		ClientSecret:      "test-secret",
		BaseURL:           srv.BaseURL(),
		TokenURL:          srv.TokenURL(),
		RequestsPerSecond: float64(time.Second) / float64(interval),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const n = 8
	start := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- c.Do(context.Background(), http.MethodGet, "/vendors", nil, nil, new(map[string]any))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
	}

	// n requests spaced by interval take at least (n-1)*interval, regardless of
	// how fast the server responds or how many goroutines ran concurrently.
	if elapsed := time.Since(start); elapsed < (n-1)*interval {
		t.Fatalf("expected pacing to take >= %v, took %v", (n-1)*interval, elapsed)
	}
}

// TestRequestsPerSecondZeroDisablesPacing confirms an unset/zero rate leaves
// the client unthrottled, so direct client construction in tests stays fast.
func TestRequestsPerSecondZeroDisablesPacing(t *testing.T) {
	c, err := New(Options{ClientID: "id", ClientSecret: "secret"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.minInterval != 0 {
		t.Fatalf("expected pacing disabled by default, got minInterval=%v", c.minInterval)
	}
	if err := c.waitRate(context.Background()); err != nil {
		t.Fatalf("waitRate with pacing disabled: %v", err)
	}
}
