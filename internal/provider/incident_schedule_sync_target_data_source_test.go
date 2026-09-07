package provider

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"
)

func TestIncidentScheduleSyncTargetDataSourceSchema(t *testing.T) {
	d := NewIncidentScheduleSyncTargetDataSource()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())
}

func TestIncidentScheduleSyncRuleDataSourceSchema(t *testing.T) {
	d := NewIncidentScheduleSyncRuleDataSource()
	var resp datasource.SchemaResponse
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())
}

func TestAccIncidentScheduleSyncTargetDataSourceAmbiguousLookup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_schedule_sync_target" "both" {
  id                  = "01ABC123DEF456GHI789JKL"
  slack_user_group_id = "S06MNNU5BMK"
}
`,
				ExpectError: regexp.MustCompile("Ambiguous lookup"),
			},
		},
	})
}

func TestAccIncidentScheduleSyncTargetDataSourceMissingLookup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_schedule_sync_target" "neither" {
}
`,
				ExpectError: regexp.MustCompile("Missing lookup"),
			},
		},
	})
}
