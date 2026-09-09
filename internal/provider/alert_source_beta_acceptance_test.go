package provider

import (
	"fmt"
	"regexp"
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

// TestAccAlertSourceBetaHeartbeatDisabled covers disabled on a heartbeat source.
//
// It never gets one paused: the API refuses to disable a heartbeat until it has received
// its first ping, and a source the test just created has received nothing. So the pause is
// the error case, which is worth an acceptance test of its own - the source is created
// before the pause is attempted, and it has to end up in state anyway or Terraform would
// lose track of a heartbeat that exists and is monitoring.
func TestAccAlertSourceBetaHeartbeatDisabled(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccAlertSourceBetaHeartbeatConfig("heartbeat-paused", 60, lo.ToPtr(true)),
				ExpectError: regexp.MustCompile(`(?s)Unable to (pause|update) alert source.*alert_source.disabled`),
			},
			// The source from the failed step is in state, so this adopts it rather than
			// leaving it behind: a heartbeat nobody is managing keeps monitoring, and the
			// test's cleanup wouldn't know to delete it.
			{
				Config: testAccAlertSourceBetaHeartbeatConfig("heartbeat-paused", 60, lo.ToPtr(false)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "source_type", "heartbeat"),
					resource.TestCheckResourceAttr("incident_alert_source_beta.test", "disabled", "false"),
				),
			},
		},
	})
}
