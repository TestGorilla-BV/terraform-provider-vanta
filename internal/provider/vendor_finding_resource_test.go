package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccVendorFindingResource_createUpdateImport(t *testing.T) {
	withMockServer(t)

	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
resource "vanta_vendor" "acme" {
  name = "Acme Inc"
}

resource "vanta_vendor_finding" "pentest" {
  vendor_id   = vanta_vendor.acme.id
  content     = "No recent penetration test."
  risk_status = "REMEDIATE"
  remediation = {
    state             = "OPEN"
    requirement_notes = "Provide an updated pentest report."
  }
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("vanta_vendor_finding.pentest", "id"),
				resource.TestCheckResourceAttrPair("vanta_vendor_finding.pentest", "vendor_id", "vanta_vendor.acme", "id"),
				resource.TestCheckResourceAttr("vanta_vendor_finding.pentest", "risk_status", "REMEDIATE"),
				resource.TestCheckResourceAttr("vanta_vendor_finding.pentest", "remediation.state", "OPEN"),
				resource.TestCheckResourceAttr("vanta_vendor_finding.pentest", "remediation.requirement_notes", "Provide an updated pentest report."),
			),
		},
		{
			Config: providerConfig + `
resource "vanta_vendor" "acme" {
  name = "Acme Inc"
}

resource "vanta_vendor_finding" "pentest" {
  vendor_id   = vanta_vendor.acme.id
  content     = "Penetration test received and reviewed."
  risk_status = "REMEDIATE"
  remediation = {
    state = "CLOSED"
  }
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("vanta_vendor_finding.pentest", "content", "Penetration test received and reviewed."),
				resource.TestCheckResourceAttr("vanta_vendor_finding.pentest", "remediation.state", "CLOSED"),
			),
		},
		{
			ResourceName:      "vanta_vendor_finding.pentest",
			ImportState:       true,
			ImportStateVerify: true,
			ImportStateIdFunc: func(s *terraform.State) (string, error) {
				rs, ok := s.RootModule().Resources["vanta_vendor_finding.pentest"]
				if !ok {
					return "", fmt.Errorf("resource not found in state")
				}
				return rs.Primary.Attributes["vendor_id"] + "/" + rs.Primary.Attributes["id"], nil
			},
		},
	})
}
