package provider

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

const alertRulesAddress = "incident_alert_rules.test"

// TestAccIncidentAlertRulesResource needs an organisation on alert rules, as the API refuses
// every write otherwise, so it skips unless TF_ACC_ALERT_RULES is set. The message template
// step needs a live template, so it skips unless TF_ACC_ALERT_MESSAGE_TEMPLATE_ID names one.
func TestAccIncidentAlertRulesResource(t *testing.T) {
	if os.Getenv("TF_ACC_ALERT_RULES") == "" {
		t.Skip("No TF_ACC_ALERT_RULES environment variable set, skipping")
	}
	templateID := os.Getenv("TF_ACC_ALERT_MESSAGE_TEMPLATE_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentAlertRulesConfig("alert.title", false, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(alertRulesAddress, "id", regexp.MustCompile("^[a-zA-Z0-9]+$")),
					resource.TestCheckResourceAttr(alertRulesAddress, "kind", "team"),
					resource.TestCheckResourceAttrPair(alertRulesAddress, "team_id", "incident_catalog_entry.team", "id"),
					resource.TestCheckResourceAttr(alertRulesAddress, "version", "1"),
					resource.TestCheckResourceAttr(alertRulesAddress, "block.0.branch.0.condition_groups.0.conditions.0.subject", "alert.title"),
					resource.TestCheckResourceAttr(alertRulesAddress, "block.0.branch.0.actions.0.type", "create_incident"),
					resource.TestCheckResourceAttr(alertRulesAddress, "block.0.branch.0.actions.0.create_incident.auto_decline", "false"),
					resource.TestCheckResourceAttr(alertRulesAddress, "block.0.branch.0.actions.1.post_slack.channel_visibility", "private"),
					resource.TestCheckResourceAttr(alertRulesAddress, "block.0.branch.0.branch.0.id", "has-description"),
				),
			},
			// Refresh and ensure there's no perpetual diff.
			{
				RefreshState: true,
				PlanOnly:     true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:      alertRulesAddress,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Changing a condition and an action is an update in place, and writes the next version.
			{
				Config: testAccIncidentAlertRulesConfig("alert.description", true, ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(alertRulesAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(alertRulesAddress, "version", "2"),
					resource.TestCheckResourceAttr(alertRulesAddress, "block.0.branch.0.condition_groups.0.conditions.0.subject", "alert.description"),
					resource.TestCheckResourceAttr(alertRulesAddress, "block.0.branch.0.actions.0.create_incident.auto_decline", "true"),
				),
			},
			// A set-level template, and a block that overrides it, each keep their spelling.
			{
				SkipFunc: func() (bool, error) { return templateID == "", nil },
				Config:   testAccIncidentAlertRulesConfig("alert.description", true, templateID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(alertRulesAddress, "message_template.value.literal", templateID),
					resource.TestCheckResourceAttr(alertRulesAddress, "block.0.message_template.value_literal", templateID),
				),
			},
			// Leaving the templates out removes them, as the API does for anything an update omits.
			{
				SkipFunc: func() (bool, error) { return templateID == "", nil },
				Config:   testAccIncidentAlertRulesConfig("alert.description", true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(alertRulesAddress, "message_template.value.literal"),
					resource.TestCheckNoResourceAttr(alertRulesAddress, "block.0.message_template.value_literal"),
				),
			},
		},
	})
}

// testAccIncidentAlertRulesConfig creates a team of its own to hold the rules, because a team
// can have only one set and the shared test teams may already.
func testAccIncidentAlertRulesConfig(subject string, autoDecline bool, messageTemplateID string) string {
	return testRunTemplate("incident_alert_rules", `
data "incident_catalog_type" "team" {
  name = {{ .TeamTypeName | quote }}
}

resource "incident_catalog_entry" "team" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = {{ .TeamName | quote }}
  name               = {{ .TeamName | quote }}
  attribute_values   = []
  managed_attributes = []
}

resource "incident_alert_rules" "test" {
  kind    = "team"
  team_id = incident_catalog_entry.team.id
{{- with .MessageTemplateID }}
  message_template = { value = { literal = {{ . | quote }} } }
{{- end }}

  block {
    id = "create-incident"
{{- with .MessageTemplateID }}
    message_template = { value_literal = {{ . | quote }} }
{{- end }}

    branch {
      id = "has-subject"
      condition_groups = [{
        conditions = [{ subject = {{ .Subject | quote }}, operation = "is_set", param_bindings = [] }]
      }]
      actions = [{
        id              = "open-incident"
        type            = "create_incident"
        create_incident = { auto_decline = {{ .AutoDecline }} }
      }, {
        id   = "post-privately"
        type = "post_slack"
        post_slack = {
          channels           = { values = ["C0123456789"] }
          mode               = "per_alert"
          channel_visibility = "private"
        }
      }]

      branch {
        id = "has-description"
        condition_groups = [{
          conditions = [{ subject = "alert.description", operation = "is_set", param_bindings = [] }]
        }]
      }
    }
  }
}
`, struct {
		TeamTypeName      string
		TeamName          string
		Subject           string
		AutoDecline       bool
		MessageTemplateID string
	}{
		TeamTypeName:      teamTypeName(),
		TeamName:          StableSuffix("tf-acceptance-test-alert-rules-team"),
		Subject:           subject,
		AutoDecline:       autoDecline,
		MessageTemplateID: messageTemplateID,
	})
}
