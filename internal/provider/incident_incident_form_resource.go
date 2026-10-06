package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ resource.Resource                   = &IncidentIncidentFormResource{}
	_ resource.ResourceWithConfigure      = &IncidentIncidentFormResource{}
	_ resource.ResourceWithImportState    = &IncidentIncidentFormResource{}
	_ resource.ResourceWithValidateConfig = &IncidentIncidentFormResource{}
	_ resource.ResourceWithModifyPlan     = &IncidentIncidentFormResource{}
)

type IncidentIncidentFormResource struct {
	resourceConfigurer
}

func NewIncidentIncidentFormResource() resource.Resource {
	return &IncidentIncidentFormResource{}
}

func (r *IncidentIncidentFormResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_form"
}

const incidentFormFormTypeCustomFields = "custom-fields"

func (r *IncidentIncidentFormResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Incident Forms V3"),
			`A form is one resource: its elements and the expressions they reference are written together, in
display order, and reconciled server-side on every apply.

Every organisation already has a default form of each lifecycle type (`+"`declare`, `accept`, `update`, `resolve`"+`
and `+"`retrospective`"+`), which can be neither created nor deleted. To manage one, import it by ID. Destroying
it afterwards removes it from state and hands it back to the dashboard, but the form itself remains. A form
for a specific incident type is created and deleted like any other resource. A `+"`custom-fields`"+` form has no
default, so one with no `+"`incident_type_id`"+` is created and deleted normally too.

The saved form always lists the `+"`name`, `incident_type`"+` and `+"`status`"+` elements first, in that order, whatever
order the configuration gives them. List them first, or state keeps your order while the form shows theirs.`),
		Attributes: map[string]schema.Attribute{
			"unlock_in_dashboard": unlockInDashboardAttribute(),
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("IncidentFormV3", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"form_type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: EnumValuesDescription("IncidentFormV3", "form_type") +
					" A form's type cannot change, so changing this replaces the form.",
				Validators: []validator.String{
					stringvalidator.OneOf(enumValues("IncidentFormV3", "form_type")...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"incident_type_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: apischema.Docstring("IncidentFormV3", "incident_type_id") +
					" A form cannot move to another incident type, so changing this replaces the form.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"expressions": optionalExpressionsAttribute(apischema.Docstring("IncidentFormV3", "expressions")),
			"lifecycle_elements": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("IncidentFormV3", "lifecycle_elements"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: incidentFormElementAttributes(),
				},
			},
		},
	}
}

func incidentFormElementAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "id"),
		},
		"element_type": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: EnumValuesDescription("IncidentFormLifecycleElementV3", "element_type"),
			Validators: []validator.String{
				stringvalidator.OneOf(enumValues("IncidentFormLifecycleElementV3", "element_type")...),
			},
		},
		"custom_field_id": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "custom_field_id"),
		},
		"incident_role_id": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "incident_role_id"),
		},
		"incident_timestamp_id": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "incident_timestamp_id"),
		},
		"show_if_condition_groups": optionalConditionGroupsAttribute(
			apischema.Docstring("IncidentFormLifecycleElementV3", "show_if_condition_groups")),
		"required_if": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(string(client.IncidentFormLifecycleElementV3RequiredIfNeverRequire)),
			MarkdownDescription: EnumValuesDescription("IncidentFormLifecycleElementV3", "required_if"),
			Validators: []validator.String{
				stringvalidator.OneOf(enumValues("IncidentFormLifecycleElementV3", "required_if")...),
			},
		},
		"required_if_condition_groups": optionalConditionGroupsAttribute(
			apischema.Docstring("IncidentFormLifecycleElementV3", "required_if_condition_groups")),
		"default_value": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "default_value"),
			Attributes:          models.ParamBindingAttributes(),
		},
		"placeholder": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "placeholder"),
		},
		"description": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "description"),
		},
		"can_select_no_value": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "can_select_no_value"),
		},
		"config": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementV3", "config"),
			Attributes: map[string]schema.Attribute{
				"require_comment": schema.BoolAttribute{
					Required:            true,
					MarkdownDescription: apischema.Docstring("IncidentFormLifecycleElementConfigV3", "require_comment"),
				},
			},
		},
	}
}

// optionalExpressionsAttribute is the shared expressions attribute, for a resource where most
// configurations have none and shouldn't have to write `expressions = []`.
func optionalExpressionsAttribute(description string) schema.SetNestedAttribute {
	attribute := models.ExpressionsAttribute()
	attribute.Required = false
	attribute.Optional = true
	attribute.MarkdownDescription = description

	return attribute
}

