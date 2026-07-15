package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccVendorResource_createUpdateImport(t *testing.T) {
	withMockServer(t)

	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
resource "vanta_vendor" "acme" {
  name                = "Acme Inc"
  website_url         = "https://acme.example.com"
  services_provided   = "Widgets"
  status              = "IN_PROCUREMENT"
  inherent_risk_level = "HIGH"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("vanta_vendor.acme", "id"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "name", "Acme Inc"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "website_url", "https://acme.example.com"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "status", "IN_PROCUREMENT"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "inherent_risk_level", "HIGH"),
				// Server-defaulted computed value.
				resource.TestCheckResourceAttr("vanta_vendor.acme", "residual_risk_level", "UNSCORED"),
			),
		},
		{
			Config: providerConfig + `
resource "vanta_vendor" "acme" {
  name                = "Acme Corporation"
  website_url         = "https://acme.example.com"
  services_provided   = "Gadgets"
  status              = "MANAGED"
  inherent_risk_level = "MEDIUM"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("vanta_vendor.acme", "name", "Acme Corporation"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "services_provided", "Gadgets"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "status", "MANAGED"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "inherent_risk_level", "MEDIUM"),
			),
		},
		{
			ResourceName:      "vanta_vendor.acme",
			ImportState:       true,
			ImportStateVerify: true,
		},
	})
}
