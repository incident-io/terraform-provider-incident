package models

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// AlertRouteV3ResourceModel is the Terraform model for the v3 alert route API.
//
// The v3 API restructures the v2 alert route: grouping configuration moves out
// of incident_config into a dedicated, required grouping_config; channel_config
// and the top-level message_template collapse into a required message_config;
// the incident template moves under incident_config.template; the escalation
// config gains an optional when_alert_joins_group block; and the incident
// template no longer carries a workspace binding.
type AlertRouteV3ResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Enabled   types.Bool   `tfsdk:"enabled"`
	IsPrivate types.Bool   `tfsdk:"is_private"`

	AlertSources     []AlertRouteAlertSourceModel       `tfsdk:"alert_sources"`
	ConditionGroups  IncidentEngineConditionGroups      `tfsdk:"condition_groups"`
	Expressions      IncidentEngineExpressions          `tfsdk:"expressions"`
	EscalationConfig *AlertRouteV3EscalationConfigModel `tfsdk:"escalation_config"`
	IncidentConfig   *AlertRouteV3IncidentConfigModel   `tfsdk:"incident_config"`
	GroupingConfig   *AlertRouteV3GroupingConfigModel   `tfsdk:"grouping_config"`
	MessageConfig    *AlertRouteV3MessageConfigModel    `tfsdk:"message_config"`
	OwningTeamIDs    types.Set                          `tfsdk:"owning_team_ids"`
}

type AlertRouteV3EscalationConfigModel struct {
	AutoCancelEscalations types.Bool                        `tfsdk:"auto_cancel_escalations"`
	EscalationTargets     []AlertRouteEscalationTargetModel `tfsdk:"escalation_targets"`
	// when_alert_joins_group is Optional + Computed (the API defaults it when
	// grouping is enabled), so it is modelled as types.Object to hold unknown.
	WhenAlertJoinsGroup types.Object `tfsdk:"when_alert_joins_group"`
}

type AlertRouteWhenAlertJoinsGroupModel struct {
	Mode               types.String `tfsdk:"mode"`
	GracePeriodSeconds types.Int64  `tfsdk:"grace_period_seconds"`
}

func whenAlertJoinsGroupAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"mode":                 types.StringType,
		"grace_period_seconds": types.Int64Type,
	}
}

// whenAlertJoinsGroupFromAPI converts the API's optional when_alert_joins_group
// to a types.Object (null when absent). Modelled as an object, rather than a
// nested struct, so the Computed attribute can carry an unknown value.
func whenAlertJoinsGroupFromAPI(in *client.AlertRouteAlertJoinsGroupV3) types.Object {
	if in == nil {
		return types.ObjectNull(whenAlertJoinsGroupAttrTypes())
	}

	gracePeriodSeconds := types.Int64Null()
	if in.GracePeriodSeconds != nil {
		gracePeriodSeconds = types.Int64Value(int64(*in.GracePeriodSeconds))
	}

	obj, _ := types.ObjectValue(whenAlertJoinsGroupAttrTypes(), map[string]attr.Value{
		"mode":                 types.StringValue(string(in.Mode)),
		"grace_period_seconds": gracePeriodSeconds,
	})
	return obj
}

func whenAlertJoinsGroupToPayload(ctx context.Context, obj types.Object) *client.AlertRouteAlertJoinsGroupPayloadV3 {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}

	var m AlertRouteWhenAlertJoinsGroupModel
	obj.As(ctx, &m, basetypes.ObjectAsOptions{})

	payload := &client.AlertRouteAlertJoinsGroupPayloadV3{
		Mode: client.AlertRouteAlertJoinsGroupPayloadV3Mode(m.Mode.ValueString()),
	}
	if !m.GracePeriodSeconds.IsNull() && !m.GracePeriodSeconds.IsUnknown() {
		payload.GracePeriodSeconds = lo32(m.GracePeriodSeconds.ValueInt64())
	}
	return payload
}

type AlertRouteV3GroupingConfigModel struct {
	Default *AlertRouteV3GroupingSettingsModel `tfsdk:"default"`
}

