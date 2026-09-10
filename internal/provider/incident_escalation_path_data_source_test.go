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

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func TestEscalationPathDataSourceSchema(t *testing.T) {
	ctx := context.Background()
	d := NewIncidentEscalationPathDataSource()

	var metaResp datasource.MetadataResponse
	d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_escalation_path" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}
}

// TestEscalationPathDataSourceValidateConfig covers the id-XOR-name lookup, including
// the case a value isn't known yet: an id read off a path created in the same apply is
// non-null but unknown, and rejecting that would fail a plan that would have applied.
func TestEscalationPathDataSourceValidateConfig(t *testing.T) {
	for _, tc := range []struct {
		name    string
		id      tftypes.Value
		lookup  tftypes.Value
		wantErr string
	}{
		{name: "id only", id: tftypes.NewValue(tftypes.String, "01PAYMENTS"), lookup: tftypes.NewValue(tftypes.String, nil)},
		{name: "name only", id: tftypes.NewValue(tftypes.String, nil), lookup: tftypes.NewValue(tftypes.String, "Urgent support")},
		{
			name:    "both",
			id:      tftypes.NewValue(tftypes.String, "01PAYMENTS"),
			lookup:  tftypes.NewValue(tftypes.String, "Urgent support"),
			wantErr: "Ambiguous lookup",
		},
		{
			name:    "neither",
			id:      tftypes.NewValue(tftypes.String, nil),
			lookup:  tftypes.NewValue(tftypes.String, nil),
			wantErr: "Missing lookup",
		},
		{
			name:   "an id another resource computes",
			id:     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			lookup: tftypes.NewValue(tftypes.String, nil),
		},
		{
			name:   "a name another resource computes",
			id:     tftypes.NewValue(tftypes.String, nil),
			lookup: tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			d := &EscalationPathDataSource{}

			var schemaResp datasource.SchemaResponse
			d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
			require.False(t, schemaResp.Diagnostics.HasError(), "schema: %s", schemaResp.Diagnostics)

			tfType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
			require.True(t, ok, "the schema's Terraform type is not an object")

			config := map[string]tftypes.Value{}
			for name, attrType := range tfType.AttributeTypes {
				config[name] = tftypes.NewValue(attrType, nil)
			}
			config["id"] = tc.id
			config["name"] = tc.lookup

			resp := &datasource.ValidateConfigResponse{}
			d.ValidateConfig(ctx, datasource.ValidateConfigRequest{
				Config: tfsdk.Config{
					Schema: schemaResp.Schema,
					Raw:    tftypes.NewValue(tfType, config),
				},
			}, resp)

			if tc.wantErr == "" {
				require.False(t, resp.Diagnostics.HasError(), "unexpected diagnostics: %s", resp.Diagnostics)
				return
			}

			require.True(t, resp.Diagnostics.HasError(), "expected an error diagnostic")
			assert.Equal(t, tc.wantErr, resp.Diagnostics.Errors()[0].Summary())
		})
	}
}

// TestEscalationPathDataSourceSchemaMatchesModel guards the seam between the data
// source schema and escalationPathModel: Read reuses the resource's buildModel, so an
// attribute the schema forgets fails every read rather than only the paths using it.
func TestEscalationPathDataSourceSchemaMatchesModel(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&EscalationPathDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
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
	model := (&escalationPathResource{}).buildModel(ctx, ep, nil, &diags)
	require.False(t, diags.HasError(), "buildModel: %s", diags)

	state := tfsdk.State{
		Schema: resp.Schema,
		Raw:    tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil),
	}
	assert.False(t, state.Set(ctx, model).HasError(), "setting state from the resource model")
}
