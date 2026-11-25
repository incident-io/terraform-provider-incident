package models

import (
	"fmt"
	"math"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// The actions an alert rule branch takes. The API reads back what it was sent, so each
// binding goes through ReconcileBinding to keep the config's spelling, and every setting the
// server defaults has the same default here, so leaving one out never plans a change.

type AlertRuleActionV3Model struct {
	ID                 types.String                          `tfsdk:"id"`
	Type               types.String                          `tfsdk:"type"`
	Escalate           *AlertRuleEscalateActionV3Model       `tfsdk:"escalate"`
	PostSlack          *AlertRulePostActionV3Model           `tfsdk:"post_slack"`
	PostMicrosoftTeams *AlertRulePostActionV3Model           `tfsdk:"post_microsoft_teams"`
	CreateIncident     *AlertRuleCreateIncidentActionV3Model `tfsdk:"create_incident"`
}

type AlertRuleEscalateActionV3Model struct {
	EscalationPaths    *Binding     `tfsdk:"escalation_paths"`
	Users              *Binding     `tfsdk:"users"`
	Urgency            *Binding     `tfsdk:"urgency"`
	Mode               types.String `tfsdk:"mode"`
	GracePeriodSeconds types.Int64  `tfsdk:"grace_period_seconds"`
	AutoCancel         types.Bool   `tfsdk:"auto_cancel"`
}

type AlertRulePostActionV3Model struct {
	Channels          *Binding     `tfsdk:"channels"`
	Mode              types.String `tfsdk:"mode"`
	ChannelVisibility types.String `tfsdk:"channel_visibility"`
}

type AlertRuleCreateIncidentActionV3Model struct {
	IncidentTemplate *Binding   `tfsdk:"incident_template"`
	MembershipTeams  *Binding   `tfsdk:"membership_teams"`
	AutoDecline      types.Bool `tfsdk:"auto_decline"`
	IsPrivate        types.Bool `tfsdk:"is_private"`
}

// Schema

func AlertRuleActionsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional:            true,
		MarkdownDescription: apischema.Docstring("AlertRuleBranchV3", "actions"),
		NestedObject: schema.NestedAttributeObject{
			Attributes: AlertRuleActionAttributes(),
		},
	}
}

// AlertRuleActionAttributes is one action, for a resource that holds actions as blocks rather
// than through AlertRuleActionsAttribute.
func AlertRuleActionAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Optional: true,
			MarkdownDescription: "Identifier for this action, unique within the set. Leave it out and the server " +
				"assigns a new one on every apply, which Terraform doesn't store.",
			Validators: []validator.String{stringvalidator.LengthBetween(1, 128)},
		},
		"type": schema.StringAttribute{
			Required: true,
			MarkdownDescription: apischema.DescribeEnumValues(
				"Which action to take. Set the attribute of the same name with its parameters. `investigate` has none.",
				"AlertRuleActionV3", "type"),
			Validators: []validator.String{
				stringvalidator.OneOf(apischema.EnumValues("AlertRuleActionV3", "type")...),
			},
		},
		"escalate": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("AlertRuleActionV3", "escalate") + " Set when type is `escalate`.",
			Attributes: map[string]schema.Attribute{
				"escalation_paths": BindingAttribute("The escalation paths to page: escalation path IDs in `values`, or an `expression_ref` that finds them, such as from the alert's team."),
				"users":            BindingAttribute("The users to page: user IDs in `values`, or a reference."),
				"urgency":          BindingAttribute("The urgency to page at: the ID of an escalation urgency catalog entry, or a reference. Leave it out to page at the alert priority's default urgency."),
				"mode": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString(string(client.AlertRuleEscalateActionV3ModeOnEachNewAlert)),
					MarkdownDescription: apischema.EnumValuesDescription("AlertRuleEscalateActionV3", "mode"),
					Validators: []validator.String{
						stringvalidator.OneOf(apischema.EnumValues("AlertRuleEscalateActionV3", "mode")...),
					},
				},
				"grace_period_seconds": schema.Int64Attribute{
					Optional: true,
					Computed: true,
					Default:  int64default.StaticInt64(0),
					// Allowed with any mode: the API keeps it, and rules converted from alert routes
					// before November 2026 have one with on_priority_increase.
					MarkdownDescription: apischema.Docstring("AlertRuleEscalateActionV3", "grace_period_seconds") +
						" Allowed with on_priority_increase, which ignores it.",
					Validators: []validator.Int64{int64validator.Between(0, math.MaxInt32)},
				},
				"auto_cancel": schema.BoolAttribute{
					Optional:            true,
					Computed:            true,
					Default:             booldefault.StaticBool(false),
					MarkdownDescription: apischema.Docstring("AlertRuleEscalateActionV3", "auto_cancel"),
				},
			},
		},
		"post_slack":           alertRulePostAttribute("post_slack", "Slack"),
		"post_microsoft_teams": alertRulePostAttribute("post_microsoft_teams", "Microsoft Teams"),
		"create_incident": schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: apischema.Docstring("AlertRuleActionV3", "create_incident") + ". Set when type is `create_incident`.",
			Attributes: map[string]schema.Attribute{
				"incident_template": BindingAttribute("The incident template to create from: its ID, or a reference. Leave it out to use the organisation's default template."),
				"membership_teams":  BindingAttribute("The teams whose members can see the incident and join its channel when it is private: team catalog entry IDs in `values`, or a reference. Leave it out to use the template's teams."),
				"auto_decline": schema.BoolAttribute{
					Optional:            true,
					Computed:            true,
					Default:             booldefault.StaticBool(false),
					MarkdownDescription: apischema.Docstring("AlertRuleCreateIncidentActionV3", "auto_decline"),
				},
				"is_private": schema.BoolAttribute{
					Optional:            true,
					Computed:            true,
					Default:             booldefault.StaticBool(false),
					MarkdownDescription: apischema.Docstring("AlertRuleCreateIncidentActionV3", "is_private"),
				},
			},
		},
	}
}