type AlertRouteV3GroupingSettingsModel struct {
	Enabled       types.Bool              `tfsdk:"enabled"`
	GroupKeys     []AlertRouteGroupingKey `tfsdk:"group_keys"`
	WindowSeconds types.Int64             `tfsdk:"window_seconds"`
	WindowType    types.String            `tfsdk:"window_type"`
}

type AlertRouteV3MessageConfigModel struct {
	Destinations    []AlertRouteChannelConfigModel `tfsdk:"destinations"`
	MessageTemplate *IncidentEngineParamBinding    `tfsdk:"message_template"`
}

type AlertRouteV3IncidentConfigModel struct {
	AutoDeclineEnabled types.Bool                         `tfsdk:"auto_decline_enabled"`
	ConditionGroups    IncidentEngineConditionGroups      `tfsdk:"condition_groups"`
	Enabled            types.Bool                         `tfsdk:"enabled"`
	Template           *AlertRouteV3IncidentTemplateModel `tfsdk:"template"`
}

// AlertRouteV3IncidentTemplateModel mirrors the v2 incident template but drops
// the workspace binding, which the v3 API no longer accepts.
type AlertRouteV3IncidentTemplateModel struct {
	CustomFields  []AlertRouteCustomFieldModel         `tfsdk:"custom_fields"`
	IncidentMode  *IncidentEngineParamBinding          `tfsdk:"incident_mode"`
	IncidentType  *IncidentEngineParamBinding          `tfsdk:"incident_type"`
	Name          *AlertRouteAutoGeneratedParamBinding `tfsdk:"name"`
	Severity      types.Object                         `tfsdk:"severity"`
	StartInTriage *IncidentEngineParamBinding          `tfsdk:"start_in_triage"`
	Summary       *AlertRouteAutoGeneratedParamBinding `tfsdk:"summary"`
}

