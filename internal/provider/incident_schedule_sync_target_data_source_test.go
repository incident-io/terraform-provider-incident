package provider

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccIncidentScheduleSyncTargetDataSource(t *testing.T) {
	slackUserGroupID := os.Getenv("TF_ACC_SLACK_USER_GROUP_ID")
	if slackUserGroupID == "" {
		t.Skip("TF_ACC_SLACK_USER_GROUP_ID is not set, skipping")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccScheduleSyncTargetDataSourceConfig(scheduleSyncTargetDataSourceFixture{
					SlackUserGroupID: slackUserGroupID,
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Lookup by id matches the underlying resource.
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule_sync_target.by_id", "id",
						"incident_schedule_sync_target.example", "id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule_sync_target.by_id", "slack_user_group_id",
						"incident_schedule_sync_target.example", "slack_user_group_id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule_sync_target.by_id", "slack_team_id",
						"incident_schedule_sync_target.example", "slack_team_id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule_sync_target.by_id", "add_bot_to_group",
						"incident_schedule_sync_target.example", "add_bot_to_group"),
					// Lookup by slack_user_group_id paginates List and finds the same record.
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule_sync_target.by_slack_user_group_id", "id",
						"incident_schedule_sync_target.example", "id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_schedule_sync_target.by_slack_user_group_id", "slack_team_id",
						"incident_schedule_sync_target.example", "slack_team_id"),
				),
			},
		},
	})
}

func TestAccIncidentScheduleSyncTargetDataSource_NeitherKeySet(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_schedule_sync_target" "no_keys" {}
`,
				ExpectError: regexp.MustCompile(`Either 'id' or 'slack_user_group_id' must be provided`),
			},
		},
	})
}

func TestAccIncidentScheduleSyncTargetDataSource_BothKeysSet(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_schedule_sync_target" "both_keys" {
  id                  = "01J0000000000000000000000Z"
  slack_user_group_id = "S012ABC345"
}
`,
				ExpectError: regexp.MustCompile(`Only one of 'id' or 'slack_user_group_id' may be provided`),
			},
		},
	})
}

var scheduleSyncTargetDataSourceTemplate = template.Must(template.New("incident_schedule_sync_target_data_source").Parse(`
resource "incident_schedule_sync_target" "example" {
  slack_user_group_id = "{{ .SlackUserGroupID }}"
  add_bot_to_group    = false
}

data "incident_schedule_sync_target" "by_id" {
  id = incident_schedule_sync_target.example.id
}

data "incident_schedule_sync_target" "by_slack_user_group_id" {
  slack_user_group_id = incident_schedule_sync_target.example.slack_user_group_id
}
`))

type scheduleSyncTargetDataSourceFixture struct {
	SlackUserGroupID string
}

func testAccScheduleSyncTargetDataSourceConfig(fixture scheduleSyncTargetDataSourceFixture) string {
	var buf bytes.Buffer
	if err := scheduleSyncTargetDataSourceTemplate.Execute(&buf, fixture); err != nil {
		panic(fmt.Errorf("rendering schedule_sync_target data source template: %w", err))
	}
	return buf.String()
}
