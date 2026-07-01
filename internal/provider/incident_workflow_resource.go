package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                   = &IncidentWorkflowResource{}
	_ resource.ResourceWithImportState    = &IncidentWorkflowResource{}
	_ resource.ResourceWithValidateConfig = &IncidentWorkflowResource{}
)

// privateIncidentScopes are the valid values for the private_incident_scope attribute.
var privateIncidentScopes = []string{"all", "owning_teams", "none"}

type IncidentWorkflowResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
}

func NewIncidentWorkflowResource() resource.Resource {
	return &IncidentWorkflowResource{}
}

type IncidentWorkflowResourceModel struct {
	ID                        types.String                         `tfsdk:"id"`
	Name                      types.String                         `tfsdk:"name"`
	Folder                    types.String                         `tfsdk:"folder"`
	Shortform                 types.String                         `tfsdk:"shortform"`
	Trigger                   types.String                         `tfsdk:"trigger"`
	ConditionGroups           models.IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	Steps                     []IncidentWorkflowStep               `tfsdk:"steps"`
	Expressions               models.IncidentEngineExpressions     `tfsdk:"expressions"`
	OnceFor                   []types.String                       `tfsdk:"once_for"`
	IncludePrivateIncidents   types.Bool                           `tfsdk:"include_private_incidents"`
	PrivateIncidentScope      types.String                         `tfsdk:"private_incident_scope"`
	IncludePrivateEscalations types.Bool                           `tfsdk:"include_private_escalations"`
	OwningTeamIDs             types.Set                            `tfsdk:"owning_team_ids"`
	ContinueOnStepError       types.Bool                           `tfsdk:"continue_on_step_error"`
	Delay                     *IncidentWorkflowDelay               `tfsdk:"delay"`
	RunsOnIncidents           types.String                         `tfsdk:"runs_on_incidents"`
	RunsOnIncidentModes       types.Set                            `tfsdk:"runs_on_incident_modes"`
	State                     types.String                         `tfsdk:"state"`
	FormFields                []IncidentWorkflowFormField          `tfsdk:"form_fields"`
}

