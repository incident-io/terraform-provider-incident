package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// tgpValidateConfig assembles an incident_team_grouping_preference config around one
// settings block. The literal team ID keeps the plan hermetic.
func tgpValidateConfig(settings string) string {
	return fmt.Sprintf(`
resource "incident_team_grouping_preference" "test" {
  team_id = "01TEAM"

  default = {
    settings = {
%s
    }
  }
}
`, settings)
}

// TestIncidentTeamGroupingPreferenceResource_ValidateConfig runs plan-only steps with no
// live organisation, so it checks the enabled and disabled rules and nothing else.
func TestIncidentTeamGroupingPreferenceResource_ValidateConfig(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		errRe    string // empty => expect a successful (non-empty) plan
	}{
		{
			name: "enabled with window and keys",
			settings: `
      enabled        = true
      window_type    = "rolling"
      window_seconds = 1800
      grouping_keys  = [{ reference = "alert.title" }]`,
		},
		{
			name: "enabled without keys groups all alerts",
			settings: `
      enabled        = true
      window_type    = "fixed"
      window_seconds = 600`,
		},
		{
			name: "disabled with nothing else",
			settings: `
      enabled = false`,
		},
		{
			name: "enabled requires window_seconds",
			settings: `
      enabled     = true
      window_type = "rolling"`,
			errRe: "`window_seconds` is required when `default.settings.enabled` is true",
		},
		{
			name: "enabled requires window_type",
			settings: `
      enabled        = true
      window_seconds = 1800`,
			errRe: "`window_type` is required when `default.settings.enabled` is true",
		},
		{
			name: "disabled forbids window_seconds",
			settings: `
      enabled        = false
      window_seconds = 1800`,
			errRe: "`window_seconds` must not be set when `default.settings.enabled` is false",
		},
		{
			name: "disabled forbids window_type",
			settings: `
      enabled     = false
      window_type = "rolling"`,
			errRe: "`window_type` must not be set when `default.settings.enabled` is false",
		},
		{
			name: "disabled forbids grouping_keys",
			settings: `
      enabled       = false
      grouping_keys = [{ reference = "alert.title" }]`,
			errRe: "`grouping_keys` must not be set when `default.settings.enabled` is false",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := resource.TestStep{
				Config:             tgpValidateConfig(tc.settings),
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
