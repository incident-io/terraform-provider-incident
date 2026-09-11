package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ resource.Resource                   = &escalationPathTemplateResource{}
	_ resource.ResourceWithImportState    = &escalationPathTemplateResource{}
	_ resource.ResourceWithValidateConfig = &escalationPathTemplateResource{}
	_ resource.ResourceWithModifyPlan     = &escalationPathTemplateResource{}
)

func NewEscalationPathTemplateResource() resource.Resource {
	return &escalationPathTemplateResource{}
}

type escalationPathTemplateResource struct {
	resourceConfigurer
}

type escalationPathTemplateModel struct {
	ID           types.String                     `tfsdk:"id"`
	Name         types.String                     `tfsdk:"name"`
	Description  types.String                     `tfsdk:"description"`
	Start        types.String                     `tfsdk:"start"`
	Sequences    types.Map                        `tfsdk:"sequences"`
	Params       types.List                       `tfsdk:"params"`
	Expressions  models.IncidentEngineExpressions `tfsdk:"expressions"`
	WorkingHours types.List                       `tfsdk:"working_hours"`
	RepeatConfig types.Object                     `tfsdk:"repeat_config"`
}

// escalationPathTemplateParam is one of the template's declared parameters: what a templated
// path has to supply a value for.
type escalationPathTemplateParam struct {
	Name         types.String                       `tfsdk:"name"`
	Label        types.String                       `tfsdk:"label"`
	Type         types.String                       `tfsdk:"type"`
	Array        types.Bool                         `tfsdk:"array"`
	Optional     types.Bool                         `tfsdk:"optional"`
	Description  types.String                       `tfsdk:"description"`
	DefaultValue *models.IncidentEngineParamBinding `tfsdk:"default_value"`
}

func escalationPathTemplateParamAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":          types.StringType,
		"label":         types.StringType,
		"type":          types.StringType,
		"array":         types.BoolType,
		"optional":      types.BoolType,
		"description":   types.StringType,
		"default_value": types.ObjectType{AttrTypes: models.ParamBindingAttrTypes()},
	}
}

func (r *escalationPathTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_escalation_path_template"
}

func (r *escalationPathTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	expressions := models.ExpressionsAttribute()
	expressions.Required = false
	expressions.Optional = true
	expressions.MarkdownDescription = "Expressions a target `binding` can name through `expression_ref`, each starting from one of the template's `params` (its `root_reference`). Use one to page something derived from a parameter, such as a team's lead."

	resp.Schema = schema.Schema{
		MarkdownDescription: `Create and manage escalation path templates: an escalation path with parameters, from which many escalation paths can be built.

A template is written the same way as ` + "`incident_escalation_path_beta`" + `, as a flat map of named node sequences, with two additions. It declares ` + "`params`" + `, and a level or notify_channel target may carry a ` + "`binding`" + ` to one of them instead of naming an ` + "`id`" + `. An ` + "`incident_escalation_path_beta`" + ` built from the template sets ` + "`template_id`" + ` and binds each parameter in ` + "`param_bindings`" + `, and the template's nodes, working hours and repeat config apply to every path built from it.

## Beta, and what happens next

This resource follows the ` + "`incident_escalation_path_beta`" + ` schema, which is in beta and may still change in ways that are not backwards compatible, so pin the provider version if that matters to you.`,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "id"),
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "name"),
				Required:            true,
			},

			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "description"),
				Optional:            true,
			},

			"params": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "params") +
					" A templated path binds each one in `param_bindings`, keyed by `name`.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "name") +
								" This is the key a templated path binds it under, and what a target's `value_reference` names.",
							Required: true,
						},
						"label": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "label"),
							Required:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "type") +
								" A schedule is `CatalogEntry[\"Schedule\"]`, a user `CatalogEntry[\"User\"]`, and a catalog type is `CatalogEntry[\"<type id>\"]`.",
							Required: true,
						},
						"array": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "array"),
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(false),
						},
						"optional": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "optional"),
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(false),
						},
						"description": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("EngineParamV2", "description"),
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString(""),
						},
						"default_value": schema.SingleNestedAttribute{
							MarkdownDescription: "The value a templated path gets when it doesn't bind this parameter.",
							Optional:            true,
							Attributes:          models.ParamBindingAttributes(),
						},
					},
				},
			},

			"expressions": expressions,

			"start": schema.StringAttribute{
				MarkdownDescription: "The key of the sequence this template begins with.",
				Required:            true,
			},

			"sequences": schema.MapNestedAttribute{
				MarkdownDescription: "Named sequences of nodes, keyed by a name you choose. Each sequence either ends with a `branch` node or runs off the end of the escalation path. Branches reference other sequences by key. A level or notify_channel target may set `binding` instead of `id`, to be resolved per templated path.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"nodes": schema.ListNestedAttribute{
							MarkdownDescription: "The nodes in this sequence, in the order they run.",
							Required:            true,
							NestedObject:        escalationPathTemplateNodeSchema(),
						},
					},
				},
			},

			"working_hours": schema.ListNestedAttribute{
				MarkdownDescription: apischema.Docstring("EscalationPathTemplateV2", "working_hours"),
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: models.IncidentWeekdayIntervalConfig{}.Attributes(),
				},
			},

			"repeat_config": schema.SingleNestedAttribute{
				MarkdownDescription: "Controls if an escalation will repeat after acknowledgement, when the alert is unresolved. When configured, it will repeat after the specified delay.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"repeat_after_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRepeatConfigV2", "repeat_after_seconds"),
						Required:            true,
					},
					"delay_repeat_on_activity": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRepeatConfigV2", "delay_repeat_on_activity"),
						Required:            true,
					},
				},
			},
		},
	}
}

