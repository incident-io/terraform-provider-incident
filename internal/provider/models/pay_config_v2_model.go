package models

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/timestamptypes"
)

// PayConfigModel is the Terraform model for a pay config, shared by the resource and the
// data source: every attribute the resource manages is one the data source reports.
//
// The rules are ordered lists rather than sets, and the order is the API's. Weekly rules
// are evaluated in order, so a shift priced by two of them is paid by the earlier one and
// the order is part of the configuration. One-off rules may not overlap, so their order
// has no effect on pricing, but the API keeps them in the order they were added and a list
// lets the resource change one rule in place rather than replacing it.
type PayConfigModel struct {
	ID            types.String               `tfsdk:"id"`
	Name          types.String               `tfsdk:"name"`
	Timezone      types.String               `tfsdk:"timezone"`
	Currency      types.String               `tfsdk:"currency"`
	BaseRateCents types.Int64                `tfsdk:"base_rate_cents"`
	RateTimeUnit  types.String               `tfsdk:"rate_time_unit"`
	WeeklyRules   []PayConfigWeeklyRuleModel `tfsdk:"weekly_rules"`
	OneOffRules   []PayConfigOneOffRuleModel `tfsdk:"one_off_rules"`
	PublishedAt   timetypes.RFC3339          `tfsdk:"published_at"`
	CreatedAt     timetypes.RFC3339          `tfsdk:"created_at"`
	UpdatedAt     timetypes.RFC3339          `tfsdk:"updated_at"`
}

// PayConfigWeeklyRuleModel is a rule that applies every week, over the same days and
// hours. Its ID is the API's, stable across edits, so the resource can change the rule
// in place.
type PayConfigWeeklyRuleModel struct {
	ID        types.String `tfsdk:"id"`
	Weekdays  types.Set    `tfsdk:"weekdays"`
	StartTime types.String `tfsdk:"start_time"`
	EndTime   types.String `tfsdk:"end_time"`
	RateCents types.Int64  `tfsdk:"rate_cents"`
}

// PayConfigOneOffRuleModel is a rule that applies over a single window of time, such as
// a public holiday.
//
// The window is a pair of timestamptypes.Instant rather than timetypes.RFC3339: the API
// reports a window in UTC whatever offset it was written in, and only Instant treats the
// two spellings of one moment as equal.
type PayConfigOneOffRuleModel struct {
	ID        types.String           `tfsdk:"id"`
	Name      types.String           `tfsdk:"name"`
	StartAt   timestamptypes.Instant `tfsdk:"start_at"`
	EndAt     timestamptypes.Instant `tfsdk:"end_at"`
	RateCents types.Int64            `tfsdk:"rate_cents"`
}

// AttrTypes is the object type of a weekly rule, for the empty default the resource
// gives the list. It must match the schema attribute for attribute, or the framework
// panics at runtime.
func (PayConfigWeeklyRuleModel) AttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":         types.StringType,
		"weekdays":   types.SetType{ElemType: types.StringType},
		"start_time": types.StringType,
		"end_time":   types.StringType,
		"rate_cents": types.Int64Type,
	}
}

// AttrTypes is the object type of a one-off rule. See PayConfigWeeklyRuleModel.AttrTypes.
func (PayConfigOneOffRuleModel) AttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":         types.StringType,
		"name":       types.StringType,
		"start_at":   timestamptypes.InstantType{},
		"end_at":     timestamptypes.InstantType{},
		"rate_cents": types.Int64Type,
	}
}

// FromAPI converts an API pay config into the Terraform model.
func (PayConfigModel) FromAPI(config client.PayConfigV2) PayConfigModel {
	return PayConfigModel{
		ID:            types.StringValue(config.Id),
		Name:          types.StringValue(config.Name),
		Timezone:      types.StringValue(config.Timezone),
		Currency:      types.StringValue(config.Currency),
		BaseRateCents: types.Int64Value(config.BaseRateCents),
		RateTimeUnit:  types.StringValue(string(config.RateTimeUnit)),
		// Both lists are always non-nil: a config with no rules has an empty list, which
		// is what the schema defaults an unset attribute to.
		WeeklyRules: lo.Map(config.WeeklyRules, func(rule client.PayConfigWeeklyRuleV2, _ int) PayConfigWeeklyRuleModel {
			return PayConfigWeeklyRuleModel{}.FromAPI(rule)
		}),
		OneOffRules: lo.Map(config.OneOffRules, func(rule client.PayConfigOneOffRuleV2, _ int) PayConfigOneOffRuleModel {
			return PayConfigOneOffRuleModel{}.FromAPI(rule)
		}),
		PublishedAt: timetypes.NewRFC3339TimePointerValue(config.PublishedAt),
		CreatedAt:   timetypes.NewRFC3339TimeValue(config.CreatedAt),
		UpdatedAt:   timetypes.NewRFC3339TimeValue(config.UpdatedAt),
	}
}