// IncidentWorkflowFormField represents a form field that is presented to the
// user when a workflow with a manual trigger is triggered by hand.
type IncidentWorkflowFormField struct {
	ID           types.String                       `tfsdk:"id"`
	Name         types.String                       `tfsdk:"name"`
	Type         types.String                       `tfsdk:"type"`
	Description  types.String                       `tfsdk:"description"`
	Placeholder  types.String                       `tfsdk:"placeholder"`
	Array        types.Bool                         `tfsdk:"array"`
	Required     types.Bool                         `tfsdk:"required"`
	DefaultValue *models.IncidentEngineParamBinding `tfsdk:"default_value"`
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
				Optional:            true,
				Computed:            true,
				DeprecationMessage:  "Use `private_incident_scope` instead. When both are set they must agree (include_private_incidents is true for the all and owning_teams scopes, false for none).",
			},
			"private_incident_scope": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("WorkflowV2", "private_incident_scope"),
				Optional:            true,
				Computed:            true,
			},
			"include_private_escalations": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "include_private_escalations"),
				Optional:            true,
				Computed:            true,
			},
			"owning_team_ids": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "owning_team_ids"),
				Optional:            true,
				ElementType:         types.StringType,
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
			"runs_on_incident_modes": schema.SetAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "runs_on_incident_modes"),
				Required:            true,
				ElementType:         types.StringType,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: EnumValuesDescription("WorkflowV2", "state"),
				Required:            true,
			},
			"form_fields": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("WorkflowV2", "form_fields"),
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "id"),
							Optional:            true,
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "name"),
							Required:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "type"),
							Required:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "description"),
							Optional:            true,
						},
						"placeholder": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "placeholder"),
							Optional:            true,
						},
						"array": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "array"),
							Required:            true,
						},
						"required": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("WorkflowFormFieldV2", "required"),
							Required:            true,
						},
						"default_value": schema.SingleNestedAttribute{
							MarkdownDescription: "The default value to pre-populate this form field with",
							Optional:            true,
							Attributes:          models.ParamBindingAttributes(),
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

	onceFor := []string{}
	for _, v := range data.OnceFor {
		onceFor = append(onceFor, v.ValueString())
	}

	runsOnIncidentModes := []client.WorkflowsCreateWorkflowPayloadV2RunsOnIncidentModes{}
	for _, v := range data.RunsOnIncidentModes.Elements() {
		if str, ok := v.(types.String); ok {
			runsOnIncidentModes = append(runsOnIncidentModes, client.WorkflowsCreateWorkflowPayloadV2RunsOnIncidentModes(str.ValueString()))
		}
	}

	payload := client.WorkflowsCreateWorkflowPayloadV2{
		Trigger:             data.Trigger.ValueString(),
		Name:                data.Name.ValueString(),
		OnceFor:             onceFor,
		ConditionGroups:     data.ConditionGroups.ToPayload(),
		Steps:               toPayloadSteps(data.Steps),
		Expressions:         data.Expressions.ToPayload(),
		RunsOnIncidents:     client.WorkflowsCreateWorkflowPayloadV2RunsOnIncidents(data.RunsOnIncidents.ValueString()),
		RunsOnIncidentModes: runsOnIncidentModes,
		Folder:              data.Folder.ValueStringPointer(),
		Shortform:           data.Shortform.ValueStringPointer(),
		OwningTeamIds:       toOwningTeamIDs(data.OwningTeamIDs),
		ContinueOnStepError: data.ContinueOnStepError.ValueBool(),
		State:               lo.ToPtr(client.WorkflowsCreateWorkflowPayloadV2State(data.State.ValueString())),
		FormFields:          toPayloadFormFields(data.FormFields),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
	}

	// Forward whichever privacy fields the user set in config (ValidateConfig
	// already ensures they agree). Read from config, not the plan: both are
	// Computed, so the plan can carry a value from state the user never set.
	var cfgScope types.String
	var cfgBool types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("private_incident_scope"), &cfgScope)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("include_private_incidents"), &cfgBool)...)
	if !cfgScope.IsNull() {
		payload.PrivateIncidentScope = lo.ToPtr(client.WorkflowsCreateWorkflowPayloadV2PrivateIncidentScope(cfgScope.ValueString()))
	}
	if !cfgBool.IsNull() {
		payload.IncludePrivateIncidents = lo.ToPtr(cfgBool.ValueBool())
	}

	if !data.IncludePrivateEscalations.IsNull() {
		payload.IncludePrivateEscalations = lo.ToPtr(data.IncludePrivateEscalations.ValueBool())
	}

	if data.Delay != nil {
		payload.Delay = &client.WorkflowDelayV2{
			ConditionsApplyOverDelay: data.Delay.ConditionsApplyOverDelay.ValueBool(),
			ForSeconds:               data.Delay.ForSeconds.ValueInt64(),
		}
	}

	result, err := r.client.WorkflowsV2CreateWorkflowWithResponse(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create workflow, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a workflow resource with id=%s", result.JSON201.Workflow.Id))
	data = r.buildModel(ctx, result.JSON201.Workflow)
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
	for _, v := range data.RunsOnIncidentModes.Elements() {
		if str, ok := v.(types.String); ok {
			runsOnIncidentModes = append(runsOnIncidentModes, client.WorkflowsUpdateWorkflowPayloadV2RunsOnIncidentModes(str.ValueString()))
		}
	}

	payload := client.WorkflowsV2UpdateWorkflowJSONRequestBody{
		Name:                data.Name.ValueString(),
		ConditionGroups:     data.ConditionGroups.ToPayload(),
		Steps:               toPayloadSteps(data.Steps),
		Expressions:         data.Expressions.ToPayload(),
		OnceFor:             onceFor,
		RunsOnIncidents:     client.WorkflowsUpdateWorkflowPayloadV2RunsOnIncidents(data.RunsOnIncidents.ValueString()),
		RunsOnIncidentModes: runsOnIncidentModes,
		Folder:              data.Folder.ValueStringPointer(),
		Shortform:           data.Shortform.ValueStringPointer(),
		OwningTeamIds:       toOwningTeamIDs(data.OwningTeamIDs),
		ContinueOnStepError: data.ContinueOnStepError.ValueBool(),
		State:               lo.ToPtr(client.WorkflowsUpdateWorkflowPayloadV2State(data.State.ValueString())),
		FormFields:          toPayloadFormFields(data.FormFields),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
		SkipStepUpgrades: lo.ToPtr(true),
	}

	// Forward whichever privacy fields the user set in config (ValidateConfig
	// already ensures they agree). Read from config, not the plan: both are
	// Computed, so the plan can carry a value from state the user never set.
	var cfgScope types.String
	var cfgBool types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("private_incident_scope"), &cfgScope)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("include_private_incidents"), &cfgBool)...)
	if !cfgScope.IsNull() {
		payload.PrivateIncidentScope = lo.ToPtr(client.WorkflowsUpdateWorkflowPayloadV2PrivateIncidentScope(cfgScope.ValueString()))
	}
	if !cfgBool.IsNull() {
		payload.IncludePrivateIncidents = lo.ToPtr(cfgBool.ValueBool())
	}

	if !data.IncludePrivateEscalations.IsNull() {
		payload.IncludePrivateEscalations = lo.ToPtr(data.IncludePrivateEscalations.ValueBool())
	}

	if data.Delay != nil {
		payload.Delay = &client.WorkflowDelayV2{
			ConditionsApplyOverDelay: data.Delay.ConditionsApplyOverDelay.ValueBool(),
			ForSeconds:               data.Delay.ForSeconds.ValueInt64(),
		}
	}

	result, err := r.client.WorkflowsV2UpdateWorkflowWithResponse(ctx, state.ID.ValueString(), payload)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update workflow, got error: %s", err))
		return
	}

	data = r.buildModel(ctx, result.JSON200.Workflow)
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
	if err != nil {
		// Check if error message contains any indication of a 404 not found
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Workflow with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read workflow, got error: %s", err))
		return
	}

	data = r.buildModel(ctx, result.JSON200.Workflow)
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
	claimResource(ctx, r.client, req.ID, &resp.Diagnostics, client.Workflow, r.terraformVersion)
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

