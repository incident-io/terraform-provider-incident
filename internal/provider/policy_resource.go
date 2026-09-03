package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ resource.Resource                     = &incidentPolicyResource{}
	_ resource.ResourceWithConfigure        = &incidentPolicyResource{}
	_ resource.ResourceWithImportState      = &incidentPolicyResource{}
	_ resource.ResourceWithConfigValidators = &incidentPolicyResource{}
)

func NewIncidentPolicyResource() resource.Resource {
	return &incidentPolicyResource{}
}

type incidentPolicyResource struct {
	resourceConfigurer
}

// incidentPolicyResourceModel is one policy. Exactly one config block is set, and which
// one determines the policy type: policy_type is computed from it rather than written, so
// a config cannot contradict itself.
type incidentPolicyResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
	PolicyType  types.String `tfsdk:"policy_type"`

	ConditionGroups models.IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	AssignmentRules *incidentPolicyAssignmentRules       `tfsdk:"assignment_rules"`
	Expressions     models.IncidentEngineExpressions     `tfsdk:"expressions"`

	// follow_up, debrief and post_mortem are the same shape: each states the
	// requirements its resource must satisfy and when it falls due.
	FollowUp   *incidentPolicyIncidentConfig `tfsdk:"follow_up"`
	Debrief    *incidentPolicyIncidentConfig `tfsdk:"debrief"`
	PostMortem *incidentPolicyIncidentConfig `tfsdk:"post_mortem"`

	Schedule        *incidentPolicySchedule        `tfsdk:"schedule"`
	OnCallReadiness *incidentPolicyOnCallReadiness `tfsdk:"on_call_readiness"`

	// VacationConflict carries no fields: the type has nothing to configure. It
	// exists so that "exactly one block" holds for all six types, which is what lets
	// the type be derived rather than written.
	VacationConflict *incidentPolicyVacationConflict `tfsdk:"vacation_conflict"`
}

type incidentPolicyVacationConflict struct{}

// policyType returns the type the config's block implies. Exactly one is set, which
// ExactlyOneOf enforces before this runs.
func (model *incidentPolicyResourceModel) policyType() string {
	switch {
	case model.FollowUp != nil:
		return "follow_up"
	case model.Debrief != nil:
		return "debrief"
	case model.PostMortem != nil:
		return "post_mortem"
	case model.Schedule != nil:
		return "schedule"
	case model.OnCallReadiness != nil:
		return "on_call_readiness"
	case model.VacationConflict != nil:
		return "vacation_conflict"
	}

	return ""
}

type incidentPolicyAssignmentRules struct {
	Bindings                        models.IncidentEngineParamBindings `tfsdk:"bindings"`
	ReminderDueDateOffsetHours      []types.Int64                      `tfsdk:"reminder_due_date_offset_hours"`
	ReminderDetectedDateOffsetHours []types.Int64                      `tfsdk:"reminder_detected_date_offset_hours"`
	ReminderCadenceBefore           *incidentPolicyReminderCadence     `tfsdk:"reminder_cadence_before"`
	ReminderCadenceAfter            *incidentPolicyReminderCadence     `tfsdk:"reminder_cadence_after"`
}

// incidentPolicyReminderCadence is a recurring reminder, which repeats once per interval
// until the violation is resolved. Direction is which field holds it rather than a value on
// it, the way the sign of a one-off offset says before or after.
type incidentPolicyReminderCadence struct {
	Interval types.String `tfsdk:"interval"`
}

type incidentPolicyIncidentConfig struct {
	Requirements          models.IncidentEngineConditionGroups `tfsdk:"requirements"`
	DueDateConfig         *incidentPolicyDueDateConfig         `tfsdk:"due_date_config"`
	RunOnPrivateIncidents types.Bool                           `tfsdk:"run_on_private_incidents"`
}

type incidentPolicyDueDateConfig struct {
	IncidentTimestampID types.String                      `tfsdk:"incident_timestamp_id"`
	Days                models.IncidentEngineParamBinding `tfsdk:"days"`
	CalculationType     types.String                      `tfsdk:"calculation_type"`
	CalculationTimezone types.String                      `tfsdk:"calculation_timezone"`
	AppliesFrom         timetypes.RFC3339                 `tfsdk:"applies_from"`
}

