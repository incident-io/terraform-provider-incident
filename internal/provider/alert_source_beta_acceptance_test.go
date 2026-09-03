package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// accAlertSourceBeta covers the basic lifecycle: create, read back unchanged,
// import, and update.
func accAlertSourceBeta(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceBetaConfig("test-source", "a title", "a description"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "name", StableSuffix("test-source")),
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "source_type", "http"),
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "title.literal", "a title"),
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "description.literal", "a description"),
					resource.TestCheckResourceAttrSet("incident_alert_source_beta.test", "id"),
					resource.TestCheckResourceAttrSet("incident_alert_source_beta.test", "secret_token"),
				),
			},
			{
				ResourceName:      "incident_alert_source_beta.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccAlertSourceBetaConfig("test-source-renamed", "a title", "a description"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "name", StableSuffix("test-source-renamed")),
				),
			},
		},
	})
}

func testAccAlertSourceBetaConfig(name, title, description string) string {
	return testRunTemplate("incident_alert_source_beta", `
resource "incident_alert_source_beta" "test" {
  name        = {{ quote .Name }}
  source_type = "http"

  title       = { literal = {{ quote .Title }} }
  description = { literal = {{ quote .Description }} }
}
`, struct {
		Name        string
		Title       string
		Description string
	}{
		Name:        StableSuffix(name),
		Title:       title,
		Description: description,
	})
}
