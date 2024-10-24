package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
)

type IncidentScheduleResourceModelV1 struct {
	ID                   types.String            `tfsdk:"id"`
	Name                 types.String            `tfsdk:"name"`
	Timezone             types.String            `tfsdk:"timezone"`
	Rotations            []RotationV1            `tfsdk:"rotations"`
	HolidaysPublicConfig *HolidaysPublicConfigV1 `tfsdk:"holidays_public_config"`
}

type RotationV1 struct {
	ID       types.String        `tfsdk:"id"`
	Name     types.String        `tfsdk:"name"`
	Versions []RotationVersionV1 `tfsdk:"versions"`
}

type RotationVersionV1 struct {
	EffectiveFrom    types.String        `tfsdk:"effective_from"`
	HandoverStartAt  types.String        `tfsdk:"handover_start_at"`
	Handovers        []HandoverV1        `tfsdk:"handovers"`
	Users            []types.String      `tfsdk:"users"`
	WorkingIntervals []WorkingIntervalV1 `tfsdk:"working_intervals"`
	Layers           []LayerV1           `tfsdk:"layers"`
}

// Deprecated: this has been replaced by IncidentWeekdayInterval.
type WorkingIntervalV1 struct {
	Start types.String `tfsdk:"start"`
	End   types.String `tfsdk:"end"`
	Day   types.String `tfsdk:"day"`
}

type LayerV1 struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type HandoverV1 struct {
	Interval     types.Int64  `tfsdk:"interval"`
	IntervalType types.String `tfsdk:"interval_type"`
}

type HolidaysPublicConfigV1 struct {
	CountryCodes []types.String `tfsdk:"country_codes"`
}

func (m *IncidentScheduleResourceModelV1) Upgrade() *IncidentScheduleResourceModelV2 {
	return &IncidentScheduleResourceModelV2{
		ID:                   m.ID,
		Name:                 m.Name,
		Timezone:             m.Timezone,
		Rotations:            lo.Map(m.Rotations, func(r RotationV1, _ int) RotationV2 { return r.Upgrade() }),
		HolidaysPublicConfig: m.HolidaysPublicConfig.Upgrade(),
	}
}

func (m *HolidaysPublicConfigV1) Upgrade() *HolidaysPublicConfigV2 {
	if m == nil {
		return nil
	}
	return &HolidaysPublicConfigV2{
		CountryCodes: m.CountryCodes,
	}
}

func (m *RotationV1) Upgrade() RotationV2 {
	return RotationV2{
		ID:       m.ID,
		Name:     m.Name,
		Versions: lo.Map(m.Versions, func(v RotationVersionV1, _ int) RotationVersionV2 { return v.Upgrade() }),
	}
}

func (m *RotationVersionV1) Upgrade() RotationVersionV2 {
	return RotationVersionV2{
		EffectiveFrom:   m.EffectiveFrom,
		HandoverStartAt: m.HandoverStartAt,
		Handovers:       lo.Map(m.Handovers, func(h HandoverV1, _ int) HandoverV2 { return h.Upgrade() }),
		Users:           m.Users,
		WorkingIntervals: lo.Map(m.WorkingIntervals, func(w WorkingIntervalV1, _ int) IncidentWeekdayInterval {
			return IncidentWeekdayInterval{
				StartTime: w.Start,
				EndTime:   w.End,
				Weekday:   w.Day,
			}
		}),
		Layers: lo.Map(m.Layers, func(l LayerV1, _ int) LayerV2 { return l.Upgrade() }),
	}
}

func (m *LayerV1) Upgrade() LayerV2 {
	return LayerV2{
		ID:   m.ID,
		Name: m.Name,
	}
}

func (m *HandoverV1) Upgrade() HandoverV2 {
	return HandoverV2{
		Interval:     m.Interval,
		IntervalType: m.IntervalType,
	}
}