// optionalConditionGroupsAttribute is the shared condition groups attribute, made optional
// for the per-element conditions a form element usually doesn't have.
func optionalConditionGroupsAttribute(description string) schema.ListNestedAttribute {
	attribute := models.ConditionGroupsAttribute()
	attribute.Required = false
	attribute.Optional = true
	attribute.MarkdownDescription = description

	return attribute
}

// ValidateConfig applies what the API enforces on each element and can be judged without
// an account: the resource ID an element type needs, and the conditions required_if allows.
func (r *IncidentIncidentFormResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var elements types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("lifecycle_elements"), &elements)...)
	if resp.Diagnostics.HasError() || elements.IsNull() || elements.IsUnknown() {
		return
	}

	for idx, value := range elements.Elements() {
		element, ok := value.(types.Object)
		if !ok || element.IsNull() || element.IsUnknown() {
			continue
		}

		attrs := element.Attributes()
		elementPath := path.Root("lifecycle_elements").AtListIndex(idx)
		elementType, ok := knownString(attrs["element_type"])
		if !ok {
			continue
		}

		required := incidentFormElementResourceAttribute(elementType)
		for _, name := range []string{"custom_field_id", "incident_role_id", "incident_timestamp_id"} {
			value := attrs[name]
			isSet := value != nil && !value.IsNull()
			switch {
			case name == required && !isSet:
				resp.Diagnostics.AddAttributeError(elementPath.AtName(name), "Missing required attribute",
					fmt.Sprintf("`%s` is required when `element_type` is %q.", name, elementType))
			case name != required && isSet && !value.IsUnknown():
				resp.Diagnostics.AddAttributeError(elementPath.AtName(name), "Invalid attribute combination",
					fmt.Sprintf("`%s` must not be set when `element_type` is %q.", name, elementType))
			}
		}

		// A required_if left out defaults to never_require, so the groups are just as invalid.
		requiredIf, requiredIfKnown := knownString(attrs["required_if"])
		if !requiredIfKnown && (attrs["required_if"] == nil || attrs["required_if"].IsNull()) {
			requiredIf, requiredIfKnown = string(client.IncidentFormLifecycleElementV3RequiredIfNeverRequire), true
		}
		if groups, ok := attrs["required_if_condition_groups"].(types.List); ok && !groups.IsNull() && !groups.IsUnknown() &&
			requiredIfKnown && requiredIf != string(client.IncidentFormLifecycleElementV3RequiredIfCheckEngineConfig) {
			resp.Diagnostics.AddAttributeError(elementPath.AtName("required_if_condition_groups"), "Invalid attribute combination",
				"`required_if_condition_groups` only applies when `required_if` is \"check_engine_config\".")
		}
	}
}

// incidentFormElementResourceAttribute names the resource ID attribute an element type
// needs, or "" for a type that needs none.
func incidentFormElementResourceAttribute(elementType string) string {
	switch client.IncidentFormLifecycleElementV3ElementType(elementType) {
	case client.IncidentFormLifecycleElementV3ElementTypeCustomField:
		return "custom_field_id"
	case client.IncidentFormLifecycleElementV3ElementTypeIncidentRole:
		return "incident_role_id"
	case client.IncidentFormLifecycleElementV3ElementTypeTimestamp:
		return "incident_timestamp_id"
	default:
		return ""
	}
}

