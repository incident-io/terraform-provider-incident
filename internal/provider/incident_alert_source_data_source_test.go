package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestIncidentAlertSourceDataSourceSchemaMatchesModel(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&IncidentAlertSourceDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), "schema: %s", resp.Diagnostics)

	source := client.AlertSourceV2{
		Id:             "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:           "PagerDuty",
		SourceType:     client.AlertSourceV2SourceTypeHttp,
		SecretToken:    lo.ToPtr("secret"),
		AlertEventsUrl: lo.ToPtr("https://api.incident.io/v2/alert_events/http/01FCNDV6P870EA6S7TK1DSYDG0"),
		EmailOptions: &client.AlertSourceEmailOptionsV2{
			EmailAddress: "alerts@example.com",
		},
		JiraOptions: &client.AlertSourceJiraOptionsV2{
			ProjectIds: []string{"PROJ"},
		},
		Template: client.AlertTemplateV2{
			Title: client.EngineParamBindingValueV2{
				Literal: lo.ToPtr(`{"type":"doc","content":[]}`),
			},
			Description: client.EngineParamBindingValueV2{
				Literal: lo.ToPtr(`{"type":"doc","content":[]}`),
			},
			Attributes: []client.AlertTemplateAttributeV2{
				{
					AlertAttributeId: "01GW2G3V0S59R238FAHPDS1R66",
					Binding: client.AlertTemplateAttributeBindingV2{
						Value: &client.EngineParamBindingValueV2{
							Literal: lo.ToPtr("production"),
						},
						MergeStrategy: lo.ToPtr(client.AlertTemplateAttributeBindingV2MergeStrategyFirstWins),
					},
				},
			},
			Expressions: []client.ExpressionV2{
				{
					Label:         "Team",
					Reference:     "team",
					RootReference: "payload.team",
					Returns:       client.ReturnsMetaV2{Array: false, Type: "String"},
					Operations: []client.ExpressionOperationV2{
						{OperationType: client.ExpressionOperationV2OperationTypeCount},
					},
				},
			},
			IsPrivate: true,
			VisibleToTeams: &client.EngineParamBindingV2{
				ArrayValue: &[]client.EngineParamBindingValueV2{
					{Literal: lo.ToPtr("01G0J1EXE7AXZ2C93K61WBPYEH")},
				},
			},
		},
	}

	state := tfsdk.State{
		Schema: resp.Schema,
		Raw:    tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil),
	}
	assert.False(t, state.Set(ctx, alertSourceDataSourceItemFromAPI(source)).HasError(), "setting state from the API model")
}

func accIncidentAlertSourceDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAlertSourceDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.incident_alert_source.by_id", "id",
						"incident_alert_source.test", "id"),
					resource.TestCheckResourceAttrPair(
						"data.incident_alert_source.by_id", "name",
						"incident_alert_source.test", "name"),
					resource.TestCheckResourceAttr(
						"data.incident_alert_source.by_id", "source_type", "http"),
					resource.TestCheckResourceAttrSet(
						"data.incident_alert_source.by_id", "secret_token"),
				),
			},
		},
	})
}

func testAccIncidentAlertSourceDataSourceConfig() string {
	return testRunTemplate("incident_alert_source_data_source", testAccIncidentAlertSourceDataSourceTemplate, nil)
}

const testAccIncidentAlertSourceDataSourceTemplate = `
resource "incident_alert_source" "test" {
  name        = {{ stableSuffix "Test HTTP Alert Source" | quote }}
  source_type = "http"
  template = {
    title = {
      literal = "{\"content\":[{\"content\":[{\"text\":\"Test Alert Title\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
    }
    description = {
      literal = "{\"content\":[{\"content\":[{\"text\":\"Test Alert Description\",\"type\":\"text\"}],\"type\":\"paragraph\"}],\"type\":\"doc\"}"
    }
    attributes = []
    expressions = []
  }
}

data "incident_alert_source" "by_id" {
  id = incident_alert_source.test.id
}
`
