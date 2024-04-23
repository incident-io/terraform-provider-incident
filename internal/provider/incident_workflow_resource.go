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
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentWorkflowResource{}
	_ resource.ResourceWithImportState = &IncidentWorkflowResource{}
)

type IncidentWorkflowResource struct {
	client *client.ClientWithResponses
}

func NewIncidentWorkflowResource() resource.Resource {
	return &IncidentWorkflowResource{}
}

type IncidentWorkflowResourceModel struct {
	ID               types.String                  `tfsdk:"id"`
	Name             types.String                  `tfsdk:"name"`
	Folder           types.String                  `tfsdk:"folder"`
	Trigger          types.String                  `tfsdk:"trigger"`
	TerraformRepoURL types.String                  `tfsdk:"terraform_repo_url"`
	ConditionGroups  IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	Steps            []IncidentWorkflowStep        `tfsdk:"steps"`
	Expressions      []IncidentEngineExpression    `tfsdk:"expressions"`
}

type IncidentWorkflowStep struct {
	ForEach       types.String                 `tfsdk:"for_each"`
	ID            types.String                 `tfsdk:"id"`
	Name          types.String                 `tfsdk:"name"`
	ParamBindings []IncidentEngineParamBinding `tfsdk:"param_bindings"`
}

func (r *IncidentWorkflowResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (r *IncidentWorkflowResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	paramBindingValueAttributes := map[string]schema.Attribute{
		"literal": schema.StringAttribute{
			Optional: true,
		},
		"reference": schema.StringAttribute{
			Optional: true,
		},
	}

	paramBindingAttributes := map[string]schema.Attribute{
		"array_value": schema.SetNestedAttribute{
			Optional: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: paramBindingValueAttributes,
			},
		},
		"value": schema.SingleNestedAttribute{
			Optional:   true,
			Attributes: paramBindingValueAttributes,
		},
	}

	paramBindingsAttribute := schema.ListNestedAttribute{
		Required: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: paramBindingAttributes,
		},
	}

	conditionsAttribute := schema.SetNestedAttribute{
		Required: true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"operation": schema.StringAttribute{
					Required: true,
				},
				"param_bindings": paramBindingsAttribute,
				"subject": schema.StringAttribute{
					Required: true,
				},
			},
		},
	}

	returnsAttribute := schema.SingleNestedAttribute{
		Required: true,
		Attributes: map[string]schema.Attribute{
			"array": schema.BoolAttribute{
				Required: true,
			},
			"type": schema.StringAttribute{
				Required: true,
			},
		},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Workflows V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"folder": schema.StringAttribute{
				Optional: true,
			},
			"trigger": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"terraform_repo_url": schema.StringAttribute{
				Required: true,
			},
			"condition_groups": schema.SetNestedAttribute{
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"conditions": conditionsAttribute,
					},
				},
			},
			"steps": schema.ListNestedAttribute{
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"for_each": schema.StringAttribute{
							Optional: true,
						},
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Required: true,
						},
						"param_bindings": paramBindingsAttribute,
					},
				},
			},
			"expressions": schema.ListNestedAttribute{
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"else_branch": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"result": schema.SingleNestedAttribute{
									Required:   true,
									Attributes: paramBindingAttributes,
								},
							},
						},
						"id": schema.StringAttribute{
							Computed: true,
						},
						"label": schema.StringAttribute{
							Required: true,
						},
						"operations": schema.ListNestedAttribute{
							Required: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"branches": schema.SingleNestedAttribute{
										Optional: true,
										Attributes: map[string]schema.Attribute{
											"branches": schema.ListNestedAttribute{
												Required: true,
												NestedObject: schema.NestedAttributeObject{
													Attributes: map[string]schema.Attribute{
														"conditions": conditionsAttribute,
														"result":     paramBindingsAttribute,
													},
												},
											},
											"returns": returnsAttribute,
										},
									},
									"filter": schema.SingleNestedAttribute{
										Optional: true,
										Attributes: map[string]schema.Attribute{
											"conditions": conditionsAttribute,
										},
									},
									"navigate": schema.SingleNestedAttribute{
										Optional: true,
										Attributes: map[string]schema.Attribute{
											"reference": schema.StringAttribute{
												Required: true,
											},
										},
									},
									"operation_type": schema.StringAttribute{
										Required: true,
									},
									"parse": schema.SingleNestedAttribute{
										Optional: true,
										Attributes: map[string]schema.Attribute{
											"returns": returnsAttribute,
											"source": schema.StringAttribute{
												Required: true,
											},
										},
									},
								},
							},
						},
						"reference": schema.StringAttribute{
							Required: true,
						},
						"root_reference": schema.StringAttribute{
							Required: true,
						},
					},
				},
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

	payload := client.CreateWorkflowRequestBody{
		Trigger: data.Trigger.ValueString(),
		Workflow: client.WorkflowPayload{
			Name:             data.Name.ValueString(),
			TerraformRepoUrl: data.TerraformRepoURL.ValueStringPointer(),
			OnceFor:          []string{"incident.url"},
			ConditionGroups:  toPayloadConditionGroups(data.ConditionGroups),
			Steps:            toPayloadSteps(data.Steps),
			Expressions:      toPayloadExpressions(data.Expressions),
			RunsOnIncidents:  "newly_created",
			IsDraft:          true,
		},
	}
	if folder := data.Folder.ValueString(); folder != "" {
		payload.Folder = &folder
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

	payload := client.WorkflowsV2UpdateWorkflowJSONRequestBody{
		Workflow: client.WorkflowPayload{
			Name:             data.Name.ValueString(),
			TerraformRepoUrl: data.TerraformRepoURL.ValueStringPointer(),
			OnceFor:          []string{"incident.url"},
			ConditionGroups:  toPayloadConditionGroups(data.ConditionGroups),
			Steps:            toPayloadSteps(data.Steps),
			Expressions:      toPayloadExpressions(data.Expressions),
			RunsOnIncidents:  "newly_created",
			IsDraft:          true,
		},
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
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *IncidentWorkflowResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

// buildModel converts from the response type to the terraform model/schema type.
func (r *IncidentWorkflowResource) buildModel(workflow client.Workflow) *IncidentWorkflowResourceModel {
	model := &IncidentWorkflowResourceModel{
		ID:              types.StringValue(workflow.Id),
		Name:            types.StringValue(workflow.Name),
		Trigger:         types.StringValue(workflow.Trigger.Name),
		ConditionGroups: r.buildConditionGroups(workflow.ConditionGroups),
		Steps:           r.buildSteps(workflow.Steps),
		Expressions:     r.buildExpressions(workflow.Expressions),
	}
	if workflow.Folder != nil {
		model.Folder = types.StringValue(*workflow.Folder)
	}
	if workflow.TerraformRepoUrl != nil {
		model.TerraformRepoURL = types.StringValue(*workflow.TerraformRepoUrl)
	}
	return model
}

func (r *IncidentWorkflowResource) buildConditionGroups(groups []client.ExpressionFilterOptsV2) IncidentEngineConditionGroups {
	var out IncidentEngineConditionGroups

	for _, g := range groups {
		out = append(out, IncidentEngineConditionGroup{
			Conditions: r.buildConditions(g.Conditions),
		})
	}

	return out
}

func (r *IncidentWorkflowResource) buildConditions(conditions []client.ConditionV2) []IncidentEngineCondition {
	var out []IncidentEngineCondition

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
	var out []IncidentWorkflowStep

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
	var out []IncidentEngineParamBinding

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

func (r *IncidentWorkflowResource) buildExpressions(expressions []client.ExpressionV2) []IncidentEngineExpression {
	var out []IncidentEngineExpression

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
	var out []IncidentEngineExpressionOperation

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
				Conditions: r.buildConditions(o.Filter.Conditions),
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
	var out []IncidentEngineBranch

	for _, b := range branches {
		out = append(out, IncidentEngineBranch{
			Conditions: r.buildConditions(b.Conditions),
			Result:     r.buildParamBinding(b.Result),
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
func toPayloadConditionGroups(groups IncidentEngineConditionGroups) []client.ExpressionFilterOptsPayloadV2 {
	var payload []client.ExpressionFilterOptsPayloadV2

	for _, group := range groups {
		payload = append(payload, client.ExpressionFilterOptsPayloadV2{
			Conditions: toPayloadConditions(group.Conditions),
		})
	}

	return payload
}

func toPayloadConditions(conditions []IncidentEngineCondition) []client.ConditionPayloadV2 {
	var out []client.ConditionPayloadV2

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
	var out []client.StepConfigPayload

	for _, step := range steps {
		out = append(out, client.StepConfigPayload{
			ForEach:       step.ForEach.ValueStringPointer(),
			Id:            step.ID.ValueStringPointer(),
			Name:          step.Name.ValueString(),
			ParamBindings: toPayloadParamBindings(step.ParamBindings),
		})
	}

	return out
}

func toPayloadParamBindings(pbs []IncidentEngineParamBinding) []client.EngineParamBindingPayloadV2 {
	var paramBindings []client.EngineParamBindingPayloadV2

	for _, binding := range pbs {
		paramBindings = append(paramBindings, toPayloadParamBinding(binding))
	}

	return paramBindings
}

func toPayloadParamBinding(binding IncidentEngineParamBinding) client.EngineParamBindingPayloadV2 {
	var arrayValue []client.EngineParamBindingValuePayloadV2
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

func toPayloadExpressions(expressions []IncidentEngineExpression) []client.ExpressionPayloadV2 {
	var out []client.ExpressionPayloadV2

	for _, e := range expressions {
		expression := client.ExpressionPayloadV2{
			Label:         e.Label.String(),
			Operations:    toPayloadOperations(e.Operations),
			Reference:     e.Reference.String(),
			RootReference: e.RootReference.String(),
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
	var out []client.ExpressionOperationPayloadV2

	for _, o := range operations {
		operation := client.ExpressionOperationPayloadV2{
			OperationType: client.ExpressionOperationPayloadV2OperationType(o.OperationType.String()),
		}
		if o.Branches != nil {
			operation.Branches = &client.ExpressionBranchesOptsPayloadV2{
				Branches: toPayloadBranches(o.Branches.Branches),
				Returns:  toPayloadReturns(o.Branches.Returns),
			}
		}
		if o.Filter != nil {
			operation.Filter = &client.ExpressionFilterOptsPayloadV2{
				Conditions: toPayloadConditions(o.Filter.Conditions),
			}
		}
		if o.Navigate != nil {
			operation.Navigate = &client.ExpressionNavigateOptsPayloadV2{
				Reference: o.Navigate.Reference.String(),
			}
		}
		if o.Parse != nil {
			operation.Parse = &client.ExpressionParseOptsV2{
				Returns: toPayloadReturns(o.Parse.Returns),
				Source:  o.Parse.Source.String(),
			}
		}
		out = append(out, operation)
	}

	return out
}

func toPayloadBranches(branches []IncidentEngineBranch) []client.ExpressionBranchPayloadV2 {
	var out []client.ExpressionBranchPayloadV2

	for _, b := range branches {
		out = append(out, client.ExpressionBranchPayloadV2{
			Conditions: toPayloadConditions(b.Conditions),
			Result:     toPayloadParamBinding(b.Result),
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
