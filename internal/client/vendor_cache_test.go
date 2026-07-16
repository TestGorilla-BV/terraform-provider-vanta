package client

import (
	"context"
	"testing"

	"github.com/TestGorilla-BV/terraform-provider-vanta/internal/mockserver"
)

// TestGetVendorByNameUsesCachedList verifies that resolving several vendors by
// name issues a single list request (the shared cache), not one per name — the
// behavior that keeps a bulk apply/import from tripping the API rate limit.
func TestGetVendorByNameUsesCachedList(t *testing.T) {
	srv := mockserver.New()
	defer srv.Close()
	srv.SeedVendor(map[string]any{"name": "Acme Inc", "status": "MANAGED"})
	srv.SeedVendor(map[string]any{"name": "Globex", "status": "MANAGED"})

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

	for _, name := range []string{"Acme Inc", "Globex", "Acme Inc", "Globex"} {
		v, err := c.GetVendorByName(ctx, name)
		if err != nil {
			t.Fatalf("GetVendorByName(%q): %v", name, err)
		}
		if v.Name != name {
			t.Fatalf("GetVendorByName(%q) returned %q", name, v.Name)
		}
	}
	// A missing name is still resolved from the same cache, without a new call.
	if _, err := c.GetVendorByName(ctx, "Nonexistent"); !IsNotFound(err) {
		t.Fatalf("expected NotFound for missing vendor, got %v", err)
	}

	if n := srv.ListVendorsCalls(); n != 1 {
		t.Fatalf("expected exactly 1 list-vendors request (cached), got %d", n)
	}
}
