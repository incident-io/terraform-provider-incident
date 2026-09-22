package models

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/timestamptypes"
)

func payConfigFixture() client.PayConfigV2 {
	return client.PayConfigV2{
		Id:            "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:          "Platform on-call",
		Timezone:      "Europe/London",
		Currency:      "GBP",
		BaseRateCents: 500,
		RateTimeUnit:  "hour",
		WeeklyRules: []client.PayConfigWeeklyRuleV2{
			{Id: "01WEEKEND", Weekdays: []client.PayConfigWeeklyRuleV2Weekdays{"saturday", "sunday"}, StartTime: "00:00", EndTime: "00:00", RateCents: 1500},
			{Id: "01NIGHTS", Weekdays: []client.PayConfigWeeklyRuleV2Weekdays{"monday", "friday"}, StartTime: "18:00", EndTime: "09:00", RateCents: 1000},
		},
		OneOffRules: []client.PayConfigOneOffRuleV2{
			{Id: "01XMAS", Name: "Christmas Day", StartAt: time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2026, 12, 26, 0, 0, 0, 0, time.UTC), RateCents: 3000},
		},
		PublishedAt: lo.ToPtr(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)),
		CreatedAt:   time.Date(2026, 8, 17, 13, 28, 57, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
	}
}

func TestPayConfigModelFromAPI(t *testing.T) {
	t.Parallel()

	model := PayConfigModel{}.FromAPI(payConfigFixture())

	assert.Equal(t, "01FCNDV6P870EA6S7TK1DSYDG0", model.ID.ValueString())
	assert.Equal(t, "Platform on-call", model.Name.ValueString())
	assert.Equal(t, "Europe/London", model.Timezone.ValueString())
	assert.Equal(t, "GBP", model.Currency.ValueString())
	assert.Equal(t, int64(500), model.BaseRateCents.ValueInt64())
	assert.Equal(t, "hour", model.RateTimeUnit.ValueString())
	assert.Equal(t, "2026-09-01T09:00:00Z", model.PublishedAt.ValueString())

	// Rules keep the API's order and their IDs.
	require.Len(t, model.WeeklyRules, 2)
	assert.Equal(t, "01WEEKEND", model.WeeklyRules[0].ID.ValueString())
	assert.Equal(t, "01NIGHTS", model.WeeklyRules[1].ID.ValueString())
	assert.Equal(t, "18:00", model.WeeklyRules[1].StartTime.ValueString())
	assert.ElementsMatch(t, []string{"saturday", "sunday"}, weekdaysAs[string](model.WeeklyRules[0].Weekdays))

	require.Len(t, model.OneOffRules, 1)
	assert.Equal(t, "01XMAS", model.OneOffRules[0].ID.ValueString())
	assert.Equal(t, "Christmas Day", model.OneOffRules[0].Name.ValueString())
	assert.Equal(t, "2026-12-25T00:00:00Z", model.OneOffRules[0].StartAt.ValueString())
	assert.Equal(t, int64(3000), model.OneOffRules[0].RateCents.ValueInt64())
}

func TestPayConfigModelFromAPIWithoutRules(t *testing.T) {
	t.Parallel()

	config := payConfigFixture()
	config.WeeklyRules = nil
	config.OneOffRules = nil
	config.PublishedAt = nil

	model := PayConfigModel{}.FromAPI(config)

	// An unpublished draft has no published_at, and a config with no rules has empty
	// lists rather than null ones, which is what the schema defaults an unset list to.
	assert.True(t, model.PublishedAt.IsNull())
	assert.NotNil(t, model.WeeklyRules)
	assert.Empty(t, model.WeeklyRules)
	assert.NotNil(t, model.OneOffRules)
	assert.Empty(t, model.OneOffRules)
}

func TestPayConfigModelToCreatePayload(t *testing.T) {
	t.Parallel()

	model := PayConfigModel{}.FromAPI(payConfigFixture())

	payload := model.ToCreatePayload()

	assert.Equal(t, "Platform on-call", payload.Name)
	assert.Equal(t, "Europe/London", payload.Timezone)
	assert.Equal(t, "GBP", payload.Currency)
	assert.Equal(t, int64(500), payload.BaseRateCents)
	assert.Equal(t, client.PayConfigsCreatePayloadV2RateTimeUnit("hour"), payload.RateTimeUnit)

	require.NotNil(t, payload.WeeklyRules)
	require.Len(t, *payload.WeeklyRules, 2)
	// IDs are the API's to mint: a create never sends one.
	assert.Nil(t, (*payload.WeeklyRules)[0].Id)
	assert.Equal(t, "00:00", (*payload.WeeklyRules)[0].StartTime)
	assert.Equal(t, int64(1000), (*payload.WeeklyRules)[1].RateCents)
	assert.ElementsMatch(t, []client.PayConfigWeeklyRulePayloadV2Weekdays{"monday", "friday"}, (*payload.WeeklyRules)[1].Weekdays)

	require.NotNil(t, payload.OneOffRules)
	require.Len(t, *payload.OneOffRules, 1)
	assert.Nil(t, (*payload.OneOffRules)[0].Id)
	assert.Equal(t, "Christmas Day", (*payload.OneOffRules)[0].Name)
	assert.True(t, (*payload.OneOffRules)[0].StartAt.Equal(time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC)))
}

