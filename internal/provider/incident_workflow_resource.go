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
}

func (r *IncidentWorkflowResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (r *IncidentWorkflowResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	paramBindingAttributes := map[string]schema.Attribute{
		"literal": schema.StringAttribute{
			Optional: true,
		},
		"reference": schema.StringAttribute{
			Optional: true,
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
						"conditions": schema.SetNestedAttribute{
							Required: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"operation": schema.StringAttribute{
										Required: true,
									},
									"param_bindings": schema.SetNestedAttribute{
										Required: true,
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"array_value": schema.SetNestedAttribute{
													Optional: true,
													NestedObject: schema.NestedAttributeObject{
														Attributes: paramBindingAttributes,
													},
												},
												"value": schema.SingleNestedAttribute{
													Optional:   true,
													Attributes: paramBindingAttributes,
												},
											},
										},
									},
									"subject": schema.StringAttribute{
										Required: true,
									},
								},
							},
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
			ConditionGroups:  toPayloadConditionGrouos(data.ConditionGroups),
			Steps:            []client.StepConfigPayload{},
			Expressions:      []client.ExpressionPayloadV2{},
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
			ConditionGroups:  toPayloadConditionGrouos(data.ConditionGroups),
			Steps:            []client.StepConfigPayload{},
			Expressions:      []client.ExpressionPayloadV2{},
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
		conditions := []IncidentEngineCondition{}

		for _, c := range g.Conditions {
			conditions = append(conditions, IncidentEngineCondition{
				Operation:     types.StringValue(c.Operation.Value),
				ParamBindings: r.buildParamBindings(c.ParamBindings),
				Subject:       types.StringValue(c.Subject.Reference),
			})
		}

		out = append(out, IncidentEngineConditionGroup{Conditions: conditions})
	}

	return out
}

func (r *IncidentWorkflowResource) buildParamBindings(pbs []client.EngineParamBindingV2) []IncidentEngineParamBinding {
	var out []IncidentEngineParamBinding

	for _, pb := range pbs {
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

		out = append(out, IncidentEngineParamBinding{
			ArrayValue: arrayValue,
			Value:      value,
		})
	}

	return out
}

// toPayloadConditionGrouos converts from the terraform model to the http payload type.
// The payload type is different from the response type, which includes more information such as labels.
func toPayloadConditionGrouos(groups IncidentEngineConditionGroups) []client.ExpressionFilterOptsPayloadV2 {
	var payload []client.ExpressionFilterOptsPayloadV2

	for _, group := range groups {
		var conditions []client.ConditionPayloadV2

		for _, condition := range group.Conditions {
			conditions = append(conditions, client.ConditionPayloadV2{
				Operation:     condition.Operation.ValueString(),
				ParamBindings: toPayloadParamBindings(condition.ParamBindings),
				Subject:       condition.Subject.ValueString(),
			})
		}

		payload = append(payload, client.ExpressionFilterOptsPayloadV2{
			Conditions: conditions,
		})
	}

	return payload
}

func toPayloadParamBindings(pbs []IncidentEngineParamBinding) []client.EngineParamBindingPayloadV2 {
	var paramBindings []client.EngineParamBindingPayloadV2

	for _, binding := range pbs {
		arrayValue := []client.EngineParamBindingValuePayloadV2{}
		for _, v := range binding.ArrayValue {
			arrayValue = append(arrayValue, client.EngineParamBindingValuePayloadV2{
				Literal:   v.Literal.ValueStringPointer(),
				Reference: v.Reference.ValueStringPointer(),
			})
		}

		var value *client.EngineParamBindingValuePayloadV2
		if binding.Value != nil {
			value = &client.EngineParamBindingValuePayloadV2{
				Literal:   binding.Value.Literal.ValueStringPointer(),
				Reference: binding.Value.Reference.ValueStringPointer(),
			}
		}

		paramBindings = append(paramBindings, client.EngineParamBindingPayloadV2{
			ArrayValue: &arrayValue,
			Value:      value,
		})
	}

	return paramBindings
}