type incidentPolicySchedule struct {
	RequirementType types.String `tfsdk:"requirement_type"`
	EvaluationLevel types.String `tfsdk:"evaluation_level"`
}

type incidentPolicyOnCallReadiness struct {
	HighUrgency []incidentPolicyReadinessRule `tfsdk:"high_urgency"`
	LowUrgency  []incidentPolicyReadinessRule `tfsdk:"low_urgency"`
	Enforcement types.String                  `tfsdk:"enforcement"`
}

type incidentPolicyReadinessRule struct {
	MethodTypes     []types.String `tfsdk:"method_types"`
	MaxDelaySeconds types.Int64    `tfsdk:"max_delay_seconds"`
}

// policyBlocks are the config blocks, exactly one of which every policy sets.
var policyBlocks = []string{
	"follow_up", "debrief", "post_mortem", "schedule", "on_call_readiness", "vacation_conflict",
}

// policyTypesWithForcedAssignee are the types that assign the user in violation. The API
// picks their assignee itself and replaces whatever a request sends, so a config cannot set
// one and a read must drop what comes back.
var policyTypesWithForcedAssignee = []string{"on_call_readiness", "vacation_conflict"}

func (r *incidentPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (r *incidentPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	expressions := models.ExpressionsAttribute()
	expressions.Required = false
	expressions.Optional = true

	resp.Schema = schema.Schema{
		MarkdownDescription: `Manage the policies that encode how your organisation should handle incidents.

A policy scopes itself to a set of resources with ` + "`condition_groups`" + `, states the
requirements those resources must meet, and describes who to chase when they fall short.

` + "`policy_type`" + ` selects exactly one matching config block: a ` + "`follow_up`" + ` policy
carries ` + "`follow_up`" + ` config, a ` + "`schedule`" + ` policy carries ` + "`schedule`" + `
config, and so on. A ` + "`vacation_conflict`" + ` policy has no configuration of its own and
so carries no block.
`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PolicyV2", "id"),
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PolicyV2", "name"),
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("PolicyV2", "description"),
				Required:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: apischema.EnumValuesDescription("PolicyV2", "status"),
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(string(client.PolicyV2StatusEnabled)),
			},
			// Derived from whichever config block is set, so a config cannot claim one
			// type and configure another. Readable for outputs and references.
			"policy_type": schema.StringAttribute{
				MarkdownDescription: apischema.EnumValuesDescription("PolicyV2", "policy_type") +
					" Determined by which config block is set.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"condition_groups": models.ConditionGroupsAttribute(),
			"expressions":      expressions,

			"assignment_rules": schema.SingleNestedAttribute{
				MarkdownDescription: "Who to assign a violation to, and when to remind them. Omit it for a policy type that assigns the user in violation.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"bindings": schema.ListNestedAttribute{
						MarkdownDescription: apischema.Docstring("PolicyAssignmentRulesV2", "bindings"),
						Required:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: models.ParamBindingAttributes(),
						},
					},
					"reminder_due_date_offset_hours": schema.ListAttribute{
						MarkdownDescription: apischema.Docstring("PolicyAssignmentRulesV2", "reminder_due_date_offset_hours"),
						ElementType:         types.Int64Type,
						Required:            true,
					},
					"reminder_detected_date_offset_hours": schema.ListAttribute{
						MarkdownDescription: apischema.Docstring("PolicyAssignmentRulesV2", "reminder_detected_date_offset_hours"),
						ElementType:         types.Int64Type,
						Optional:            true,
					},
					"reminder_cadence_before": policyReminderCadenceAttribute("reminder_cadence_before"),
					"reminder_cadence_after":  policyReminderCadenceAttribute("reminder_cadence_after"),
				},
			},

			"follow_up":   policyIncidentConfigAttribute("follow_up", "PolicyFollowUpV2"),
			"debrief":     policyIncidentConfigAttribute("debrief", "PolicyDebriefV2"),
			"post_mortem": policyIncidentConfigAttribute("post_mortem", "PolicyPostMortemV2"),

			"schedule": schema.SingleNestedAttribute{
				MarkdownDescription: "Makes this a schedule policy, which detects gaps in on-call coverage.",
				Optional:            true,
				PlanModifiers:       []planmodifier.Object{policyBlockRequiresReplace()},
				Attributes: map[string]schema.Attribute{
					"requirement_type": schema.StringAttribute{
						MarkdownDescription: apischema.DescribeEnumValues(
							"The kind of schedule requirement to check",
							"PolicyScheduleV2", "requirement_type"),
						Required: true,
					},
					"evaluation_level": schema.StringAttribute{
						MarkdownDescription: apischema.EnumValuesDescription("PolicyScheduleV2", "evaluation_level"),
						Optional:            true,
						Computed:            true,
					},
				},
			},

			"on_call_readiness": schema.SingleNestedAttribute{
				MarkdownDescription: "Makes this an on-call readiness policy, which checks that users have suitable notification methods. The assignee is always the user in violation, so `assignment_rules` cannot be set alongside it.",
				Optional:            true,
				PlanModifiers:       []planmodifier.Object{policyBlockRequiresReplace()},
				Attributes: map[string]schema.Attribute{
					"high_urgency": policyReadinessRulesAttribute("high urgency"),
					"low_urgency":  policyReadinessRulesAttribute("low urgency"),
					"enforcement": schema.StringAttribute{
						MarkdownDescription: apischema.EnumValuesDescription("PolicyOnCallReadinessV2", "enforcement"),
						Optional:            true,
						Computed:            true,
					},
				},
			},

			// Empty because the type has nothing to configure. Set it to `{}` to make a
			// vacation-conflict policy, which keeps "exactly one block" true for every type.
			"vacation_conflict": schema.SingleNestedAttribute{
				MarkdownDescription: "Makes this a vacation-conflict policy, which flags responders rota'd on while they are away. It takes no configuration, so set it to an empty object. The assignee is always the user in violation, so `assignment_rules` cannot be set alongside it.",
				Optional:            true,
				PlanModifiers:       []planmodifier.Object{policyBlockRequiresReplace()},
				Attributes:          map[string]schema.Attribute{},
			},
		},
	}
}