func toOwningTeamIDs(set types.Set) *[]string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	teamIDs := []string{}
	for _, elem := range set.Elements() {
		if str, ok := elem.(types.String); ok {
			teamIDs = append(teamIDs, str.ValueString())
		}
	}

	return &teamIDs
}

// ValidateConfig blocks an unrecognised private_incident_scope, or an include_private_incidents
// that contradicts it. The bool is true whenever the scope touches private incidents (all or
// owning_teams), false for none; the API accepts both when they agree, so only disagreement errors.
func (r *IncidentWorkflowResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var includePrivate types.Bool
	var scope types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("include_private_incidents"), &includePrivate)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("private_incident_scope"), &scope)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scopeSet := !scope.IsNull() && !scope.IsUnknown()
	boolSet := !includePrivate.IsNull() && !includePrivate.IsUnknown()

	if scopeSet && !lo.Contains(privateIncidentScopes, scope.ValueString()) {
		resp.Diagnostics.Append(diag.NewErrorDiagnostic(
			"Invalid private_incident_scope",
			fmt.Sprintf("private_incident_scope must be one of %v, got %q.", privateIncidentScopes, scope.ValueString()),
		))
		return
	}

	if scopeSet && boolSet {
		touchesPrivate := scope.ValueString() != "none"
		if includePrivate.ValueBool() != touchesPrivate {
			resp.Diagnostics.Append(diag.NewErrorDiagnostic(
				"include_private_incidents and private_incident_scope disagree",
				"include_private_incidents is deprecated in favour of private_incident_scope. When both are set they must agree: include_private_incidents is true for the all and owning_teams scopes and false for none. Prefer setting only private_incident_scope.",
			))
		}
	}
}

func toPayloadFormFields(fields []IncidentWorkflowFormField) *[]client.WorkflowFormFieldPayloadV2 {
	if fields == nil {
		return nil
	}

	out := []client.WorkflowFormFieldPayloadV2{}
	for _, field := range fields {
		payload := client.WorkflowFormFieldPayloadV2{
			Id:          field.ID.ValueStringPointer(),
			Name:        field.Name.ValueString(),
			Type:        field.Type.ValueString(),
			Array:       field.Array.ValueBool(),
			Required:    field.Required.ValueBool(),
			Description: field.Description.ValueStringPointer(),
			Placeholder: field.Placeholder.ValueStringPointer(),
		}
		if field.DefaultValue != nil {
			payload.DefaultValue = lo.ToPtr(field.DefaultValue.ToPayload())
		}
		out = append(out, payload)
	}

	return &out
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
