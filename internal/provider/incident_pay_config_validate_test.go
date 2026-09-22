package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// payConfigValidateCase is one config to plan. Zero values take sensible defaults, so a
// case names only what it wants wrong.
type payConfigValidateCase struct {
	name          string
	timezone      string
	currency      string
	baseRateCents string
	rateTimeUnit  string
	rules         string
	errRe         string // empty => expect a successful (non-empty) plan
}

// payConfigValidateConfig assembles an incident_pay_config config for a case. The plan is
// hermetic: nothing in it reaches the API.
func payConfigValidateConfig(tc payConfigValidateCase) string {
	if tc.timezone == "" {
		tc.timezone = "Europe/London"
	}
	if tc.currency == "" {
		tc.currency = "GBP"
	}
	if tc.baseRateCents == "" {
		tc.baseRateCents = "500"
	}
	if tc.rateTimeUnit == "" {
		tc.rateTimeUnit = "hour"
	}

	return fmt.Sprintf(`
resource "incident_pay_config" "test" {
  name            = "Platform on-call"
  timezone        = %q
  currency        = %q
  base_rate_cents = %s
  rate_time_unit  = %q
%s
}
`, tc.timezone, tc.currency, tc.baseRateCents, tc.rateTimeUnit, tc.rules)
}

// TestIncidentPayConfigResourceValidateConfig runs plan-only steps with no live
// organisation, so it covers the plan-time checks and nothing else: the schema's own
// validators and ValidateConfig, which between them mirror what the API rejects and catch
// what it would store and only trip over later.
func TestIncidentPayConfigResourceValidateConfig(t *testing.T) {
	cases := []payConfigValidateCase{
		{
			name: "well formed",
			rules: `
  weekly_rules = [
    { weekdays = ["saturday", "sunday"], start_time = "00:00", end_time = "00:00", rate_cents = 1500 },
    { weekdays = ["friday"], start_time = "18:00", end_time = "09:00", rate_cents = 1000 },
  ]
  one_off_rules = [
    { name = "Christmas Day", start_at = "2026-12-25T00:00:00Z", end_at = "2026-12-26T00:00:00Z", rate_cents = 3000 },
    { name = "Boxing Day", start_at = "2026-12-26T00:00:00Z", end_at = "2026-12-27T00:00:00Z", rate_cents = 2500 },
  ]`,
		},
		{
			name: "no rules",
		},
		{
			name:     "unknown timezone",
			timezone: "Europe/Londinium",
			errRe:    `isn't an IANA timezone name`,
		},
		{
			name:     "Local is not a timezone",
			timezone: "Local",
			errRe:    `isn't an IANA timezone name`,
		},
		{
			name:     "currency that isn't a code",
			currency: "pounds",
			errRe:    `three-letter ISO 4217 currency code`,
		},
		{
			name:          "negative base rate",
			baseRateCents: "-1",
			errRe:         `value must be at least 0`,
		},
		{
			name:         "rate time unit outside the enum",
			rateTimeUnit: "week",
			errRe:        `value must be one of`,
		},
		{
			name: "weekly rule with a day that isn't",
			rules: `
  weekly_rules = [
    { weekdays = ["funday"], start_time = "00:00", end_time = "00:00", rate_cents = 1500 },
  ]`,
			errRe: `value must be one of`,
		},
		{
			name: "weekly rule with no days",
			rules: `
  weekly_rules = [
    { weekdays = [], start_time = "00:00", end_time = "00:00", rate_cents = 1500 },
  ]`,
			errRe: `set must contain at least 1 elements`,
		},
		{
			name: "weekly rule with a time that isn't a time of day",
			rules: `
  weekly_rules = [
    { weekdays = ["monday"], start_time = "9:00", end_time = "25:00", rate_cents = 1500 },
  ]`,
			errRe: `"9:00" isn't a time of day`,
		},
		{
			name: "weekly rule with a negative rate",
			rules: `
  weekly_rules = [
    { weekdays = ["monday"], start_time = "09:00", end_time = "17:00", rate_cents = -5 },
  ]`,
			errRe: `value must be at least 0`,
		},
		{
			name: "one-off rule that isn't a timestamp",
			rules: `
  one_off_rules = [
    { name = "Christmas Day", start_at = "25/12/2026", end_at = "2026-12-26T00:00:00Z", rate_cents = 3000 },
  ]`,
			errRe: `isn't an RFC 3339 timestamp`,
		},
		{
			name: "one-off rule that ends before it starts",
			rules: `
  one_off_rules = [
    { name = "Christmas Day", start_at = "2026-12-26T00:00:00Z", end_at = "2026-12-25T00:00:00Z", rate_cents = 3000 },
  ]`,
			errRe: `start_at must be before end_at`,
		},
		{
			name: "one-off rules that overlap",
			rules: `
  one_off_rules = [
    { name = "Christmas Day", start_at = "2026-12-25T00:00:00Z", end_at = "2026-12-26T00:00:00Z", rate_cents = 3000 },
    { name = "Festive break", start_at = "2026-12-24T00:00:00Z", end_at = "2026-12-28T00:00:00Z", rate_cents = 2800 },
  ]`,
			errRe: `Rule "Festive break" overlaps rule "Christmas Day"`,
		},
		{
			// The API's check strictly contains, so back-to-back windows are fine, and so
			// are windows written in different offsets that don't actually meet.
			name: "one-off rules back to back in different offsets",
			rules: `
  one_off_rules = [
    { name = "Christmas Day", start_at = "2026-12-25T00:00:00Z", end_at = "2026-12-26T00:00:00Z", rate_cents = 3000 },
    { name = "Boxing Day", start_at = "2026-12-26T01:00:00+01:00", end_at = "2026-12-27T01:00:00+01:00", rate_cents = 2500 },
  ]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := resource.TestStep{
				Config:             payConfigValidateConfig(tc),
				PlanOnly:           true,
				ExpectNonEmptyPlan: tc.errRe == "",
			}
			if tc.errRe != "" {
				step.ExpectError = wrapRe(tc.errRe)
			}

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{step},
			})
		})
	}
}
