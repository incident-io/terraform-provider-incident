package models

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
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
	Enabled        types.Bool              `tfsdk:"enabled"`
	AIEnabled      types.Bool              `tfsdk:"ai_enabled"`
	AIInstructions types.String            `tfsdk:"ai_instructions"`
	GroupingKeys   []AlertRouteGroupingKey `tfsdk:"grouping_keys"`
	WindowSeconds  types.Int64             `tfsdk:"window_seconds"`
	WindowType     types.String            `tfsdk:"window_type"`
}

// GroupingDefaultAttribute is a preference's default branch, the settings for alerts no
// conditional branch matches.
func GroupingDefaultAttribute(description string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: description,
		Attributes: map[string]schema.Attribute{
			"settings": groupingSettingsAttribute(),
		},
	}
}

func groupingSettingsAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: apischema.Docstring("AlertGroupingDefaultBranchV3", "settings"),
		Attributes: map[string]schema.Attribute{
			"enabled": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("AlertGroupingSettingsV3", "enabled"),
			},
			"ai_enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: apischema.Docstring("AlertGroupingSettingsV3", "ai_enabled"),
			},
			"ai_instructions": AIInstructionsAttribute(apischema.Docstring("AlertGroupingSettingsV3", "ai_instructions")),
			"grouping_keys": schema.SetNestedAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertGroupingSettingsV3", "grouping_keys"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"reference": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("GroupingKeyV3", "reference"),
						},
					},
				},
			},
			"window_seconds": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("AlertGroupingSettingsV3", "window_seconds"),
			},
			"window_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: apischema.EnumValuesDescription("AlertGroupingSettingsV3", "window_type"),
			},
		},
	}
}

// FromAPI builds the model from the API's preference.
func (TeamGroupingPreferenceResourceModel) FromAPI(preference client.TeamGroupingPreferenceV3, prior *TeamGroupingPreferenceResourceModel) TeamGroupingPreferenceResourceModel {
	var priorDefault *TeamGroupingPreferenceBranchModel
	if prior != nil {
		priorDefault = prior.Default
	}

	return TeamGroupingPreferenceResourceModel{
		ID:      types.StringValue(preference.Id),
		TeamID:  types.StringValue(preference.TeamId),
		Version: types.Int64Value(preference.Version),
		Default: groupingDefaultFromAPI(preference.Default, priorDefault),
	}
}

func (m TeamGroupingPreferenceResourceModel) ToCreatePayload() client.TeamGroupingPreferencesCreatePayloadV3 {
	return client.TeamGroupingPreferencesCreatePayloadV3{
		TeamId:  m.TeamID.ValueString(),
		Default: m.Default.toPayload(),
	}
}

// ToUpdatePayload carries the version the update creates, which the caller reads back from
// the API first so a concurrent edit is rejected rather than overwritten.
func (m TeamGroupingPreferenceResourceModel) ToUpdatePayload(version int64) client.TeamGroupingPreferencesUpdatePayloadV3 {
	return client.TeamGroupingPreferencesUpdatePayloadV3{
		Version: version,
		Default: m.Default.toPayload(),
	}
}

func groupingDefaultFromAPI(api client.AlertGroupingDefaultBranchV3, prior *TeamGroupingPreferenceBranchModel) *TeamGroupingPreferenceBranchModel {
	var priorSettings *TeamGroupingPreferenceSettingsModel
	if prior != nil {
		priorSettings = prior.Settings
	}

	return &TeamGroupingPreferenceBranchModel{Settings: groupingSettingsFromAPI(api.Settings, priorSettings)}
}

func (b *TeamGroupingPreferenceBranchModel) toPayload() client.AlertGroupingDefaultBranchV3 {
	if b == nil {
		return client.AlertGroupingDefaultBranchV3{}
	}

	return client.AlertGroupingDefaultBranchV3{Settings: b.Settings.toPayload()}
}

// groupingSettingsFromAPI reads empty grouping_keys back as absent or as an empty set to
// match the prior plan or state, or apply reports a diff.
func groupingSettingsFromAPI(api client.AlertGroupingSettingsV3, prior *TeamGroupingPreferenceSettingsModel) *TeamGroupingPreferenceSettingsModel {
	settings := &TeamGroupingPreferenceSettingsModel{
		Enabled:        types.BoolValue(api.Enabled),
		AIEnabled:      types.BoolValue(lo.FromPtr(api.AiEnabled)),
		AIInstructions: AIInstructionsFromAPI(api.AiInstructions),
		WindowSeconds:  types.Int64Null(),
		WindowType:     types.StringNull(),
	}
	if api.WindowSeconds != nil {
		settings.WindowSeconds = types.Int64Value(int64(*api.WindowSeconds))
	}
	if api.WindowType != nil {
		settings.WindowType = types.StringValue(string(*api.WindowType))
	}

	switch {
	case api.GroupingKeys != nil && len(*api.GroupingKeys) > 0:
		settings.GroupingKeys = lo.Map(*api.GroupingKeys, func(key client.GroupingKeyV3, _ int) AlertRouteGroupingKey {
			return AlertRouteGroupingKey{Reference: types.StringValue(key.Reference)}
		})
	case prior != nil && prior.GroupingKeys != nil:
		settings.GroupingKeys = []AlertRouteGroupingKey{}
	}

	return settings
}

// toPayload only sends the window, keys and AI setting when grouping is enabled; the API
// rejects them otherwise.
func (s *TeamGroupingPreferenceSettingsModel) toPayload() client.AlertGroupingSettingsV3 {
	settings := client.AlertGroupingSettingsV3{}
	if s == nil {
		return settings
	}

	settings.Enabled = s.Enabled.ValueBool()
	// Sent whether or not grouping is enabled, as the API stores instructions either way.
	// Left out, the API clears them.
	settings.AiInstructions = AIInstructionsPayload(s.AIInstructions)
	if !settings.Enabled {
		return settings
	}

	keys := lo.Map(s.GroupingKeys, func(key AlertRouteGroupingKey, _ int) client.GroupingKeyV3 {
		return client.GroupingKeyV3{Reference: key.Reference.ValueString()}
	})
	settings.GroupingKeys = &keys
	settings.AiEnabled = lo.ToPtr(s.AIEnabled.ValueBool())
	if !s.WindowSeconds.IsNull() {
		settings.WindowSeconds = lo32(s.WindowSeconds.ValueInt64())
	}
	if !s.WindowType.IsNull() {
		settings.WindowType = lo.ToPtr(client.AlertGroupingSettingsV3WindowType(s.WindowType.ValueString()))
	}

	return settings
}
