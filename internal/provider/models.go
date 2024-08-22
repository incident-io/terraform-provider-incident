package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

type IncidentWeekdayIntervalConfig struct {
	ID               types.String              `tfsdk:"id"`
	Name             types.String              `tfsdk:"name"`
	Timezone         types.String              `tfsdk:"timezone"`
	WeekdayIntervals []IncidentWeekdayInterval `tfsdk:"weekday_intervals"`
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

func (IncidentWeekdayIntervalConfig) FromClientV2(config client.WeekdayIntervalConfigV2) IncidentWeekdayIntervalConfig {
	intervals := make([]IncidentWeekdayInterval, len(config.WeekdayIntervals))
	for i, interval := range config.WeekdayIntervals {
		intervals[i] = IncidentWeekdayInterval{}.FromClientV2(interval)
	}

	return IncidentWeekdayIntervalConfig{
		ID:               types.StringValue(config.Id),
		Name:             types.StringValue(config.Name),
		Timezone:         types.StringValue(config.Timezone),
		WeekdayIntervals: intervals,
	}
}

func (c IncidentWeekdayIntervalConfig) ToClientV2() client.WeekdayIntervalConfigV2 {
	intervals := make([]client.WeekdayIntervalV2, len(c.WeekdayIntervals))
	for i, interval := range c.WeekdayIntervals {
		intervals[i] = client.WeekdayIntervalV2{
			StartTime: interval.StartTime.ValueString(),
			EndTime:   interval.EndTime.ValueString(),
			Weekday:   interval.Weekday.ValueString(),
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
		Weekday:   types.StringValue(interval.Weekday),
	}
}
