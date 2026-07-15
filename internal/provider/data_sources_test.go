package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPeopleDataSource_basic(t *testing.T) {
	withMockServer(t)
	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
data "vanta_people" "all" {}

data "vanta_people" "current" {
  employment_status = "CURRENT"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.vanta_people.all", "people.#", "2"),
				resource.TestCheckResourceAttr("data.vanta_people.current", "people.#", "1"),
				resource.TestCheckResourceAttr("data.vanta_people.current", "people.0.email", "alice@example.com"),
				resource.TestCheckResourceAttr("data.vanta_people.current", "people.0.job_title", "Engineer"),
			),
		},
	})
}

func TestAccFrameworksDataSource_basic(t *testing.T) {
	withMockServer(t)
	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
data "vanta_frameworks" "all" {}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.vanta_frameworks.all", "frameworks.#", "1"),
				resource.TestCheckResourceAttr("data.vanta_frameworks.all", "frameworks.0.id", "soc2"),
				resource.TestCheckResourceAttr("data.vanta_frameworks.all", "frameworks.0.num_controls_total", "86"),
			),
		},
	})
}

func TestAccTestsDataSource_basic(t *testing.T) {
	withMockServer(t)
	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
data "vanta_tests" "all" {}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.vanta_tests.all", "tests.#", "1"),
				resource.TestCheckResourceAttr("data.vanta_tests.all", "tests.0.status", "OK"),
			),
		},
	})
}

func TestAccVendorsDataSource_basic(t *testing.T) {
	withMockServer(t)
	runResourceTest(t, []resource.TestStep{
		{
			Config: providerConfig + `
resource "vanta_vendor" "a" {
  name   = "Alpha"
  status = "MANAGED"
}

resource "vanta_vendor" "b" {
  name   = "Beta"
  status = "ARCHIVED"
}

data "vanta_vendors" "managed" {
  status_matches_any = ["MANAGED"]
  depends_on         = [vanta_vendor.a, vanta_vendor.b]
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.vanta_vendors.managed", "vendors.#", "1"),
				resource.TestCheckResourceAttr("data.vanta_vendors.managed", "vendors.0.name", "Alpha"),
			),
		},
	})
}
