package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccEscalationPathTemplate creates a template, builds a path from it, rebinds the path,
// then edits the template. Every step has to leave an empty plan, which is where a binding
// or sequence that doesn't round-trip would show.
func TestAccEscalationPathTemplate(t *testing.T) {
	// Escalation path templates are not on every incident.io yet, and the shared test
	// account runs whatever is released rather than what is merged. Without the gate this
	// fails as a 404 on every CI run until the API ships.
	if os.Getenv("TF_ACC_ESCALATION_PATH_TEMPLATES") == "" {
		t.Skip("TF_ACC_ESCALATION_PATH_TEMPLATES is not set: skipping test that requires escalation path templates")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccEscalationPathTemplateConfig(300),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_escalation_path_template.test", "name", StableSuffix("acc-template")),
					resource.TestCheckResourceAttr("incident_escalation_path_template.test", "params.%", "1"),
					resource.TestCheckResourceAttr("incident_escalation_path_template.test", "params.primary_schedule.label", "Primary schedule"),
					resource.TestCheckResourceAttr("incident_escalation_path_template.test", "sequences.main.nodes.0.level.targets.0.binding.value_reference", "primary_schedule"),
					resource.TestCheckNoResourceAttr("incident_escalation_path_template.test", "sequences.main.nodes.0.level.targets.0.id"),
					resource.TestCheckResourceAttrSet("incident_escalation_path_template.test", "id"),

					resource.TestCheckResourceAttr("incident_escalation_path.templated", "kind", "templated"),
					resource.TestCheckResourceAttrPair("incident_escalation_path.templated", "template_id", "incident_escalation_path_template.test", "id"),
					resource.TestCheckResourceAttrPair("incident_escalation_path.templated", "param_bindings.primary_schedule.value_literal", "incident_schedule.first", "id"),
					resource.TestCheckNoResourceAttr("incident_escalation_path.templated", "start"),
					resource.TestCheckNoResourceAttr("incident_escalation_path.templated", "sequences.%"),
				),
			},
			{
				ResourceName:      "incident_escalation_path_template.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Sequence names and binding spellings are ours, so an import derives them.
				ImportStateVerifyIgnore: []string{"start", "sequences"},
			},
			{
				ResourceName:      "incident_escalation_path.templated",
				ImportState:       true,
				ImportStateVerify: true,
				// A binding's spelling is ours; the import reads the long form back.
				ImportStateVerifyIgnore: []string{"param_bindings"},
			},
			// Rebinding the path to another schedule, and slowing the template's level: an
			// update on each side of the pair.
			{
				Config: testAccEscalationPathTemplateConfigRebound(600),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_escalation_path_template.test", "sequences.main.nodes.0.level.time_to_ack_seconds", "600"),
					resource.TestCheckResourceAttrPair("incident_escalation_path.templated", "param_bindings.primary_schedule.value_literal", "incident_schedule.second", "id"),
				),
			},
		},
	})
}

const escalationPathTemplateAcceptanceSchedules = `
resource "incident_schedule" "first" {
  name     = {{ stableSuffix "acc-template-schedule-1" | quote }}
  timezone = "Europe/London"
}

resource "incident_schedule" "second" {
  name     = {{ stableSuffix "acc-template-schedule-2" | quote }}
  timezone = "Europe/London"
}

resource "incident_escalation_path_template" "test" {
  name        = {{ stableSuffix "acc-template" | quote }}
  description = "Pages the primary schedule"

  params = {
    primary_schedule = {
      label = "Primary schedule"
      type  = "CatalogEntry[\"Schedule\"]"
    }
  }

  start = "main"

  sequences = {
    main = {
      nodes = [{
        level = {
          targets = [{
            type    = "schedule"
            urgency = "high"
            binding = { value_reference = "primary_schedule" }
          }]
          time_to_ack_seconds = {{ .TimeToAck }}
        }
      }]
    }
  }
}
`

func testAccEscalationPathTemplateConfig(timeToAck int) string {
	return testRunTemplate("escalation_path_template_acc", escalationPathTemplateAcceptanceSchedules+`
resource "incident_escalation_path" "templated" {
  name        = {{ stableSuffix "acc-templated-path" | quote }}
  team_ids    = []
  kind        = "templated"
  template_id = incident_escalation_path_template.test.id

  param_bindings = {
    primary_schedule = { value_literal = incident_schedule.first.id }
  }
}
`, map[string]any{"TimeToAck": timeToAck})
}

func testAccEscalationPathTemplateConfigRebound(timeToAck int) string {
	return testRunTemplate("escalation_path_template_acc_rebound", escalationPathTemplateAcceptanceSchedules+`
resource "incident_escalation_path" "templated" {
  name        = {{ stableSuffix "acc-templated-path" | quote }}
  team_ids    = []
  kind        = "templated"
  template_id = incident_escalation_path_template.test.id

  param_bindings = {
    primary_schedule = { value_literal = incident_schedule.second.id }
  }
}
`, map[string]any{"TimeToAck": timeToAck})
}