func alertRulePostAttribute(actionType, product string) schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: fmt.Sprintf("Post the alert to %s channels. Set when type is `%s`.", product, actionType),
		Attributes: map[string]schema.Attribute{
			"channels": schema.SingleNestedAttribute{
				Required:    true,
				Description: "The channels to post to: channel IDs in `values`, or a reference.",
				Attributes:  BindingAttributes(),
			},
			"mode": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.EnumValuesDescription("AlertRulePostActionV3", "mode"),
				Validators: []validator.String{
					stringvalidator.OneOf(apischema.EnumValues("AlertRulePostActionV3", "mode")...),
				},
			},
			"channel_visibility": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(string(client.AlertRulePostActionV3ChannelVisibilityPublic)),
				MarkdownDescription: apischema.EnumValuesDescription("AlertRulePostActionV3", "channel_visibility"),
				Validators: []validator.String{
					stringvalidator.OneOf(apischema.EnumValues("AlertRulePostActionV3", "channel_visibility")...),
				},
			},
		},
	}
}

// Validation

// ValidateAlertRuleActions runs the checks the schema can't express, for a resource's
// ValidateConfig. known is the set of expression names an expression_ref may use.
func ValidateAlertRuleActions(actions []AlertRuleActionV3Model, at path.Path, known map[string]bool, diags *diag.Diagnostics) {
	for idx, action := range actions {
		validateAlertRuleAction(action, at.AtListIndex(idx), known, diags)
	}
}

