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
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/samber/lo"
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
		MarkdownDescription: apischema.TagDocstring("Workflows V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "id"),
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "name"),
				Required:            true,
			},
			"folder": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "folder"),
				Optional:            true,
			},
			"trigger": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("TriggerSlimResponseBody", "name"),
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"condition_groups": conditionGroupsAttribute,
			"steps": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "steps"),
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
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "once_for"),
				Required:            true,
				ElementType:         types.StringType,
			},
			"include_private_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "include_private_incidents"),
				Required:            true,
			},
			"continue_on_step_error": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "continue_on_step_error"),
				Required:            true,
			},
			"delay": schema.SingleNestedAttribute{
				MarkdownDescription: "Configuration controlling workflow delay behaviour",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"conditions_apply_over_delay": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("WorkflowDelayRequestBody", "conditions_apply_over_delay"),
						Required:            true,
					},
					"for_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("WorkflowDelayRequestBody", "for_seconds"),
						Required:            true,
					},
				},
			},
			"runs_on_incidents": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "runs_on_incidents"),
				Required:            true,
			},
			"runs_on_incident_modes": schema.ListAttribute{
				MarkdownDescription: "Incidents in these modes will be affected by the workflow",
				Required:            true,
				ElementType:         types.StringType,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowResponseBody", "state"),
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

	runsOnIncidentModes := []client.CreateWorkflowRequestBodyRunsOnIncidentModes{}
	for _, v := range data.RunsOnIncidentModes {
		runsOnIncidentModes = append(runsOnIncidentModes, client.CreateWorkflowRequestBodyRunsOnIncidentModes(v.ValueString()))
	}

	payload := client.CreateWorkflowRequestBody{
		Trigger:                 data.Trigger.ValueString(),
		Name:                    data.Name.ValueString(),
		OnceFor:                 onceFor,
		ConditionGroups:         toPayloadConditionGroups(data.ConditionGroups),
		Steps:                   toPayloadSteps(data.Steps),
		Expressions:             toPayloadExpressions(data.Expressions),
		RunsOnIncidents:         client.CreateWorkflowRequestBodyRunsOnIncidents(data.RunsOnIncidents.ValueString()),
		RunsOnIncidentModes:     runsOnIncidentModes,
		Folder:                  data.Folder.ValueStringPointer(),
		IncludePrivateIncidents: data.IncludePrivateIncidents.ValueBool(),
		ContinueOnStepError:     data.ContinueOnStepError.ValueBool(),
		State:                   lo.ToPtr(client.CreateWorkflowRequestBodyState(data.State.ValueString())),
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

	runsOnIncidentModes := []client.UpdateWorkflowRequestBodyRunsOnIncidentModes{}
	for _, v := range data.RunsOnIncidentModes {
		runsOnIncidentModes = append(runsOnIncidentModes, client.UpdateWorkflowRequestBodyRunsOnIncidentModes(v.ValueString()))
	}

	payload := client.WorkflowsV2UpdateWorkflowJSONRequestBody{
		Name:                    data.Name.ValueString(),
		ConditionGroups:         toPayloadConditionGroups(data.ConditionGroups),
		Steps:                   toPayloadSteps(data.Steps),
		Expressions:             toPayloadExpressions(data.Expressions),
		OnceFor:                 onceFor,
		RunsOnIncidents:         client.UpdateWorkflowRequestBodyRunsOnIncidents(data.RunsOnIncidents.ValueString()),
		RunsOnIncidentModes:     runsOnIncidentModes,
		Folder:                  data.Folder.ValueStringPointer(),
		IncludePrivateIncidents: data.IncludePrivateIncidents.ValueBool(),
		ContinueOnStepError:     data.ContinueOnStepError.ValueBool(),
		State:                   lo.ToPtr(client.UpdateWorkflowRequestBodyState(data.State.ValueString())),
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

// buildModel converts from the response type to the terraform model/schema type.
func (r *IncidentWorkflowResource) buildModel(workflow client.Workflow) *IncidentWorkflowResourceModel {
	model := &IncidentWorkflowResourceModel{
		ID:                      types.StringValue(workflow.Id),
		Name:                    types.StringValue(workflow.Name),
		Trigger:                 types.StringValue(workflow.Trigger.Name),
		ConditionGroups:         r.buildConditionGroups(workflow.ConditionGroups),
		Steps:                   r.buildSteps(workflow.Steps),
		Expressions:             r.buildExpressions(workflow.Expressions),
		OnceFor:                 r.buildOnceFor(workflow.OnceFor),
		RunsOnIncidentModes:     r.buildRunsOnIncidentModes(workflow.RunsOnIncidentModes),
		IncludePrivateIncidents: types.BoolValue(workflow.IncludePrivateIncidents),
		ContinueOnStepError:     types.BoolValue(workflow.ContinueOnStepError),
		RunsOnIncidents:         types.StringValue(string(workflow.RunsOnIncidents)),
		State:                   types.StringValue(string(workflow.State)),
	}
	if workflow.Folder != nil {
		model.Folder = types.StringValue(*workflow.Folder)
	}
	if workflow.Delay != nil {
		model.Delay = &IncidentWorkflowDelay{
			ConditionsApplyOverDelay: types.BoolValue(workflow.Delay.ConditionsApplyOverDelay),
			ForSeconds:               types.Int64Value(workflow.Delay.ForSeconds),
		}
	}
	return model
}

func (r *IncidentWorkflowResource) buildOnceFor(onceFor []client.EngineReferenceV2) []basetypes.StringValue {
	out := []basetypes.StringValue{}

	for _, ref := range onceFor {
		out = append(out, types.StringValue(ref.Key))
	}

	return out
}

func (r *IncidentWorkflowResource) buildRunsOnIncidentModes(modes []client.WorkflowRunsOnIncidentModes) []basetypes.StringValue {
	out := []basetypes.StringValue{}

	for _, mode := range modes {
		out = append(out, types.StringValue(string(mode)))
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditionGroups(groups []client.ConditionGroupV2) IncidentEngineConditionGroups {
	var out IncidentEngineConditionGroups

	for _, g := range groups {
		out = append(out, IncidentEngineConditionGroup{
			Conditions: r.buildConditions(g.Conditions),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditions(conditions []client.ConditionV2) []IncidentEngineCondition {
	out := []IncidentEngineCondition{}

	for _, c := range conditions {
		out = append(out, IncidentEngineCondition{
			Subject:       types.StringValue(c.Subject.Reference),
			Operation:     types.StringValue(c.Operation.Value),
			ParamBindings: r.buildParamBindings(c.ParamBindings),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildSteps(steps []client.StepConfig) []IncidentWorkflowStep {
	out := []IncidentWorkflowStep{}

	for _, s := range steps {
		out = append(out, IncidentWorkflowStep{
			ForEach:       types.StringPointerValue(s.ForEach),
			ID:            types.StringValue(s.Id),
			Name:          types.StringValue(s.Name),
			ParamBindings: r.buildParamBindings(s.ParamBindings),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildParamBindings(pbs []client.EngineParamBindingV2) []IncidentEngineParamBinding {
	out := []IncidentEngineParamBinding{}

	for _, pb := range pbs {
		out = append(out, r.buildParamBinding(pb))
	}

	return out
}

func (r *IncidentWorkflowResource) buildParamBinding(pb client.EngineParamBindingV2) IncidentEngineParamBinding {
	var arrayValue []IncidentEngineParamBindingValue
	if pb.ArrayValue != nil {
		for _, v := range *pb.ArrayValue {
			arrayValue = append(arrayValue, IncidentEngineParamBindingValue{
				Literal:   types.StringPointerValue(v.Literal),
				Reference: types.StringPointerValue(v.Reference),
			})
		}
	}

	var value *IncidentEngineParamBindingValue
	if pb.Value != nil {
		value = &IncidentEngineParamBindingValue{
			Literal:   types.StringPointerValue(pb.Value.Literal),
			Reference: types.StringPointerValue(pb.Value.Reference),
		}
	}

	return IncidentEngineParamBinding{
		ArrayValue: arrayValue,
		Value:      value,
	}
}

func (r *IncidentWorkflowResource) buildExpressions(expressions []client.ExpressionV2) IncidentEngineExpressions {
	out := IncidentEngineExpressions{}

	for _, e := range expressions {
		expression := IncidentEngineExpression{
			ID:            types.StringValue(e.Id),
			Label:         types.StringValue(e.Label),
			Operations:    r.buildOperations(e.Operations),
			Reference:     types.StringValue(e.Reference),
			RootReference: types.StringValue(e.RootReference),
		}
		if e.ElseBranch != nil {
			expression.ElseBranch = &IncidentEngineElseBranch{
				Result: r.buildParamBinding(e.ElseBranch.Result),
			}
		}
		out = append(out, expression)
	}

	return out
}

func (r *IncidentWorkflowResource) buildOperations(operations []client.ExpressionOperationV2) []IncidentEngineExpressionOperation {
	out := []IncidentEngineExpressionOperation{}

	for _, o := range operations {
		operation := IncidentEngineExpressionOperation{
			OperationType: types.StringValue(string(o.OperationType)),
		}
		if o.Branches != nil {
			operation.Branches = &IncidentEngineExpressionBranchesOpts{
				Branches: r.buildBranches(o.Branches.Branches),
				Returns:  r.buildReturns(o.Branches.Returns),
			}
		}
		if o.Filter != nil {
			operation.Filter = &IncidentEngineExpressionFilterOpts{
				ConditionGroups: r.buildConditionGroups(o.Filter.ConditionGroups),
			}
		}
		if o.Navigate != nil {
			operation.Navigate = &IncidentEngineExpressionNavigateOpts{
				Reference: types.StringValue(o.Navigate.Reference),
			}
		}
		if o.Parse != nil {
			operation.Parse = &IncidentEngineExpressionParseOpts{
				Returns: r.buildReturns(o.Parse.Returns),
				Source:  types.StringValue(o.Parse.Source),
			}
		}
		out = append(out, operation)
	}

	return out
}

func (r *IncidentWorkflowResource) buildBranches(branches []client.ExpressionBranchV2) []IncidentEngineBranch {
	out := []IncidentEngineBranch{}

	for _, b := range branches {
		out = append(out, IncidentEngineBranch{
			ConditionGroups: r.buildConditionGroups(b.ConditionGroups),
			Result:          r.buildParamBinding(b.Result),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildReturns(returns client.ReturnsMetaV2) IncidentEngineReturnsMeta {
	return IncidentEngineReturnsMeta{
		Array: types.BoolValue(returns.Array),
		Type:  types.StringValue(returns.Type),
	}
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
			Id:            e.ID.ValueString(),
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
			operation.Parse = &client.ExpressionParseOptsV2{
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
