package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
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
	_ resource.Resource                   = &alertSourceBetaResource{}
	_ resource.ResourceWithConfigure      = &alertSourceBetaResource{}
	_ resource.ResourceWithImportState    = &alertSourceBetaResource{}
	_ resource.ResourceWithValidateConfig = &alertSourceBetaResource{}
	_ resource.ResourceWithModifyPlan     = &alertSourceBetaResource{}
)

func NewAlertSourceBetaResource() resource.Resource {
	return &alertSourceBetaResource{}
}

type alertSourceBetaResource struct {
	resourceConfigurer
}

// alertSourceExpressions stores names as written: only this source's attributes, which share a
// namespace with each other, need one of their own.
var alertSourceExpressions = models.ExpressionNamespace{}

// alertSourceBetaModel carries no attribute bindings: each one is its own
// incident_alert_source_attribute_beta resource, so holding them here would mean an apply of
// this resource wiping whatever those manage.
//
// priority and visible_to_teams take a value directly rather than through an unnamed expression
// block, because an unnamed block couldn't say which field it binds. A field wanting an
// expression names one of this resource's named_expression blocks with expression_ref.
//
// title and description carry rich text: a literal interpolates the scope with "{{ variable }}",
// or holds a raw AST document for anything a template can't express.
type alertSourceBetaModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	SourceType types.String `tfsdk:"source_type"`

	SecretToken    types.String `tfsdk:"secret_token"`
	AlertEventsURL types.String `tfsdk:"alert_events_url"`
	EmailAddress   types.String `tfsdk:"email_address"`

	OwningTeamIDs types.Set  `tfsdk:"owning_team_ids"`
	IsPrivate     types.Bool `tfsdk:"is_private"`

	Title       *models.TemplatedTextValue `tfsdk:"title"`
	Description *models.TemplatedTextValue `tfsdk:"description"`

	Priority       *models.Binding `tfsdk:"priority"`
	VisibleToTeams *models.Binding `tfsdk:"visible_to_teams"`

	NamedExpressions []models.NamedExpression `tfsdk:"named_expression"`

	JiraOptions       *alertSourceJiraOptions       `tfsdk:"jira_options"`
	HeartbeatOptions  *alertSourceHeartbeatOptions  `tfsdk:"heartbeat_options"`
	EmailOptions      *alertSourceEmailOptions      `tfsdk:"email_options"`
	HTTPCustomOptions *alertSourceHTTPCustomOptions `tfsdk:"http_custom_options"`

	RateLimitSharding *alertSourceRateLimitSharding `tfsdk:"rate_limit_sharding"`

	AutoResolveTimeoutMinutes types.Int64 `tfsdk:"auto_resolve_timeout_minutes"`
	AutoResolveIncidentAlerts types.Bool  `tfsdk:"auto_resolve_incident_alerts"`

	Version types.Int64 `tfsdk:"version"`
}

func (r *alertSourceBetaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source_beta"
}