// convertEngineType bridges between the structurally-identical v2 and v3
// incident-engine API types. The shared engine model logic in engine.go is
// written against the v2 client types; rather than duplicate the recursive
// condition/expression/param-binding conversion code for v3, we round-trip
// through JSON, which is safe because both versions share the same JSON field
// names (the only difference is a response-only `label` field on v2 param
// binding values, which the model never reads).
func convertEngineType[Out any](in any) Out {
	var out Out
	raw, err := json.Marshal(in)
	if err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func conditionGroupsFromV3(groups []client.ConditionGroupV3) IncidentEngineConditionGroups {
	return IncidentEngineConditionGroups{}.FromAPI(convertEngineType[[]client.ConditionGroupV2](groups))
}

func conditionGroupsToV3Payload(groups IncidentEngineConditionGroups) []client.ConditionGroupPayloadV3 {
	return convertEngineType[[]client.ConditionGroupPayloadV3](groups.ToPayload())
}

func paramBindingFromV3(pb client.EngineParamBindingV3) IncidentEngineParamBinding {
	return IncidentEngineParamBinding{}.FromAPI(convertEngineType[client.EngineParamBindingV2](pb))
}

func paramBindingToV3Payload(binding IncidentEngineParamBinding) client.EngineParamBindingPayloadV3 {
	return convertEngineType[client.EngineParamBindingPayloadV3](binding.ToPayload())
}

func expressionsFromV3(expressions []client.ExpressionV3) IncidentEngineExpressions {
	return IncidentEngineExpressions{}.FromAPI(convertEngineType[[]client.ExpressionV2](expressions))
}

func expressionsToV3Payload(expressions IncidentEngineExpressions) []client.ExpressionPayloadV3 {
	return convertEngineType[[]client.ExpressionPayloadV3](expressions.ToPayload())
}

// severityFromAPIV3 reuses the v2 severity reconciliation logic by converting
// the v3 severity binding (structurally identical) to its v2 form.
func severityFromAPIV3(severity *client.AlertRouteSeverityBindingV3) types.Object {
	if severity == nil {
		return SeverityObjectNull()
	}

	return SeverityFromAPI(convertEngineType[*client.AlertRouteSeverityBindingV2](severity))
}

func severityToPayloadV3(ctx context.Context, severityObj types.Object) *client.AlertRouteSeverityBindingPayloadV3 {
	v2 := SeverityToPayload(ctx, severityObj)
	if v2 == nil {
		return nil
	}

	return convertEngineType[*client.AlertRouteSeverityBindingPayloadV3](v2)
}

func createAutoGeneratedBindingV3(binding *IncidentEngineParamBinding, autogenerated types.Bool) client.AlertRouteAutoGeneratedTemplateBindingPayloadV3 {
	return convertEngineType[client.AlertRouteAutoGeneratedTemplateBindingPayloadV3](createAutoGeneratedBinding(binding, autogenerated))
}

func (AlertRouteV3ResourceModel) FromAPI(apiModel client.AlertRouteV3) AlertRouteV3ResourceModel {
	return AlertRouteV3ResourceModel{}.FromAPIWithPlan(apiModel, nil)
}

func (AlertRouteV3ResourceModel) FromAPIWithPlan(apiModel client.AlertRouteV3, plan *AlertRouteV3ResourceModel) AlertRouteV3ResourceModel {
	result := AlertRouteV3ResourceModel{}

	result.ID = types.StringValue(apiModel.Id)
	result.Name = types.StringValue(apiModel.Name)
	result.Enabled = types.BoolValue(apiModel.Enabled)
	result.IsPrivate = types.BoolValue(apiModel.IsPrivate)

	result.OwningTeamIDs = types.SetNull(types.StringType)
	if apiModel.OwningTeamIds != nil {
		teamIDValues := []attr.Value{}
		for _, teamID := range *apiModel.OwningTeamIds {
			teamIDValues = append(teamIDValues, types.StringValue(teamID))
		}

		result.OwningTeamIDs, _ = types.SetValue(types.StringType, teamIDValues)
	}

	result.AlertSources = []AlertRouteAlertSourceModel{}
	for _, alertSource := range apiModel.AlertSources {
		model := AlertRouteAlertSourceModel{
			AlertSourceID:   types.StringValue(alertSource.AlertSourceId),
			ConditionGroups: conditionGroupsFromV3(alertSource.ConditionGroups),
		}

		result.AlertSources = append(result.AlertSources, model)
	}

	result.ConditionGroups = conditionGroupsFromV3(apiModel.ConditionGroups)

	result.Expressions = IncidentEngineExpressions{}
	if len(apiModel.Expressions) > 0 {
		result.Expressions = expressionsFromV3(apiModel.Expressions)
	}

	// Escalation config.
	result.EscalationConfig = &AlertRouteV3EscalationConfigModel{
		AutoCancelEscalations: types.BoolValue(apiModel.EscalationConfig.AutoCancelEscalations),
		EscalationTargets:     []AlertRouteEscalationTargetModel{},
	}

	for _, target := range apiModel.EscalationConfig.EscalationTargets {
		model := AlertRouteEscalationTargetModel{}

		if target.Users != nil {
			binding := paramBindingFromV3(*target.Users)
			model.Users = &binding
		}

		if target.EscalationPaths != nil {
			binding := paramBindingFromV3(*target.EscalationPaths)
			model.EscalationPaths = &binding
		}

		result.EscalationConfig.EscalationTargets = append(result.EscalationConfig.EscalationTargets, model)
	}

	result.EscalationConfig.WhenAlertJoinsGroup = whenAlertJoinsGroupFromAPI(apiModel.EscalationConfig.WhenAlertJoinsGroup)

	// Grouping config. The detail fields (group_keys, window_seconds,
	// window_type) are optional in the API and only returned when grouping is
	// enabled. As they're required attributes in the schema, fall back to the
	// planned values when the API omits them, to avoid drift on a disabled
	// route.
	var planGrouping *AlertRouteV3GroupingSettingsModel
	if plan != nil && plan.GroupingConfig != nil {
		planGrouping = plan.GroupingConfig.Default
	}
	groupingDefault := &AlertRouteV3GroupingSettingsModel{
		Enabled:       types.BoolValue(apiModel.GroupingConfig.Default.Enabled),
		WindowSeconds: types.Int64Null(),
		WindowType:    types.StringNull(),
	}
	switch {
	case apiModel.GroupingConfig.Default.WindowSeconds != nil:
		groupingDefault.WindowSeconds = types.Int64Value(int64(*apiModel.GroupingConfig.Default.WindowSeconds))
	case planGrouping != nil:
		groupingDefault.WindowSeconds = planGrouping.WindowSeconds
	}
	switch {
	case apiModel.GroupingConfig.Default.WindowType != nil:
		groupingDefault.WindowType = types.StringValue(string(*apiModel.GroupingConfig.Default.WindowType))
	case planGrouping != nil:
		groupingDefault.WindowType = planGrouping.WindowType
	}
	// Leave GroupKeys nil (null) by default. Populate it (to a possibly-empty
	// slice) only when the API returns it, or fall back to the plan; this keeps
	// an omitted optional null on import rather than drifting to an empty set.
	switch {
	case apiModel.GroupingConfig.Default.GroupKeys != nil:
		groupingDefault.GroupKeys = []AlertRouteGroupingKey{}
		for _, gk := range *apiModel.GroupingConfig.Default.GroupKeys {
			groupingDefault.GroupKeys = append(groupingDefault.GroupKeys, AlertRouteGroupingKey{
				Reference: types.StringValue(gk.Reference),
			})
		}
	case planGrouping != nil:
		groupingDefault.GroupKeys = planGrouping.GroupKeys
	}
	result.GroupingConfig = &AlertRouteV3GroupingConfigModel{Default: groupingDefault}

	// Message config. Leave Destinations nil (rather than an empty slice) when
	// the API returns none, so an omitted optional `destinations` stays null and
	// doesn't produce drift.
	result.MessageConfig = &AlertRouteV3MessageConfigModel{}
	for _, destination := range apiModel.MessageConfig.Destinations {
		model := AlertRouteChannelConfigModel{
			ConditionGroups: conditionGroupsFromV3(destination.ConditionGroups),
		}

		if destination.SlackTargets != nil {
			binding := paramBindingFromV3(destination.SlackTargets.Binding)
			model.SlackTargets = &AlertRouteChannelTargetModel{
				ChannelVisibility: types.StringValue(destination.SlackTargets.ChannelVisibility),
				Binding:           &binding,
			}
		}

		if destination.MsTeamsTargets != nil {
			binding := paramBindingFromV3(destination.MsTeamsTargets.Binding)
			model.MsTeamsTargets = &AlertRouteChannelTargetModel{
				ChannelVisibility: types.StringValue(destination.MsTeamsTargets.ChannelVisibility),
				Binding:           &binding,
			}
		}

		result.MessageConfig.Destinations = append(result.MessageConfig.Destinations, model)
	}

	if apiModel.MessageConfig.MessageTemplate != nil {
		binding := paramBindingFromV3(*apiModel.MessageConfig.MessageTemplate)
		result.MessageConfig.MessageTemplate = &binding
	}

	// Mirror the planned shape of the optional `destinations` set. The API
	// returns no destinations both when the user omitted the attribute (null)
	// and when they set it to an explicit empty list ([]). To avoid perpetual
	// diffs on refresh, only normalise to a non-nil empty slice when the plan
	// carried a non-nil (explicit, possibly empty) destinations slice;
	// otherwise leave it nil so an omitted optional stays null.
	if len(result.MessageConfig.Destinations) == 0 &&
		plan != nil && plan.MessageConfig != nil && plan.MessageConfig.Destinations != nil {
		result.MessageConfig.Destinations = []AlertRouteChannelConfigModel{}
	}

	// Incident config. auto_decline_enabled and condition_groups are optional in
	// the API and only populated when incident creation is enabled, so fall back
	// to the planned auto_decline_enabled when the API omits it (disabled route).
	var planIncident *AlertRouteV3IncidentConfigModel
	if plan != nil {
		planIncident = plan.IncidentConfig
	}
	var incidentConditionGroups []client.ConditionGroupV3
	if apiModel.IncidentConfig.ConditionGroups != nil {
		incidentConditionGroups = *apiModel.IncidentConfig.ConditionGroups
	}
	// Leave AutoDeclineEnabled null by default so an omitted optional stays null
	// on import (incident creation disabled); populate from the API when present,
	// else fall back to the plan.
	result.IncidentConfig = &AlertRouteV3IncidentConfigModel{
		AutoDeclineEnabled: types.BoolNull(),
		ConditionGroups:    conditionGroupsFromV3(incidentConditionGroups),
		Enabled:            types.BoolValue(apiModel.IncidentConfig.Enabled),
	}
	switch {
	case apiModel.IncidentConfig.AutoDeclineEnabled != nil:
		result.IncidentConfig.AutoDeclineEnabled = types.BoolValue(*apiModel.IncidentConfig.AutoDeclineEnabled)
	case planIncident != nil:
		result.IncidentConfig.AutoDeclineEnabled = planIncident.AutoDeclineEnabled
	}

	var planTemplate *AlertRouteV3IncidentTemplateModel
	if plan != nil && plan.IncidentConfig != nil {
		planTemplate = plan.IncidentConfig.Template
	}

	if apiModel.IncidentConfig.Template != nil {
		result.IncidentConfig.Template = incidentTemplateFromAPIV3(apiModel.IncidentConfig.Template, planTemplate)
	} else if planTemplate != nil {
		// Preserve the user's planned template if the API didn't return one, to
		// avoid drift.
		result.IncidentConfig.Template = planTemplate
	}

	return result
}

func incidentTemplateFromAPIV3(apiTemplate *client.AlertRouteIncidentTemplateV3, plan *AlertRouteV3IncidentTemplateModel) *AlertRouteV3IncidentTemplateModel {
	template := &AlertRouteV3IncidentTemplateModel{}

	emptyListType := types.ObjectType{
		AttrTypes: ParamBindingValueAttrTypes(),
	}

	// Name (auto-generatable, plan-aware).
	var planNameBinding *AlertRouteAutoGeneratedParamBinding
	if plan != nil {
		planNameBinding = plan.Name
	}

	nameBinding := AlertRouteAutoGeneratedParamBinding{
		Autogenerated: types.BoolValue(apiTemplate.Name.Autogenerated),
		ArrayValue:    types.ListNull(emptyListType),
	}
	if apiTemplate.Name.Binding != nil {
		paramBinding := paramBindingFromV3(*apiTemplate.Name.Binding)
		nameBinding = AlertRouteAutoGeneratedParamBinding{}.FromParamBindingWithPlan(paramBinding, nameBinding.Autogenerated, planNameBinding)
	} else if planNameBinding != nil {
		nameBinding = AlertRouteAutoGeneratedParamBinding{}.FromParamBindingWithPlan(IncidentEngineParamBinding{}, nameBinding.Autogenerated, planNameBinding)
	}
	template.Name = &nameBinding

	// Summary (auto-generatable, plan-aware).
	var planSummaryBinding *AlertRouteAutoGeneratedParamBinding
	if plan != nil {
		planSummaryBinding = plan.Summary
	}

	summaryBinding := AlertRouteAutoGeneratedParamBinding{
		Autogenerated: types.BoolValue(false),
		ArrayValue:    types.ListNull(emptyListType),
	}
	if apiTemplate.Summary != nil {
		summaryBinding.Autogenerated = types.BoolValue(apiTemplate.Summary.Autogenerated)

		if apiTemplate.Summary.Binding != nil {
			paramBinding := paramBindingFromV3(*apiTemplate.Summary.Binding)
			summaryBinding = AlertRouteAutoGeneratedParamBinding{}.FromParamBindingWithPlan(paramBinding, summaryBinding.Autogenerated, planSummaryBinding)
		} else if planSummaryBinding != nil {
			summaryBinding = AlertRouteAutoGeneratedParamBinding{}.FromParamBindingWithPlan(IncidentEngineParamBinding{}, summaryBinding.Autogenerated, planSummaryBinding)
		}
	} else if planSummaryBinding != nil {
		summaryBinding = AlertRouteAutoGeneratedParamBinding{}.FromParamBindingWithPlan(IncidentEngineParamBinding{}, summaryBinding.Autogenerated, planSummaryBinding)
	}
	template.Summary = &summaryBinding

	template.Severity = severityFromAPIV3(apiTemplate.Severity)

	if apiTemplate.IncidentMode != nil && apiTemplate.IncidentMode.Binding != nil {
		binding := paramBindingFromV3(*apiTemplate.IncidentMode.Binding)
		template.IncidentMode = &binding
	}

	if apiTemplate.IncidentType != nil && apiTemplate.IncidentType.Binding != nil {
		binding := paramBindingFromV3(*apiTemplate.IncidentType.Binding)
		template.IncidentType = &binding
	}

	if apiTemplate.StartInTriage != nil && apiTemplate.StartInTriage.Binding != nil {
		binding := paramBindingFromV3(*apiTemplate.StartInTriage.Binding)
		template.StartInTriage = &binding
	}

	// Custom fields: only populate when the API returns them or the plan has
	// them, otherwise leave nil to preserve null state in Terraform.
	if apiTemplate.CustomFields != nil && len(*apiTemplate.CustomFields) > 0 {
		template.CustomFields = []AlertRouteCustomFieldModel{}
		for _, cf := range *apiTemplate.CustomFields {
			model := AlertRouteCustomFieldModel{
				CustomFieldID: types.StringValue(cf.CustomFieldId),
				MergeStrategy: types.StringValue(string(cf.MergeStrategy)),
			}

			binding := paramBindingFromV3(cf.Binding)
			if binding.Value != nil && binding.Value.Reference.IsNull() {
				binding.Value.Reference = types.StringNull()
			}
			model.Binding = &binding

			template.CustomFields = append(template.CustomFields, model)
		}
	} else if plan != nil && len(plan.CustomFields) > 0 {
		template.CustomFields = []AlertRouteCustomFieldModel{}
		for _, cf := range plan.CustomFields {
			model := AlertRouteCustomFieldModel{
				CustomFieldID: cf.CustomFieldID,
				MergeStrategy: cf.MergeStrategy,
			}
			if cf.Binding != nil {
				bindingCopy := *cf.Binding
				model.Binding = &bindingCopy
			}
			template.CustomFields = append(template.CustomFields, model)
		}
	}

	// Mirror the planned shape of the optional `custom_fields` set: the API
	// returns none both when the attribute is omitted (null) and when it's an
	// explicit empty list ([]). Only normalise to a non-nil empty slice when the
	// plan carried a non-nil (explicit, possibly empty) slice; otherwise leave it
	// nil so an omitted optional stays null and doesn't produce drift.
	if len(template.CustomFields) == 0 && plan != nil && plan.CustomFields != nil {
		template.CustomFields = []AlertRouteCustomFieldModel{}
	}

	return template
}

func (m AlertRouteV3ResourceModel) ToCreatePayload() client.AlertRoutesCreatePayloadV3 {
	var owningTeamIDs *[]string
	if !m.OwningTeamIDs.IsNull() {
		teamIDs := []string{}
		for _, elem := range m.OwningTeamIDs.Elements() {
			if str, ok := elem.(types.String); ok {
				teamIDs = append(teamIDs, str.ValueString())
			}
		}

		owningTeamIDs = &teamIDs
	}

	payload := client.AlertRoutesCreatePayloadV3{
		Name:            m.Name.ValueString(),
		Enabled:         m.Enabled.ValueBool(),
		IsPrivate:       m.IsPrivate.ValueBool(),
		AlertSources:    []client.AlertRouteAlertSourcePayloadV3{},
		ConditionGroups: []client.ConditionGroupPayloadV3{},
		Expressions:     []client.ExpressionPayloadV3{},
		EscalationConfig: client.AlertRouteEscalationConfigPayloadV3{
			AutoCancelEscalations: false,
			EscalationTargets:     []client.AlertRouteEscalationTargetPayloadV3{},
		},
		IncidentConfig: client.AlertRouteIncidentConfigPayloadV3{
			AutoDeclineEnabled: lo.ToPtr(false),
			ConditionGroups:    &[]client.ConditionGroupPayloadV3{},
		},
		GroupingConfig: client.AlertGroupingConfigV3{
			Default: client.GroupingSettingsV3{
				GroupKeys: &[]client.GroupingKeyV3{},
			},
		},
		MessageConfig: client.AlertMessageConfigPayloadV3{
			Destinations: []client.AlertMessageDestinationPayloadV3{},
		},
		OwningTeamIds: owningTeamIDs,
	}

	alertSources := []client.AlertRouteAlertSourcePayloadV3{}
	for _, src := range m.AlertSources {
		alertSources = append(alertSources, client.AlertRouteAlertSourcePayloadV3{
			AlertSourceId:   src.AlertSourceID.ValueString(),
			ConditionGroups: conditionGroupsToV3Payload(src.ConditionGroups),
		})
	}
	payload.AlertSources = alertSources

	payload.ConditionGroups = conditionGroupsToV3Payload(m.ConditionGroups)

	if len(m.Expressions) > 0 {
		payload.Expressions = expressionsToV3Payload(m.Expressions)
	}

	if m.EscalationConfig != nil {
		payload.EscalationConfig = client.AlertRouteEscalationConfigPayloadV3{
			AutoCancelEscalations: m.EscalationConfig.AutoCancelEscalations.ValueBool(),
			EscalationTargets:     []client.AlertRouteEscalationTargetPayloadV3{},
		}

		for _, target := range m.EscalationConfig.EscalationTargets {
			escalationTarget := client.AlertRouteEscalationTargetPayloadV3{}
			if target.Users != nil {
				userBinding := paramBindingToV3Payload(*target.Users)
				escalationTarget.Users = &userBinding
			}
			if target.EscalationPaths != nil {
				pathBinding := paramBindingToV3Payload(*target.EscalationPaths)
				escalationTarget.EscalationPaths = &pathBinding
			}
			payload.EscalationConfig.EscalationTargets = append(payload.EscalationConfig.EscalationTargets, escalationTarget)
		}

		payload.EscalationConfig.WhenAlertJoinsGroup = whenAlertJoinsGroupToPayload(context.Background(), m.EscalationConfig.WhenAlertJoinsGroup)
	}

	if m.GroupingConfig != nil && m.GroupingConfig.Default != nil {
		groupingDefault := client.GroupingSettingsV3{
			Enabled: m.GroupingConfig.Default.Enabled.ValueBool(),
		}
		// The detail fields are only valid when grouping is enabled; the API
		// rejects them otherwise.
		if m.GroupingConfig.Default.Enabled.ValueBool() {
			groupKeys := []client.GroupingKeyV3{}
			for _, gk := range m.GroupingConfig.Default.GroupKeys {
				groupKeys = append(groupKeys, client.GroupingKeyV3{
					Reference: gk.Reference.ValueString(),
				})
			}
			groupingDefault.GroupKeys = &groupKeys
			if !m.GroupingConfig.Default.WindowSeconds.IsNull() {
				groupingDefault.WindowSeconds = lo32(m.GroupingConfig.Default.WindowSeconds.ValueInt64())
			}
			if !m.GroupingConfig.Default.WindowType.IsNull() {
				groupingDefault.WindowType = lo.ToPtr(client.GroupingSettingsV3WindowType(m.GroupingConfig.Default.WindowType.ValueString()))
			}
		}
		payload.GroupingConfig = client.AlertGroupingConfigV3{Default: groupingDefault}
	}

	if m.MessageConfig != nil {
		destinations := []client.AlertMessageDestinationPayloadV3{}
		for _, destination := range m.MessageConfig.Destinations {
			payloadDestination := client.AlertMessageDestinationPayloadV3{
				ConditionGroups: conditionGroupsToV3Payload(destination.ConditionGroups),
			}

			if destination.SlackTargets != nil && destination.SlackTargets.Binding != nil {
				payloadDestination.SlackTargets = &client.AlertRouteChannelTargetPayloadV3{
					ChannelVisibility: client.AlertRouteChannelTargetPayloadV3ChannelVisibility(destination.SlackTargets.ChannelVisibility.ValueString()),
					Binding:           paramBindingToV3Payload(*destination.SlackTargets.Binding),
				}
			}

			if destination.MsTeamsTargets != nil && destination.MsTeamsTargets.Binding != nil {
				payloadDestination.MsTeamsTargets = &client.AlertRouteChannelTargetPayloadV3{
					ChannelVisibility: client.AlertRouteChannelTargetPayloadV3ChannelVisibility(destination.MsTeamsTargets.ChannelVisibility.ValueString()),
					Binding:           paramBindingToV3Payload(*destination.MsTeamsTargets.Binding),
				}
			}

			destinations = append(destinations, payloadDestination)
		}

		payload.MessageConfig = client.AlertMessageConfigPayloadV3{
			Destinations: destinations,
		}

		if m.MessageConfig.MessageTemplate != nil {
			messageTemplateBinding := paramBindingToV3Payload(*m.MessageConfig.MessageTemplate)
			payload.MessageConfig.MessageTemplate = &messageTemplateBinding
		}
	}

	if m.IncidentConfig != nil {
		payload.IncidentConfig = client.AlertRouteIncidentConfigPayloadV3{
			Enabled: m.IncidentConfig.Enabled.ValueBool(),
		}
		// auto_decline_enabled and condition_groups are required when incident
		// creation is enabled, and must be unset otherwise.
		if m.IncidentConfig.Enabled.ValueBool() {
			payload.IncidentConfig.AutoDeclineEnabled = lo.ToPtr(m.IncidentConfig.AutoDeclineEnabled.ValueBool())
			payload.IncidentConfig.ConditionGroups = lo.ToPtr(conditionGroupsToV3Payload(m.IncidentConfig.ConditionGroups))
		}

		if m.IncidentConfig.Template != nil {
			payload.IncidentConfig.Template = m.IncidentConfig.Template.ToPayload()
		}
	}

	return payload
}

func (t AlertRouteV3IncidentTemplateModel) ToPayload() *client.AlertRouteIncidentTemplatePayloadV3 {
	template := &client.AlertRouteIncidentTemplatePayloadV3{}

	if t.Name != nil {
		nameParamBinding := t.Name.ToParamBinding()
		template.Name = createAutoGeneratedBindingV3(&nameParamBinding, t.Name.Autogenerated)
	}

	if t.Summary != nil {
		summaryParamBinding := t.Summary.ToParamBinding()
		summaryPayload := createAutoGeneratedBindingV3(&summaryParamBinding, t.Summary.Autogenerated)
		template.Summary = &summaryPayload
	} else {
		emptyBinding := createAutoGeneratedBindingV3(nil, types.BoolValue(false))
		template.Summary = &emptyBinding
	}

	if t.IncidentMode != nil {
		binding := paramBindingToV3Payload(*t.IncidentMode)
		template.IncidentMode = &client.AlertRouteTemplateBindingPayloadV3{Binding: &binding}
	}

	if t.IncidentType != nil {
		binding := paramBindingToV3Payload(*t.IncidentType)
		template.IncidentType = &client.AlertRouteTemplateBindingPayloadV3{Binding: &binding}
	}

	if t.StartInTriage != nil {
		binding := paramBindingToV3Payload(*t.StartInTriage)
		template.StartInTriage = &client.AlertRouteTemplateBindingPayloadV3{Binding: &binding}
	}

	template.Severity = severityToPayloadV3(context.Background(), t.Severity)

	customFields := []client.AlertRouteCustomFieldBindingPayloadV3{}
	for _, cf := range t.CustomFields {
		if cf.Binding == nil {
			continue
		}

		customFields = append(customFields, client.AlertRouteCustomFieldBindingPayloadV3{
			CustomFieldId: cf.CustomFieldID.ValueString(),
			Binding:       paramBindingToV3Payload(*cf.Binding),
			MergeStrategy: client.AlertRouteCustomFieldBindingPayloadV3MergeStrategy(cf.MergeStrategy.ValueString()),
		})
	}
	template.CustomFields = &customFields

	return template
}

func (m AlertRouteV3ResourceModel) ToUpdatePayload() client.AlertRoutesUpdatePayloadV3 {
	createPayload := m.ToCreatePayload()

	return client.AlertRoutesUpdatePayloadV3{
		Name:             createPayload.Name,
		Enabled:          createPayload.Enabled,
		IsPrivate:        createPayload.IsPrivate,
		AlertSources:     createPayload.AlertSources,
		ConditionGroups:  createPayload.ConditionGroups,
		Expressions:      createPayload.Expressions,
		EscalationConfig: createPayload.EscalationConfig,
		IncidentConfig:   createPayload.IncidentConfig,
		GroupingConfig:   createPayload.GroupingConfig,
		MessageConfig:    createPayload.MessageConfig,
		OwningTeamIds:    createPayload.OwningTeamIds,
	}
}

// lo32 returns a pointer to the int32 representation of the given int64.
func lo32(v int64) *int32 {
	out := int32(v)
	return &out
}
