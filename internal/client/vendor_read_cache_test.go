package client

import (
	"context"
	"testing"

	"github.com/TestGorilla-BV/terraform-provider-vanta/internal/mockserver"
)

// TestGetVendorFromCacheServesFromList verifies that reading many vendors by ID
// issues a single list request (the shared cache) and no per-ID GETs — the
// behavior that keeps a whole-fleet refresh from tripping the API rate limit.
// A missing ID falls back to a direct GET.
func TestGetVendorFromCacheServesFromList(t *testing.T) {
	srv := mockserver.New()
	defer srv.Close()
	id1 := srv.SeedVendor(map[string]any{"name": "Acme Inc", "status": "MANAGED"})
	id2 := srv.SeedVendor(map[string]any{"name": "Globex", "status": "MANAGED"})

	c, err := New(Options{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		BaseURL:      srv.BaseURL(),
		TokenURL:     srv.TokenURL(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	// Repeated reads of both vendors resolve from one cached list, no per-ID GET.
	for _, id := range []string{id1, id2, id1, id2, id1} {
		v, err := c.GetVendorFromCache(ctx, id)
		if err != nil {
			t.Fatalf("GetVendorFromCache(%q): %v", id, err)
		}
		if v.ID != id {
			t.Fatalf("GetVendorFromCache(%q) returned id %q", id, v.ID)
		}
	}
	if n := srv.ListVendorsCalls(); n != 1 {
		t.Fatalf("expected exactly 1 list request (cached), got %d", n)
	}
	if n := srv.GetVendorCalls(); n != 0 {
		t.Fatalf("expected 0 per-ID GETs for cached vendors, got %d", n)
	}

	// An ID absent from the cached list falls back to a direct GET (which 404s).
	if _, err := c.GetVendorFromCache(ctx, "nonexistent"); !IsNotFound(err) {
		t.Fatalf("expected NotFound for missing vendor, got %v", err)
	}
	if n := srv.GetVendorCalls(); n != 1 {
		t.Fatalf("expected 1 fallback GET for the missing ID, got %d", n)
	}
}
