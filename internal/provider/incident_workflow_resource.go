package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentWorkflowResource{}
	_ resource.ResourceWithImportState = &IncidentWorkflowResource{}
)

type IncidentWorkflowResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

func NewIncidentWorkflowResource() resource.Resource {
	return &IncidentWorkflowResource{}
}

type IncidentWorkflowResourceModel struct {
	ID                      types.String                  `tfsdk:"id"`
	Name                    types.String                  `tfsdk:"name"`
	Folder                  types.String                  `tfsdk:"folder"`
	Trigger                 types.String                  `tfsdk:"trigger"`
	ConditionGroups         IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	Steps                   []IncidentWorkflowStep        `tfsdk:"steps"`
	Expressions             IncidentEngineExpressions     `tfsdk:"expressions"`
	OnceFor                 []types.String                `tfsdk:"once_for"`
	IncludePrivateIncidents types.Bool                    `tfsdk:"include_private_incidents"`
	ContinueOnStepError     types.Bool                    `tfsdk:"continue_on_step_error"`
	Delay                   *IncidentWorkflowDelay        `tfsdk:"delay"`
	RunsOnIncidents         types.String                  `tfsdk:"runs_on_incidents"`
	RunsOnIncidentModes     []types.String                `tfsdk:"runs_on_incident_modes"`
	State                   types.String                  `tfsdk:"state"`
}

type IncidentWorkflowStep struct {
	ForEach       types.String                 `tfsdk:"for_each"`
	ID            types.String                 `tfsdk:"id"`
	Name          types.String                 `tfsdk:"name"`
	ParamBindings []IncidentEngineParamBinding `tfsdk:"param_bindings"`
}

type IncidentWorkflowDelay struct {
	ConditionsApplyOverDelay types.Bool  `tfsdk:"conditions_apply_over_delay"`
	ForSeconds               types.Int64 `tfsdk:"for_seconds"`
}

