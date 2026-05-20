package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// ScheduleSyncTargetResourceModel is the Terraform model for schedule sync targets.
type ScheduleSyncTargetResourceModel struct {
	ID                  types.String                   `tfsdk:"id"`
	AddBotToGroup       types.Bool                     `tfsdk:"add_bot_to_group"`
	SlackUserGroupID    types.String                   `tfsdk:"slack_user_group_id"`
	SlackTeamID         types.String                   `tfsdk:"slack_team_id"`
	NewSlackUserGroup   *NewSlackUserGroupModel        `tfsdk:"new_slack_user_group"`
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
