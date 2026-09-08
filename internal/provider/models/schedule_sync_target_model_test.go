package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestScheduleSyncTargetDataSourceModelFromAPI(t *testing.T) {
	t.Parallel()

	target := client.ScheduleSyncTargetResourceV2{
		Id:               "target_1",
		AddBotToGroup:    true,
		SlackUserGroupId: "S123",
		SlackTeamId:      "T456",
		LinkedSchedules: []client.LinkedScheduleV2{
			{
				Id:      "sched_1",
				Name:    "Primary",
				TeamIds: []string{"team_a"},
			},
		},
	}

	model := ScheduleSyncTargetDataSourceModel{}.FromAPIDataSource(target)

	assert.Equal(t, "target_1", model.ID.ValueString())
	assert.True(t, model.AddBotToGroup.ValueBool())
	assert.Equal(t, "S123", model.SlackUserGroupID.ValueString())
	assert.Equal(t, "T456", model.SlackTeamID.ValueString())
	require.Len(t, model.LinkedSchedules, 1)
	assert.Equal(t, "sched_1", model.LinkedSchedules[0].ID.ValueString())
	assert.Equal(t, "Primary", model.LinkedSchedules[0].Name.ValueString())
	assert.Equal(t, []string{"team_a"}, setToStrings(t, model.LinkedSchedules[0].TeamIDs))
}

func TestScheduleSyncTargetDataSourceModelFromAPIEmptyLinkedSchedules(t *testing.T) {
	t.Parallel()

	model := ScheduleSyncTargetDataSourceModel{}.FromAPIDataSource(client.ScheduleSyncTargetResourceV2{
		Id:               "target_1",
		SlackUserGroupId: "S123",
		SlackTeamId:      "T456",
	})

	assert.Empty(t, model.LinkedSchedules)
}