func (r *escalationPathTemplateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data *escalationPathTemplateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// The sequence checks read the model through the path resource's type, which shares
	// every field they look at.
	validateSequences(ctx, &escalationPathBetaModel{Start: data.Start, Sequences: data.Sequences}, &resp.Diagnostics)
	validateSequenceConditions(ctx, &escalationPathBetaModel{Sequences: data.Sequences, WorkingHours: data.WorkingHours}, &resp.Diagnostics)
	validateEscalationPathTemplateTargets(ctx, data.Sequences, &resp.Diagnostics)

	// The API refuses a template with no parameters: a template that binds nothing is just
	// an escalation path. Say so at plan time rather than part way through an apply.
	if !data.Params.IsNull() && !data.Params.IsUnknown() && len(data.Params.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("params"),
			"Template has no parameters",
			"An escalation path template needs at least one parameter for its paths to bind. Use incident_escalation_path_beta for a path that takes no parameters.",
		)
	}
	if data.Params.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("params"),
			"Missing params",
			"An escalation path template needs at least one parameter for its paths to bind.",
		)
	}
}

// ModifyPlan asks the API what this edit would do to the escalation paths built from the
// template, and reports anything that would stop one paging as a warning. A template is
// edited in one place and read by many paths, so the blast radius of a change is the thing
// a plan can't otherwise show.
//
// The check needs the template to exist, so a create has nothing to ask about.
func (r *escalationPathTemplateResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A destroy plans no template, and an unconfigured provider has no client.
	if r.client == nil || req.Plan.Raw.IsNull() {
		return
	}

	// The framework runs this for every resource in the plan, changed or not.
	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	// A target pointing at something this same apply creates is unknown until it exists,
	// and checking around the gaps reports problems the apply won't hit.
	if !escalationPathBetaPlanSettled(req.Plan.Raw) {
		return
	}

	var data escalationPathTemplateModel
	if req.Plan.Get(ctx, &data).HasError() {
		return
	}

	// Only an existing template has linked paths to check.
	templateID := data.ID.ValueString()
	if templateID == "" {
		return
	}

	payload := r.toPayload(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, escalationPathValidateTimeout)
	defer cancel()

	result, err := r.client.EscalationPathTemplatesV2ValidateWithResponse(ctx, client.EscalationPathTemplatesV2ValidateJSONRequestBody{
		Id:           templateID,
		Name:         data.Name.ValueString(),
		Description:  data.Description.ValueStringPointer(),
		Params:       payload.Params,
		Expressions:  payload.Expressions,
		Path:         payload.Path,
		WorkingHours: payload.WorkingHours,
		RepeatConfig: payload.RepeatConfig,
	})
	if err == nil {
		addEscalationPathTemplateValidateWarnings(result, &resp.Diagnostics)
		return
	}

	// 422 is the API rejecting this template, which is the whole point.
	var httpErr client.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnprocessableEntity {
		resp.Diagnostics.AddError("Invalid escalation path template", httpErr.Error())
		return
	}

	// Anything else means the check didn't run, not that the config is bad, so failing
	// here would break plans that would have applied fine.
	resp.Diagnostics.AddWarning(
		"Could not check the escalation paths using this template",
		fmt.Sprintf("The template was not checked, and the edit may still break paths built from it: %s", err),
	)
}

