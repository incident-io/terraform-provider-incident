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

// TestIncidentWorkflowDataSourceSchemaMatchesModel guards the seam between the data
// source schema and IncidentWorkflowResourceModel. Read reuses the resource's
// buildModel, so an attribute the schema forgets (or types differently) fails
// State.Set for every workflow read rather than only for workflows using it — and
// nothing else would catch that until someone ran a plan against a real workspace.
func TestIncidentWorkflowDataSourceSchemaMatchesModel(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&IncidentWorkflowDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), "schema: %s", resp.Diagnostics)

	// Everything the API can hand back, so a nested attribute missing from the
	// schema shows up as an error rather than an untouched null.
	workflow := client.WorkflowV2{
		Id:                  "01HY0QGB4M1XKGQGKQD0N9MHKG",
		Name:                "My workflow",
		Trigger:             client.TriggerSlimV2{Name: "incident.updated"},
		Folder:              lo.ToPtr("My folder"),
		Shortform:           lo.ToPtr("page-the-ceo"),
		State:               client.WorkflowV2State("active"),
		RunsOnIncidents:     client.WorkflowV2RunsOnIncidents("newly_created"),
		RunsOnIncidentModes: []client.WorkflowV2RunsOnIncidentModes{"standard"},
		OnceFor:             []client.EngineReferenceV2{{Key: "incident"}},
		OwningTeamIds:       lo.ToPtr([]string{"01G0J1EXE7AXZ2C93K61WBPYEH"}),
		ConditionGroups: []client.ConditionGroupV2{
			{
				Conditions: []client.ConditionV2{
					{
						Subject:   client.ConditionSubjectV2{Reference: "incident.status.category"},
						Operation: client.ConditionOperationV2{Value: "one_of"},
						ParamBindings: []client.EngineParamBindingV2{
							{
								ArrayValue: &[]client.EngineParamBindingValueV2{
									{Literal: lo.ToPtr("open")},
								},
							},
						},
					},
				},
			},
		},
		Steps: []client.StepConfigV2{
			{
				Id:      "01HXVEA7Y0VWQBJB4F2X8WNRW6",
				Name:    "incident.create_follow_ups",
				ForEach: lo.ToPtr("participants"),
				ParamBindings: []client.EngineParamBindingV2{
					{Value: &client.EngineParamBindingValueV2{Reference: lo.ToPtr("incident")}},
				},
			},
		},
		Expressions: []client.ExpressionV2{
			{
				Label:         "Count active participants",
				Reference:     "participants_cnt",
				RootReference: "incident.active_participants",
				Operations: []client.ExpressionOperationV2{
					{OperationType: client.ExpressionOperationV2OperationType("count")},
				},
			},
		},
		FormFields: &[]client.WorkflowFormFieldV2{
			{
				Id:          "01FCNDV6P870EA6S7TK1DSYDG0",
				Key:         "affected_customer",
				Title:       "Affected customer",
				Type:        "User",
				Array:       true,
				Required:    true,
				Description: lo.ToPtr("The customer affected by this incident"),
			},
		},
		Delay: &client.WorkflowDelayV2{
			ConditionsApplyOverDelay: true,
			ForSeconds:               60,
		},
		ContinueOnStepError:       true,
		IncludePrivateIncidents:   true,
		IncludePrivateEscalations: true,
		PrivateIncidentScope:      client.WorkflowV2PrivateIncidentScope("owning_teams"),
	}

	model := (&IncidentWorkflowResource{}).buildModel(ctx, workflow, nil)

	state := tfsdk.State{
		Schema: resp.Schema,
		Raw:    tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil),
	}
	diags := state.Set(ctx, model)
	assert.False(t, diags.HasError(), "setting state from the resource model: %s", diags)
}

// TestAccIncidentWorkflowDataSource checks that the data source reads back the full
// workflow definition, not just its metadata: the nested steps, expressions and
// condition groups have to survive the round-trip too.
func TestAccIncidentWorkflowDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentWorkflowDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Top-level attributes match the resource we created
					resource.TestCheckResourceAttrPair(
						"data.incident_workflow.by_id", "id",
						"incident_workflow.example", "id"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "name", incidentWorkflowDefault().Name),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "trigger", "incident.updated"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "state", "draft"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "runs_on_incidents", "newly_created"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "runs_on_incident_modes.#", "1"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "continue_on_step_error", "false"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "once_for.0", "incident"),

					// Nested condition groups are returned
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "condition_groups.0.conditions.0.subject", "incident.status.category"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "condition_groups.0.conditions.0.operation", "one_of"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "condition_groups.0.conditions.0.param_bindings.0.array_value.0.literal",
						incidentWorkflowDefault().ConditionParam),

					// Nested steps are returned, including their param bindings
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "steps.0.name", "incident.create_follow_ups"),
					resource.TestCheckResourceAttrPair(
						"data.incident_workflow.by_id", "steps.0.id",
						"incident_workflow.example", "steps.0.id"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "steps.0.param_bindings.0.value.reference", "incident"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "steps.0.param_bindings.1.array_value.0.literal",
						incidentWorkflowDefault().StepFollowUpName),

					// Nested expressions are returned
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "expressions.0.label", incidentWorkflowDefault().ExpressionLabel),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "expressions.0.reference", "participants_cnt"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "expressions.0.root_reference", "incident.active_participants"),
					resource.TestCheckResourceAttr(
						"data.incident_workflow.by_id", "expressions.0.operations.0.operation_type", "count"),

					// This workflow has no delay or form fields, so both read back unset
					resource.TestCheckNoResourceAttr(
						"data.incident_workflow.by_id", "delay.for_seconds"),
					resource.TestCheckNoResourceAttr(
						"data.incident_workflow.by_id", "form_fields.#"),
				),
			},
		},
	})
}

// testAccIncidentWorkflowDataSourceConfig reuses the resource test's workflow so the
// data source is always read against the same definition the resource tests assert on.
func testAccIncidentWorkflowDataSourceConfig() string {
	return testAccIncidentWorkflowResourceConfig(nil) + `
data "incident_workflow" "by_id" {
  id = incident_workflow.example.id
}
`
}
