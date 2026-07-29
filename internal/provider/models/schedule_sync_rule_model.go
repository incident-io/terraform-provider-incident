package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// ScheduleSyncRuleResourceModel is the Terraform model for schedule sync rules.
type ScheduleSyncRuleResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	ScheduleID             types.String `tfsdk:"schedule_id"`
	ScheduleSyncTargetID   types.String `tfsdk:"schedule_sync_target_id"`
	SyncType               types.String `tfsdk:"sync_type"`
	RotationID             types.String `tfsdk:"rotation_id"`
	PermanentMemberUserIDs types.Set    `tfsdk:"permanent_member_user_ids"`
}

// FromAPI converts an API response to the Terraform model.
//
// permanent_member_user_ids is always present on the API response (possibly
// empty). When the response is empty we store null so an unset attribute in
// config does not produce a perpetual null↔[] diff. Callers that had an
// explicit empty set in plan/state should restore it via
// PreserveEmptyPermanentMemberUserIDs.
func (ScheduleSyncRuleResourceModel) FromAPI(rule client.ScheduleSyncRuleV2) ScheduleSyncRuleResourceModel {
	return ScheduleSyncRuleResourceModel{
		ID:                     types.StringValue(rule.Id),
		ScheduleID:             types.StringValue(rule.ScheduleId),
		ScheduleSyncTargetID:   types.StringValue(rule.ScheduleSyncTargetId),
		SyncType:               types.StringValue(string(rule.SyncType)),
		RotationID:             types.StringPointerValue(rule.RotationId),
		PermanentMemberUserIDs: permanentMemberUserIDsFromAPI(rule.PermanentMemberUserIds),
	}
}

// PreserveEmptyPermanentMemberUserIDs keeps an explicitly empty
// permanent_member_user_ids set from prior when FromAPI collapsed an empty API
// response to null. Without this, `permanent_member_user_ids = []` would
// become null after apply ("Provider produced inconsistent result").
//
// Only empty priors are restored — if prior had members and the API now
// returns none, state should reflect the clear.
func (m *ScheduleSyncRuleResourceModel) PreserveEmptyPermanentMemberUserIDs(prior types.Set) {
	if prior.IsNull() || prior.IsUnknown() || len(prior.Elements()) > 0 {
		return
	}
	if m.PermanentMemberUserIDs.IsNull() {
		m.PermanentMemberUserIDs = prior
	}
}

// PermanentMemberUserIDsPayload returns the value to send on create/update, or
// nil when the attribute is unset so the API leaves existing members unchanged.
func (m ScheduleSyncRuleResourceModel) PermanentMemberUserIDsPayload() *[]string {
	if m.PermanentMemberUserIDs.IsNull() || m.PermanentMemberUserIDs.IsUnknown() {
		return nil
	}

	ids := []string{}
	for _, elem := range m.PermanentMemberUserIDs.Elements() {
		if str, ok := elem.(types.String); ok {
			ids = append(ids, str.ValueString())
		}
	}
	return &ids
}

func permanentMemberUserIDsFromAPI(ids []string) types.Set {
	if len(ids) == 0 {
		return types.SetNull(types.StringType)
	}

	elements := make([]attr.Value, len(ids))
	for i, id := range ids {
		elements[i] = types.StringValue(id)
	}
	return types.SetValueMust(types.StringType, elements)
}