func validateAlertRuleAction(action AlertRuleActionV3Model, at path.Path, known map[string]bool, diags *diag.Diagnostics) {
	// The server refuses an object that doesn't match the type, rather than drop its settings.
	if !action.Type.IsUnknown() {
		actionType := action.Type.ValueString()
		for _, object := range []struct {
			name string
			set  bool
		}{
			{string(client.Escalate), action.Escalate != nil},
			{string(client.PostSlack), action.PostSlack != nil},
			{string(client.PostMicrosoftTeams), action.PostMicrosoftTeams != nil},
			{string(client.CreateIncident), action.CreateIncident != nil},
		} {
			switch {
			case object.name == actionType && !object.set:
				diags.AddAttributeError(at.AtName(object.name), "Missing action parameters",
					fmt.Sprintf("A %s action needs its %s attribute.", actionType, object.name))
			case object.name != actionType && object.set:
				diags.AddAttributeError(at.AtName(object.name), "Parameters for another action",
					fmt.Sprintf("%s is only for %s actions, and this action's type is %s.", object.name, object.name, actionType))
			}
		}
	}

	if escalate := action.Escalate; escalate != nil {
		escalateAt := at.AtName("escalate")
		if escalate.EscalationPaths == nil && escalate.Users == nil {
			diags.AddAttributeError(escalateAt, "Nobody to page",
				"Set escalation_paths, users, or both.")
		}

		validateActionParam(escalate.EscalationPaths, escalateAt, "escalation_paths", true, known, diags)
		validateActionParam(escalate.Users, escalateAt, "users", true, known, diags)
		validateActionParam(escalate.Urgency, escalateAt, "urgency", false, known, diags)
	}

	for _, post := range []struct {
		name  string
		model *AlertRulePostActionV3Model
	}{
		{"post_slack", action.PostSlack},
		{"post_microsoft_teams", action.PostMicrosoftTeams},
	} {
		if post.model != nil {
			postAt := at.AtName(post.name)
			validateActionParam(post.model.Channels, postAt, "channels", true, known, diags)
			validatePrivateChannel(post.name, post.model, postAt, diags)
		}
	}

	if create := action.CreateIncident; create != nil {
		validateActionParam(create.IncidentTemplate, at.AtName("create_incident"), "incident_template", false, known, diags)
		validateActionParam(create.MembershipTeams, at.AtName("create_incident"), "membership_teams", true, known, diags)
	}
}

// validatePrivateChannel checks a private post as the server does on save: only Slack has
// private channels, and the post must name exactly one, by its ID. A literal that is still
// unknown passes, as it will be an ID by apply.
func validatePrivateChannel(name string, post *AlertRulePostActionV3Model, at path.Path, diags *diag.Diagnostics) {
	if post.ChannelVisibility.ValueString() != string(client.AlertRulePostActionV3ChannelVisibilityPrivate) {
		return
	}

	if name != string(client.PostSlack) {
		diags.AddAttributeError(at.AtName("channel_visibility"), "Private channel outside Slack",
			fmt.Sprintf("channel_visibility private is only for post_slack, not %s.", name))
		return
	}

	channels := post.Channels
	if channels == nil || channels.Values.IsUnknown() || channels.BindingArrayUnsettled() {
		return
	}
	values, array := channels.BindingValues(), channels.BindingArray()
	oneID := len(values)+len(array) == 1 && (len(values) == 1 || array[0].Reference.IsNull())
	if !oneID {
		diags.AddAttributeError(at.AtName("channels"), "Private channel needs one channel ID",
			"When channel_visibility is private, channels must hold exactly one Slack channel ID.")
	}
}

// validateActionParam also checks the binding's shape against the param, as the server's
// engine.BindParam does on save: a single-value param takes no list, and a list param takes
// no fixed single value. A reference passes either way, as it can resolve to a list.
func validateActionParam(binding *Binding, at path.Path, name string, list bool, known map[string]bool, diags *diag.Diagnostics) {
	at = at.AtName(name)
	ValidateBinding(binding, at, known, diags)
	if binding == nil || SetBindingForms(binding) != 1 {
		return
	}

	value, hasValue := binding.BindingObject()
	switch {
	case !list && (len(binding.BindingValues()) > 0 || len(binding.BindingArray()) > 0):
		diags.AddAttributeError(at, "List for a single value", fmt.Sprintf(
			"%s takes a single value: use value_literal, value_reference or expression_ref, not values or array_value.", name))
	case list && (isSet(binding.ValueLiteral) || hasValue && isSet(value.Literal)):
		diags.AddAttributeError(at, "Single value for a list", fmt.Sprintf(
			"%s takes a list: put fixed values in values, or use value_reference or expression_ref, not value_literal or value.literal.", name))
	}
}

// Payload

// AlertRuleActionsV3ToPayload maps a branch's actions for a create or update. The API needs
// the list even when it is empty.
func AlertRuleActionsV3ToPayload(actions []AlertRuleActionV3Model) ([]client.AlertRuleActionV3, error) {
	out := make([]client.AlertRuleActionV3, 0, len(actions))
	for idx, action := range actions {
		payload, err := action.ToPayload()
		if err != nil {
			return nil, fmt.Errorf("action %d: %w", idx, err)
		}
		out = append(out, payload)
	}

	return out, nil
}

