package models

import (
	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// DefaultScheduleReplicaMirrorWindowDays is the API default when mirror_window_days
// is omitted on create.
const DefaultScheduleReplicaMirrorWindowDays int64 = 14

// ScheduleReplicaModel is the Terraform model for a schedule replica, shared by
// the resource and data source.
type ScheduleReplicaModel struct {
	ID                    types.String                     `tfsdk:"id"`
	ScheduleID            types.String                     `tfsdk:"schedule_id"`
	ReplicaProvider       types.String                     `tfsdk:"replica_provider"`
	ReplicaProviderID     types.String                     `tfsdk:"replica_provider_id"`
	ReplicaFallbackUserID types.String                     `tfsdk:"replica_fallback_user_id"`
	MirrorWindowDays      types.Int64                      `tfsdk:"mirror_window_days"`
	Sources               []ScheduleReplicaSourceModel     `tfsdk:"sources"`
	CreatedAt             timetypes.RFC3339                `tfsdk:"created_at"`
	UpdatedAt             timetypes.RFC3339                `tfsdk:"updated_at"`
	LastSyncedAt          timetypes.RFC3339                `tfsdk:"last_synced_at"`
	LastSyncError         types.String                     `tfsdk:"last_sync_error"`
	UserStatuses          []ScheduleReplicaUserStatusModel `tfsdk:"user_statuses"`
}

// ScheduleReplicaSourceModel is one rotation/layer pair to replicate.
type ScheduleReplicaSourceModel struct {
	RotationID types.String `tfsdk:"rotation_id"`
	LayerID    types.String `tfsdk:"layer_id"`
}

// ScheduleReplicaUserStatusModel is the mapping of an incident.io user to an
// external provider user.
type ScheduleReplicaUserStatusModel struct {
	UserID         types.String `tfsdk:"user_id"`
	ExternalUserID types.String `tfsdk:"external_user_id"`
}

// FromAPI converts an API schedule replica into the Terraform model.
func (ScheduleReplicaModel) FromAPI(replica client.ScheduleReplicaV2) ScheduleReplicaModel {
	sources := make([]ScheduleReplicaSourceModel, 0, len(replica.Sources))
	for _, source := range replica.Sources {
		sources = append(sources, ScheduleReplicaSourceModel{
			RotationID: types.StringValue(source.RotationId),
			LayerID:    types.StringValue(source.LayerId),
		})
	}

	userStatuses := make([]ScheduleReplicaUserStatusModel, 0, len(replica.UserStatuses))
	for _, status := range replica.UserStatuses {
		userStatuses = append(userStatuses, ScheduleReplicaUserStatusModel{
			UserID:         types.StringValue(status.UserId),
			ExternalUserID: types.StringPointerValue(status.ExternalUserId),
		})
	}

	mirrorWindowDays := DefaultScheduleReplicaMirrorWindowDays
	if replica.MirrorWindowDays != nil {
		mirrorWindowDays = *replica.MirrorWindowDays
	}

	return ScheduleReplicaModel{
		ID:                    types.StringValue(replica.Id),
		ScheduleID:            types.StringValue(replica.ScheduleId),
		ReplicaProvider:       types.StringValue(string(replica.ReplicaProvider)),
		ReplicaProviderID:     types.StringValue(replica.ReplicaProviderId),
		ReplicaFallbackUserID: types.StringValue(replica.ReplicaFallbackUserId),
		MirrorWindowDays:      types.Int64Value(mirrorWindowDays),
		Sources:               sources,
		CreatedAt:             timetypes.NewRFC3339TimeValue(replica.CreatedAt),
		UpdatedAt:             timetypes.NewRFC3339TimeValue(replica.UpdatedAt),
		LastSyncedAt:          timetypes.NewRFC3339TimePointerValue(replica.LastSyncedAt),
		LastSyncError:         types.StringPointerValue(replica.LastSyncError),
		UserStatuses:          userStatuses,
	}
}

// ToCreatePayload converts the Terraform model to an API create payload.
func (m ScheduleReplicaModel) ToCreatePayload() client.ScheduleReplicaCreatePayloadV2 {
	sources := make([]client.ScheduleReplicaSourceV2, 0, len(m.Sources))
	for _, source := range m.Sources {
		sources = append(sources, client.ScheduleReplicaSourceV2{
			RotationId: source.RotationID.ValueString(),
			LayerId:    source.LayerID.ValueString(),
		})
	}

	payload := client.ScheduleReplicaCreatePayloadV2{
		ReplicaFallbackUserId: m.ReplicaFallbackUserID.ValueString(),
		ReplicaProvider:       client.ScheduleReplicaCreatePayloadV2ReplicaProvider(m.ReplicaProvider.ValueString()),
		ReplicaProviderId:     m.ReplicaProviderID.ValueString(),
		Sources:               sources,
	}

	if !m.MirrorWindowDays.IsNull() && !m.MirrorWindowDays.IsUnknown() {
		payload.MirrorWindowDays = lo.ToPtr(m.MirrorWindowDays.ValueInt64())
	}

	return payload
}