// addEscalationPathTemplateValidateWarnings reports each escalation path the edit would
// leave degraded or broken. A path that stays healthy is not worth a line.
func addEscalationPathTemplateValidateWarnings(result *client.EscalationPathTemplatesV2ValidateResponse, diags *diag.Diagnostics) {
	if result == nil || result.JSON200 == nil {
		return
	}

	for _, check := range result.JSON200.Results {
		if check.Verdict == client.Healthy {
			continue
		}

		reasons := lo.Map(check.Reasons, func(reason client.EscalationPathBrokenReasonV2, _ int) string {
			return fmt.Sprintf("%s: %s", reason.Summary, reason.Detail)
		})
		diags.AddWarning(
			fmt.Sprintf("Escalation path %q would be %s", check.EscalationPath.Name, check.Verdict),
			strings.Join(reasons, "\n"),
		)
	}

	// A template with more paths than the API checked leaves the rest unreported, so say so
	// rather than implying the checked ones are all of them.
	if unchecked := result.JSON200.TotalCount - result.JSON200.CheckedCount; unchecked > 0 {
		diags.AddWarning(
			"Some escalation paths were not checked",
			fmt.Sprintf("%d of the %d escalation paths using this template were not checked, and may also be affected.",
				unchecked, result.JSON200.TotalCount),
		)
	}
}