// ToPayload sends every object the config set, and leaves the server to refuse one that
// doesn't match the type, as ValidateAlertRuleActions already does at plan time.
func (m AlertRuleActionV3Model) ToPayload() (client.AlertRuleActionV3, error) {
	payload := client.AlertRuleActionV3{
		Id:   m.ID.ValueStringPointer(),
		Type: client.AlertRuleActionV3Type(m.Type.ValueString()),
	}

	var err error
	if payload.Escalate, err = m.Escalate.toPayload(); err != nil {
		return payload, fmt.Errorf("escalate: %w", err)
	}
	if payload.PostSlack, err = m.PostSlack.toPayload(); err != nil {
		return payload, fmt.Errorf("post_slack: %w", err)
	}
	if payload.PostMicrosoftTeams, err = m.PostMicrosoftTeams.toPayload(); err != nil {
		return payload, fmt.Errorf("post_microsoft_teams: %w", err)
	}
	if payload.CreateIncident, err = m.CreateIncident.toPayload(); err != nil {
		return payload, fmt.Errorf("create_incident: %w", err)
	}

	return payload, nil
}

func (m *AlertRuleEscalateActionV3Model) toPayload() (*client.AlertRuleEscalateActionV3, error) {
	if m == nil {
		return nil, nil
	}

	payload := &client.AlertRuleEscalateActionV3{
		Mode:       (*client.AlertRuleEscalateActionV3Mode)(m.Mode.ValueStringPointer()),
		AutoCancel: m.AutoCancel.ValueBoolPointer(),
	}
	if !m.GracePeriodSeconds.IsNull() {
		payload.GracePeriodSeconds = lo32(m.GracePeriodSeconds.ValueInt64())
	}

	return payload, bindingsToPayload(
		bindingField{"escalation_paths", m.EscalationPaths, &payload.EscalationPaths},
		bindingField{"users", m.Users, &payload.Users},
		bindingField{"urgency", m.Urgency, &payload.Urgency},
	)
}

func (m *AlertRulePostActionV3Model) toPayload() (*client.AlertRulePostActionV3, error) {
	if m == nil {
		return nil, nil
	}

	payload := &client.AlertRulePostActionV3{
		Mode:              client.AlertRulePostActionV3Mode(m.Mode.ValueString()),
		ChannelVisibility: lo.ToPtr(client.AlertRulePostActionV3ChannelVisibility(m.ChannelVisibility.ValueString())),
	}

	// The payload holds channels by value. The schema requires it, and the server refuses
	// the empty one sent if it's missing anyway.
	var channels *client.EngineParamBindingPayloadV3
	err := bindingsToPayload(
		bindingField{"channels", m.Channels, &channels},
	)
	payload.Channels = lo.FromPtr(channels)

	return payload, err
}

func (m *AlertRuleCreateIncidentActionV3Model) toPayload() (*client.AlertRuleCreateIncidentActionV3, error) {
	if m == nil {
		return nil, nil
	}

	payload := &client.AlertRuleCreateIncidentActionV3{
		AutoDecline: m.AutoDecline.ValueBoolPointer(),
		IsPrivate:   m.IsPrivate.ValueBoolPointer(),
	}

	return payload, bindingsToPayload(
		bindingField{"incident_template", m.IncidentTemplate, &payload.IncidentTemplate},
		bindingField{"membership_teams", m.MembershipTeams, &payload.MembershipTeams},
	)
}

type bindingField struct {
	name    string
	binding *Binding
	into    **client.EngineParamBindingPayloadV3
}

// bindingsToPayload maps each binding into its field. A binding left out stays nil, which
// is how the API is told to use its default.
func bindingsToPayload(fields ...bindingField) error {
	for _, field := range fields {
		payload, err := BindingToPayload(field.binding)
		if err != nil {
			return fmt.Errorf("%s: %w", field.name, err)
		}
		*field.into = payload
	}

	return nil
}

// Read

