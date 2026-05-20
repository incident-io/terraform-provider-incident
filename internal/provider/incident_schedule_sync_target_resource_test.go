package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccIncidentScheduleSyncTargetResource tests creating and updating schedule sync targets.
//
// NOTE: This test requires Slack usergroups:write scope, which is not available in CI.
// Set TF_ACC_SLACK_USER_GROUPS=1 to run this test locally with a workspace that has the scope.
func TestAccIncidentScheduleSyncTargetResource(t *testing.T) {
	if os.Getenv("TF_ACC_SLACK_USER_GROUPS") == "" {
		t.Skip("TF_ACC_SLACK_USER_GROUPS is not set: skipping test that requires Slack usergroups:write scope")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with new Slack user group and add_bot_to_group = true
			{
				Config: testAccScheduleSyncTargetResourceConfigNewWithBot("test-oncall", "platform-oncall", "Platform team on-call group", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_sync_target.test", "add_bot_to_group", "true"),
					resource.TestCheckResourceAttrSet("incident_schedule_sync_target.test", "id"),
					resource.TestCheckResourceAttrSet("incident_schedule_sync_target.test", "slack_user_group_id"),
					resource.TestCheckResourceAttrSet("incident_schedule_sync_target.test", "slack_team_id"),
				),
			},
			// Update add_bot_to_group to false
			{
				Config: testAccScheduleSyncTargetResourceConfigNewWithBot("test-oncall", "platform-oncall", "Platform team on-call group", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_sync_target.test", "add_bot_to_group", "false"),
				),
			},
			// Import
			{
				ResourceName:      "incident_schedule_sync_target.test",
				ImportState:       true,
				ImportStateVerify: true,
				// new_slack_user_group is not returned by API
				ImportStateVerifyIgnore: []string{"new_slack_user_group"},
			},
		},
	})
}

func testAccScheduleSyncTargetResourceConfigNewWithBot(name, handle, description string, addBotToGroup bool) string {
	return testRunTemplate("incident_schedule_sync_target", `
resource "incident_schedule_sync_target" "test" {
  add_bot_to_group = {{ .AddBotToGroup }}

  new_slack_user_group = {
    name        = {{ quote .Name }}
    handle      = {{ quote .Handle }}
    description = {{ quote .Description }}
  }
}
`, struct {
		Name          string
		Handle        string
		Description   string
		AddBotToGroup bool
	}{
		Name:          name,
		Handle:        handle,
		Description:   description,
		AddBotToGroup: addBotToGroup,
	})
}
