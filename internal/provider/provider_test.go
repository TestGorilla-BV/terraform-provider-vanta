package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/TestGorilla-BV/terraform-provider-vanta/internal/mockserver"
)

// providerConfig is prepended to every test config block. Credentials and
// endpoints come from environment variables set by withMockServer.
const providerConfig = `
provider "vanta" {}
`

// protoV6ProviderFactories serves the provider in-process for tests.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vanta": providerserver.NewProtocol6WithError(New("test")()),
}

// withMockServer spins up the mock Vanta API and points the provider at it via
// env vars, exercising the real OAuth client-credentials exchange.
func withMockServer(t *testing.T) *mockserver.Server {
	t.Helper()
	srv := mockserver.New()
	t.Cleanup(srv.Close)
	t.Setenv("VANTA_CLIENT_ID", "test-client-id")
	t.Setenv("VANTA_CLIENT_SECRET", "test-client-secret")
	t.Setenv("VANTA_BASE_URL", srv.BaseURL())
	t.Setenv("VANTA_TOKEN_URL", srv.TokenURL())
	return srv
}

func runResourceTest(t *testing.T, steps []resource.TestStep) {
	t.Helper()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps:                    steps,
	})
}