func (r *escalationPathTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *escalationPathTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.toPayload(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationPathTemplatesV2CreateWithResponse(ctx, client.EscalationPathTemplatesV2CreateJSONRequestBody{
		Name:         data.Name.ValueString(),
		Description:  data.Description.ValueStringPointer(),
		Params:       payload.Params,
		Expressions:  payload.Expressions,
		Path:         payload.Path,
		WorkingHours: payload.WorkingHours,
		RepeatConfig: payload.RepeatConfig,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create escalation path template, got error: %s", err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to create escalation path template: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, result.JSON201.EscalationPathTemplate.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPathTemplate, r.terraformVersion)

	tflog.Trace(ctx, fmt.Sprintf("created an escalation path template resource with id=%s", result.JSON201.EscalationPathTemplate.Id))
	model := r.buildModel(ctx, result.JSON201.EscalationPathTemplate, data, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *escalationPathTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *escalationPathTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationPathTemplatesV2ShowWithResponse(ctx, data.ID.ValueString())
	if isNotFound(err) {
		tflog.Warn(ctx, fmt.Sprintf("Escalation path template with ID %s not found: removing from state.", data.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read escalation path template, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read escalation path template: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	model := r.buildModel(ctx, result.JSON200.EscalationPathTemplate, data, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *escalationPathTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *escalationPathTemplateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := r.toPayload(ctx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.EscalationPathTemplatesV2UpdateWithResponse(ctx, data.ID.ValueString(), client.EscalationPathTemplatesV2UpdateJSONRequestBody{
		Name:         data.Name.ValueString(),
		Description:  data.Description.ValueStringPointer(),
		Params:       payload.Params,
		Expressions:  payload.Expressions,
		Path:         payload.Path,
		WorkingHours: payload.WorkingHours,
		RepeatConfig: payload.RepeatConfig,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update escalation path template, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to update escalation path template: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, result.JSON200.EscalationPathTemplate.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPathTemplate, r.terraformVersion)

	model := r.buildModel(ctx, result.JSON200.EscalationPathTemplate, data, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *escalationPathTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *escalationPathTemplateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.EscalationPathTemplatesV2DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete escalation path template, got error: %s", err))
		return
	}
}

func (r *escalationPathTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPathTemplate,
		r.terraformVersion, r.markImportedAsManaged)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

type escalationPathTemplatePayload struct {
	Params       *[]client.EngineParamV2
	Expressions  *[]client.ExpressionPayloadV2
	Path         []client.EscalationPathTemplateNodePayloadV2
	WorkingHours *[]client.WeekdayIntervalConfigV2
	RepeatConfig *client.EscalationPathRepeatConfigV2
}

func (r *escalationPathTemplateResource) toPayload(ctx context.Context, data *escalationPathTemplateModel, diags *diag.Diagnostics) escalationPathTemplatePayload {
	sequences := decodeSequences(ctx, data.Sequences, diags)
	if diags.HasError() {
		return escalationPathTemplatePayload{}
	}

	payload := escalationPathTemplatePayload{
		Path:         unflattenSequencesWith(ctx, templateSequenceCodec{}, data.Start.ValueString(), sequences, diags),
		WorkingHours: escalationPathWorkingHoursToPayload(ctx, data.WorkingHours, diags),
		RepeatConfig: escalationPathRepeatConfigToPayload(ctx, data.RepeatConfig, diags),
	}

	// The API treats an absent params or expressions the same as an empty list, so always
	// sending one means a config that drops the last param clears it rather than keeping it.
	params := lo.Map(decodeTemplateParams(ctx, data.Params, diags), func(param escalationPathTemplateParam, _ int) client.EngineParamV2 {
		out := client.EngineParamV2{
			Name:        param.Name.ValueString(),
			Label:       param.Label.ValueString(),
			Type:        param.Type.ValueString(),
			Array:       param.Array.ValueBool(),
			Optional:    param.Optional.ValueBool(),
			Description: param.Description.ValueString(),
		}
		if param.DefaultValue != nil && !param.DefaultValue.IsEmpty() {
			out.DefaultValue = lo.ToPtr(engineParamBindingFromPayload(param.DefaultValue.ToPayload()))
		}
		return out
	})
	payload.Params = &params

	expressions := data.Expressions.ToPayload()
	payload.Expressions = &expressions

	return payload
}

// engineParamBindingFromPayload widens a binding payload into the response-shaped type a
// param's default_value is declared as. The label the response type carries is display-only,
// so it's left empty.
func engineParamBindingFromPayload(payload client.EngineParamBindingPayloadV2) client.EngineParamBindingV2 {
	out := client.EngineParamBindingV2{}
	if payload.Value != nil {
		out.Value = &client.EngineParamBindingValueV2{
			Literal:   payload.Value.Literal,
			Reference: payload.Value.Reference,
		}
	}
	if payload.ArrayValue != nil {
		values := lo.Map(*payload.ArrayValue, func(value client.EngineParamBindingValuePayloadV2, _ int) client.EngineParamBindingValueV2 {
			return client.EngineParamBindingValueV2{Literal: value.Literal, Reference: value.Reference}
		})
		out.ArrayValue = &values
	}
	return out
}

func decodeTemplateParams(ctx context.Context, list types.List, diags *diag.Diagnostics) []escalationPathTemplateParam {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var params []escalationPathTemplateParam
	diags.Append(list.ElementsAs(ctx, &params, false)...)
	return params
}

// buildModel converts what the API returned into state. Sequence names and binding
// spellings are ours rather than the API's, so prior is where both come from: the plan on a
// create or update, and the state on a read.
func (r *escalationPathTemplateResource) buildModel(ctx context.Context, template client.EscalationPathTemplateV2, prior *escalationPathTemplateModel, diags *diag.Diagnostics) *escalationPathTemplateModel {
	priorNames := escalationPathBetaPriorNames{}
	var priorParams []escalationPathTemplateParam
	var priorExpressions models.IncidentEngineExpressions
	if prior != nil {
		priorNames = escalationPathBetaPriorNamesFrom(ctx, &escalationPathBetaModel{Start: prior.Start, Sequences: prior.Sequences})
		var ignored diag.Diagnostics
		priorParams = decodeTemplateParams(ctx, prior.Params, &ignored)
		priorExpressions = prior.Expressions
	}

	start, sequences := flattenSequencesWith(ctx, templateSequenceCodec{}, template.Path, priorNames, diags)
	reconcileTemplateBindingSpelling(ctx, sequences, priorNames.sequences, diags)

	params := lo.Map(template.Params, func(param client.EngineParamV2, index int) escalationPathTemplateParam {
		out := escalationPathTemplateParam{
			Name:        types.StringValue(param.Name),
			Label:       types.StringValue(param.Label),
			Type:        types.StringValue(param.Type),
			Array:       types.BoolValue(param.Array),
			Optional:    types.BoolValue(param.Optional),
			Description: types.StringValue(param.Description),
		}
		if param.DefaultValue != nil {
			out.DefaultValue = lo.ToPtr(models.IncidentEngineParamBinding{}.FromAPI(*param.DefaultValue))
			if index < len(priorParams) {
				out.DefaultValue = models.ReconcileBindingSpelling(out.DefaultValue, priorParams[index].DefaultValue)
			}
		}
		return out
	})
	paramsList, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: escalationPathTemplateParamAttrTypes()}, params)
	diags.Append(d...)

	// A template with no expressions reads back as an empty list; a config that never wrote
	// any holds null, and the two would plan a change against each other.
	var expressions models.IncidentEngineExpressions
	if len(template.Expressions) > 0 || len(priorExpressions) > 0 {
		expressions = models.IncidentEngineExpressions{}.FromAPI(template.Expressions)
		expressions.ReconcileSpelling(priorExpressions)
	}

	description := types.StringNull()
	if template.Description != nil && *template.Description != "" {
		description = types.StringValue(*template.Description)
	}

	return &escalationPathTemplateModel{
		ID:           types.StringValue(template.Id),
		Name:         types.StringValue(template.Name),
		Description:  description,
		Start:        types.StringValue(start),
		Sequences:    sequencesToMap(ctx, escalationPathTemplateNodeAttrTypes(), sequences, diags),
		Params:       paramsList,
		Expressions:  expressions,
		WorkingHours: escalationPathWorkingHoursFromAPI(ctx, template.WorkingHours, diags),
		RepeatConfig: escalationPathRepeatConfigFromAPI(template.RepeatConfig),
	}
}
