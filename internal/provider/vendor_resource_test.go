package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccVendorResource_createUpdateImport(t *testing.T) {
	withMockServer(t)

	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
resource "vanta_vendor" "acme" {
  name                  = "Acme Inc"
  website_url           = "https://acme.example.com"
  services_provided     = "Widgets"
  status                = "IN_PROCUREMENT"
  inherent_risk_level   = "HIGH"
  authentication_method = "GOOGLE_WORKSPACE"
  contract_amount = {
    amount   = 1000
    currency = "USD"
  }
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("vanta_vendor.acme", "id"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "name", "Acme Inc"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "website_url", "https://acme.example.com"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "status", "IN_PROCUREMENT"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "inherent_risk_level", "HIGH"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "authentication_method", "GOOGLE_WORKSPACE"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "contract_amount.amount", "1000"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "contract_amount.currency", "USD"),
				// Server-defaulted computed value.
				resource.TestCheckResourceAttr("vanta_vendor.acme", "residual_risk_level", "UNSCORED"),
			),
		},
		{
			Config: providerConfig + `
resource "vanta_vendor" "acme" {
  name                  = "Acme Corporation"
  website_url           = "https://acme.example.com"
  services_provided     = "Gadgets"
  status                = "MANAGED"
  inherent_risk_level   = "MEDIUM"
  authentication_method = "OTHER"
  contract_amount = {
    amount   = 2500.5
    currency = "EUR"
  }
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("vanta_vendor.acme", "name", "Acme Corporation"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "services_provided", "Gadgets"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "status", "MANAGED"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "inherent_risk_level", "MEDIUM"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "authentication_method", "OTHER"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "contract_amount.amount", "2500.5"),
				resource.TestCheckResourceAttr("vanta_vendor.acme", "contract_amount.currency", "EUR"),
			),
		},
		{
			ResourceName:      "vanta_vendor.acme",
			ImportState:       true,
			ImportStateVerify: true,
			// archive_on_destroy is a Terraform-local flag not returned by the API.
			ImportStateVerifyIgnore: []string{"archive_on_destroy"},
		},
	})
}

// TestAccVendorResource_adoptExisting verifies that creating a resource with
// adopt_existing = true adopts a same-named vendor already present in Vanta and
// updates it in place, rather than creating a duplicate.
func TestAccVendorResource_adoptExisting(t *testing.T) {
	srv := withMockServer(t)
	existingID := srv.SeedVendor(map[string]any{
		"name":              "Preexisting Vendor",
		"status":            "MANAGED",
		"inherentRiskLevel": "LOW",
	})

	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
resource "vanta_vendor" "adopted" {
  name                = "Preexisting Vendor"
  adopt_existing      = true
  inherent_risk_level = "HIGH"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				// Adopted the existing vendor's ID rather than minting a new one.
				resource.TestCheckResourceAttr("vanta_vendor.adopted", "id", existingID),
				resource.TestCheckResourceAttr("vanta_vendor.adopted", "inherent_risk_level", "HIGH"),
				resource.TestCheckResourceAttr("vanta_vendor.adopted", "adopt_existing", "true"),
				// No duplicate was created.
				func(_ *terraform.State) error {
					if n := srv.VendorCount(); n != 1 {
						return fmt.Errorf("expected 1 vendor after adoption, got %d", n)
					}
					return nil
				},
			),
		},
	})
}

// TestAccVendorResource_archiveOnDestroy verifies that destroying a vendor with
// archive_on_destroy = true archives it (status ARCHIVED) rather than deleting
// it, leaving the vendor present in Vanta.
func TestAccVendorResource_archiveOnDestroy(t *testing.T) {
	srv := withMockServer(t)

	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
resource "vanta_vendor" "archived" {
  name               = "Archive Me"
  status             = "MANAGED"
  archive_on_destroy = true
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("vanta_vendor.archived", "archive_on_destroy", "true"),
				resource.TestCheckResourceAttr("vanta_vendor.archived", "status", "MANAGED"),
			),
		},
		{
			// Removing the resource from config triggers destroy; the vendor must
			// survive in the mock with status ARCHIVED rather than be deleted.
			Config: providerConfig,
			Check: func(_ *terraform.State) error {
				if n := srv.VendorCountByStatus("ARCHIVED"); n != 1 {
					return fmt.Errorf("expected 1 archived vendor after destroy, got %d", n)
				}
				if n := srv.VendorCount(); n != 1 {
					return fmt.Errorf("expected vendor to remain in Vanta after archive, got %d vendors", n)
				}
				return nil
			},
		},
	})
}
