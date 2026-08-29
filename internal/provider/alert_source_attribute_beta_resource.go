package provider

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ resource.Resource                   = &alertSourceAttributeBetaResource{}
	_ resource.ResourceWithConfigure      = &alertSourceAttributeBetaResource{}
	_ resource.ResourceWithImportState    = &alertSourceAttributeBetaResource{}
	_ resource.ResourceWithValidateConfig = &alertSourceAttributeBetaResource{}
	_ resource.ResourceWithModifyPlan     = &alertSourceAttributeBetaResource{}
)

func NewAlertSourceAttributeBetaResource() resource.Resource {
	return &alertSourceAttributeBetaResource{}
}

type alertSourceAttributeBetaResource struct {
	client *client.ClientWithResponses
}

// alertSourceAttributeBetaModel is one attribute of one alert source. The value spellings sit
// at the top level rather than under a `binding` key, because the resource is the binding.
//
// There is no id: an attribute is bound at most once per source, so the pair of ids is the
// identity.
type alertSourceAttributeBetaModel struct {
	AlertSourceID    types.String `tfsdk:"alert_source_id"`
	AlertAttributeID types.String `tfsdk:"alert_attribute_id"`
	MergeStrategy    types.String `tfsdk:"merge_strategy"`

	ValueLiteral   types.String          `tfsdk:"value_literal"`
	ValueReference types.String          `tfsdk:"value_reference"`
	ExpressionRef  types.String          `tfsdk:"expression_ref"`
	Values         []types.String        `tfsdk:"values"`
	Value          *models.BindingValue  `tfsdk:"value"`
	ArrayValue     []models.BindingValue `tfsdk:"array_value"`

	Expression       *models.Expression       `tfsdk:"expression"`
	NamedExpressions []models.NamedExpression `tfsdk:"named_expression"`
}

func (r *alertSourceAttributeBetaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source_attribute_beta"
}

func (r *alertSourceAttributeBetaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"alert_source_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceAttributeV3", "alert_source_id"),
			PlanModifiers: []planmodifier.String{
				// Moving a binding to another source is a delete and a create.
				stringplanmodifier.RequiresReplace(),
			},
		},
		"alert_attribute_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceAttributeV3", "alert_attribute_id"),
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"merge_strategy": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: EnumValuesDescription("AlertSourceAttributeV3", "merge_strategy"),
			PlanModifiers: []planmodifier.String{
				// The API fills this from the source type, so a config that leaves it out
				// keeps whatever was chosen rather than planning a change every time.
				stringplanmodifier.UseStateForUnknown(),
			},
		},
	}

	// The value spellings, exactly as every other bindable field takes them.
	maps.Copy(attributes, models.BindingAttributes())

	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Alert Sources V3"), `What fills in one attribute of an alert source, and how values are merged when an alert is updated.

Each attribute is its own resource. Editing one doesn't mean rewriting the source, and two people editing different attributes don't race each other. The source is an `+"`incident_alert_source_beta`"+` resource.

A `+"`named_expression`"+` name only has to be unique within this resource: two attributes of one source can each have one called `+"`severity`"+`.

## How this differs from `+"`incident_alert_source`"+`

`+"`incident_alert_source`"+` declares a source and every attribute it populates together, under
one `+"`template.attributes`"+` list. Filling in one more attribute means rewriting that whole
list, and two people editing different attributes are editing the same resource.

This resource splits an attribute binding out as its own resource, with its own lifecycle,
so filling in one more attribute is an add rather than an edit of something else.

## Beta, and what happens next

This resource is in beta. Its schema may still change in ways that are not backwards
compatible, so pin the provider version if that matters to you.

