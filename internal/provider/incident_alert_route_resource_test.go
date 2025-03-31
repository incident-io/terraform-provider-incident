package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccIncidentAlertRouteResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccIncidentAlertRouteResourceConfig("test-route"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("incident_alert_route.test", "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr("incident_alert_route.test", "name", "test-route"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "enabled", "true"),
					resource.TestCheckResourceAttr("incident_alert_route.test", "is_private", "false"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "incident_alert_route.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update testing
			{
				Config: testAccIncidentAlertRouteResourceConfig("test-route-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_route.test", "name", "test-route-updated"),
				),
			},
		},
	})
}

func testAccIncidentAlertRouteResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "incident_alert_route" "test" {
  name = %[1]q
  enabled = true
  is_private = false
  
  alert_sources = []
  channel_config = []
  condition_groups = []
  expressions = []
  
  escalation_config = {
    auto_cancel_escalations = true
    escalation_targets = []
  }
  
  incident_config = {
    auto_decline_enabled = true
    condition_groups     = []
    defer_time_seconds   = 0
    grouping_keys        = []
	enabled              = true
  }
  
  incident_template = {
  }
}
`, name)
}
