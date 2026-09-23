package models

import (
	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// AnnouncementRuleModel is the Terraform model for an announcement rule, shared by the
// resource and the data source.
type AnnouncementRuleModel struct {
	ID                               types.String                  `tfsdk:"id"`
	Name                             types.String                  `tfsdk:"name"`
	ConditionGroups                  IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	SlackChannelIDs                  types.Set                     `tfsdk:"slack_channel_ids"`
	MicrosoftTeamsChannelIDs         types.Set                     `tfsdk:"microsoft_teams_channel_ids"`
	UpdateSharingMode                types.String                  `tfsdk:"update_sharing_mode"`
	Mode                             types.String                  `tfsdk:"mode"`
	PrivateIncidentScope             types.String                  `tfsdk:"private_incident_scope"`
	ConditionsNoLongerApplyBehaviour types.String                  `tfsdk:"conditions_no_longer_apply_behaviour"`
	TemplateID                       types.String                  `tfsdk:"template_id"`
	OwningTeamIDs                    types.Set                     `tfsdk:"owning_team_ids"`
	CreatedAt                        timetypes.RFC3339             `tfsdk:"created_at"`
	UpdatedAt                        timetypes.RFC3339             `tfsdk:"updated_at"`
}

// FromAPI converts an API announcement rule into the Terraform model. prior is the plan or
// prior state, when there is one: condition groups come back in the API's own spelling of
// an operation or binding, so they're reconciled against what the config wrote.
func (AnnouncementRuleModel) FromAPI(rule client.AnnouncementRuleV2, prior *AnnouncementRuleModel) AnnouncementRuleModel {
	model := AnnouncementRuleModel{
		ID:                               types.StringValue(rule.Id),
		Name:                             types.StringValue(rule.Name),
		ConditionGroups:                  IncidentEngineConditionGroups{}.FromAPI(rule.ConditionGroups),
		SlackChannelIDs:                  announcementStringSet(rule.SlackChannelIds),
		MicrosoftTeamsChannelIDs:         announcementStringSet(rule.MicrosoftTeamsChannelIds),
		UpdateSharingMode:                types.StringValue(string(rule.UpdateSharingMode)),
		Mode:                             types.StringValue(string(rule.Mode)),
		PrivateIncidentScope:             types.StringPointerValue((*string)(rule.PrivateIncidentScope)),
		ConditionsNoLongerApplyBehaviour: types.StringValue(string(rule.ConditionsNoLongerApplyBehaviour)),
		TemplateID:                       types.StringPointerValue(rule.TemplateId),
		OwningTeamIDs:                    announcementStringSet(rule.OwningTeamIds),
		CreatedAt:                        timetypes.NewRFC3339TimeValue(rule.CreatedAt),
		UpdatedAt:                        timetypes.NewRFC3339TimeValue(rule.UpdatedAt),
	}

	if prior != nil {
		model.ConditionGroups.ReconcileSpelling(prior.ConditionGroups)
	}

	return model
}

// ToCreatePayload converts the Terraform model to an API create payload.
func (m AnnouncementRuleModel) ToCreatePayload() client.AnnouncementRulesCreatePayloadV2 {
	return client.AnnouncementRulesCreatePayloadV2{
		Name:                             m.Name.ValueString(),
		ConditionGroups:                  m.ConditionGroups.ToPayload(),
		SlackChannelIds:                  lo.ToPtr(announcementSetStrings(m.SlackChannelIDs)),
		MicrosoftTeamsChannelIds:         lo.ToPtr(announcementSetStrings(m.MicrosoftTeamsChannelIDs)),
		UpdateSharingMode:                client.AnnouncementRulesCreatePayloadV2UpdateSharingMode(m.UpdateSharingMode.ValueString()),
		Mode:                             client.AnnouncementRulesCreatePayloadV2Mode(m.Mode.ValueString()),
		PrivateIncidentScope:             knownEnum[client.AnnouncementRulesCreatePayloadV2PrivateIncidentScope](m.PrivateIncidentScope),
		ConditionsNoLongerApplyBehaviour: knownEnum[client.AnnouncementRulesCreatePayloadV2ConditionsNoLongerApplyBehaviour](m.ConditionsNoLongerApplyBehaviour),
		TemplateId:                       knownStringPtr(m.TemplateID),
		OwningTeamIds:                    lo.ToPtr(announcementSetStrings(m.OwningTeamIDs)),
	}
}

// ToUpdatePayload converts the Terraform model to an API update payload. The channel and
// owning team lists are always sent, because the API reads an omitted list as "leave
// unchanged" and an empty set in config means "none".
func (m AnnouncementRuleModel) ToUpdatePayload() client.AnnouncementRulesUpdatePayloadV2 {
	return client.AnnouncementRulesUpdatePayloadV2{
		Name:                             m.Name.ValueString(),
		ConditionGroups:                  m.ConditionGroups.ToPayload(),
		SlackChannelIds:                  lo.ToPtr(announcementSetStrings(m.SlackChannelIDs)),
		MicrosoftTeamsChannelIds:         lo.ToPtr(announcementSetStrings(m.MicrosoftTeamsChannelIDs)),
		UpdateSharingMode:                client.AnnouncementRulesUpdatePayloadV2UpdateSharingMode(m.UpdateSharingMode.ValueString()),
		Mode:                             client.AnnouncementRulesUpdatePayloadV2Mode(m.Mode.ValueString()),
		PrivateIncidentScope:             knownEnum[client.AnnouncementRulesUpdatePayloadV2PrivateIncidentScope](m.PrivateIncidentScope),
		ConditionsNoLongerApplyBehaviour: knownEnum[client.AnnouncementRulesUpdatePayloadV2ConditionsNoLongerApplyBehaviour](m.ConditionsNoLongerApplyBehaviour),
		TemplateId:                       knownStringPtr(m.TemplateID),
		OwningTeamIds:                    lo.ToPtr(announcementSetStrings(m.OwningTeamIDs)),
	}
}

// announcementStringSet builds a set of strings, which is never null: the API always
// returns these lists, as an empty array when there's nothing in them.
func announcementStringSet(values []string) types.Set {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}

	return types.SetValueMust(types.StringType, elements)
}

// announcementSetStrings reads a set of strings as a slice, which is never nil: a nil
// slice would be omitted from the payload, which the API reads as "leave unchanged".
func announcementSetStrings(set types.Set) []string {
	if set.IsNull() || set.IsUnknown() {
		return []string{}
	}

	values := make([]string, 0, len(set.Elements()))
	for _, element := range set.Elements() {
		if value, ok := element.(types.String); ok {
			values = append(values, value.ValueString())
		}
	}

	return values
}

// knownStringPtr returns a pointer to a string attribute's value, or nil when it's null or
// unknown, so the API applies its own default.
func knownStringPtr(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	return lo.ToPtr(value.ValueString())
}

// knownEnum is knownStringPtr for a payload's enum type.
func knownEnum[T ~string](value types.String) *T {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	return lo.ToPtr(T(value.ValueString()))
}
