package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type IncidentScheduleResourceModelV2 struct {
	ID                   types.String            `tfsdk:"id"`
	Name                 types.String            `tfsdk:"name"`
	Timezone             types.String            `tfsdk:"timezone"`
	Rotations            []RotationV2            `tfsdk:"rotations"`
	HolidaysPublicConfig *HolidaysPublicConfigV2 `tfsdk:"holidays_public_config"`
	TeamIDs              []types.String          `tfsdk:"team_ids"`
}

type RotationV2 struct {
	ID       types.String        `tfsdk:"id"`
	Name     types.String        `tfsdk:"name"`
	Versions []RotationVersionV2 `tfsdk:"versions"`
}

type RotationVersionV2 struct {
	EffectiveFrom    types.String              `tfsdk:"effective_from"`
	HandoverStartAt  types.String              `tfsdk:"handover_start_at"`
	Handovers        []HandoverV2              `tfsdk:"handovers"`
	Users            []types.String            `tfsdk:"users"`
	WorkingIntervals []IncidentWeekdayInterval `tfsdk:"working_intervals"`
	Layers           []LayerV2                 `tfsdk:"layers"`
}

type LayerV2 struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type HandoverV2 struct {
	Interval     types.Int64  `tfsdk:"interval"`
	IntervalType types.String `tfsdk:"interval_type"`
}

type HolidaysPublicConfigV2 struct {
	CountryCodes []types.String `tfsdk:"country_codes"`
}