// ToCreatePayload converts the model to the payload that creates a config with its rules
// in one call. Both rule lists are always sent, as an empty list when there are none: the
// API reads an omitted list as invalid rather than as empty.
func (m PayConfigModel) ToCreatePayload() client.PayConfigsCreatePayloadV2 {
	weeklyRules := lo.Map(m.WeeklyRules, func(rule PayConfigWeeklyRuleModel, _ int) client.PayConfigWeeklyRulePayloadV2 {
		return client.PayConfigWeeklyRulePayloadV2{
			Weekdays:  weekdaysAs[client.PayConfigWeeklyRulePayloadV2Weekdays](rule.Weekdays),
			StartTime: rule.StartTime.ValueString(),
			EndTime:   rule.EndTime.ValueString(),
			RateCents: rule.RateCents.ValueInt64(),
		}
	})
	oneOffRules := lo.Map(m.OneOffRules, func(rule PayConfigOneOffRuleModel, _ int) client.PayConfigOneOffRulePayloadV2 {
		return client.PayConfigOneOffRulePayloadV2{
			Name:      rule.Name.ValueString(),
			StartAt:   rule.StartAtTime(),
			EndAt:     rule.EndAtTime(),
			RateCents: rule.RateCents.ValueInt64(),
		}
	})

	return client.PayConfigsCreatePayloadV2{
		Name:          m.Name.ValueString(),
		Timezone:      m.Timezone.ValueString(),
		Currency:      m.Currency.ValueString(),
		BaseRateCents: m.BaseRateCents.ValueInt64(),
		RateTimeUnit:  client.PayConfigsCreatePayloadV2RateTimeUnit(m.RateTimeUnit.ValueString()),
		WeeklyRules:   &weeklyRules,
		OneOffRules:   &oneOffRules,
	}
}

// ToUpdatePayload converts the model to the payload that updates a config's own
// attributes. Rules are not part of it: the API changes those through their own
// endpoints, which the resource drives rule by rule.
func (m PayConfigModel) ToUpdatePayload() client.PayConfigsUpdatePayloadV2 {
	return client.PayConfigsUpdatePayloadV2{
		Name:          m.Name.ValueString(),
		Timezone:      m.Timezone.ValueString(),
		Currency:      m.Currency.ValueString(),
		BaseRateCents: m.BaseRateCents.ValueInt64(),
		RateTimeUnit:  client.PayConfigsUpdatePayloadV2RateTimeUnit(m.RateTimeUnit.ValueString()),
	}
}

// AttributesEqual reports whether two models agree on the attributes ToUpdatePayload
// sends, which is how the resource knows an update can skip that call. Skipping it is
// more than an economy: updating a config a published report priced against needs a
// scope of its own, which a change to the rules alone should not demand.
func (m PayConfigModel) AttributesEqual(other PayConfigModel) bool {
	return m.Name.Equal(other.Name) &&
		m.Timezone.Equal(other.Timezone) &&
		m.Currency.Equal(other.Currency) &&
		m.BaseRateCents.Equal(other.BaseRateCents) &&
		m.RateTimeUnit.Equal(other.RateTimeUnit)
}

// FromAPI converts an API weekly rule into the model.
func (PayConfigWeeklyRuleModel) FromAPI(rule client.PayConfigWeeklyRuleV2) PayConfigWeeklyRuleModel {
	weekdays := lo.Map(rule.Weekdays, func(weekday client.PayConfigWeeklyRuleV2Weekdays, _ int) attr.Value {
		return types.StringValue(string(weekday))
	})

	return PayConfigWeeklyRuleModel{
		ID:        types.StringValue(rule.Id),
		Weekdays:  types.SetValueMust(types.StringType, weekdays),
		StartTime: types.StringValue(rule.StartTime),
		EndTime:   types.StringValue(rule.EndTime),
		RateCents: types.Int64Value(rule.RateCents),
	}
}

