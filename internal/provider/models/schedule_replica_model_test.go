package models

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestScheduleReplicaModelFromAPI(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2021, 8, 17, 13, 28, 57, 0, time.UTC)
	syncedAt := time.Date(2023, 11, 7, 13, 33, 30, 0, time.UTC)

	replica := client.ScheduleReplicaV2{
		Id:                    "replica_1",
		ScheduleId:            "sched_1",
		ReplicaProvider:       client.ScheduleReplicaV2ReplicaProviderPagerduty,
		ReplicaProviderId:     "PO8107X",
		ReplicaFallbackUserId: "PA7AXXN",
		MirrorWindowDays:      lo.ToPtr(int64(21)),
		Sources: []client.ScheduleReplicaSourceV2{
			{RotationId: "rot_1", LayerId: "layer_1"},
		},
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		LastSyncedAt:  &syncedAt,
		LastSyncError: lo.ToPtr("Failed to find external user"),
		UserStatuses: []client.ScheduleReplicaUserStatusV2{
			{UserId: "user_1", ExternalUserId: lo.ToPtr("PJYTRGS")},
			{UserId: "user_2"},
		},
	}

	model := ScheduleReplicaModel{}.FromAPI(replica)

	assert.Equal(t, "replica_1", model.ID.ValueString())
	assert.Equal(t, "sched_1", model.ScheduleID.ValueString())
	assert.Equal(t, "pagerduty", model.ReplicaProvider.ValueString())
	assert.Equal(t, "PO8107X", model.ReplicaProviderID.ValueString())
	assert.Equal(t, "PA7AXXN", model.ReplicaFallbackUserID.ValueString())
	assert.Equal(t, int64(21), model.MirrorWindowDays.ValueInt64())
	require.Len(t, model.Sources, 1)
	assert.Equal(t, "rot_1", model.Sources[0].RotationID.ValueString())
	assert.Equal(t, "layer_1", model.Sources[0].LayerID.ValueString())
	assert.Equal(t, "Failed to find external user", model.LastSyncError.ValueString())
	require.Len(t, model.UserStatuses, 2)
	assert.Equal(t, "user_1", model.UserStatuses[0].UserID.ValueString())
	assert.Equal(t, "PJYTRGS", model.UserStatuses[0].ExternalUserID.ValueString())
	assert.True(t, model.UserStatuses[1].ExternalUserID.IsNull())
}

func TestScheduleReplicaModelFromAPIDefaultMirrorWindow(t *testing.T) {
	t.Parallel()

	replica := client.ScheduleReplicaV2{
		Id:                    "replica_1",
		ScheduleId:            "sched_1",
		ReplicaProvider:       client.ScheduleReplicaV2ReplicaProviderOpsgenie,
		ReplicaProviderId:     "ext-sched",
		ReplicaFallbackUserId: "fallback",
		Sources:               []client.ScheduleReplicaSourceV2{},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	model := ScheduleReplicaModel{}.FromAPI(replica)
	assert.Equal(t, DefaultScheduleReplicaMirrorWindowDays, model.MirrorWindowDays.ValueInt64())
	assert.Empty(t, model.Sources)
	assert.Empty(t, model.UserStatuses)
	assert.True(t, model.LastSyncError.IsNull())
}

func TestScheduleReplicaModelToCreatePayload(t *testing.T) {
	t.Parallel()

	model := ScheduleReplicaModel{
		ReplicaFallbackUserID: types.StringValue("PA7AXXN"),
		ReplicaProvider:       types.StringValue("pagerduty"),
		ReplicaProviderID:     types.StringValue("PO8107X"),
		MirrorWindowDays:      types.Int64Value(30),
		Sources: []ScheduleReplicaSourceModel{
			{
				RotationID: types.StringValue("rot_1"),
				LayerID:    types.StringValue("layer_1"),
			},
			{
				RotationID: types.StringValue("rot_2"),
				LayerID:    types.StringValue("layer_2"),
			},
		},
	}

	payload := model.ToCreatePayload()
	assert.Equal(t, "PA7AXXN", payload.ReplicaFallbackUserId)
	assert.Equal(t, client.ScheduleReplicaCreatePayloadV2ReplicaProviderPagerduty, payload.ReplicaProvider)
	assert.Equal(t, "PO8107X", payload.ReplicaProviderId)
	require.NotNil(t, payload.MirrorWindowDays)
	assert.Equal(t, int64(30), *payload.MirrorWindowDays)
	require.Len(t, payload.Sources, 2)
	assert.Equal(t, "rot_1", payload.Sources[0].RotationId)
	assert.Equal(t, "layer_1", payload.Sources[0].LayerId)
}
