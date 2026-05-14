package provider

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentScheduleSyncTargetResource(t *testing.T) {
	slackUserGroupID := os.Getenv("TF_ACC_SLACK_USER_GROUP_ID")
	if slackUserGroupID == "" {
		t.Skip("TF_ACC_SLACK_USER_GROUP_ID is not set, skipping")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with add_bot_to_group=false.
			{
				Config: testAccScheduleSyncTargetResourceConfig(scheduleSyncTargetElement{
					SlackUserGroupID: slackUserGroupID,
					AddBotToGroup:    false,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_schedule_sync_target.example", "slack_user_group_id", slackUserGroupID),
					resource.TestCheckResourceAttr(
						"incident_schedule_sync_target.example", "add_bot_to_group", "false"),
					resource.TestCheckResourceAttrSet(
						"incident_schedule_sync_target.example", "id"),
					resource.TestCheckResourceAttrSet(
						"incident_schedule_sync_target.example", "slack_team_id"),
				),
			},
			// Import round-trip.
			{
				ResourceName:      "incident_schedule_sync_target.example",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Flip add_bot_to_group to exercise the RequiresReplace recreate path.
			{
				Config: testAccScheduleSyncTargetResourceConfig(scheduleSyncTargetElement{
					SlackUserGroupID: slackUserGroupID,
					AddBotToGroup:    true,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"incident_schedule_sync_target.example", "slack_user_group_id", slackUserGroupID),
					resource.TestCheckResourceAttr(
						"incident_schedule_sync_target.example", "add_bot_to_group", "true"),
				),
			},
		},
	})
}

var scheduleSyncTargetTemplate = template.Must(template.New("incident_schedule_sync_target").Parse(`
resource "incident_schedule_sync_target" "example" {
  slack_user_group_id = "{{ .SlackUserGroupID }}"
  add_bot_to_group    = {{ .AddBotToGroup }}
}
`))

type scheduleSyncTargetElement struct {
	SlackUserGroupID string
	AddBotToGroup    bool
}

func testAccScheduleSyncTargetResourceConfig(element scheduleSyncTargetElement) string {
	var buf bytes.Buffer
	if err := scheduleSyncTargetTemplate.Execute(&buf, element); err != nil {
		panic(fmt.Errorf("rendering schedule_sync_target template: %w", err))
	}
	return buf.String()
}
