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
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
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
	ID                      types.String                         `tfsdk:"id"`
	Name                    types.String                         `tfsdk:"name"`
	Folder                  types.String                         `tfsdk:"folder"`
	Shortform               types.String                         `tfsdk:"shortform"`
	Trigger                 types.String                         `tfsdk:"trigger"`
	ConditionGroups         models.IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	Steps                   []IncidentWorkflowStep               `tfsdk:"steps"`
	Expressions             models.IncidentEngineExpressions     `tfsdk:"expressions"`
	OnceFor                 []types.String                       `tfsdk:"once_for"`
	IncludePrivateIncidents types.Bool                           `tfsdk:"include_private_incidents"`
	ContinueOnStepError     types.Bool                           `tfsdk:"continue_on_step_error"`
	Delay                   *IncidentWorkflowDelay               `tfsdk:"delay"`
	RunsOnIncidents         types.String                         `tfsdk:"runs_on_incidents"`
	RunsOnIncidentModes     []types.String                       `tfsdk:"runs_on_incident_modes"`
	State                   types.String                         `tfsdk:"state"`
}

type IncidentWorkflowStep struct {
	ForEach       types.String                       `tfsdk:"for_each"`
	ID            types.String                       `tfsdk:"id"`
	Name          types.String                       `tfsdk:"name"`
	ParamBindings models.IncidentEngineParamBindings `tfsdk:"param_bindings"`
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

We'd generally recommend building workflows in our [web dashboard](https://app.incident.io/~/workflows), and using the 'Export' flow to generate your Terraform, as it's easier to see what you've configured. You can also make changes to an existing workflow and copy the resulting Terraform without persisting it. You can learn more in this [Loom](https://www.loom.com/share/b833d7d0fd114d6ba3f24d8c72e5208f?sid=c6d3cc3f-aa93-44ba-b12d-a0a4cbe09448).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "id"),
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "name"),
				Required:            true,
			},
			"folder": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "folder"),
				Optional:            true,
			},
			"shortform": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "shortform"),
				Optional:            true,
			},
			"trigger": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("TriggerSlimV2", "name"),
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"condition_groups": models.ConditionGroupsAttribute(),
			"steps": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "steps"),
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
						"param_bindings": models.ParamBindingsAttribute(),
					},
				},
			},
			"expressions": models.ExpressionsAttribute(),
			"once_for": schema.ListAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "once_for"),
				Required:            true,
				ElementType:         types.StringType,
			},
			"include_private_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "include_private_incidents"),
				Required:            true,
			},
			"continue_on_step_error": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "continue_on_step_error"),
				Required:            true,
			},
			"delay": schema.SingleNestedAttribute{
				MarkdownDescription: "Configuration controlling workflow delay behaviour",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"conditions_apply_over_delay": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("WorkflowDelayV2", "conditions_apply_over_delay"),
						Required:            true,
					},
					"for_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("WorkflowDelayV2", "for_seconds"),
						Required:            true,
					},
				},
			},
			"runs_on_incidents": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("WorkflowV2", "runs_on_incidents"),
				Required:            true,
			},
			"runs_on_incident_modes": schema.ListAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "runs_on_incident_modes"),
				Required:            true,
				ElementType:         types.StringType,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("WorkflowV2", "state"),
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

	runsOnIncidentModes := []client.WorkflowsCreateWorkflowPayloadV2RunsOnIncidentModes{}
	for _, v := range data.RunsOnIncidentModes {
		runsOnIncidentModes = append(runsOnIncidentModes, client.WorkflowsCreateWorkflowPayloadV2RunsOnIncidentModes(v.ValueString()))
	}

	payload := client.WorkflowsCreateWorkflowPayloadV2{
		Trigger:                 data.Trigger.ValueString(),
		Name:                    data.Name.ValueString(),
		OnceFor:                 onceFor,
		ConditionGroups:         data.ConditionGroups.ToPayload(),
		Steps:                   toPayloadSteps(data.Steps),
		Expressions:             data.Expressions.ToPayload(),
		RunsOnIncidents:         client.WorkflowsCreateWorkflowPayloadV2RunsOnIncidents(data.RunsOnIncidents.ValueString()),
		RunsOnIncidentModes:     runsOnIncidentModes,
		Folder:                  data.Folder.ValueStringPointer(),
		Shortform:               data.Shortform.ValueStringPointer(),
		IncludePrivateIncidents: data.IncludePrivateIncidents.ValueBool(),
		ContinueOnStepError:     data.ContinueOnStepError.ValueBool(),
		State:                   lo.ToPtr(client.WorkflowsCreateWorkflowPayloadV2State(data.State.ValueString())),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
	}

	if data.Delay != nil {
		payload.Delay = &client.WorkflowDelayV2{
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

	runsOnIncidentModes := []client.WorkflowsUpdateWorkflowPayloadV2RunsOnIncidentModes{}
	for _, v := range data.RunsOnIncidentModes {
		runsOnIncidentModes = append(runsOnIncidentModes, client.WorkflowsUpdateWorkflowPayloadV2RunsOnIncidentModes(v.ValueString()))
	}

	payload := client.WorkflowsV2UpdateWorkflowJSONRequestBody{
		Name:                    data.Name.ValueString(),
		ConditionGroups:         data.ConditionGroups.ToPayload(),
		Steps:                   toPayloadSteps(data.Steps),
		Expressions:             data.Expressions.ToPayload(),
		OnceFor:                 onceFor,
		RunsOnIncidents:         client.WorkflowsUpdateWorkflowPayloadV2RunsOnIncidents(data.RunsOnIncidents.ValueString()),
		RunsOnIncidentModes:     runsOnIncidentModes,
		Folder:                  data.Folder.ValueStringPointer(),
		Shortform:               data.Shortform.ValueStringPointer(),
		IncludePrivateIncidents: data.IncludePrivateIncidents.ValueBool(),
		ContinueOnStepError:     data.ContinueOnStepError.ValueBool(),
		State:                   lo.ToPtr(client.WorkflowsUpdateWorkflowPayloadV2State(data.State.ValueString())),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
	}

	if data.Delay != nil {
		payload.Delay = &client.WorkflowDelayV2{
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

	result, err := r.client.WorkflowsV2ShowWorkflowWithResponse(ctx, data.ID.ValueString(), &client.WorkflowsV2ShowWorkflowParams{
		SkipStepUpgrades: lo.ToPtr(true),
	})
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
	claimResource(ctx, r.client, req.ID, resp.Diagnostics, client.Workflow, r.terraformVersion)
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

func toPayloadSteps(steps []IncidentWorkflowStep) []client.StepConfigPayloadV2 {
	out := []client.StepConfigPayloadV2{}

	for _, step := range steps {
		out = append(out, client.StepConfigPayloadV2{
			ForEach:       step.ForEach.ValueStringPointer(),
			Id:            step.ID.ValueString(),
			Name:          step.Name.ValueString(),
			ParamBindings: step.ParamBindings.ToPayload(),
		})
	}

	return out
}
