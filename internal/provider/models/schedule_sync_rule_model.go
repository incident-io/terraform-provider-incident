package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// ScheduleSyncRuleResourceModel is the Terraform model for schedule sync rules.
type ScheduleSyncRuleResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	ScheduleID           types.String `tfsdk:"schedule_id"`
	ScheduleSyncTargetID types.String `tfsdk:"schedule_sync_target_id"`
	SyncType             types.String `tfsdk:"sync_type"`
	RotationID           types.String `tfsdk:"rotation_id"`
}

// FromAPI converts an API response to the Terraform model.
func (ScheduleSyncRuleResourceModel) FromAPI(rule client.ScheduleSyncRuleV2) ScheduleSyncRuleResourceModel {
	return ScheduleSyncRuleResourceModel{
		ID:                   types.StringValue(rule.Id),
		ScheduleID:           types.StringValue(rule.ScheduleId),
		ScheduleSyncTargetID: types.StringValue(rule.ScheduleSyncTargetId),
		SyncType:             types.StringValue(string(rule.SyncType)),
		RotationID:           types.StringPointerValue(rule.RotationId),
	}
}
