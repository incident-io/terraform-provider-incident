package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/samber/lo"
)

// TestAccAlertSourceBeta covers the basic lifecycle: create, read back unchanged,
// import, and update.
func TestAccAlertSourceBeta(t *testing.T) {
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

func testAccAlertSourceBetaHeartbeatConfig(name string, intervalSeconds int, disabled *bool) string {
	disabledLine := ""
	if disabled != nil {
		disabledLine = fmt.Sprintf("  disabled = %t", *disabled)
	}

	return testRunTemplate("incident_alert_source_beta_heartbeat", `
resource "incident_alert_source_beta" "test" {
  name        = {{ quote .Name }}
  source_type = "heartbeat"

  heartbeat_options = {
    interval_seconds = {{ .IntervalSeconds }}
  }
{{ .DisabledLine }}
}
`, struct {
		Name            string
		IntervalSeconds int
		DisabledLine    string
	}{
		Name:            StableSuffix(name),
		IntervalSeconds: intervalSeconds,
		DisabledLine:    disabledLine,
	})
}

// TestAccAlertSourceBetaHeartbeatDisabled pauses and resumes a heartbeat source,
// including creating one already paused.
func TestAccAlertSourceBetaHeartbeatDisabled(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertSourceBetaHeartbeatConfig("heartbeat-paused", 60, lo.ToPtr(true)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "source_type", "heartbeat"),
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "disabled", "true"),
				),
			},
			{
				Config: testAccAlertSourceBetaHeartbeatConfig("heartbeat-paused", 60, lo.ToPtr(false)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "disabled", "false"),
				),
			},
		},
	})
}
