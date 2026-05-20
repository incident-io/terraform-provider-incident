package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentScheduleSyncTargetResource(t *testing.T) {
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

  new_slack_user_group {
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