// ToCreatePayload converts the model to the payload that adds a rule to an existing
// config.
func (m PayConfigWeeklyRuleModel) ToCreatePayload() client.PayConfigsCreateWeeklyRulePayloadV2 {
	return client.PayConfigsCreateWeeklyRulePayloadV2{
		Weekdays:  weekdaysAs[client.PayConfigsCreateWeeklyRulePayloadV2Weekdays](m.Weekdays),
		StartTime: m.StartTime.ValueString(),
		EndTime:   m.EndTime.ValueString(),
		RateCents: m.RateCents.ValueInt64(),
	}
}

// ToUpdatePayload converts the model to the payload that changes a rule in place.
func (m PayConfigWeeklyRuleModel) ToUpdatePayload() client.PayConfigsUpdateWeeklyRulePayloadV2 {
	return client.PayConfigsUpdateWeeklyRulePayloadV2{
		Weekdays:  weekdaysAs[client.PayConfigsUpdateWeeklyRulePayloadV2Weekdays](m.Weekdays),
		StartTime: m.StartTime.ValueString(),
		EndTime:   m.EndTime.ValueString(),
		RateCents: m.RateCents.ValueInt64(),
	}
}

// Equivalent reports whether two rules would price a shift the same way, ignoring their
// IDs: the resource leaves such a pair alone rather than updating one to match the other.
func (m PayConfigWeeklyRuleModel) Equivalent(other PayConfigWeeklyRuleModel) bool {
	return m.Weekdays.Equal(other.Weekdays) &&
		m.StartTime.Equal(other.StartTime) &&
		m.EndTime.Equal(other.EndTime) &&
		m.RateCents.Equal(other.RateCents)
}

// FromAPI converts an API one-off rule into the model.
func (PayConfigOneOffRuleModel) FromAPI(rule client.PayConfigOneOffRuleV2) PayConfigOneOffRuleModel {
	return PayConfigOneOffRuleModel{
		ID:        types.StringValue(rule.Id),
		Name:      types.StringValue(rule.Name),
		StartAt:   timestamptypes.NewInstantTimeValue(rule.StartAt),
		EndAt:     timestamptypes.NewInstantTimeValue(rule.EndAt),
		RateCents: types.Int64Value(rule.RateCents),
	}
}

// ToCreatePayload converts the model to the payload that adds a rule to an existing
// config.
func (m PayConfigOneOffRuleModel) ToCreatePayload() client.PayConfigsCreateOneOffRulePayloadV2 {
	return client.PayConfigsCreateOneOffRulePayloadV2{
		Name:      m.Name.ValueString(),
		StartAt:   m.StartAtTime(),
		EndAt:     m.EndAtTime(),
		RateCents: m.RateCents.ValueInt64(),
	}
}

// ToUpdatePayload converts the model to the payload that changes a rule in place.
func (m PayConfigOneOffRuleModel) ToUpdatePayload() client.PayConfigsUpdateOneOffRulePayloadV2 {
	return client.PayConfigsUpdateOneOffRulePayloadV2{
		Name:      m.Name.ValueString(),
		StartAt:   m.StartAtTime(),
		EndAt:     m.EndAtTime(),
		RateCents: m.RateCents.ValueInt64(),
	}
}

// Equivalent reports whether two rules cover the same window at the same rate under the
// same name, ignoring their IDs. The instants are compared rather than their strings, so
// a window written with an offset matches the same window the API reports in UTC.
func (m PayConfigOneOffRuleModel) Equivalent(other PayConfigOneOffRuleModel) bool {
	return m.Name.Equal(other.Name) &&
		m.StartAtTime().Equal(other.StartAtTime()) &&
		m.EndAtTime().Equal(other.EndAtTime()) &&
		m.RateCents.Equal(other.RateCents)
}

// StartAtTime is start_at as a time. The custom type has already rejected a value that
// isn't RFC 3339, so a value that fails to parse here is null or unknown and reads as the
// zero time.
func (m PayConfigOneOffRuleModel) StartAtTime() time.Time {
	value, _ := m.StartAt.ValueTime()

	return value
}

// EndAtTime is end_at as a time. See StartAtTime.
func (m PayConfigOneOffRuleModel) EndAtTime() time.Time {
	value, _ := m.EndAt.ValueTime()

	return value
}

// weekdaysAs reads a weekdays set out as a slice of whichever weekday enum the payload
// at hand uses: the generator gives every payload its own, and they share no type.
func weekdaysAs[T ~string](weekdays types.Set) []T {
	if weekdays.IsNull() || weekdays.IsUnknown() {
		return []T{}
	}

	values := make([]T, 0, len(weekdays.Elements()))
	for _, element := range weekdays.Elements() {
		if weekday, ok := element.(types.String); ok {
			values = append(values, T(weekday.ValueString()))
		}
	}

	return values
}
