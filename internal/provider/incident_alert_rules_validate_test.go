package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// alertRulesValidateConfig uses literal IDs, so the plan needs no live organisation.
func alertRulesValidateConfig(body string) string {
	return fmt.Sprintf(`
resource "incident_alert_rules" "test" {
  kind    = "team"
  team_id = "01TEAM"
%s
}
`, body)
}

// TestIncidentAlertRulesResource_ValidateConfig checks the model's validators run at plan time.
// A create looks for the team's existing rules, so the provider talks to a fake API that has none.
func TestIncidentAlertRulesResource_ValidateConfig(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{"alert_rule_sets": []any{}})
	}))
	t.Cleanup(api.Close)
	t.Setenv("INCIDENT_ENDPOINT", api.URL)

	cases := []struct {
		name  string
		body  string
		errRe string // empty => expect a successful (non-empty) plan
	}{
		{
			name: "team rules paging an escalation path from an expression",
			body: `
  named_expression {
    name       = "team_path"
    start_from = "alert.attributes.team"

    operation {
      navigate = { to = "01ESCALATIONPATHATTR" }
    }
  }

  block {
    id = "page-team"

    branch {
      id = "always"
      actions = [{
        type     = "escalate"
        escalate = { escalation_paths = { expression_ref = "team_path" } }
      }]
    }
  }`,
		},
		{
			name: "an action needs the attribute for its type",
			body: `
  block {
    id = "page"

    branch {
      id      = "always"
      actions = [{ type = "escalate" }]
    }
  }`,
			errRe: "A escalate action needs its escalate attribute",
		},
		{
			name: "a set and a block message template",
			body: `
  message_template = { value_literal = "01SETTEMPLATE" }

  block {
    id               = "page"
    message_template = { value_reference = "alert.attributes.template" }

    branch {
      id = "always"
    }
  }`,
		},
		{
			name:  "a list for the set's message template",
			body:  `message_template = { values = ["01TEMPLATE"] }`,
			errRe: "message_template takes a single value",
		},
		{
			name: "a list for a block's message template",
			body: `
  block {
    id               = "page"
    message_template = { array_value = [{ literal = "01TEMPLATE" }] }

    branch {
      id = "always"
    }
  }`,
			errRe: "message_template takes a single value",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := resource.TestStep{
				Config:             alertRulesValidateConfig(tc.body),
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
