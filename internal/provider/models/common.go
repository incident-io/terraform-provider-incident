package models

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

type IncidentWeekdayIntervalConfig struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Timezone types.String `tfsdk:"timezone"`
	// WeekdayIntervals is a types.List of IncidentWeekdayInterval objects so the
	// model can carry unknown values during planning.
	WeekdayIntervals types.List `tfsdk:"weekday_intervals"`
}

// WeekdayIntervalAttrTypes returns the attribute types for a single weekday
// interval object.
func WeekdayIntervalAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"start_time": types.StringType,
		"end_time":   types.StringType,
		"weekday":    types.StringType,
	}
}

// WeekdayIntervalConfigAttrTypes returns the attribute types for a single
// weekday interval config object.
func WeekdayIntervalConfigAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":       types.StringType,
		"name":     types.StringType,
		"timezone": types.StringType,
		"weekday_intervals": types.ListType{
			ElemType: types.ObjectType{AttrTypes: WeekdayIntervalAttrTypes()},
		},
	}
}

func (IncidentWeekdayIntervalConfig) Attributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("WeekdayIntervalConfigV2", "id"),
			Required:            true,
		},
		"name": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("WeekdayIntervalConfigV2", "name"),
			Required:            true,
		},
		"timezone": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("WeekdayIntervalConfigV2", "timezone"),
			Required:            true,
		},
		"weekday_intervals": schema.ListNestedAttribute{
			MarkdownDescription: apischema.Docstring("WeekdayIntervalConfigV2", "weekday_intervals"),
			Required:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: IncidentWeekdayInterval{}.Attributes(),
			},
		},
	}
}

func (IncidentWeekdayIntervalConfig) FromClientV2(ctx context.Context, config client.WeekdayIntervalConfigV2, diags *diag.Diagnostics) IncidentWeekdayIntervalConfig {
	intervals := make([]IncidentWeekdayInterval, len(config.WeekdayIntervals))
	for i, interval := range config.WeekdayIntervals {
		intervals[i] = IncidentWeekdayInterval{}.FromClientV2(interval)
	}

	intervalList, d := types.ListValueFrom(
		ctx,
		types.ObjectType{AttrTypes: WeekdayIntervalAttrTypes()},
		intervals,
	)
	diags.Append(d...)

	return IncidentWeekdayIntervalConfig{
		ID:               types.StringValue(config.Id),
		Name:             types.StringValue(config.Name),
		Timezone:         types.StringValue(config.Timezone),
		WeekdayIntervals: intervalList,
	}
}

func (c IncidentWeekdayIntervalConfig) ToClientV2(ctx context.Context, diags *diag.Diagnostics) client.WeekdayIntervalConfigV2 {
	var modelIntervals []IncidentWeekdayInterval
	if !c.WeekdayIntervals.IsNull() && !c.WeekdayIntervals.IsUnknown() {
		diags.Append(c.WeekdayIntervals.ElementsAs(ctx, &modelIntervals, false)...)
	}

	intervals := make([]client.WeekdayIntervalV2, len(modelIntervals))
	for i, interval := range modelIntervals {
		intervals[i] = client.WeekdayIntervalV2{
			StartTime: interval.StartTime.ValueString(),
			EndTime:   interval.EndTime.ValueString(),
			Weekday:   client.WeekdayIntervalV2Weekday(interval.Weekday.ValueString()),
		}
	}

	return client.WeekdayIntervalConfigV2{
		Id:               c.ID.ValueString(),
		Name:             c.Name.ValueString(),
		Timezone:         c.Timezone.ValueString(),
		WeekdayIntervals: intervals,
	}
}

type IncidentWeekdayInterval struct {
	StartTime types.String `tfsdk:"start_time"`
	EndTime   types.String `tfsdk:"end_time"`
	Weekday   types.String `tfsdk:"weekday"`
}

func (IncidentWeekdayInterval) Attributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"start_time": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("WeekdayIntervalV2", "start_time"),
			Required:            true,
		},
		"end_time": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("WeekdayIntervalV2", "end_time"),
			Required:            true,
		},
		"weekday": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("WeekdayIntervalV2", "weekday"),
			Required:            true,
		},
	}
}

func (IncidentWeekdayInterval) FromClientV2(interval client.WeekdayIntervalV2) IncidentWeekdayInterval {
	return IncidentWeekdayInterval{
		StartTime: types.StringValue(interval.StartTime),
		EndTime:   types.StringValue(interval.EndTime),
		Weekday:   types.StringValue(string(interval.Weekday)),
	}
}