// ModifyPlan does two things the schema can't.
//
// It copies each element's ID from state onto the planned element it will become, so IDs
// don't show as "known after apply" on every change, and divider and text elements are
// updated rather than recreated.
//
// Once the plan is fully known it asks the API to validate the configuration, so an invalid
// form fails at plan rather than apply, and a form the API will save in a different order
// gets a warning.
func (r *IncidentIncidentFormResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan models.IncidentFormResourceModel
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		// Something in the plan is still unknown in a way the model can't hold, such as the
		// whole element list. There is nothing to carry over or validate yet.
		return
	}

	// Changing form_type or incident_type_id replaces the form, so the plan creates a new one:
	// it owns none of the state's element IDs, and validate has to check it as new.
	creating := true
	var state models.IncidentFormResourceModel
	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		creating = !plan.FormType.Equal(state.FormType) || !plan.IncidentTypeID.Equal(state.IncidentTypeID)
	}

	if !creating {
		carryElementIDs(plan.LifecycleElements, state.LifecycleElements)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("lifecycle_elements"), plan.LifecycleElements)...)
	}

	if r.client == nil || !planKnownExceptIDs(req.Plan.Raw) {
		return
	}

	// A plan that changes nothing applies nothing, so there is nothing to validate.
	if !creating && req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	// A default lifecycle form always exists, so the API refuses to create one. Say so here
	// with the ID to import, rather than at apply with the API's message.
	if creating && plan.IncidentTypeID.IsNull() && plan.FormType.ValueString() != incidentFormFormTypeCustomFields {
		summary, detail := r.defaultFormExistsError(ctx, plan.FormType.ValueString())
		resp.Diagnostics.AddAttributeError(path.Root("incident_type_id"), summary, detail)
		return
	}

	id := types.StringNull()
	if !creating {
		id = state.ID
	}

	result, err := r.client.IncidentFormsV3ValidateWithResponse(ctx, plan.ToValidatePayload(id))
	if err != nil {
		if isValidationError(err) {
			resp.Diagnostics.AddError("Invalid incident form configuration", err.Error())
		} else {
			// Anything else is the API or the network, which the apply will report if it persists.
			tflog.Warn(ctx, fmt.Sprintf("could not validate incident form at plan time: %s", err))
		}
		return
	}
	if result.JSON200 == nil {
		return
	}

	for _, warning := range result.JSON200.Warnings {
		resp.Diagnostics.AddAttributeWarning(path.Root("lifecycle_elements"), warning.Summary, warning.Detail)
	}
}

// carryElementIDs copies each state element's ID onto the planned element it will become.
// Elements with a natural key match on it wherever they moved to; divider and text elements,
// which have none, match the same type positionally. Anything unmatched stays unknown, and
// gets its ID from the apply.
func carryElementIDs(plan, state []models.IncidentFormLifecycleElementModel) {
	used := map[int]bool{}

	for i := range plan {
		if !plan[i].ID.IsUnknown() {
			continue
		}
		if !plan[i].HasNaturalKey() {
			continue
		}

		for j := range state {
			if used[j] || !state[j].HasNaturalKey() || state[j].NaturalKey() != plan[i].NaturalKey() {
				continue
			}
			plan[i].ID = state[j].ID
			used[j] = true
			break
		}
	}

	for _, elementType := range models.IncidentFormElementTypesWithoutNaturalKey {
		stateIDs := []types.String{}
		for j := range state {
			if state[j].ElementType.ValueString() == elementType && !used[j] {
				stateIDs = append(stateIDs, state[j].ID)
			}
		}

		next := 0
		for i := range plan {
			if plan[i].ElementType.ValueString() != elementType || !plan[i].ID.IsUnknown() {
				continue
			}
			if next >= len(stateIDs) {
				break
			}
			plan[i].ID = stateIDs[next]
			next++
		}
	}
}

// planKnownExceptIDs reports whether everything in the plan but the computed IDs is known,
// so the validate payload built from it is complete.
func planKnownExceptIDs(plan tftypes.Value) bool {
	known := true
	_ = tftypes.Walk(plan, func(p *tftypes.AttributePath, value tftypes.Value) (bool, error) {
		if !known {
			return false, nil
		}
		if value.IsKnown() {
			return true, nil
		}

		if name, ok := lastAttributeName(p); ok && name == "id" {
			return false, nil
		}

		known = false

		return false, nil
	})

	return known
}

