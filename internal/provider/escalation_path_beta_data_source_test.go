package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestEscalationPathBetaDataSourceSchema(t *testing.T) {
	ctx := context.Background()
	d := NewEscalationPathBetaDataSource()

	var metaResp datasource.MetadataResponse
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_escalation_path_beta" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}
}

// TestEscalationPathBetaDataSourceSchemaMatchesModel guards the seam between the data
// source schema and escalationPathBetaModel: Read reuses the resource's buildModel, so an
// attribute the schema forgets fails every read rather than only the paths using it.
func TestEscalationPathBetaDataSourceSchemaMatchesModel(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&EscalationPathBetaDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), "schema: %s", resp.Diagnostics)

	ep := client.EscalationPathV2{
		Id:      "01M0TAR6EMNDC7BVA45RZDE5KM",
		Name:    "Paged by working hours",
		TeamIds: []string{"01G0J1EXE7AXZ2C93K61WBPYEH"},
		Path: []client.EscalationPathNodeV2{
			{
				Id:   "start",
				Type: client.EscalationPathNodeV2TypeIfElse,
				IfElse: &client.EscalationPathNodeIfElseV2{
					Conditions: []client.ConditionV2{
						{
							Subject:   client.ConditionSubjectV2{Reference: `escalation.working_hours["UK"]`},
							Operation: client.ConditionOperationV2{Value: "is_active"},
						},
					},
					ThenPath: []client.EscalationPathNodeV2{{
						Id:   "then-level",
						Type: client.EscalationPathNodeV2TypeLevel,
						Level: &client.EscalationPathNodeLevelV2{
							Targets: []client.EscalationPathTargetV2{{
								Id:      "01G0J1EXE7AXZ2C93K61WBPYEH",
								Type:    client.EscalationPathTargetV2TypeSchedule,
								Urgency: client.EscalationPathTargetV2UrgencyHigh,
							}},
							TimeToAckSeconds: lo.ToPtr(int64(300)),
						},
					}},
					ElsePath: []client.EscalationPathNodeV2{
						{
							Id:    "else-delay",
							Type:  client.EscalationPathNodeV2TypeDelay,
							Delay: &client.EscalationPathNodeDelayV2{DelaySeconds: lo.ToPtr(int64(120))},
						},
						{
							Id:   "else-reassign",
							Type: client.EscalationPathNodeV2TypeEscalationPath,
							EscalationPath: &client.EscalationPathNodeEscalationPathV2{
								EscalationPathId: "01FCNDV6P870EA6S7TK1DSYDG0",
							},
						},
					},
				},
			},
		},
		WorkingHours: &[]client.WeekdayIntervalConfigV2{{
			Id:       "UK",
			Name:     "UK",
			Timezone: "Europe/London",
			WeekdayIntervals: []client.WeekdayIntervalV2{
				{StartTime: "09:00", EndTime: "17:00", Weekday: client.WeekdayIntervalV2WeekdayMonday},
			},
		}},
		RepeatConfig: &client.EscalationPathRepeatConfigV2{
			RepeatAfterSeconds:    300,
			DelayRepeatOnActivity: true,
		},
	}

	var diags diag.Diagnostics
	model := (&escalationPathBetaResource{}).buildModel(ctx, ep, nil, &diags)
	require.False(t, diags.HasError(), "buildModel: %s", diags)

	state := tfsdk.State{
		Schema: resp.Schema,
		Raw:    tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil),
	}
	assert.False(t, state.Set(ctx, model).HasError(), "setting state from the resource model")
}