func TestPayConfigModelToCreatePayloadSendsEmptyRuleLists(t *testing.T) {
	t.Parallel()

	model := PayConfigModel{
		Name:          types.StringValue("Flat rate"),
		Timezone:      types.StringValue("UTC"),
		Currency:      types.StringValue("USD"),
		BaseRateCents: types.Int64Value(100),
		RateTimeUnit:  types.StringValue("day"),
	}

	payload := model.ToCreatePayload()

	// The API reads an omitted rule list as invalid rather than as empty, so both are
	// always sent, as empty lists rather than omitted ones.
	require.NotNil(t, payload.WeeklyRules)
	assert.Empty(t, *payload.WeeklyRules)
	require.NotNil(t, payload.OneOffRules)
	assert.Empty(t, *payload.OneOffRules)
}

func TestPayConfigModelToUpdatePayload(t *testing.T) {
	t.Parallel()

	model := PayConfigModel{}.FromAPI(payConfigFixture())
	model.BaseRateCents = types.Int64Value(600)
	model.RateTimeUnit = types.StringValue("day")

	payload := model.ToUpdatePayload()

	assert.Equal(t, "Platform on-call", payload.Name)
	assert.Equal(t, int64(600), payload.BaseRateCents)
	assert.Equal(t, client.PayConfigsUpdatePayloadV2RateTimeUnit("day"), payload.RateTimeUnit)
}

func TestPayConfigModelAttributesEqual(t *testing.T) {
	t.Parallel()

	base := PayConfigModel{}.FromAPI(payConfigFixture())

	same := base
	same.WeeklyRules = nil // rules aren't attributes: a change to them alone is no update
	assert.True(t, base.AttributesEqual(same))

	renamed := base
	renamed.Name = types.StringValue("Platform on-call (old)")
	assert.False(t, base.AttributesEqual(renamed))

	repriced := base
	repriced.BaseRateCents = types.Int64Value(0)
	assert.False(t, base.AttributesEqual(repriced))
}

func TestPayConfigWeeklyRuleModelEquivalent(t *testing.T) {
	t.Parallel()

	rule := func(id string, days ...string) PayConfigWeeklyRuleModel {
		return PayConfigWeeklyRuleModel{
			ID: types.StringValue(id),
			Weekdays: types.SetValueMust(types.StringType, lo.Map(days, func(day string, _ int) attr.Value {
				return types.StringValue(day)
			})),
			StartTime: types.StringValue("09:00"),
			EndTime:   types.StringValue("17:00"),
			RateCents: types.Int64Value(100),
		}
	}

	// IDs don't count, and a set's order doesn't either.
	assert.True(t, rule("01A", "monday", "tuesday").Equivalent(rule("01B", "tuesday", "monday")))
	assert.False(t, rule("01A", "monday").Equivalent(rule("01A", "monday", "tuesday")))

	later := rule("01A", "monday")
	later.EndTime = types.StringValue("18:00")
	assert.False(t, rule("01A", "monday").Equivalent(later))
}

func TestPayConfigOneOffRuleModelEquivalent(t *testing.T) {
	t.Parallel()

	rule := func(start, end string) PayConfigOneOffRuleModel {
		return PayConfigOneOffRuleModel{
			ID:        types.StringValue("01A"),
			Name:      types.StringValue("New Year's Day"),
			StartAt:   timestamptypes.NewInstantStringValue(start),
			EndAt:     timestamptypes.NewInstantStringValue(end),
			RateCents: types.Int64Value(2000),
		}
	}

	// The same instant in a different offset is the same rule: what the config wrote
	// with an offset, the API reports in UTC.
	assert.True(t, rule("2027-01-01T00:00:00+01:00", "2027-01-02T00:00:00+01:00").
		Equivalent(rule("2026-12-31T23:00:00Z", "2027-01-01T23:00:00Z")))
	assert.False(t, rule("2027-01-01T00:00:00Z", "2027-01-02T00:00:00Z").
		Equivalent(rule("2027-01-01T00:00:00Z", "2027-01-03T00:00:00Z")))
}

func TestPayConfigOneOffRuleModelPayloadsCarryInstants(t *testing.T) {
	t.Parallel()

	rule := PayConfigOneOffRuleModel{
		Name:      types.StringValue("New Year's Day"),
		StartAt:   timestamptypes.NewInstantStringValue("2027-01-01T00:00:00+01:00"),
		EndAt:     timestamptypes.NewInstantStringValue("2027-01-02T00:00:00+01:00"),
		RateCents: types.Int64Value(2000),
	}

	created := rule.ToCreatePayload()
	assert.True(t, created.StartAt.Equal(time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)))
	assert.True(t, created.EndAt.Equal(time.Date(2027, 1, 1, 23, 0, 0, 0, time.UTC)))
	assert.Equal(t, int64(2000), created.RateCents)

	updated := rule.ToUpdatePayload()
	assert.Equal(t, "New Year's Day", updated.Name)
	assert.True(t, updated.StartAt.Equal(created.StartAt))
}