// AlertRuleActionsV3FromAPI reads a branch's actions back. prior is the branch's actions in
// the plan on create and update, and in the prior state on read. It is matched by position,
// and answers what the response can't: how each binding was spelled, and whether the config
// left an id for the server to assign.
//
// An empty list reads as null unless prior holds one, as `actions = []` and leaving actions
// out both send none.
func AlertRuleActionsV3FromAPI(actions []client.AlertRuleActionV3, prior []AlertRuleActionV3Model) []AlertRuleActionV3Model {
	if len(actions) == 0 && prior == nil {
		return nil
	}

	out := make([]AlertRuleActionV3Model, 0, len(actions))
	for idx, action := range actions {
		var priorAction *AlertRuleActionV3Model
		if idx < len(prior) {
			priorAction = &prior[idx]
		}
		out = append(out, alertRuleActionV3FromAPI(action, priorAction))
	}

	return out
}

func alertRuleActionV3FromAPI(action client.AlertRuleActionV3, prior *AlertRuleActionV3Model) AlertRuleActionV3Model {
	p := lo.FromPtr(prior)

	model := AlertRuleActionV3Model{
		ID:                 types.StringPointerValue(action.Id),
		Type:               types.StringValue(string(action.Type)),
		Escalate:           escalateActionFromAPI(action.Escalate, p.Escalate),
		PostSlack:          postActionFromAPI(action.PostSlack, p.PostSlack),
		PostMicrosoftTeams: postActionFromAPI(action.PostMicrosoftTeams, p.PostMicrosoftTeams),
		CreateIncident:     createIncidentActionFromAPI(action.CreateIncident, p.CreateIncident),
	}

	// The server assigns an id the config left out, and a new one on every write. Keeping it
	// would plan its removal every time.
	if prior != nil && prior.ID.IsNull() {
		model.ID = types.StringNull()
	}

	return model
}

func escalateActionFromAPI(escalate *client.AlertRuleEscalateActionV3, prior *AlertRuleEscalateActionV3Model) *AlertRuleEscalateActionV3Model {
	if escalate == nil {
		return nil
	}
	p := lo.FromPtr(prior)

	model := &AlertRuleEscalateActionV3Model{
		EscalationPaths:    ReconcileBinding(p.EscalationPaths, escalate.EscalationPaths),
		Users:              ReconcileBinding(p.Users, escalate.Users),
		Urgency:            ReconcileBinding(p.Urgency, escalate.Urgency),
		Mode:               types.StringNull(),
		GracePeriodSeconds: types.Int64Null(),
		AutoCancel:         types.BoolPointerValue(escalate.AutoCancel),
	}
	if escalate.Mode != nil {
		model.Mode = types.StringValue(string(*escalate.Mode))
	}
	if escalate.GracePeriodSeconds != nil {
		model.GracePeriodSeconds = types.Int64Value(int64(*escalate.GracePeriodSeconds))
	}

	return model
}

func postActionFromAPI(post *client.AlertRulePostActionV3, prior *AlertRulePostActionV3Model) *AlertRulePostActionV3Model {
	if post == nil {
		return nil
	}
	p := lo.FromPtr(prior)

	return &AlertRulePostActionV3Model{
		Channels: ReconcileBinding(p.Channels, &post.Channels),
		Mode:     types.StringValue(string(post.Mode)),
		// A server from before channel_visibility leaves it out, and every post was public.
		ChannelVisibility: types.StringValue(string(lo.FromPtrOr(post.ChannelVisibility, client.AlertRulePostActionV3ChannelVisibilityPublic))),
	}
}

func createIncidentActionFromAPI(create *client.AlertRuleCreateIncidentActionV3, prior *AlertRuleCreateIncidentActionV3Model) *AlertRuleCreateIncidentActionV3Model {
	if create == nil {
		return nil
	}
	p := lo.FromPtr(prior)

	return &AlertRuleCreateIncidentActionV3Model{
		IncidentTemplate: ReconcileBinding(p.IncidentTemplate, create.IncidentTemplate),
		MembershipTeams:  ReconcileBinding(p.MembershipTeams, create.MembershipTeams),
		AutoDecline:      types.BoolPointerValue(create.AutoDecline),
		IsPrivate:        types.BoolPointerValue(create.IsPrivate),
	}
}
