package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestIncidentAlertGroupingPreferenceResource_ValidateConfig checks the model's validators run at
// plan time. The rules themselves are tested in the models package. A create looks for an
// existing preference, so the provider talks to a fake API that has none.
func TestIncidentAlertGroupingPreferenceResource_ValidateConfig(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"team_grouping_preferences": []any{}, "default_grouping_preferences": []any{}})
	}))
	t.Cleanup(api.Close)
	t.Setenv("INCIDENT_ENDPOINT", api.URL)

	cases := []struct {
		name  string
		body  string
		errRe string // empty => expect a successful (non-empty) plan
	}{
		{
			name: "a team's preference with a branch",
			body: `
  kind    = "team"
  team_id = "01TEAM"
  default = { settings = { enabled = false } }

  branch {
    condition_groups = [{
      conditions = [{ subject = "alert.title", operation = "is_set", param_bindings = [] }]
    }]
    settings = { enabled = true, window_type = "rolling", window_seconds = 600 }
  }`,
		},
		{
			name: "the organisation default can't name a team",
			body: `
  kind    = "default"
  team_id = "01TEAM"
  default = { settings = { enabled = false } }`,
			errRe: "The organisation default belongs to no team",
		},
		{
			name: "a branch's enabled settings need a window",
			body: `
  kind    = "default"
  default = { settings = { enabled = false } }

  branch {
    condition_groups = [{
      conditions = [{ subject = "alert.title", operation = "is_set", param_bindings = [] }]
    }]
    settings = { enabled = true, window_type = "rolling" }
  }`,
			errRe: "`window_seconds` is required when grouping is enabled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := resource.TestStep{
				Config:             fmt.Sprintf("resource \"incident_alert_grouping_preference\" \"test\" {\n%s\n}\n", tc.body),
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