`+"`incident_alert_source`"+` is not deprecated, and there is no need to move anything yet.`),
		Attributes: attributes,
		Blocks: map[string]schema.Block{
			"expression":       models.ExpressionBlock(),
			"named_expression": models.NamedExpressionBlock(),
		},
	}
}

func (r *alertSourceAttributeBetaResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *IncidentProviderData, got %T. This is a provider bug.", req.ProviderData),
		)
		return
	}
	r.client = data.Client
}

var alertSourceAttributeMergeStrategies = enumValues("AlertSourceAttributeV3", "merge_strategy")

const alertSourceAttributeMissingValue = "Say what fills this attribute in. Set one of " +
	"value_literal, value_reference, expression_ref, values, value or array_value, or declare an " +
	"expression block to compute it."

// ValidateConfig runs the checks the schema can't express, so they land at plan time against a
// path in the config rather than as an API rejection at apply.
func (r *alertSourceAttributeBetaResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data alertSourceAttributeBetaModel

	// The bindings and expression blocks hold values that can be computed by another
	// resource, which the model's concrete types can't represent. Decoding a config like that
	// fails outright, so give up on validating rather than reporting a spurious error.
	if req.Config.Get(ctx, &data).HasError() {
		return
	}

	r.validateValue(&data, &resp.Diagnostics)
	r.validateMergeStrategy(&data, &resp.Diagnostics)

	models.ValidateExpressions(
		models.AlertAttributeExpressions(data.AlertAttributeID.ValueString()),
		data.Expression, path.Root("expression"),
		data.NamedExpressions, path.Root("named_expression"),
		&resp.Diagnostics,
	)
}

// validateValue holds the exclusive group together: the block and the spellings all say what
// this attribute is filled with, and exactly one of them has to.
func (r *alertSourceAttributeBetaResource) validateValue(data *alertSourceAttributeBetaModel, diags *diag.Diagnostics) {
	binding := data.binding()

	switch {
	case binding != nil && data.Expression != nil:
		diags.AddAttributeError(
			path.Root("expression"),
			"Two values for one attribute",
			"An expression block computes the value, so you can't also set one directly. Remove the block, or remove the value.",
		)

	case binding == nil && data.Expression == nil:
		diags.AddError("Missing value", alertSourceAttributeMissingValue)

	case binding != nil:
		models.ValidateBinding(binding, path.Empty(), models.KnownExpressionNames(data.NamedExpressions), diags)
	}
}

func (r *alertSourceAttributeBetaResource) validateMergeStrategy(data *alertSourceAttributeBetaModel, diags *diag.Diagnostics) {
	strategy := data.MergeStrategy
	if strategy.IsNull() || strategy.IsUnknown() {
		return
	}

	if !slices.Contains(alertSourceAttributeMergeStrategies, strategy.ValueString()) {
		diags.AddAttributeError(
			path.Root("merge_strategy"),
			"Unknown merge_strategy",
			fmt.Sprintf("Set one of %s, or remove the attribute to take the source type's default.",
				strings.Join(alertSourceAttributeMergeStrategies, ", ")),
		)
	}
}

// alertSourceAttributeValidateTimeout keeps an unresponsive API from stalling a plan. A var so
// tests needn't wait it out.
var alertSourceAttributeValidateTimeout = 10 * time.Second

// ModifyPlan asks the API whether this binding would be accepted on the source as it stands,
// so a bad expression or reference surfaces here rather than part way through an apply.
//
// It can't live in ValidateConfig, which `terraform validate` also calls: that runs the
// provider without configuring it, so there is no client to ask with.
func (r *alertSourceAttributeBetaResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A destroy plans no binding, and an unconfigured provider has no client.
	if r.client == nil || req.Plan.Raw.IsNull() {
		return
	}

	// Runs for every resource in the plan, changed or not, and each check costs the API a
	// registry build.
	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	// A source this same apply creates isn't there to validate against, and a value waiting on
	// another resource would be rejected for reasons the apply won't hit.
	if !alertSourceAttributeValidateSettled(req.Plan.Raw) {
		return
	}

	// A block holding a value another resource computes doesn't decode into the model's
	// concrete types. Give up rather than fail a plan the API may well accept.
	var data alertSourceAttributeBetaModel
	if req.Plan.Get(ctx, &data).HasError() {
		return
	}

	binding := r.toPayload(&data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, alertSourceAttributeValidateTimeout)
	defer cancel()

	_, err := r.client.AlertSourcesV3ValidateAttributeWithResponse(
		ctx,
		data.AlertSourceID.ValueString(),
		client.AlertSourcesV3ValidateAttributeJSONRequestBody{
			AlertSourceAttribute: client.AlertSourceAttributeValidatePayloadV3{
				AlertAttributeId: data.AlertAttributeID.ValueString(),
				MergeStrategy:    mergeStrategyPayload[client.AlertSourceAttributeValidatePayloadV3MergeStrategy](data.MergeStrategy),
				Value:            binding.value,
				ArrayValue:       binding.arrayValue,
				Expressions:      &binding.expressions,
			},
		},
	)
	if err == nil {
		return
	}

	// 422 is the API rejecting this binding, which is the whole point.
	if httpErr, ok := apiErrorWithStatus(err, http.StatusUnprocessableEntity); ok {
		resp.Diagnostics.AddError("Invalid alert source attribute", httpErr.Error())
		return
	}

	// Anything else means the check didn't run, not that the config is bad — a 404 included,
	// since the source may be created by an apply this plan can't see.
	resp.Diagnostics.AddWarning(
		"Could not validate the alert attribute binding",
		fmt.Sprintf("The binding was not checked, and may still be rejected when you apply: %s", err),
	)
}

// alertSourceAttributeValidateUnsentAttributes are exempt from the settled gate, the check
// never sending them. merge_strategy is unknown on every create, so gating on it would skip
// the plans worth checking; omitting it asks about the default the apply would land on.
var alertSourceAttributeValidateUnsentAttributes = []string{
	"merge_strategy",
}

// alertSourceAttributeValidateSettled reports whether every value the check would send is
// known.
func alertSourceAttributeValidateSettled(plan tftypes.Value) bool {
	attributes := map[string]tftypes.Value{}
	if err := plan.As(&attributes); err != nil {
		return false
	}

	for name, value := range attributes {
		if slices.Contains(alertSourceAttributeValidateUnsentAttributes, name) {
			continue
		}

		if !value.IsFullyKnown() {
			return false
		}
	}

	return true
}

func (r *alertSourceAttributeBetaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data alertSourceAttributeBetaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	binding := r.toPayload(&data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	sourceID := data.AlertSourceID.ValueString()

	result, err := lockForAlertSource(ctx, sourceID, func(ctx context.Context) (*client.AlertSourcesV3CreateAttributeResponse, error) {
		return r.client.AlertSourcesV3CreateAttributeWithResponse(
			ctx,
			sourceID,
			client.AlertSourcesV3CreateAttributeJSONRequestBody{
				AlertSourceAttribute: client.AlertSourceAttributeCreatePayloadV3{
					AlertAttributeId: data.AlertAttributeID.ValueString(),
					MergeStrategy:    mergeStrategyPayload[client.AlertSourceAttributeCreatePayloadV3MergeStrategy](data.MergeStrategy),
					Value:            binding.value,
					ArrayValue:       binding.arrayValue,
					Expressions:      &binding.expressions,
				},
			},
		)
	})
	if isConflict(err) {
		attributeID := data.AlertAttributeID.ValueString()
		summary, detail := alertSourceAttributeConflict(
			r.attributeIsBound(ctx, sourceID, attributeID), sourceID, attributeID, err)
		resp.Diagnostics.AddError(summary, detail)

		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to bind alert attribute", err.Error())
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Unable to bind alert attribute", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, alertSourceAttributeBetaFromAPI(result.JSON201.AlertSourceAttribute, &data))...)
}

func (r *alertSourceAttributeBetaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data alertSourceAttributeBetaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertSourcesV3ShowAttributeWithResponse(
		ctx, data.AlertSourceID.ValueString(), data.AlertAttributeID.ValueString())
	// Covers the source going away as well as the binding: either way this resource is gone.
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read alert source attribute", err.Error())
		return
	}
	// The client turns any non-2xx into an error, so a missing body is an unexpected success
	// rather than an unbound attribute. Dropping it from state would have the next apply try to
	// bind it again and hit the 409, so fail instead.
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to read alert source attribute", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, alertSourceAttributeBetaFromAPI(result.JSON200.AlertSourceAttribute, &data))...)
}

func (r *alertSourceAttributeBetaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertSourceAttributeBetaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	binding := r.toPayload(&plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	sourceID := plan.AlertSourceID.ValueString()

	result, err := lockForAlertSource(ctx, sourceID, func(ctx context.Context) (*client.AlertSourcesV3UpdateAttributeResponse, error) {
		return r.client.AlertSourcesV3UpdateAttributeWithResponse(
			ctx,
			sourceID,
			plan.AlertAttributeID.ValueString(),
			client.AlertSourcesV3UpdateAttributeJSONRequestBody{
				AlertSourceAttribute: client.AlertSourceAttributeUpdatePayloadV3{
					MergeStrategy: mergeStrategyPayload[client.AlertSourceAttributeUpdatePayloadV3MergeStrategy](plan.MergeStrategy),
					Value:         binding.value,
					ArrayValue:    binding.arrayValue,
					Expressions:   &binding.expressions,
				},
			},
		)
	})
	if isConflict(err) {
		resp.Diagnostics.AddError("Unable to update alert source attribute", alertSourceAttributeContended(err))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to update alert source attribute", err.Error())
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to update alert source attribute", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, alertSourceAttributeBetaFromAPI(result.JSON200.AlertSourceAttribute, &plan))...)
}

func (r *alertSourceAttributeBetaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data alertSourceAttributeBetaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sourceID := data.AlertSourceID.ValueString()

	_, err := lockForAlertSource(ctx, sourceID, func(ctx context.Context) (*client.AlertSourcesV3DestroyAttributeResponse, error) {
		return r.client.AlertSourcesV3DestroyAttributeWithResponse(
			ctx,
			sourceID,
			data.AlertAttributeID.ValueString(),
			&client.AlertSourcesV3DestroyAttributeParams{},
		)
	})
	if isConflict(err) {
		resp.Diagnostics.AddError("Unable to unbind alert attribute", alertSourceAttributeContended(err))
		return
	}
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to unbind alert attribute", err.Error())
	}
}

func (r *alertSourceAttributeBetaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	sourceID, attributeID, found := strings.Cut(req.ID, alertSourceAttributeImportSeparator)
	if !found || sourceID == "" || attributeID == "" {
		resp.Diagnostics.AddError(
			"Unexpected import identifier",
			fmt.Sprintf("expected <alert_source_id>%s<alert_attribute_id>, got %q.", alertSourceAttributeImportSeparator, req.ID),
		)
		return
	}

	// The alert source carries the Terraform-managed marker for all of its bindings, so
	// there's nothing to claim here.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("alert_source_id"), sourceID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("alert_attribute_id"), attributeID)...)
}

const alertSourceAttributeImportSeparator = ":"

func alertSourceAttributeImportID(sourceID, attributeID string) string {
	return sourceID + alertSourceAttributeImportSeparator + attributeID
}

// binding gathers the value spellings into the shared binding model. Nil means the config set
// none, which is the case where an expression block is the value.
func (m *alertSourceAttributeBetaModel) binding() *models.Binding {
	binding := &models.Binding{
		ValueLiteral:   m.ValueLiteral,
		ValueReference: m.ValueReference,
		ExpressionRef:  m.ExpressionRef,
		Values:         m.Values,
		Value:          m.Value,
		ArrayValue:     m.ArrayValue,
	}

	if models.SetBindingForms(binding) == 0 {
		return nil
	}

	return binding
}

// mergeStrategyPayload converts the attribute to whichever enum the payload takes, or nil so
// an unset attribute takes the source type's own default.
func mergeStrategyPayload[T ~string](strategy types.String) *T {
	if strategy.IsNull() || strategy.IsUnknown() {
		return nil
	}

	return lo.ToPtr(T(strategy.ValueString()))
}

// alertSourceAttributeBinding is what one write sends: the value in whichever of the API's two
// shapes it takes, plus the expressions it reaches.
type alertSourceAttributeBinding struct {
	value       *client.EngineParamBindingValuePayloadV3
	arrayValue  *[]client.EngineParamBindingValuePayloadV3
	expressions []client.ExpressionPayloadV3
}

// toPayload maps the value and the expressions together, because declaring an expression block
// is what binds its result: the block produces both an expression and the reference to it.
//
// expressions is always sent, empty included. The API reads it as the full set this attribute
// owns, so omitting it on an edit that drops an expression would leave the old one behind.
func (r *alertSourceAttributeBetaResource) toPayload(
	data *alertSourceAttributeBetaModel,
	diags *diag.Diagnostics,
) alertSourceAttributeBinding {
	ns := models.AlertAttributeExpressions(data.AlertAttributeID.ValueString())

	expressions, boundBinding, err := models.ExpressionsToPayload(ns, data.Expression, data.NamedExpressions)
	if err != nil {
		diags.AddAttributeError(path.Root("expression"), "Invalid expression", err.Error())
		return alertSourceAttributeBinding{}
	}

	binding := alertSourceAttributeBinding{expressions: expressions}

	if data.Expression != nil {
		binding.value = boundBinding
		return binding
	}

	payload, err := models.BindingToPayload(data.binding())
	if err != nil {
		diags.AddError("Invalid value", err.Error())
		return alertSourceAttributeBinding{}
	}
	// Unreachable through a validated config: the API drops a binding that holds nothing, so
	// the next plan would add it back.
	if payload == nil {
		diags.AddError("Missing value", alertSourceAttributeMissingValue)
		return alertSourceAttributeBinding{}
	}

	payload = ns.NamespaceBinding(payload, data.NamedExpressions)

	binding.value, binding.arrayValue = payload.Value, payload.ArrayValue

	return binding
}

// alertSourceAttributeBetaFromAPI projects an API binding into Terraform state. prior is the
// plan on create and update, the prior state on read, and settles what the payload can't: the
// order the config wrote its named expressions in, and which spelling its value used.
func alertSourceAttributeBetaFromAPI(
	attribute client.AlertSourceAttributeV3,
	prior *alertSourceAttributeBetaModel,
) *alertSourceAttributeBetaModel {
	ns := models.AlertAttributeExpressions(attribute.AlertAttributeId)
	expression, named := models.ExpressionsFromPayload(
		attribute.Expressions, ns, prior.Expression, prior.NamedExpressions)

	model := &alertSourceAttributeBetaModel{
		AlertSourceID:    types.StringValue(attribute.AlertSourceId),
		AlertAttributeID: types.StringValue(attribute.AlertAttributeId),
		MergeStrategy:    types.StringValue(string(attribute.MergeStrategy)),

		Expression:       expression,
		NamedExpressions: named,
	}

	binding := models.ReconcileBinding(prior.binding(), ns.LocaliseBinding(&client.EngineParamBindingPayloadV3{
		Value:      attribute.Value,
		ArrayValue: attribute.ArrayValue,
	}))

	// Declaring the block is what binds it, so the reference it produced is not also a value.
	// Returning it as one would diff against a config that only has the block.
	if expression != nil {
		binding = nil
	}

	bindingToModel(binding, model)

	return model
}

// bindingToModel spreads a binding's spellings onto the resource's own attributes.
func bindingToModel(binding *models.Binding, model *alertSourceAttributeBetaModel) {
	model.ValueLiteral = types.StringNull()
	model.ValueReference = types.StringNull()
	model.ExpressionRef = types.StringNull()

	if binding == nil {
		return
	}

	model.ValueLiteral = binding.ValueLiteral
	model.ValueReference = binding.ValueReference
	model.ExpressionRef = binding.ExpressionRef
	model.Values = binding.Values
	model.Value = binding.Value
	model.ArrayValue = binding.ArrayValue
}

// attributeIsBound reports whether the attribute is bound on the source.
//
// Anything other than a clear yes reads as no, so a create that hit a conflict for some other
// reason is described as what it is rather than sent to import a binding that isn't there.
func (r *alertSourceAttributeBetaResource) attributeIsBound(ctx context.Context, sourceID, attributeID string) bool {
	result, err := r.client.AlertSourcesV3ShowAttributeWithResponse(ctx, sourceID, attributeID)

	return err == nil && result.JSON200 != nil
}

// alertSourceAttributeConflict explains a 409 from a create. The API answers 409 both for an
// attribute that is already bound and for losing a race on the source's config lock, and the
// two need opposite advice — adopt the existing binding, or just run again. Which one it was
// comes from asking whether the attribute is bound, not from reading the message.
func alertSourceAttributeConflict(bound bool, sourceID, attributeID string, err error) (string, string) {
	if bound {
		return "This attribute is already bound on this alert source",
			fmt.Sprintf(
				"Something else already fills this attribute in, either the dashboard or another "+
					"Terraform resource. To manage it here instead, import it:\n\n"+
					"    terraform import <address of this resource> %s\n\n"+
					"Underlying error: %s",
				alertSourceAttributeImportID(sourceID, attributeID),
				err.Error(),
			)
	}

	return "Unable to bind alert attribute", alertSourceAttributeContended(err)
}

// alertSourceAttributeContended explains the 409 that means a lost race rather than an existing
// binding. Only a create has to tell the two apart: an attribute that isn't bound answers 404 to an
// update or a destroy, so their only 409 is this one.
func alertSourceAttributeContended(err error) string {
	return fmt.Sprintf(
		"The provider serialises writes to one alert source, and this one lost that race. Try "+
			"again.\n\nUnderlying error: %s",
		err.Error(),
	)
}
