package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccIncidentScheduleSyncRuleResource tests creating and updating schedule sync rules.
//
// NOTE: This test requires Slack usergroups:write scope, which is not available in CI.
// Set TF_ACC_SLACK_USER_GROUPS=1 to run this test locally with a workspace that has the scope.
func TestAccIncidentScheduleSyncRuleResource(t *testing.T) {
	if os.Getenv("TF_ACC_SLACK_USER_GROUPS") == "" {
		t.Skip("TF_ACC_SLACK_USER_GROUPS is not set: skipping test that requires Slack usergroups:write scope")
	}

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
				ImportStateIdFunc: importScheduleSyncRuleStateIDFunc("incident_schedule_sync_rule.test"),
			},
		},
	})
}

// TestAccIncidentScheduleSyncRuleResource_InvalidImportID tests that invalid import IDs are rejected.
//
// NOTE: This test requires Slack usergroups:write scope, which is not available in CI.
// Set TF_ACC_SLACK_USER_GROUPS=1 to run this test locally with a workspace that has the scope.
func TestAccIncidentScheduleSyncRuleResource_InvalidImportID(t *testing.T) {
	if os.Getenv("TF_ACC_SLACK_USER_GROUPS") == "" {
		t.Skip("TF_ACC_SLACK_USER_GROUPS is not set: skipping test that requires Slack usergroups:write scope")
	}

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

// TestAccIncidentScheduleSyncRuleResource_Rotation tests creating a sync rule
// scoped to a single rotation.
//
// NOTE: This test requires Slack usergroups:write scope, which is not available
// in CI. Set TF_ACC_SLACK_USER_GROUPS=1 to run this test locally with a
// workspace that has the scope.
func TestAccIncidentScheduleSyncRuleResource_Rotation(t *testing.T) {
	if os.Getenv("TF_ACC_SLACK_USER_GROUPS") == "" {
		t.Skip("TF_ACC_SLACK_USER_GROUPS is not set: skipping test that requires Slack usergroups:write scope")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create scoped to the "primary" rotation.
			{
				Config: testAccScheduleSyncRuleResourceConfigWithRotation("primary"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_sync_rule.test", "rotation_id", "primary"),
					resource.TestCheckResourceAttr("incident_schedule_sync_rule.test", "sync_type", "on_call"),
				),
			},
			// Import round-trips the rotation scope.
			{
				ResourceName:      "incident_schedule_sync_rule.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: importScheduleSyncRuleStateIDFunc("incident_schedule_sync_rule.test"),
			},
		},
	})
}

func testAccScheduleSyncRuleResourceConfig(syncType string) string {
	return testRunTemplate("incident_schedule_sync_rule", `
resource "incident_schedule" "test" {
  name     = "Test Schedule for Sync Rule"
  timezone = "Europe/London"

  rotations = [{
    id   = "primary"
    name = "Primary"

    versions = [{
      handover_start_at = "2024-05-01T12:00:00Z"
      users             = []
      layers = [{
        id   = "primary"
        name = "Primary"
      }]
      handovers = [{
        interval_type = "daily"
        interval      = 1
      }]
    }]
  }]
}

resource "incident_schedule_sync_target" "test" {
  add_bot_to_group = true

  new_slack_user_group = {
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

func testAccScheduleSyncRuleResourceConfigWithRotation(rotationID string) string {
	return testRunTemplate("incident_schedule_sync_rule", `
resource "incident_schedule" "test" {
  name     = "Test Schedule for Sync Rule"
  timezone = "Europe/London"

  rotations = [{
    id   = "primary"
    name = "Primary"

    versions = [{
      handover_start_at = "2024-05-01T12:00:00Z"
      users             = []
      layers = [{
        id   = "primary"
        name = "Primary"
      }]
      handovers = [{
        interval_type = "daily"
        interval      = 1
      }]
    }]
  }]
}

resource "incident_schedule_sync_target" "test" {
  add_bot_to_group = true

  new_slack_user_group = {
    name        = "Test Sync Rule Rotation Target"
    handle      = "test-sync-rule-rotation-target"
    description = "Target for testing rotation-scoped schedule sync rules"
  }
}

resource "incident_schedule_sync_rule" "test" {
  schedule_id             = incident_schedule.test.id
  schedule_sync_target_id = incident_schedule_sync_target.test.id
  sync_type               = "on_call"
  rotation_id             = {{ quote .RotationID }}
}
`, struct {
		RotationID string
	}{
		RotationID: rotationID,
	})
}

// importScheduleSyncRuleStateIDFunc returns a function that generates the composite import ID.
func importScheduleSyncRuleStateIDFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", nil
		}
		return rs.Primary.Attributes["schedule_id"] + ":" + rs.Primary.Attributes["id"], nil
	}
}