func (r *IncidentWorkflowResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (r *IncidentWorkflowResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `This resource is used to manage Workflows.

We'd generally recommend building workflows in our [web dashboard](https://app.incident.io/workflows), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing workflow and copy the resulting Terraform without persisting it. You can learn more in this [Loom](https://www.loom.com/share/b833d7d0fd114d6ba3f24d8c72e5208f?sid=c6d3cc3f-aa93-44ba-b12d-a0a4cbe09448).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "id"),
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "name"),
				Required:            true,
			},
			"folder": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "folder"),
				Optional:            true,
			},
			"trigger": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("TriggerSlim", "name"),
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"condition_groups": conditionGroupsAttribute,
			"steps": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "steps"),
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"for_each": schema.StringAttribute{
							Optional: true,
						},
						"id": schema.StringAttribute{
							Required: true,
						},
						"name": schema.StringAttribute{
							Required: true,
						},
						"param_bindings": paramBindingsAttribute,
					},
				},
			},
			"expressions": expressionsAttribute,
			"once_for": schema.ListAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "once_for"),
				Required:            true,
				ElementType:         types.StringType,
			},
			"include_private_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "include_private_incidents"),
				Required:            true,
			},
			"continue_on_step_error": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "continue_on_step_error"),
				Required:            true,
			},
			"delay": schema.SingleNestedAttribute{
				MarkdownDescription: "Configuration controlling workflow delay behaviour",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"conditions_apply_over_delay": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("WorkflowDelay", "conditions_apply_over_delay"),
						Required:            true,
					},
					"for_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("WorkflowDelay", "for_seconds"),
						Required:            true,
					},
				},
			},
			"runs_on_incidents": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "runs_on_incidents"),
				Required:            true,
			},
			"runs_on_incident_modes": schema.ListAttribute{
				MarkdownDescription: "Incidents in these modes will be affected by the workflow",
				Required:            true,
				ElementType:         types.StringType,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("Workflow", "state"),
				Required:            true,
			},
		},
	}
}

func (r *IncidentWorkflowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentWorkflowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	onceFor := []string{}
	for _, v := range data.OnceFor {
		onceFor = append(onceFor, v.ValueString())
	}

	runsOnIncidentModes := []client.CreateWorkflowPayloadRunsOnIncidentModes{}
	for _, v := range data.RunsOnIncidentModes {
		runsOnIncidentModes = append(runsOnIncidentModes, client.CreateWorkflowPayloadRunsOnIncidentModes(v.ValueString()))
	}

	payload := client.CreateWorkflowPayload{
		Trigger:                 data.Trigger.ValueString(),
		Name:                    data.Name.ValueString(),
		OnceFor:                 onceFor,
		ConditionGroups:         toPayloadConditionGroups(data.ConditionGroups),
		Steps:                   toPayloadSteps(data.Steps),
		Expressions:             toPayloadExpressions(data.Expressions),
		RunsOnIncidents:         client.CreateWorkflowPayloadRunsOnIncidents(data.RunsOnIncidents.ValueString()),
		RunsOnIncidentModes:     runsOnIncidentModes,
		Folder:                  data.Folder.ValueStringPointer(),
		IncludePrivateIncidents: data.IncludePrivateIncidents.ValueBool(),
		ContinueOnStepError:     data.ContinueOnStepError.ValueBool(),
		State:                   lo.ToPtr(client.CreateWorkflowPayloadState(data.State.ValueString())),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
	}

	if data.Delay != nil {
		payload.Delay = &client.WorkflowDelay{
			ConditionsApplyOverDelay: data.Delay.ConditionsApplyOverDelay.ValueBool(),
			ForSeconds:               data.Delay.ForSeconds.ValueInt64(),
		}
	}

	result, err := r.client.WorkflowsV2CreateWorkflowWithResponse(ctx, payload)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create workflow, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a workflow resource with id=%s", result.JSON201.Workflow.Id))
	data = r.buildModel(result.JSON201.Workflow)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentWorkflowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state *IncidentWorkflowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var data *IncidentWorkflowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	onceFor := []string{}
	for _, v := range data.OnceFor {
		onceFor = append(onceFor, v.ValueString())
	}

	runsOnIncidentModes := []client.UpdateWorkflowPayloadRunsOnIncidentModes{}
	for _, v := range data.RunsOnIncidentModes {
		runsOnIncidentModes = append(runsOnIncidentModes, client.UpdateWorkflowPayloadRunsOnIncidentModes(v.ValueString()))
	}

	payload := client.WorkflowsV2UpdateWorkflowJSONRequestBody{
		Name:                    data.Name.ValueString(),
		ConditionGroups:         toPayloadConditionGroups(data.ConditionGroups),
		Steps:                   toPayloadSteps(data.Steps),
		Expressions:             toPayloadExpressions(data.Expressions),
		OnceFor:                 onceFor,
		RunsOnIncidents:         client.UpdateWorkflowPayloadRunsOnIncidents(data.RunsOnIncidents.ValueString()),
		RunsOnIncidentModes:     runsOnIncidentModes,
		Folder:                  data.Folder.ValueStringPointer(),
		IncludePrivateIncidents: data.IncludePrivateIncidents.ValueBool(),
		ContinueOnStepError:     data.ContinueOnStepError.ValueBool(),
		State:                   lo.ToPtr(client.UpdateWorkflowPayloadState(data.State.ValueString())),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
	}

	if data.Delay != nil {
		payload.Delay = &client.WorkflowDelay{
			ConditionsApplyOverDelay: data.Delay.ConditionsApplyOverDelay.ValueBool(),
			ForSeconds:               data.Delay.ForSeconds.ValueInt64(),
		}
	}

	result, err := r.client.WorkflowsV2UpdateWorkflowWithResponse(ctx, state.ID.ValueString(), payload)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update workflow, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.Workflow)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentWorkflowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentWorkflowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.WorkflowsV2ShowWorkflowWithResponse(ctx, data.ID.ValueString())
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read workflow, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.Workflow)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentWorkflowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentWorkflowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.WorkflowsV2DestroyWorkflowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete workflow, got error: %s", err))
		return
	}
}

func (r *IncidentWorkflowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResource(ctx, r.client, req, resp, client.ManagedResourceV2ResourceTypeWorkflow, r.terraformVersion)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentWorkflowResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client.Client
	r.terraformVersion = client.TerraformVersion
}

// toPayloadConditionGroups converts from the terraform model to the http payload type.
// The payload type is different from the response type, which includes more information such as labels.
func toPayloadConditionGroups(groups IncidentEngineConditionGroups) []client.ConditionGroupPayloadV2 {
	var payload []client.ConditionGroupPayloadV2

	for _, group := range groups {
		payload = append(payload, client.ConditionGroupPayloadV2{
			Conditions: toPayloadConditions(group.Conditions),
		})
	}

	return payload
}

func toPayloadConditions(conditions []IncidentEngineCondition) []client.ConditionPayloadV2 {
	out := []client.ConditionPayloadV2{}

	for _, c := range conditions {
		out = append(out, client.ConditionPayloadV2{
			Subject:       c.Subject.ValueString(),
			Operation:     c.Operation.ValueString(),
			ParamBindings: toPayloadParamBindings(c.ParamBindings),
		})
	}

	return out
}

func toPayloadSteps(steps []IncidentWorkflowStep) []client.StepConfigPayload {
	out := []client.StepConfigPayload{}

	for _, step := range steps {
		out = append(out, client.StepConfigPayload{
			ForEach:       step.ForEach.ValueStringPointer(),
			Id:            step.ID.ValueString(),
			Name:          step.Name.ValueString(),
			ParamBindings: toPayloadParamBindings(step.ParamBindings),
		})
	}

	return out
}

func toPayloadParamBindings(pbs []IncidentEngineParamBinding) []client.EngineParamBindingPayloadV2 {
	paramBindings := []client.EngineParamBindingPayloadV2{}

	for _, binding := range pbs {
		paramBindings = append(paramBindings, toPayloadParamBinding(binding))
	}

	return paramBindings
}

func toPayloadParamBinding(binding IncidentEngineParamBinding) client.EngineParamBindingPayloadV2 {
	arrayValue := []client.EngineParamBindingValuePayloadV2{}
	for _, v := range binding.ArrayValue {
		arrayValue = append(arrayValue, *toPayloadParamBindingValue(&v))
	}

	var value *client.EngineParamBindingValuePayloadV2
	if binding.Value != nil {
		value = toPayloadParamBindingValue(binding.Value)
	}

	return client.EngineParamBindingPayloadV2{
		ArrayValue: &arrayValue,
		Value:      value,
	}
}

func toPayloadParamBindingValue(v *IncidentEngineParamBindingValue) *client.EngineParamBindingValuePayloadV2 {
	return &client.EngineParamBindingValuePayloadV2{
		Literal:   v.Literal.ValueStringPointer(),
		Reference: v.Reference.ValueStringPointer(),
	}
}

func toPayloadExpressions(expressions IncidentEngineExpressions) []client.ExpressionPayloadV2 {
	out := []client.ExpressionPayloadV2{}

	for _, e := range expressions {
		expression := client.ExpressionPayloadV2{
			Label:         e.Label.ValueString(),
			Operations:    toPayloadOperations(e.Operations),
			Reference:     e.Reference.ValueString(),
			RootReference: e.RootReference.ValueString(),
		}
		if e.ElseBranch != nil {
			expression.ElseBranch = &client.ExpressionElseBranchPayloadV2{
				Result: toPayloadParamBinding(e.ElseBranch.Result),
			}
		}
		out = append(out, expression)
	}

	return out
}

func toPayloadOperations(operations []IncidentEngineExpressionOperation) []client.ExpressionOperationPayloadV2 {
	out := []client.ExpressionOperationPayloadV2{}

	for _, o := range operations {
		operation := client.ExpressionOperationPayloadV2{
			OperationType: client.ExpressionOperationPayloadV2OperationType(o.OperationType.ValueString()),
		}
		if o.Branches != nil {
			operation.Branches = &client.ExpressionBranchesOptsPayloadV2{
				Branches: toPayloadBranches(o.Branches.Branches),
				Returns:  toPayloadReturns(o.Branches.Returns),
			}
		}
		if o.Filter != nil {
			operation.Filter = &client.ExpressionFilterOptsPayloadV2{
				ConditionGroups: toPayloadConditionGroups(o.Filter.ConditionGroups),
			}
		}
		if o.Navigate != nil {
			operation.Navigate = &client.ExpressionNavigateOptsPayloadV2{
				Reference: o.Navigate.Reference.ValueString(),
			}
		}
		if o.Parse != nil {
			operation.Parse = &client.ExpressionParseOptsPayloadV2{
				Returns: toPayloadReturns(o.Parse.Returns),
				Source:  o.Parse.Source.ValueString(),
			}
		}
		out = append(out, operation)
	}

	return out
}

func toPayloadBranches(branches []IncidentEngineBranch) []client.ExpressionBranchPayloadV2 {
	out := []client.ExpressionBranchPayloadV2{}

	for _, b := range branches {
		out = append(out, client.ExpressionBranchPayloadV2{
			ConditionGroups: toPayloadConditionGroups(b.ConditionGroups),
			Result:          toPayloadParamBinding(b.Result),
		})
	}

	return out
}

func toPayloadReturns(returns IncidentEngineReturnsMeta) client.ReturnsMetaV2 {
	return client.ReturnsMetaV2{
		Array: returns.Array.ValueBool(),
		Type:  returns.Type.ValueString(),
	}
}