// ConfigValidators enforces that a policy sets exactly one config block, which is what
// determines its type, and that a type picking its own assignee carries none.
func (r *incidentPolicyResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	blocks := lo.Map(policyBlocks, func(block string, _ int) path.Expression {
		return path.MatchRoot(block)
	})

	validators := []resource.ConfigValidator{resourcevalidator.ExactlyOneOf(blocks...)}

	// These types pick their own assignee, so a config setting one would be silently
	// replaced. Say so at plan time instead.
	for _, block := range policyTypesWithForcedAssignee {
		validators = append(validators, resourcevalidator.Conflicting(
			path.MatchRoot(block),
			path.MatchRoot("assignment_rules"),
		))
	}

	return validators
}

// policyBlockRequiresReplace replaces the policy when a block is added or removed, which
// is the only way its type can change. The API refuses to change the type of an existing
// policy. Editing a block in place stays an ordinary update.
func policyBlockRequiresReplace() planmodifier.Object {
	description := "Adding or removing this block changes the policy type, which requires a new policy."

	return objectplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.ObjectRequest, resp *objectplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = req.StateValue.IsNull() != req.ConfigValue.IsNull()
		},
		description, description,
	)
}

func policyIncidentConfigAttribute(name, definition string) schema.SingleNestedAttribute {
	// The API rejects an empty requirements list: a policy that requires nothing would
	// never find a resource non-compliant. Saying so here fails the plan rather than the
	// apply.
	requirements := models.ConditionGroupsAttribute()
	requirements.Validators = []validator.List{listvalidator.SizeAtLeast(1)}

	return schema.SingleNestedAttribute{
		MarkdownDescription: fmt.Sprintf("Makes this a %s policy, stating what a %s must satisfy and when it falls due.", name, name),
		Optional:            true,
		PlanModifiers:       []planmodifier.Object{policyBlockRequiresReplace()},
		Attributes: map[string]schema.Attribute{
			"requirements": requirements,
			// Required, because these are the three types that carry a due date and the
			// API rejects an update to one without it. Leaving it optional let a config
			// create a policy that then failed on its next apply, which is a worse place
			// to find out.
			"due_date_config": schema.SingleNestedAttribute{
				MarkdownDescription: apischema.Docstring(definition, "due_date_config"),
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"incident_timestamp_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("PolicyDueDateConfigV2", "incident_timestamp_id"),
						Required:            true,
					},
					"days": schema.SingleNestedAttribute{
						MarkdownDescription: apischema.Docstring("PolicyDueDateConfigV2", "days"),
						Required:            true,
						Attributes:          models.ParamBindingAttributes(),
					},
					"calculation_type": schema.StringAttribute{
						MarkdownDescription: apischema.DescribeEnumValues(
							"Whether to count all days or only weekdays",
							"PolicyDueDateConfigV2", "calculation_type"),
						Required: true,
					},
					"calculation_timezone": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("PolicyDueDateConfigV2", "calculation_timezone"),
						Optional:            true,
					},
					"applies_from": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("PolicyDueDateConfigV2", "applies_from"),
						CustomType:          timetypes.RFC3339Type{},
						Optional:            true,
					},
				},
			},
			"run_on_private_incidents": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring(definition, "run_on_private_incidents"),
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func policyReminderCadenceAttribute(field string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: apischema.Docstring("PolicyAssignmentRulesV2", field),
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"interval": schema.StringAttribute{
				MarkdownDescription: apischema.EnumValuesDescription("PolicyReminderCadenceV2", "interval"),
				Required:            true,
			},
		},
	}
}