func (r *alertSourceBetaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Alert Sources V3"), `An alert source, without the attributes it populates — each of those is an `+"`incident_alert_source_attribute_beta`"+` resource. Editing one attribute therefore doesn't mean rewriting the source, and two people editing different attributes don't race each other.

## How this differs from `+"`incident_alert_source`"+`

`+"`incident_alert_source`"+` declares a source and every attribute it populates together, under
one `+"`template.attributes`"+` list. Filling in one more attribute means rewriting that whole
list, and two people editing different attributes are editing the same resource.

This resource splits the two apart: the source holds its own configuration — name, type,
title, description, priority — and each attribute binding is its own
`+"`incident_alert_source_attribute_beta`"+` resource with its own lifecycle.

## Beta, and what happens next

This resource is in beta. Its schema may still change in ways that are not backwards
compatible, so pin the provider version if that matters to you.

`+"`incident_alert_source`"+` is not deprecated, and there is no need to move anything yet.`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "name"),
			},
			"source_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: EnumValuesDescription("AlertSourceV3", "source_type"),
				PlanModifiers: []planmodifier.String{
					// A source's type is fixed once it exists.
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secret_token": schema.StringAttribute{
				Computed: true,
				// This token authenticates anyone sending events to the alert source, so
				// it is more sensitive than the rest of the resource and should not be
				// printed in plan output, which often ends up in CI logs.
				//
				// It is still stored in plain text in state, as all Terraform values are.
				// To read it during setup, wrap it in nonsensitive():
				//
				//   output "secret_token" {
				//     value = nonsensitive(incident_alert_source_beta.example.secret_token)
				//   }
				Sensitive:           true,
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "secret_token"),
				PlanModifiers: []planmodifier.String{
					// Including null: most source types have no token, so the plain modifier
					// would skip and leave this planning "known after apply" on every plan.
					useStateForUnknownIncludingNull{},
				},
			},
			"alert_events_url": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "alert_events_url"),
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIncludingNull{},
				},
			},
			// Computed only, despite V2 also marking it Optional: there is no field for it on
			// either write payload, so a config setting it would plan a value we never send and
			// fail the apply as an inconsistent result.
			"email_address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceEmailOptionsV3", "email_address"),
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIncludingNull{},
				},
			},
			"owning_team_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "owning_team_ids"),
			},
			"is_private": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "is_private"),
			},

			// The feature sets are the server's, from sourceconfigs.TitleParam and
			// DescriptionParam. A title renders as one line of plain text, so formatting and
			// line breaks in one are dropped on read.
			"title": models.TemplatedTextAttribute(
				apischema.Docstring("AlertSourceV3", "title"), "plain_single_line"),
			"description": models.TemplatedTextAttribute(
				apischema.Docstring("AlertSourceV3", "description"), "rich"),

			"priority":         models.BindingAttribute(apischema.Docstring("AlertSourceV3", "priority")),
			"visible_to_teams": models.BindingAttribute(apischema.Docstring("AlertSourceV3", "visible_to_teams")),

			"jira_options":        jiraOptionsAttribute(),
			"heartbeat_options":   heartbeatOptionsAttribute(),
			"email_options":       emailOptionsAttribute(),
			"http_custom_options": httpCustomOptionsAttribute(),
			"rate_limit_sharding": rateLimitShardingAttribute(),

			"auto_resolve_timeout_minutes": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "auto_resolve_timeout_minutes"),
			},
			"auto_resolve_incident_alerts": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "auto_resolve_incident_alerts"),
				PlanModifiers: []planmodifier.Bool{
					useStateForUnknownIncludingNull{},
				},
			},

			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("AlertSourceV3", "version"),
			},
		},
		Blocks: map[string]schema.Block{
			"named_expression": models.NamedExpressionBlock(),
		},
	}
}

// ValidateConfig runs the checks the schema can't express, so they land at plan time against a
// path in the config rather than as an API rejection at apply.
func (r *alertSourceBetaResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data alertSourceBetaModel

	// The bindings and expression blocks hold values that can be computed by another
	// resource, which the model's concrete types can't represent. Decoding a config like that
	// fails outright, so give up on validating rather than reporting a spurious error.
	if req.Config.Get(ctx, &data).HasError() {
		return
	}

	r.validateOptionsMatchSourceType(ctx, req, resp)
	r.validateHeartbeatTemplate(&data, &resp.Diagnostics)
	r.validateTemplateProvided(&data, &resp.Diagnostics)
	r.validatePrivacy(&data, &resp.Diagnostics)

	// An empty list stores no options at all, so the block would read back absent and fail the
	// apply. A Jira source watching no projects does nothing anyway.
	if data.JiraOptions != nil && len(data.JiraOptions.ProjectIDs) == 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("jira_options").AtName("project_ids"),
			"No Jira projects",
			"List at least one project to watch for new issues.",
		)
	}

	validateRateLimitSharding(data.RateLimitSharding, &resp.Diagnostics)

	models.ValidateExpressions(
		alertSourceExpressions, nil, path.Empty(),
		data.NamedExpressions, path.Root("named_expression"), &resp.Diagnostics)

	for name, value := range map[string]*models.TemplatedTextValue{
		"title":       data.Title,
		"description": data.Description,
	} {
		models.ValidateTemplatedTextValue(value, path.Root(name), &resp.Diagnostics)
	}

	known := models.KnownExpressionNames(data.NamedExpressions)
	for name, binding := range map[string]*models.Binding{
		"priority":         data.Priority,
		"visible_to_teams": data.VisibleToTeams,
	} {
		models.ValidateBinding(binding, path.Root(name), known, &resp.Diagnostics)
	}
}

// validateOptionsMatchSourceType checks each options block against the source type that reads
// it. It reads the blocks as objects rather than off the decoded model so it can tell "not set"
// apart from "computed from another resource, not known yet" — Terraform re-runs validation at
// apply, by which point the type is known.
func (r *alertSourceBetaResource) validateOptionsMatchSourceType(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var sourceType types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("source_type"), &sourceType)...)
	if resp.Diagnostics.HasError() || sourceType.IsUnknown() {
		return
	}

	for _, options := range []struct {
		name string
		// The source type that reads these options.
		sourceType string
		// Whether that source type cannot be created without them.
		required bool
		why      string
	}{
		{"jira_options", "jira", true, "which projects to watch for new issues"},
		{"heartbeat_options", "heartbeat", true, "the interval a ping is expected within"},
		{"http_custom_options", "http_custom", true, "the transform expression and deduplication key path"},
		// Email sources work without them: no options means no redactions and no transform.
		{"email_options", "email", false, ""},
	} {
		at := path.Root(options.name)

		var block types.Object
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, at, &block)...)
		if resp.Diagnostics.HasError() || block.IsUnknown() {
			continue
		}

		set := !block.IsNull()
		matches := sourceType.ValueString() == options.sourceType

		if set && !matches {
			resp.Diagnostics.AddAttributeError(
				at,
				fmt.Sprintf("%s is only for %s alert sources", options.name, options.sourceType),
				fmt.Sprintf("This source's type is %q, which doesn't read these options. Remove the block.", sourceType.ValueString()),
			)
			continue
		}

		if !set && matches && options.required {
			resp.Diagnostics.AddAttributeError(
				at,
				fmt.Sprintf("%s is required for %s alert sources", options.name, options.sourceType),
				fmt.Sprintf("A %s source needs %s.", options.sourceType, options.why),
			)
		}
	}
}

// validateHeartbeatTemplate rejects a title or description on a heartbeat source. The API
// generates both, so anything set here is silently replaced.
func (r *alertSourceBetaResource) validateHeartbeatTemplate(data *alertSourceBetaModel, diags *diag.Diagnostics) {
	if data.SourceType.ValueString() != "heartbeat" {
		return
	}

	for name, value := range map[string]*models.TemplatedTextValue{
		"title":       data.Title,
		"description": data.Description,
	} {
		if value == nil {
			continue
		}

		diags.AddAttributeError(
			path.Root(name),
			fmt.Sprintf("%s can't be set on a heartbeat alert source", name),
			fmt.Sprintf("Heartbeat sources write their own %s, so this value would be replaced.", name),
		)
	}
}

// validateTemplateProvided requires a title and description from the source types that accept
// one. A create sending neither gets the API's own default template, which an Optional attribute
// has nowhere to put, while the same absence on an update clears the field instead.
func (r *alertSourceBetaResource) validateTemplateProvided(data *alertSourceBetaModel, diags *diag.Diagnostics) {
	if data.SourceType.IsNull() || data.SourceType.IsUnknown() {
		return
	}

	// Heartbeat sources write their own, and validateHeartbeatTemplate rejects setting one.
	if data.SourceType.ValueString() == "heartbeat" {
		return
	}

	for name, value := range map[string]*models.TemplatedTextValue{
		"title":       data.Title,
		"description": data.Description,
	} {
		if value != nil {
			continue
		}

		diags.AddAttributeError(
			path.Root(name),
			fmt.Sprintf("%s is required for source_type %q", name, data.SourceType.ValueString()),
			fmt.Sprintf("Set a %s. Without one the API writes its own default, which this resource "+
				"can't store.", name),
		)
	}
}

// validatePrivacy ties visible_to_teams to is_private: it says who can see a private source's
// alerts, so each is meaningless without the other.
func (r *alertSourceBetaResource) validatePrivacy(data *alertSourceBetaModel, diags *diag.Diagnostics) {
	if data.IsPrivate.IsUnknown() {
		return
	}

	private := data.IsPrivate.ValueBool()

	switch {
	case data.VisibleToTeams != nil && !private:
		diags.AddAttributeError(
			path.Root("visible_to_teams"),
			"visible_to_teams needs is_private",
			"This says which teams can see a private source's alerts. Set is_private = true, or remove it.",
		)

	case data.VisibleToTeams == nil && private:
		diags.AddAttributeError(
			path.Root("visible_to_teams"),
			"visible_to_teams is required when is_private is true",
			"A private source's alerts are visible to nobody until you say which teams can see them.",
		)
	}
}

// alertSourceBetaValidateTimeout keeps an unresponsive API from stalling a plan. A var so
// tests needn't wait it out.
var alertSourceBetaValidateTimeout = 10 * time.Second

// ModifyPlan asks the API whether the planned source would be accepted, so an expression
// that doesn't compile surfaces here rather than part way through an apply.
//
// It can't live in ValidateConfig, which `terraform validate` also calls: that runs the
// provider without configuring it, so there is no client to ask with.
func (r *alertSourceBetaResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// A destroy plans no source, and an unconfigured provider has no client.
	if r.client == nil || req.Plan.Raw.IsNull() {
		return
	}

	// The framework runs this for every resource in the plan, changed or not. A rejection
	// on a source planning no change isn't something an apply could fix, and asking costs
	// the API a registry build per source on every plan.
	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	// An expression pointing at a catalog type this same apply creates is unknown until it
	// exists, and validating around the gaps reports errors the apply won't hit.
	if !alertSourceBetaValidateSettled(req.Plan.Raw) {
		return
	}

	// The options blocks can hold values another resource computes, which the model's
	// concrete types can't represent. Decoding one fails outright, so give up on validating
	// rather than failing the plan over a config the API may well accept.
	var data alertSourceBetaModel
	if req.Plan.Get(ctx, &data).HasError() {
		return
	}

	expressions, bindings := r.toPayloads(&data, &resp.Diagnostics)
	owningTeamIDs := r.toTeamIDsPayload(ctx, data.OwningTeamIDs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, alertSourceBetaValidateTimeout)
	defer cancel()

	result, err := r.client.AlertSourcesV3ValidateWithResponse(ctx, client.AlertSourcesV3ValidateJSONRequestBody{
		AlertSource: client.AlertSourceValidatePayloadV3{
			SourceType:     client.AlertSourceValidatePayloadV3SourceType(data.SourceType.ValueString()),
			OwningTeamIds:  owningTeamIDs,
			IsPrivate:      data.IsPrivate.ValueBoolPointer(),
			Title:          bindings.title,
			Description:    bindings.description,
			Priority:       bindings.priority,
			VisibleToTeams: bindings.visibleToTeams,
			Expressions:    &expressions,

			// Carried so the source-type capability check runs at plan time rather than
			// surfacing as a 422 half way through an apply.
			RateLimitSharding: data.RateLimitSharding.toPayload(),
		},
	})
	if err == nil {
		addAlertSourceBetaValidateWarnings(result, &resp.Diagnostics)
		return
	}

	// 422 is the API rejecting this source, which is the whole point.
	if httpErr, ok := apiErrorWithStatus(err, http.StatusUnprocessableEntity); ok {
		resp.Diagnostics.AddError("Invalid alert source", httpErr.Error())
		return
	}

	// Anything else means the check didn't run, not that the config is bad, so failing here
	// would break plans that would have applied fine.
	resp.Diagnostics.AddWarning(
		"Could not validate the alert source",
		fmt.Sprintf("The alert source was not checked, and may still be rejected when you apply: %s", err),
	)
}

// alertSourceBetaWarningPaths maps a payload field the API warns about onto the attribute
// that holds it. Only the literal can carry a reference the scope doesn't have, which is
// what these warnings are about.
var alertSourceBetaWarningPaths = map[string]path.Path{
	"title":       path.Root("title").AtName("literal"),
	"description": path.Root("description").AtName("literal"),
}

// addAlertSourceBetaValidateWarnings reports what the API found suspect without rejecting,
// such as a "{{ payload.sumary }}" that will silently render as "(not set)".
func addAlertSourceBetaValidateWarnings(result *client.AlertSourcesV3ValidateResponse, diags *diag.Diagnostics) {
	if result == nil || result.JSON200 == nil {
		return
	}

	for _, warning := range result.JSON200.Warnings {
		if attribute, ok := alertSourceBetaWarningPaths[warning.Path]; ok {
			diags.AddAttributeWarning(attribute, warning.Summary, warning.Detail)
			continue
		}

		// A path we don't know still says something worth reading, just not against a
		// particular attribute: a new check on the API shouldn't need a provider release.
		diags.AddWarning(warning.Summary, warning.Detail)
	}
}

// alertSourceBetaValidatedAttributes are the attributes the validate payload is built
// from. Gating on the whole plan instead would skip every create, which plans id and
// secret_token unknown.
var alertSourceBetaValidatedAttributes = []string{
	"source_type",
	"owning_team_ids",
	"is_private",
	"title",
	"description",
	"priority",
	"visible_to_teams",
	"named_expression",
	"rate_limit_sharding",
}

// alertSourceBetaValidateSettled reports whether every value the check would send is
// known. The only Optional+Computed one is is_private, whose static default lands before
// this runs, so anything unknown here is waiting on another resource.
func alertSourceBetaValidateSettled(plan tftypes.Value) bool {
	attributes := map[string]tftypes.Value{}
	if err := plan.As(&attributes); err != nil {
		return false
	}

	for _, name := range alertSourceBetaValidatedAttributes {
		value, ok := attributes[name]
		if !ok || !value.IsFullyKnown() {
			return false
		}
	}

	return true
}

func (r *alertSourceBetaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data alertSourceBetaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	expressions, bindings := r.toPayloads(&data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := client.AlertSourceCreatePayloadV3{
		Name:           data.Name.ValueString(),
		SourceType:     client.AlertSourceCreatePayloadV3SourceType(data.SourceType.ValueString()),
		OwningTeamIds:  r.toTeamIDsPayload(ctx, data.OwningTeamIDs, &resp.Diagnostics),
		IsPrivate:      data.IsPrivate.ValueBoolPointer(),
		Title:          bindings.title,
		Description:    bindings.description,
		Priority:       bindings.priority,
		VisibleToTeams: bindings.visibleToTeams,
		Expressions:    &expressions,

		JiraOptions:       data.JiraOptions.toPayload(),
		HeartbeatOptions:  data.HeartbeatOptions.toPayload(),
		EmailOptions:      data.EmailOptions.toPayload(),
		HttpCustomOptions: data.HTTPCustomOptions.toPayload(),
		RateLimitSharding: data.RateLimitSharding.toPayload(),

		Annotations: r.annotations(),
	}
	r.applyAutoResolve(&data, &payload.AutoResolveTimeoutMinutes, &payload.AutoResolveIncidentAlerts)

	// Re-checked here because a path from a variable is unknown at plan time, so ValidateConfig
	// has nothing to judge.
	validateRateLimitSharding(data.RateLimitSharding, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertSourcesV3CreateWithResponse(ctx, client.AlertSourcesV3CreateJSONRequestBody{
		AlertSource: payload,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create alert source", err.Error())
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError("Unable to create alert source", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, alertSourceBetaFromAPI(result.JSON201.AlertSource, &data, &resp.Diagnostics))...)
}

func (r *alertSourceBetaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data alertSourceBetaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertSourcesV3ShowWithResponse(ctx, data.ID.ValueString())
	if isNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read alert source", err.Error())
		return
	}
	// The client turns any non-2xx into an error, so a missing body is an unexpected success
	// rather than a deleted source. Dropping it from state would have the next apply create a
	// second one, so fail instead.
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to read alert source", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, alertSourceBetaFromAPI(result.JSON200.AlertSource, &data, &resp.Diagnostics))...)
}

func (r *alertSourceBetaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state alertSourceBetaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	expressions, bindings := r.toPayloads(&plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Always send owning_team_ids, as an empty list when the attribute is unset: omitting it
	// tells the API to leave ownership alone, which would strand the source on its old teams
	// after they're removed from the config.
	owningTeamIDs := r.toTeamIDsPayload(ctx, plan.OwningTeamIDs, &resp.Diagnostics)
	if owningTeamIDs == nil {
		owningTeamIDs = &[]string{}
	}

	payload := client.AlertSourceUpdatePayloadV3{
		Name:           plan.Name.ValueString(),
		OwningTeamIds:  owningTeamIDs,
		IsPrivate:      plan.IsPrivate.ValueBool(),
		Title:          bindings.title,
		Description:    bindings.description,
		Priority:       bindings.priority,
		VisibleToTeams: bindings.visibleToTeams,
		Expressions:    &expressions,

		JiraOptions:       plan.JiraOptions.toPayload(),
		HeartbeatOptions:  plan.HeartbeatOptions.toPayload(),
		EmailOptions:      emailOptionsUpdatePayload(plan.EmailOptions, plan.SourceType),
		HttpCustomOptions: plan.HTTPCustomOptions.toPayload(),
		RateLimitSharding: rateLimitShardingUpdatePayload(plan.RateLimitSharding),

		// No expected_version, deliberately: a version covers the whole source, and each
		// attribute is its own resource writing the same one. Removing an attribute in the
		// apply that also edits the source bumps the version before this write, because
		// Terraform destroys a dependent before touching what it depends on — so pinning the
		// version read at refresh rejects a legitimate apply.
		//
		// Nothing is lost by leaving it out. The write is a read-modify-write under a row lock
		// that pins the version it just read, so it can neither clobber a concurrent write nor
		// touch an attribute's binding, and a change made outside Terraform still shows as
		// drift on the next plan.

		// Re-asserted on every write, so the source stays claimed even if something cleared
		// the marker, and the recorded version tracks the Terraform in use.
		Annotations: r.annotations(),
	}
	r.applyAutoResolve(&plan, &payload.AutoResolveTimeoutMinutes, &payload.AutoResolveIncidentAlerts)

	validateRateLimitSharding(plan.RateLimitSharding, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AlertSourcesV3UpdateWithResponse(ctx, state.ID.ValueString(), client.AlertSourcesV3UpdateJSONRequestBody{
		AlertSource: payload,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update alert source", err.Error())
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Unable to update alert source", fmt.Sprintf("unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, alertSourceBetaFromAPI(result.JSON200.AlertSource, &plan, &resp.Diagnostics))...)
}

func (r *alertSourceBetaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data alertSourceBetaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.AlertSourcesV3DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete alert source", err.Error())
	}
}

func (r *alertSourceBetaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Create and Update carry the Terraform annotation in their payload, but an import writes
	// nothing, so claim the source here instead.
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeAlertSource, r.terraformVersion, r.markImportedAsManaged)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *alertSourceBetaResource) annotations() *map[string]string {
	return &map[string]string{
		"incident.io/terraform/version": r.terraformVersion,
	}
}

// alertSourceBindings is the four bindable fields, mapped together so Create and Update don't
// each repeat the error handling.
type alertSourceBindings struct {
	title          *client.EngineParamBindingPayloadV3
	description    *client.EngineParamBindingPayloadV3
	priority       *client.EngineParamBindingPayloadV3
	visibleToTeams *client.EngineParamBindingPayloadV3
}

func (r *alertSourceBetaResource) toPayloads(
	data *alertSourceBetaModel,
	diags *diag.Diagnostics,
) ([]client.ExpressionPayloadV3, alertSourceBindings) {
	// Nil for the unnamed block: this resource owns only named expressions, so the binding
	// ExpressionsToPayload returns for it is unused.
	expressions, _, err := models.ExpressionsToPayload(alertSourceExpressions, nil, data.NamedExpressions)
	if err != nil {
		diags.AddAttributeError(path.Root("named_expression"), "Invalid expression", err.Error())
		return nil, alertSourceBindings{}
	}

	bindings := alertSourceBindings{}

	for _, field := range []struct {
		name  string
		value *models.TemplatedTextValue
		into  **client.EngineParamBindingPayloadV3
	}{
		{"title", data.Title, &bindings.title},
		{"description", data.Description, &bindings.description},
	} {
		payload, err := models.TemplatedTextValueToPayload(field.value)
		if err != nil {
			diags.AddAttributeError(path.Root(field.name), "Invalid value", err.Error())
			continue
		}
		*field.into = payload
	}

	for _, field := range []struct {
		name    string
		binding *models.Binding
		into    **client.EngineParamBindingPayloadV3
	}{
		{"priority", data.Priority, &bindings.priority},
		{"visible_to_teams", data.VisibleToTeams, &bindings.visibleToTeams},
	} {
		payload, err := models.BindingToPayload(field.binding)
		if err != nil {
			diags.AddAttributeError(path.Root(field.name), "Invalid value", err.Error())
			continue
		}
		*field.into = payload
	}

	return expressions, bindings
}

// templatedTextFromAPI reads a rich text binding back, reporting anything the attribute can't
// hold rather than dropping it: the API assigns title and description unconditionally, so
// reading one as absent would have the next apply delete it.
func templatedTextFromAPI(
	binding *client.EngineParamBindingPayloadV3,
	name string,
	diags *diag.Diagnostics,
) *models.TemplatedTextValue {
	value, err := models.TemplatedTextValueFromPayload(binding)
	if err != nil {
		diags.AddAttributeError(
			path.Root(name),
			fmt.Sprintf("This alert source's %s isn't manageable here", name),
			fmt.Sprintf(
				"Its %s %s. Manage this source in the dashboard until that is supported, so the "+
					"existing value isn't lost.",
				name, err.Error(),
			),
		)

		return nil
	}

	return value
}

// applyAutoResolve sets the auto-resolve fields only when the config gave them a value. Some
// source types reject them outright — a heartbeat source has nothing to auto-resolve — and the
// API reads a field being present as the caller setting it.
func (r *alertSourceBetaResource) applyAutoResolve(data *alertSourceBetaModel, timeout **int64, incidentAlerts **bool) {
	if !data.AutoResolveTimeoutMinutes.IsNull() && !data.AutoResolveTimeoutMinutes.IsUnknown() {
		*timeout = data.AutoResolveTimeoutMinutes.ValueInt64Pointer()
	}
	if !data.AutoResolveIncidentAlerts.IsNull() && !data.AutoResolveIncidentAlerts.IsUnknown() {
		*incidentAlerts = data.AutoResolveIncidentAlerts.ValueBoolPointer()
	}
}

func (r *alertSourceBetaResource) toTeamIDsPayload(ctx context.Context, set types.Set, diags *diag.Diagnostics) *[]string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	ids := []string{}
	diags.Append(set.ElementsAs(ctx, &ids, false)...)
	if diags.HasError() {
		return nil
	}

	return &ids
}

// alertSourceBetaFromAPI projects an API alert source into Terraform state. config is the plan
// or the prior state, which the read needs wherever the API's answer doesn't pin a single
// spelling: which named expressions were written in what order, which of a binding's
// interchangeable spellings it used, and whether "nothing" was spelled as an empty collection or
// an omitted attribute.
func alertSourceBetaFromAPI(
	source client.AlertSourceV3,
	config *alertSourceBetaModel,
	diags *diag.Diagnostics,
) *alertSourceBetaModel {
	// No unnamed block on this resource, so no prior for one either.
	_, named := models.ExpressionsFromPayload(
		source.Expressions, alertSourceExpressions, nil, config.NamedExpressions)

	model := &alertSourceBetaModel{
		ID:         types.StringValue(source.Id),
		Name:       types.StringValue(source.Name),
		SourceType: types.StringValue(string(source.SourceType)),
		Version:    types.Int64Value(source.Version),

		SecretToken:    types.StringPointerValue(source.SecretToken),
		AlertEventsURL: types.StringPointerValue(source.AlertEventsUrl),

		OwningTeamIDs: teamIDsToState(lo.FromPtr(source.OwningTeamIds), config.OwningTeamIDs),
		IsPrivate:     types.BoolValue(source.IsPrivate),

		Title:       templatedTextFromAPI(source.Title, "title", diags),
		Description: templatedTextFromAPI(source.Description, "description", diags),

		Priority:       models.ReconcileBinding(config.Priority, source.Priority),
		VisibleToTeams: models.ReconcileBinding(config.VisibleToTeams, source.VisibleToTeams),

		NamedExpressions: named,

		JiraOptions:       jiraOptionsFromAPI(source.JiraOptions),
		HeartbeatOptions:  heartbeatOptionsFromAPI(source.HeartbeatOptions),
		EmailOptions:      emailOptionsFromAPI(source.EmailOptions),
		HTTPCustomOptions: httpCustomOptionsFromAPI(source.HttpCustomOptions),
		RateLimitSharding: rateLimitShardingFromAPI(source.RateLimitSharding),

		AutoResolveTimeoutMinutes: types.Int64PointerValue(source.AutoResolveTimeoutMinutes),
		AutoResolveIncidentAlerts: types.BoolPointerValue(source.AutoResolveIncidentAlerts),

		// Minted for email sources and absent for every other type, so it lives at the top
		// level rather than inside the optional email_options block.
		EmailAddress: types.StringNull(),
	}

	if source.EmailOptions != nil {
		model.EmailAddress = types.StringValue(source.EmailOptions.EmailAddress)
	}

	// An email source always reads back with options, because the address we mint for it lives
	// in them. A config that set no block would otherwise go from null to an object, which
	// Terraform rejects as an inconsistent result.
	if config.EmailOptions == nil && model.EmailOptions != nil &&
		model.EmailOptions.TransformExpression.IsNull() && len(model.EmailOptions.Redactions) == 0 {
		model.EmailOptions = nil
	}

	// The API ignores auto_resolve_incident_alerts where there's no timeout to resolve against,
	// and never sends it for a heartbeat source. Keep what was asked for, or Terraform sees the
	// result as inconsistent with the plan and every later plan as a change.
	//
	// The unknown check is what makes this safe on create: the attribute is Optional+Computed,
	// so a config that omits it plans unknown, and storing that would fail the apply outright.
	if model.AutoResolveTimeoutMinutes.IsNull() && model.AutoResolveIncidentAlerts.IsNull() &&
		!config.AutoResolveIncidentAlerts.IsUnknown() {
		model.AutoResolveIncidentAlerts = config.AutoResolveIncidentAlerts
	}

	// Heartbeat sources generate their own title and description. ValidateConfig rejects
	// setting either, so drop what the API generated rather than showing a permanent diff
	// against a config that can't hold it.
	if source.SourceType == "heartbeat" {
		model.Title = nil
		model.Description = nil
	}

	return model
}
