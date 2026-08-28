package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestScheduleSyncRuleResourceModelFromAPI(t *testing.T) {
	t.Parallel()

	rule := client.ScheduleSyncRuleV2{
		Id:                     "rule_1",
		ScheduleId:             "sched_1",
		ScheduleSyncTargetId:   "target_1",
		SyncType:               client.ScheduleSyncRuleV2SyncTypeOnCall,
		PermanentMemberUserIds: []string{"user_a", "user_b"},
	}

	model := ScheduleSyncRuleResourceModel{}.FromAPI(rule)

	assert.Equal(t, "rule_1", model.ID.ValueString())
	assert.Equal(t, "sched_1", model.ScheduleID.ValueString())
	assert.Equal(t, "target_1", model.ScheduleSyncTargetID.ValueString())
	assert.Equal(t, "on_call", model.SyncType.ValueString())
	require.False(t, model.PermanentMemberUserIDs.IsNull())
	assert.ElementsMatch(t, []string{"user_a", "user_b"}, setToStrings(t, model.PermanentMemberUserIDs))
}

func TestScheduleSyncRuleResourceModelFromAPIEmptyPermanentMembers(t *testing.T) {
	t.Parallel()

	rule := client.ScheduleSyncRuleV2{
		Id:                     "rule_1",
		ScheduleId:             "sched_1",
		ScheduleSyncTargetId:   "target_1",
		SyncType:               client.ScheduleSyncRuleV2SyncTypeOnCall,
		PermanentMemberUserIds: []string{},
	}

	model := ScheduleSyncRuleResourceModel{}.FromAPI(rule)
	assert.True(t, model.PermanentMemberUserIDs.IsNull())
}

func TestPreserveEmptyPermanentMemberUserIDs(t *testing.T) {
	t.Parallel()

	empty := types.SetValueMust(types.StringType, []attr.Value{})
	withMembers := types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("user_a"),
	})

	t.Run("restores explicit empty set", func(t *testing.T) {
		model := ScheduleSyncRuleResourceModel{
			PermanentMemberUserIDs: types.SetNull(types.StringType),
		}
		model.PreserveEmptyPermanentMemberUserIDs(empty)
		assert.False(t, model.PermanentMemberUserIDs.IsNull())
		assert.Empty(t, model.PermanentMemberUserIDs.Elements())
	})

	t.Run("does not restore prior members over an API clear", func(t *testing.T) {
		model := ScheduleSyncRuleResourceModel{
			PermanentMemberUserIDs: types.SetNull(types.StringType),
		}
		model.PreserveEmptyPermanentMemberUserIDs(withMembers)
		assert.True(t, model.PermanentMemberUserIDs.IsNull())
	})
}

func TestPermanentMemberUserIDsPayload(t *testing.T) {
	t.Parallel()

	t.Run("null omits the field", func(t *testing.T) {
		model := ScheduleSyncRuleResourceModel{
			PermanentMemberUserIDs: types.SetNull(types.StringType),
		}
		assert.Nil(t, model.PermanentMemberUserIDsPayload())
	})

	t.Run("empty set clears members", func(t *testing.T) {
		model := ScheduleSyncRuleResourceModel{
			PermanentMemberUserIDs: types.SetValueMust(types.StringType, []attr.Value{}),
		}
		payload := model.PermanentMemberUserIDsPayload()
		require.NotNil(t, payload)
		assert.Empty(t, *payload)
	})

	t.Run("set sends values", func(t *testing.T) {
		model := ScheduleSyncRuleResourceModel{
			PermanentMemberUserIDs: types.SetValueMust(types.StringType, []attr.Value{
				types.StringValue("user_a"),
			}),
		}
		payload := model.PermanentMemberUserIDsPayload()
		require.NotNil(t, payload)
		assert.Equal(t, []string{"user_a"}, *payload)
	})
}

func setToStrings(t *testing.T, set types.Set) []string {
	t.Helper()
	out := make([]string, 0, len(set.Elements()))
	for _, elem := range set.Elements() {
		str, ok := elem.(types.String)
		require.True(t, ok)
		out = append(out, str.ValueString())
	}
	return out
}
