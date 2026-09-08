package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// ScheduleSyncTargetResourceModel is the Terraform model for schedule sync targets.
type ScheduleSyncTargetResourceModel struct {
	ID                types.String            `tfsdk:"id"`
	AddBotToGroup     types.Bool              `tfsdk:"add_bot_to_group"`
	SlackUserGroupID  types.String            `tfsdk:"slack_user_group_id"`
	SlackTeamID       types.String            `tfsdk:"slack_team_id"`
	NewSlackUserGroup *NewSlackUserGroupModel `tfsdk:"new_slack_user_group"`
}

// NewSlackUserGroupModel represents the configuration for creating a new Slack user group.
type NewSlackUserGroupModel struct {
	Name        types.String `tfsdk:"name"`
	Handle      types.String `tfsdk:"handle"`
	Description types.String `tfsdk:"description"`
	SlackTeamID types.String `tfsdk:"slack_team_id"`
}

// FromAPI converts an API response to the Terraform model.
func (ScheduleSyncTargetResourceModel) FromAPI(target client.ScheduleSyncTargetResourceV2) ScheduleSyncTargetResourceModel {
	return ScheduleSyncTargetResourceModel{
		ID:               types.StringValue(target.Id),
		AddBotToGroup:    types.BoolValue(target.AddBotToGroup),
		SlackUserGroupID: types.StringValue(target.SlackUserGroupId),
		SlackTeamID:      types.StringValue(target.SlackTeamId),
		// NewSlackUserGroup is not returned by the API, it's only used for creation
		NewSlackUserGroup: nil,
	}
}

// ToPayload converts the Terraform model to an API create payload.
func (m ScheduleSyncTargetResourceModel) ToPayload() client.ScheduleSyncTargetCreatePayloadV2 {
	payload := client.ScheduleSyncTargetCreatePayloadV2{
		AddBotToGroup: m.AddBotToGroup.ValueBool(),
	}

	if !m.SlackUserGroupID.IsNull() && !m.SlackUserGroupID.IsUnknown() {
		payload.SlackUserGroupId = m.SlackUserGroupID.ValueStringPointer()
	}

	if m.NewSlackUserGroup != nil {
		payload.NewSlackUserGroup = &client.NewSlackUserGroupPayloadV2{
			Name:        m.NewSlackUserGroup.Name.ValueString(),
			Handle:      m.NewSlackUserGroup.Handle.ValueString(),
			Description: m.NewSlackUserGroup.Description.ValueString(),
		}
		if !m.NewSlackUserGroup.SlackTeamID.IsNull() && !m.NewSlackUserGroup.SlackTeamID.IsUnknown() {
			payload.NewSlackUserGroup.SlackTeamId = m.NewSlackUserGroup.SlackTeamID.ValueStringPointer()
		}
	}

	return payload
}

// ScheduleSyncTargetDataSourceModel is the Terraform model for the schedule
// sync target data source. It omits new_slack_user_group (create-only) and
// includes linked_schedules from the API, which the resource does not manage.
type ScheduleSyncTargetDataSourceModel struct {
	ID               types.String                            `tfsdk:"id"`
	AddBotToGroup    types.Bool                              `tfsdk:"add_bot_to_group"`
	SlackUserGroupID types.String                            `tfsdk:"slack_user_group_id"`
	SlackTeamID      types.String                            `tfsdk:"slack_team_id"`
	LinkedSchedules  []ScheduleSyncTargetLinkedScheduleModel `tfsdk:"linked_schedules"`
}

// ScheduleSyncTargetLinkedScheduleModel is a schedule with an active sync rule
// pointing at this target.
type ScheduleSyncTargetLinkedScheduleModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	TeamIDs types.Set    `tfsdk:"team_ids"`
}

// FromAPIDataSource converts an API sync target into the data source model.
func (ScheduleSyncTargetDataSourceModel) FromAPIDataSource(target client.ScheduleSyncTargetResourceV2) ScheduleSyncTargetDataSourceModel {
	linked := make([]ScheduleSyncTargetLinkedScheduleModel, 0, len(target.LinkedSchedules))
	for _, schedule := range target.LinkedSchedules {
		linked = append(linked, ScheduleSyncTargetLinkedScheduleModel{
			ID:      types.StringValue(schedule.Id),
			Name:    types.StringValue(schedule.Name),
			TeamIDs: stringSliceToSet(schedule.TeamIds),
		})
	}

	return ScheduleSyncTargetDataSourceModel{
		ID:               types.StringValue(target.Id),
		AddBotToGroup:    types.BoolValue(target.AddBotToGroup),
		SlackUserGroupID: types.StringValue(target.SlackUserGroupId),
		SlackTeamID:      types.StringValue(target.SlackTeamId),
		LinkedSchedules:  linked,
	}
}

func stringSliceToSet(ids []string) types.Set {
	if len(ids) == 0 {
		return types.SetValueMust(types.StringType, []attr.Value{})
	}

	elements := make([]attr.Value, len(ids))
	for i, id := range ids {
		elements[i] = types.StringValue(id)
	}
	return types.SetValueMust(types.StringType, elements)
}
