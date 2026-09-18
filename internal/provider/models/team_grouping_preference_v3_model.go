package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// TeamGroupingPreferenceResourceModel is a team's grouping preference: how that team's
// alerts group on every alert route, ahead of each route's own default.
type TeamGroupingPreferenceResourceModel struct {
	ID                types.String                       `tfsdk:"id"`
	UnlockInDashboard types.Bool                         `tfsdk:"unlock_in_dashboard"`
	TeamID            types.String                       `tfsdk:"team_id"`
	Version           types.Int64                        `tfsdk:"version"`
	Default           *TeamGroupingPreferenceBranchModel `tfsdk:"default"`
}

type TeamGroupingPreferenceBranchModel struct {
	Settings *TeamGroupingPreferenceSettingsModel `tfsdk:"settings"`
}

// TeamGroupingPreferenceSettingsModel mirrors the alert route's grouping settings, minus the
// escalation fields, which stay the route's.
type TeamGroupingPreferenceSettingsModel struct {
	Enabled       types.Bool              `tfsdk:"enabled"`
	GroupingKeys  []AlertRouteGroupingKey `tfsdk:"grouping_keys"`
	WindowSeconds types.Int64             `tfsdk:"window_seconds"`
	WindowType    types.String            `tfsdk:"window_type"`
}

// FromAPI builds the model from the API's preference. Empty grouping_keys reads back as
// absent or as an empty set to match the prior plan or state, or apply reports a diff.
func (TeamGroupingPreferenceResourceModel) FromAPI(preference client.TeamGroupingPreferenceV3, prior *TeamGroupingPreferenceResourceModel) TeamGroupingPreferenceResourceModel {
	api := preference.Default.Settings

	settings := &TeamGroupingPreferenceSettingsModel{
		Enabled:       types.BoolValue(api.Enabled),
		WindowSeconds: types.Int64Null(),
		WindowType:    types.StringNull(),
	}
	if api.WindowSeconds != nil {
		settings.WindowSeconds = types.Int64Value(int64(*api.WindowSeconds))
	}
	if api.WindowType != nil {
		settings.WindowType = types.StringValue(string(*api.WindowType))
	}

	var priorSettings *TeamGroupingPreferenceSettingsModel
	if prior != nil && prior.Default != nil {
		priorSettings = prior.Default.Settings
	}
	switch {
	case api.GroupingKeys != nil && len(*api.GroupingKeys) > 0:
		settings.GroupingKeys = lo.Map(*api.GroupingKeys, func(key client.GroupingKeyV3, _ int) AlertRouteGroupingKey {
			return AlertRouteGroupingKey{Reference: types.StringValue(key.Reference)}
		})
	case api.Enabled && priorSettings != nil && priorSettings.GroupingKeys != nil:
		settings.GroupingKeys = []AlertRouteGroupingKey{}
	}

	return TeamGroupingPreferenceResourceModel{
		ID:      types.StringValue(preference.Id),
		TeamID:  types.StringValue(preference.TeamId),
		Version: types.Int64Value(preference.Version),
		Default: &TeamGroupingPreferenceBranchModel{Settings: settings},
	}
}

func (m TeamGroupingPreferenceResourceModel) ToCreatePayload() client.TeamGroupingPreferencesCreatePayloadV3 {
	return client.TeamGroupingPreferencesCreatePayloadV3{
		TeamId:  m.TeamID.ValueString(),
		Default: m.toBranchPayload(),
	}
}

// ToUpdatePayload carries the version the update creates, which the caller reads back from
// the API first so a concurrent edit is rejected rather than overwritten.
func (m TeamGroupingPreferenceResourceModel) ToUpdatePayload(version int64) client.TeamGroupingPreferencesUpdatePayloadV3 {
	return client.TeamGroupingPreferencesUpdatePayloadV3{
		Version: version,
		Default: m.toBranchPayload(),
	}
}

// toBranchPayload only sends the window and keys when grouping is enabled; the API rejects
// them otherwise.
func (m TeamGroupingPreferenceResourceModel) toBranchPayload() client.TeamGroupingBranchV3 {
	settings := client.TeamGroupingSettingsV3{}
	if m.Default == nil || m.Default.Settings == nil {
		return client.TeamGroupingBranchV3{Settings: settings}
	}

	config := m.Default.Settings
	settings.Enabled = config.Enabled.ValueBool()
	if !settings.Enabled {
		return client.TeamGroupingBranchV3{Settings: settings}
	}

	keys := lo.Map(config.GroupingKeys, func(key AlertRouteGroupingKey, _ int) client.GroupingKeyV3 {
		return client.GroupingKeyV3{Reference: key.Reference.ValueString()}
	})
	if keys == nil {
		keys = []client.GroupingKeyV3{}
	}
	settings.GroupingKeys = &keys
	if !config.WindowSeconds.IsNull() {
		settings.WindowSeconds = lo32(config.WindowSeconds.ValueInt64())
	}
	if !config.WindowType.IsNull() {
		settings.WindowType = lo.ToPtr(client.TeamGroupingSettingsV3WindowType(config.WindowType.ValueString()))
	}

	return client.TeamGroupingBranchV3{Settings: settings}
}
