package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccIncidentScheduleSyncRuleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with on_call sync type
			{
				Config: testAccScheduleSyncRuleResourceConfig("on_call"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_sync_rule.test", "sync_type", "on_call"),
					resource.TestCheckResourceAttrSet("incident_schedule_sync_rule.test", "id"),
					resource.TestCheckResourceAttrSet("incident_schedule_sync_rule.test", "schedule_id"),
					resource.TestCheckResourceAttrSet("incident_schedule_sync_rule.test", "schedule_sync_target_id"),
				),
			},
			// Update sync type to all_users
			{
				Config: testAccScheduleSyncRuleResourceConfig("all_users"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_sync_rule.test", "sync_type", "all_users"),
				),
			},
			// Import using composite ID format
			{
				ResourceName:      "incident_schedule_sync_rule.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importScheduleSyncRuleStateIdFunc("incident_schedule_sync_rule.test"),
			},
		},
	})
}

func TestAccIncidentScheduleSyncRuleResource_InvalidImportID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScheduleSyncRuleResourceConfig("on_call"),
			},
			{
				ResourceName:  "incident_schedule_sync_rule.test",
				ImportState:   true,
				ImportStateId: "invalid-id-without-colon",
				ExpectError:   regexp.MustCompile(`The import ID must be in the format: schedule_id:rule_id`),
			},
		},
	})
}

func testAccScheduleSyncRuleResourceConfig(syncType string) string {
	return testRunTemplate("incident_schedule_sync_rule", `
resource "incident_schedule" "test" {
  name     = "Test Schedule for Sync Rule"
  timezone = "Europe/London"
}

resource "incident_schedule_sync_target" "test" {
  add_bot_to_group = true

  new_slack_user_group {
    name        = "Test Sync Rule Target"
    handle      = "test-sync-rule-target"
    description = "Target for testing schedule sync rules"
  }
}

resource "incident_schedule_sync_rule" "test" {
  schedule_id             = incident_schedule.test.id
  schedule_sync_target_id = incident_schedule_sync_target.test.id
  sync_type               = {{ quote .SyncType }}
}
`, struct {
		SyncType string
	}{
		SyncType: syncType,
	})
}

// importScheduleSyncRuleStateIdFunc returns a function that generates the composite import ID
func importScheduleSyncRuleStateIdFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", nil
		}
		return rs.Primary.Attributes["schedule_id"] + ":" + rs.Primary.Attributes["id"], nil
	}
}