func (r *IncidentIncidentFormResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.IncidentFormResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.IncidentTypeID.IsNull() && plan.FormType.ValueString() != incidentFormFormTypeCustomFields {
		summary, detail := r.defaultFormExistsError(ctx, plan.FormType.ValueString())
		resp.Diagnostics.AddAttributeError(path.Root("incident_type_id"), summary, detail)
		return
	}

	result, err := r.client.IncidentFormsV3CreateWithResponse(ctx, plan.ToCreatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create incident form, got error: %s", err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to create incident form: unexpected response from API (status %s)", result.Status()))
		return
	}

	form := result.JSON201.IncidentForm
	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, form.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeIncidentForm, r.terraformVersion)
	}
	tflog.Trace(ctx, fmt.Sprintf("created an incident form with id=%s", form.Id))

	state := models.IncidentFormResourceModel{}.FromAPI(form, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IncidentIncidentFormResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.IncidentFormResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	form, found := r.show(ctx, state.ID.ValueString(), resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		tflog.Warn(ctx, fmt.Sprintf("Incident form with ID %s not found: removing from state.", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}

	newState := models.IncidentFormResourceModel{}.FromAPI(*form, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *IncidentIncidentFormResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.IncidentFormResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.IncidentFormsV3UpdateWithResponse(ctx, plan.ID.ValueString(), plan.ToUpdatePayload())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update incident form, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error",
			fmt.Sprintf("Unable to update incident form: unexpected response from API (status %s)", result.Status()))
		return
	}

	form := result.JSON200.IncidentForm
	if shouldClaim(plan.UnlockInDashboard) {
		claimResource(ctx, r.client, form.Id, &resp.Diagnostics,
			client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeIncidentForm, r.terraformVersion)
	} else {
		unclaimResource(ctx, r.client, form.Id, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeIncidentForm)
	}

	state := models.IncidentFormResourceModel{}.FromAPI(form, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete archives the form. The API refuses to archive a default form, with a 422. That
// form leaves state anyway and goes back to the dashboard, so a configuration can stop
// managing it without `terraform state rm`.
func (r *IncidentIncidentFormResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.IncidentFormResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.IncidentFormsV3DeleteWithResponse(ctx, state.ID.ValueString())
	switch {
	case err == nil, isNotFound(err):
		return
	case isValidationError(err) && state.IncidentTypeID.IsNull() && state.FormType.ValueString() != incidentFormFormTypeCustomFields:
		unclaimResource(ctx, r.client, state.ID.ValueString(), &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeIncidentForm)
		resp.Diagnostics.AddWarning("Default incident form left in place",
			fmt.Sprintf("The default %s form cannot be deleted, so it has been removed from Terraform state and unlocked "+
				"in the dashboard, keeping the configuration it last had. Import it again to manage it from Terraform.",
				state.FormType.ValueString()))
	default:
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete incident form, got error: %s", err))
	}
}

func (r *IncidentIncidentFormResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeIncidentForm, r.terraformVersion,
		r.markImportedAsManaged)
	if resp.Diagnostics.HasError() {
		return
	}

	form, found := r.show(ctx, req.ID, resp.Diagnostics.AddError)
	if resp.Diagnostics.HasError() {
		return
	}
	if !found {
		resp.Diagnostics.AddError("Incident Form Not Found",
			fmt.Sprintf("No incident form with ID %q exists. Escalate forms are not managed by this resource.", req.ID))
		return
	}

	state := models.IncidentFormResourceModel{}.FromAPI(*form, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// show reads one form, reporting a 404 as not found rather than an error so each caller can
// decide what a missing form means for it.
func (r *IncidentIncidentFormResource) show(ctx context.Context, id string, addError func(summary, detail string)) (*client.IncidentFormV3, bool) {
	result, err := r.client.IncidentFormsV3ShowWithResponse(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return nil, false
		}
		addError("Client Error", fmt.Sprintf("Unable to read incident form, got error: %s", err))
		return nil, false
	}
	if result.JSON200 == nil {
		addError("Client Error",
			fmt.Sprintf("Unable to read incident form: unexpected response from API (status %s)", result.Status()))
		return nil, false
	}

	return &result.JSON200.IncidentForm, true
}

// defaultFormExistsError explains that a default form can't be created, naming the ID to
// import when it can be found.
func (r *IncidentIncidentFormResource) defaultFormExistsError(ctx context.Context, formType string) (summary, detail string) {
	detail = fmt.Sprintf("Every organisation already has a default %s form, so one cannot be created. "+
		"Set `incident_type_id` to create one for a specific incident type instead, or import the default form to manage it:",
		formType)

	if id, ok := r.findDefaultFormID(ctx, formType); ok {
		detail += fmt.Sprintf("\n\n  terraform import <address> %s", id)
	} else {
		detail += " its ID is in the dashboard's URL when editing the form."
	}

	return "Default incident form already exists", detail
}

// findDefaultFormID pages through the organisation's forms for the default of a type.
func (r *IncidentIncidentFormResource) findDefaultFormID(ctx context.Context, formType string) (string, bool) {
	params := &client.IncidentFormsV3ListParams{PageSize: lo.ToPtr(int64(250))}
	for {
		result, err := r.client.IncidentFormsV3ListWithResponse(ctx, params)
		if err != nil || result.JSON200 == nil {
			return "", false
		}

		for _, form := range result.JSON200.IncidentForms {
			if string(form.FormType) == formType && form.IncidentTypeId == nil {
				return form.Id, true
			}
		}

		if result.JSON200.PaginationMeta.After == nil {
			return "", false
		}
		params.After = result.JSON200.PaginationMeta.After
	}
}

// isValidationError reports whether err is a 400 or 422: the API rejected the payload,
// rather than failing to answer.
func isValidationError(err error) bool {
	httpErr := client.HTTPError{}
	return errors.As(err, &httpErr) && (httpErr.StatusCode == 400 || httpErr.StatusCode == 422)
}
