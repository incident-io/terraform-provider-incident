package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccEscalationPathTemplate creates a template, builds a path from it, rebinds the path,
// then edits the template. Every step has to leave an empty plan, which is where a binding
// or sequence that doesn't round-trip would show.
func TestAccEscalationPathTemplate(t *testing.T) {
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
					resource.TestCheckResourceAttr("incident_escalation_path_template.test", "params.#", "1"),
					resource.TestCheckResourceAttr("incident_escalation_path_template.test", "params.0.name", "primary_schedule"),
					resource.TestCheckResourceAttr("incident_escalation_path_template.test", "sequences.main.nodes.0.level.targets.0.binding.value_reference", "primary_schedule"),
					resource.TestCheckNoResourceAttr("incident_escalation_path_template.test", "sequences.main.nodes.0.level.targets.0.id"),
					resource.TestCheckResourceAttrSet("incident_escalation_path_template.test", "id"),

					resource.TestCheckResourceAttr("incident_escalation_path_beta.templated", "kind", "templated"),
					resource.TestCheckResourceAttrPair("incident_escalation_path_beta.templated", "template_id", "incident_escalation_path_template.test", "id"),
					resource.TestCheckResourceAttrPair("incident_escalation_path_beta.templated", "param_bindings.primary_schedule.value_literal", "incident_schedule_beta.first", "id"),
					resource.TestCheckNoResourceAttr("incident_escalation_path_beta.templated", "start"),
					resource.TestCheckNoResourceAttr("incident_escalation_path_beta.templated", "sequences.%"),
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
				ResourceName:      "incident_escalation_path_beta.templated",
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
					resource.TestCheckResourceAttrPair("incident_escalation_path_beta.templated", "param_bindings.primary_schedule.value_literal", "incident_schedule_beta.second", "id"),
				),
			},
		},
	})
}

const escalationPathTemplateAcceptanceSchedules = `
resource "incident_schedule_beta" "first" {
  name     = {{ stableSuffix "acc-template-schedule-1" | quote }}
  timezone = "Europe/London"
}

resource "incident_schedule_beta" "second" {
  name     = {{ stableSuffix "acc-template-schedule-2" | quote }}
  timezone = "Europe/London"
}

resource "incident_escalation_path_template" "test" {
  name        = {{ stableSuffix "acc-template" | quote }}
  description = "Pages the primary schedule"

  params = [{
    name  = "primary_schedule"
    label = "Primary schedule"
    type  = "CatalogEntry[\"Schedule\"]"
  }]

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
resource "incident_escalation_path_beta" "templated" {
  name        = {{ stableSuffix "acc-templated-path" | quote }}
  team_ids    = []
  template_id = incident_escalation_path_template.test.id

  param_bindings = {
    primary_schedule = { value_literal = incident_schedule_beta.first.id }
  }
}
`, map[string]any{"TimeToAck": timeToAck})
}

func testAccEscalationPathTemplateConfigRebound(timeToAck int) string {
	return testRunTemplate("escalation_path_template_acc_rebound", escalationPathTemplateAcceptanceSchedules+`
resource "incident_escalation_path_beta" "templated" {
  name        = {{ stableSuffix "acc-templated-path" | quote }}
  team_ids    = []
  template_id = incident_escalation_path_template.test.id

  param_bindings = {
    primary_schedule = { value_literal = incident_schedule_beta.second.id }
  }
}
`, map[string]any{"TimeToAck": timeToAck})
}
