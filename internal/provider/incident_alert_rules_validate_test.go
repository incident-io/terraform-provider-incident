package provider

import (
	"fmt"
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
func TestIncidentAlertRulesResource_ValidateConfig(t *testing.T) {
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
