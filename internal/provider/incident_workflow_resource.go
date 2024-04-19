package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Folder  types.String `tfsdk:"folder"`
	Version types.Int64  `tfsdk:"version"`

	Trigger         types.String                        `tfsdk:"trigger"`
	OnceFor         []IncidentEngineReferenceModel      `tfsdk:"once_for"`
	Expressions     []IncidentEngineExpressionModel     `tfsdk:"expressions"`
	ConditionGroups []IncidentEngineConditionGroupModel `tfsdk:"condition_groups"`
	Steps           []IncidentWorkflowStepConfigModel   `tfsdk:"steps"`

	DelayForSeconds               types.Int64  `tfsdk:"delay_for_seconds"`
	ConditionsApplyOverDelay      types.Bool   `tfsdk:"conditions_apply_over_delay"`
	IncludePrivateIncidents       types.Bool   `tfsdk:"include_private_incidents"`
	IncludeTestIncidents          types.Bool   `tfsdk:"include_test_incidents"`
	IncludeRetrospectiveIncidents types.Bool   `tfsdk:"include_retrospective_incidents"`
	RunsOnIncidents               types.Bool   `tfsdk:"runs_on_incidents"`
	RunsFrom                      types.String `tfsdk:"runs_from"`
	TerraformRepoURL              types.String `tfsdk:"terraform_repo_url"`
	IsDraft                       types.Bool   `tfsdk:"is_draft"`

	DisabledAt types.String `tfsdk:"disabled_at"`
}

type IncidentEngineReferenceModel struct {
	Key        types.String `tfsdk:"key"`
	Label      types.String `tfsdk:"label"`
	NodeLabel  types.String `tfsdk:"node_label"`
	Type       types.String `tfsdk:"type"`
	HideFilter types.Bool   `tfsdk:"hide_filter"`
	Array      types.Bool   `tfsdk:"array"`
	Parent     types.String `tfsdk:"parent"`
	Icon       types.String `tfsdk:"icon"`
}

type IncidentEngineExpressionModel struct{} // TODO(CAT-250)

type IncidentEngineConditionGroupModel struct{} // TODO(CAT-248)

type IncidentWorkflowStepConfigModel struct{} // TODO(CAT-249)

func (r *IncidentWorkflowResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (r *IncidentWorkflowResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Workflows V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowResponseBody", "id"),
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "name"),
				Required:            true,
			},
			"folder": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "folder"),
				Optional:            true,
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "version"),
				Required:            true,
			},
			"trigger": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "trigger"),
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"name":        schema.StringAttribute{},
					"icon":        schema.StringAttribute{},
					"label":       schema.StringAttribute{},
					"group_label": schema.StringAttribute{},
				},
			},
			"once_for": schema.SetNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "once_for"),
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key":         schema.StringAttribute{},
						"label":       schema.StringAttribute{},
						"node_label":  schema.StringAttribute{},
						"type":        schema.StringAttribute{},
						"hide_filter": schema.BoolAttribute{},
						"array":       schema.BoolAttribute{},
						"parent":      schema.StringAttribute{},
						"icon":        schema.StringAttribute{},
					},
				},
			},
			"expressions": schema.SetNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "expressions"),
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{},
				},
			},
			"condition_groups": schema.SetNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "condition_groups"),
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{},
				},
			},
			"steps": schema.SetNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "steps"),
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{},
				},
			},
			"delay_for_seconds": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "delay_for_seconds"),
				Optional:            true,
			},
			"conditions_apply_over_delay": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "conditions_apply_over_delay"),
				Optional:            true,
			},
			"include_private_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "include_private_incidents"),
				Required:            true,
			},
			"include_test_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "include_test_incidents"),
				Required:            true,
			},
			"include_retrospective_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "include_retrospective_incidents"),
				Required:            true,
			},
			"runs_on_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "runs_on_incidents"),
				Required:            true,
			},
			"runs_from": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "runs_from"),
				Optional:            true,
			},
			"terraform_repo_url": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "terraform_repo_url"),
				Optional:            true,
			},
			"is_draft": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "is_draft"),
				Required:            true,
			},
			"disabled_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowsV2CreateWorkflowRequestBody", "disabled_at"),
				Optional:            true, // computed?
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
			Name: data.Name.ValueString(),
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
	panic("unimplemented")
}

func (r *IncidentWorkflowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	panic("unimplemented")
}

func (r *IncidentWorkflowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	panic("unimplemented")
}

func (r *IncidentWorkflowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	panic("unimplemented")
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

func (r *IncidentWorkflowResource) buildModel(workflow client.Workflow) *IncidentWorkflowResourceModel {
	return nil
}