func policyReadinessRulesAttribute(urgency string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: fmt.Sprintf("Rules that must be satisfied for %s notifications", urgency),
		Optional:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"method_types": schema.ListAttribute{
					MarkdownDescription: apischema.DescribeEnumValues(
						"The notification methods that satisfy this rule",
						"PolicyReadinessRuleV2", "method_types"),
					ElementType: types.StringType,
					Required:    true,
				},
				"max_delay_seconds": schema.Int64Attribute{
					MarkdownDescription: apischema.Docstring("PolicyReadinessRuleV2", "max_delay_seconds"),
					Optional:            true,
					Computed:            true,
				},
			},
		},
	}
}

func (r *incidentPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *incidentPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.PoliciesV2CreateWithResponse(ctx, client.PoliciesV2CreateJSONRequestBody{
		Name:            data.Name.ValueString(),
		Description:     data.Description.ValueString(),
		Status:          (*client.PoliciesCreatePayloadV2Status)(data.Status.ValueStringPointer()),
		PolicyType:      client.PoliciesCreatePayloadV2PolicyType(data.policyType()),
		Conditions:      data.ConditionGroups.ToPayload(),
		Expressions:     policyExpressionsPayload(data.Expressions),
		AssignmentRules: data.AssignmentRules.toPayload(),
		FollowUp:        data.FollowUp.toFollowUpPayload(),
		Debrief:         data.Debrief.toDebriefPayload(),
		PostMortem:      data.PostMortem.toPostMortemPayload(),
		Schedule:        data.Schedule.toPayload(),
		OnCallReadiness: data.OnCallReadiness.toPayload(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create policy, got error: %s", err))
		return
	}
	if result.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to create policy: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, result.JSON201.Policy.Id, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypePolicy, r.terraformVersion)

	tflog.Trace(ctx, fmt.Sprintf("created a policy resource with id=%s", result.JSON201.Policy.Id))
	resp.Diagnostics.Append(resp.State.Set(ctx, policyFromAPI(result.JSON201.Policy, data))...)
}

func (r *incidentPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *incidentPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.PoliciesV2ShowWithResponse(ctx, data.ID.ValueString())
	if isNotFound(err) {
		tflog.Warn(ctx, fmt.Sprintf("Policy with ID %s not found: removing from state.", data.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read policy, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read policy: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, policyFromAPI(result.JSON200.Policy, data))...)
}

func (r *incidentPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *incidentPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.PoliciesV2UpdateWithResponse(ctx, data.ID.ValueString(), client.PoliciesV2UpdateJSONRequestBody{
		Name:            data.Name.ValueString(),
		Description:     data.Description.ValueString(),
		Status:          (*client.PoliciesUpdatePayloadV2Status)(data.Status.ValueStringPointer()),
		PolicyType:      client.PoliciesUpdatePayloadV2PolicyType(data.policyType()),
		Conditions:      data.ConditionGroups.ToPayload(),
		Expressions:     policyExpressionsPayload(data.Expressions),
		AssignmentRules: data.AssignmentRules.toPayload(),
		FollowUp:        data.FollowUp.toFollowUpPayload(),
		Debrief:         data.Debrief.toDebriefPayload(),
		PostMortem:      data.PostMortem.toPostMortemPayload(),
		Schedule:        data.Schedule.toPayload(),
		OnCallReadiness: data.OnCallReadiness.toPayload(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update policy, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to update policy: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	claimResource(ctx, r.client, data.ID.ValueString(), &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypePolicy, r.terraformVersion)

	resp.Diagnostics.Append(resp.State.Set(ctx, policyFromAPI(result.JSON200.Policy, data))...)
}

func (r *incidentPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *incidentPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.PoliciesV2DeleteWithResponse(ctx, data.ID.ValueString())
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete policy, got error: %s", err))
		return
	}
}

func (r *incidentPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	claimResourceOnImport(ctx, r.client, req.ID, &resp.Diagnostics,
		client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypePolicy, r.terraformVersion,
		r.markImportedAsManaged)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// policyFromAPI converts the API response into the model. prior is the plan
// (create/update) or the prior state (read), and nil on import. It lets conditions and
// bindings keep the spelling the config used, rather than the long form the API answers
// with, which Terraform would otherwise read as a change.
func policyFromAPI(policy client.PolicyV2, prior *incidentPolicyResourceModel) *incidentPolicyResourceModel {
	model := &incidentPolicyResourceModel{
		ID:              types.StringValue(policy.Id),
		Name:            types.StringValue(policy.Name),
		Description:     types.StringPointerValue(policy.Description),
		Status:          types.StringValue(string(policy.Status)),
		PolicyType:      types.StringValue(string(policy.PolicyType)),
		ConditionGroups: models.IncidentEngineConditionGroups{}.FromAPI(policy.Conditions),
	}

	if policy.Expressions != nil {
		model.Expressions = models.IncidentEngineExpressions{}.FromAPI(*policy.Expressions)
	}
	if policy.AssignmentRules != nil {
		model.AssignmentRules = assignmentRulesFromAPI(*policy.AssignmentRules)
	}
	if policy.FollowUp != nil {
		model.FollowUp = incidentConfigFromAPI(
			policy.FollowUp.Requirements, policy.FollowUp.DueDateConfig, policy.FollowUp.RunOnPrivateIncidents)
	}
	if policy.Debrief != nil {
		model.Debrief = incidentConfigFromAPI(
			policy.Debrief.Requirements, policy.Debrief.DueDateConfig, policy.Debrief.RunOnPrivateIncidents)
	}
	if policy.PostMortem != nil {
		model.PostMortem = incidentConfigFromAPI(
			policy.PostMortem.Requirements, policy.PostMortem.DueDateConfig, policy.PostMortem.RunOnPrivateIncidents)
	}
	if policy.Schedule != nil {
		model.Schedule = &incidentPolicySchedule{
			RequirementType: types.StringValue(string(policy.Schedule.RequirementType)),
			EvaluationLevel: types.StringNull(),
		}
		if policy.Schedule.EvaluationLevel != nil {
			model.Schedule.EvaluationLevel = types.StringValue(string(*policy.Schedule.EvaluationLevel))
		}
	}
	if policy.OnCallReadiness != nil {
		model.OnCallReadiness = onCallReadinessFromAPI(*policy.OnCallReadiness)
	}

	// The API sends no block for this type, having nothing to put in one, so the marker
	// has to come from policy_type. Without it a read leaves every block null and the
	// next plan sees a config that sets one.
	if string(policy.PolicyType) == "vacation_conflict" {
		model.VacationConflict = &incidentPolicyVacationConflict{}
	}

	// Drop the assignee the API picked for itself. This keys off the type so that it holds
	// on import too, where there is no prior to compare against.
	if lo.Contains(policyTypesWithForcedAssignee, string(policy.PolicyType)) {
		model.AssignmentRules = nil
	}

	// An import has no configuration to reconcile against, so the API's answer stands as
	// it comes. The first apply after an import settles any difference.
	if prior != nil && !prior.isImport() {
		model.reconcileWith(prior)
	}

	return model
}

// isImport reports whether this is the state a read sees during an import, which carries
// the ID and nothing else. Every other read has a policy_type: it is Computed, so it is
// set in state once a policy exists and unknown (not null) in a create plan.
func (model *incidentPolicyResourceModel) isImport() bool {
	return model.PolicyType.IsNull()
}

// reconcileWith puts back the shorthands the config wrote. A read only ever sees the
// stored form, so without this a config written as value_literal reads back as value and
// fails the apply as an inconsistent result.
func (model *incidentPolicyResourceModel) reconcileWith(prior *incidentPolicyResourceModel) {
	model.ConditionGroups.ReconcileSpelling(prior.ConditionGroups)
	model.Expressions.ReconcileSpelling(prior.Expressions)

	switch {
	// A safety net for any other rules the API adds of its own accord: keeping them would
	// put rules in state the config never asked for and fail the apply as an inconsistent
	// result. The forced assignees above are dropped before this, so that they go on an
	// import too.
	case prior.AssignmentRules == nil:
		model.AssignmentRules = nil

	// Assignee bindings go through the scalar variant: the API binds them against an
	// array param, so it stores a scalar as a one-element array and answers with the
	// array. The due-date days binding below is scalar on both sides, so it doesn't.
	case model.AssignmentRules != nil:
		model.AssignmentRules.Bindings = model.AssignmentRules.Bindings.
			ReconcileScalarSpelling(prior.AssignmentRules.Bindings)
	}

	for _, pair := range []struct{ applied, prior *incidentPolicyIncidentConfig }{
		{model.FollowUp, prior.FollowUp},
		{model.Debrief, prior.Debrief},
		{model.PostMortem, prior.PostMortem},
	} {
		if pair.applied == nil || pair.prior == nil {
			continue
		}

		pair.applied.Requirements.ReconcileSpelling(pair.prior.Requirements)
		if pair.applied.DueDateConfig != nil && pair.prior.DueDateConfig != nil {
			pair.applied.DueDateConfig.Days = pair.applied.DueDateConfig.Days.
				ReconcileSpelling(pair.prior.DueDateConfig.Days)
		}
	}
}

func assignmentRulesFromAPI(rules client.PolicyAssignmentRulesV2) *incidentPolicyAssignmentRules {
	out := &incidentPolicyAssignmentRules{
		Bindings: models.IncidentEngineParamBindings{}.FromAPI(rules.Bindings),
		ReminderDueDateOffsetHours: lo.Map(rules.ReminderDueDateOffsetHours,
			func(hours int64, _ int) types.Int64 { return types.Int64Value(hours) }),
	}
	if rules.ReminderDetectedDateOffsetHours != nil {
		out.ReminderDetectedDateOffsetHours = lo.Map(*rules.ReminderDetectedDateOffsetHours,
			func(hours int64, _ int) types.Int64 { return types.Int64Value(hours) })
	}
	out.ReminderCadenceBefore = reminderCadenceFromAPI(rules.ReminderCadenceBefore)
	out.ReminderCadenceAfter = reminderCadenceFromAPI(rules.ReminderCadenceAfter)

	return out
}

func reminderCadenceFromAPI(cadence *client.PolicyReminderCadenceV2) *incidentPolicyReminderCadence {
	if cadence == nil {
		return nil
	}

	return &incidentPolicyReminderCadence{Interval: types.StringValue(string(cadence.Interval))}
}

func (cadence *incidentPolicyReminderCadence) toPayload() *client.PolicyReminderCadenceV2 {
	if cadence == nil {
		return nil
	}

	return &client.PolicyReminderCadenceV2{
		Interval: client.PolicyReminderCadenceV2Interval(cadence.Interval.ValueString()),
	}
}

func incidentConfigFromAPI(
	requirements []client.ConditionGroupV2, dueDate *client.PolicyDueDateConfigV2, runOnPrivate *bool,
) *incidentPolicyIncidentConfig {
	out := &incidentPolicyIncidentConfig{
		Requirements:          models.IncidentEngineConditionGroups{}.FromAPI(requirements),
		RunOnPrivateIncidents: types.BoolPointerValue(runOnPrivate),
	}

	if dueDate != nil {
		out.DueDateConfig = &incidentPolicyDueDateConfig{
			IncidentTimestampID: types.StringValue(dueDate.IncidentTimestampId),
			Days:                models.IncidentEngineParamBinding{}.FromAPI(dueDate.Days),
			CalculationType:     types.StringValue(string(dueDate.CalculationType)),
			CalculationTimezone: types.StringPointerValue(dueDate.CalculationTimezone),
			AppliesFrom:         timetypes.NewRFC3339Null(),
		}
		if dueDate.AppliesFrom != nil {
			out.DueDateConfig.AppliesFrom = timetypes.NewRFC3339TimeValue(*dueDate.AppliesFrom)
		}
	}

	return out
}

func onCallReadinessFromAPI(readiness client.PolicyOnCallReadinessV2) *incidentPolicyOnCallReadiness {
	out := &incidentPolicyOnCallReadiness{
		Enforcement: types.StringNull(),
	}
	if readiness.Enforcement != nil {
		out.Enforcement = types.StringValue(string(*readiness.Enforcement))
	}
	if readiness.HighUrgency != nil {
		out.HighUrgency = readinessRulesFromAPI(*readiness.HighUrgency)
	}
	if readiness.LowUrgency != nil {
		out.LowUrgency = readinessRulesFromAPI(*readiness.LowUrgency)
	}

	return out
}

func readinessRulesFromAPI(rules []client.PolicyReadinessRuleV2) []incidentPolicyReadinessRule {
	return lo.Map(rules, func(rule client.PolicyReadinessRuleV2, _ int) incidentPolicyReadinessRule {
		return incidentPolicyReadinessRule{
			MethodTypes: lo.Map(rule.MethodTypes, func(method client.PolicyReadinessRuleV2MethodTypes, _ int) types.String {
				return types.StringValue(string(method))
			}),
			MaxDelaySeconds: types.Int64PointerValue(rule.MaxDelaySeconds),
		}
	})
}

func policyExpressionsPayload(expressions models.IncidentEngineExpressions) *[]client.ExpressionPayloadV2 {
	if len(expressions) == 0 {
		return nil
	}

	return lo.ToPtr(expressions.ToPayload())
}

func (rules *incidentPolicyAssignmentRules) toPayload() *client.PolicyAssignmentRulesPayloadV2 {
	if rules == nil {
		return nil
	}

	out := &client.PolicyAssignmentRulesPayloadV2{
		Bindings: rules.Bindings.ToPayload(),
		ReminderDueDateOffsetHours: lo.Map(rules.ReminderDueDateOffsetHours,
			func(hours types.Int64, _ int) int64 { return hours.ValueInt64() }),
	}
	if len(rules.ReminderDetectedDateOffsetHours) > 0 {
		out.ReminderDetectedDateOffsetHours = lo.ToPtr(lo.Map(rules.ReminderDetectedDateOffsetHours,
			func(hours types.Int64, _ int) int64 { return hours.ValueInt64() }))
	}
	out.ReminderCadenceBefore = rules.ReminderCadenceBefore.toPayload()
	out.ReminderCadenceAfter = rules.ReminderCadenceAfter.toPayload()

	return out
}

func (config *incidentPolicyIncidentConfig) toFollowUpPayload() *client.PolicyFollowUpPayloadV2 {
	if config == nil {
		return nil
	}

	return &client.PolicyFollowUpPayloadV2{
		Requirements:          config.Requirements.ToPayload(),
		DueDateConfig:         config.DueDateConfig.toPayload(),
		RunOnPrivateIncidents: config.RunOnPrivateIncidents.ValueBoolPointer(),
	}
}

func (config *incidentPolicyIncidentConfig) toDebriefPayload() *client.PolicyDebriefPayloadV2 {
	if config == nil {
		return nil
	}

	return &client.PolicyDebriefPayloadV2{
		Requirements:          config.Requirements.ToPayload(),
		DueDateConfig:         config.DueDateConfig.toPayload(),
		RunOnPrivateIncidents: config.RunOnPrivateIncidents.ValueBoolPointer(),
	}
}

func (config *incidentPolicyIncidentConfig) toPostMortemPayload() *client.PolicyPostMortemPayloadV2 {
	if config == nil {
		return nil
	}

	return &client.PolicyPostMortemPayloadV2{
		Requirements:          config.Requirements.ToPayload(),
		DueDateConfig:         config.DueDateConfig.toPayload(),
		RunOnPrivateIncidents: config.RunOnPrivateIncidents.ValueBoolPointer(),
	}
}

func (config *incidentPolicyDueDateConfig) toPayload() *client.PolicyDueDateConfigPayloadV2 {
	if config == nil {
		return nil
	}

	out := &client.PolicyDueDateConfigPayloadV2{
		IncidentTimestampId: config.IncidentTimestampID.ValueString(),
		Days:                config.Days.ToPayload(),
		CalculationType:     client.PolicyDueDateConfigPayloadV2CalculationType(config.CalculationType.ValueString()),
		CalculationTimezone: config.CalculationTimezone.ValueStringPointer(),
	}

	if !config.AppliesFrom.IsNull() && !config.AppliesFrom.IsUnknown() {
		appliesFrom, diags := config.AppliesFrom.ValueRFC3339Time()
		if !diags.HasError() {
			out.AppliesFrom = lo.ToPtr(appliesFrom)
		}
	}

	return out
}

func (schedule *incidentPolicySchedule) toPayload() *client.PolicyScheduleV2 {
	if schedule == nil {
		return nil
	}

	out := &client.PolicyScheduleV2{
		RequirementType: client.PolicyScheduleV2RequirementType(schedule.RequirementType.ValueString()),
	}
	if !schedule.EvaluationLevel.IsNull() && !schedule.EvaluationLevel.IsUnknown() {
		out.EvaluationLevel = lo.ToPtr(client.PolicyScheduleV2EvaluationLevel(schedule.EvaluationLevel.ValueString()))
	}

	return out
}

func (readiness *incidentPolicyOnCallReadiness) toPayload() *client.PolicyOnCallReadinessV2 {
	if readiness == nil {
		return nil
	}

	out := &client.PolicyOnCallReadinessV2{}
	if !readiness.Enforcement.IsNull() && !readiness.Enforcement.IsUnknown() {
		out.Enforcement = lo.ToPtr(client.PolicyOnCallReadinessV2Enforcement(readiness.Enforcement.ValueString()))
	}
	if len(readiness.HighUrgency) > 0 {
		out.HighUrgency = lo.ToPtr(readinessRulesPayload(readiness.HighUrgency))
	}
	if len(readiness.LowUrgency) > 0 {
		out.LowUrgency = lo.ToPtr(readinessRulesPayload(readiness.LowUrgency))
	}

	return out
}

func readinessRulesPayload(rules []incidentPolicyReadinessRule) []client.PolicyReadinessRuleV2 {
	return lo.Map(rules, func(rule incidentPolicyReadinessRule, _ int) client.PolicyReadinessRuleV2 {
		return client.PolicyReadinessRuleV2{
			MethodTypes: lo.Map(rule.MethodTypes, func(method types.String, _ int) client.PolicyReadinessRuleV2MethodTypes {
				return client.PolicyReadinessRuleV2MethodTypes(method.ValueString())
			}),
			MaxDelaySeconds: rule.MaxDelaySeconds.ValueInt64Pointer(),
		}
	})
}
