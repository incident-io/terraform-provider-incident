package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccEscalationPathBeta covers an escalation path written as sequences,
// against the API: create, read back unchanged, import, and update.
//
// This resource had no acceptance test before now, and it is the shape v7 makes
// the only one. What it exercises is what the flat shape brought and the nested
// one couldn't: a branch naming the sequences to continue down, and a loop
// naming the node to go back to. Both are converted to and from the API's nested
// path on every read and write, which is the part most worth applying for real.
func TestAccEscalationPathBeta(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccEscalationPathBetaConfig(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "name", StableSuffix("acc-escalation-path")),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "start", "main"),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.%", "3"),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.main.nodes.#", "1"),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.main.nodes.0.branch.if.working_hours_active", "UK"),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.main.nodes.0.branch.then", "in_hours"),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.main.nodes.0.branch.else", "out_of_hours"),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.in_hours.nodes.1.loop.back_to", "start"),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.in_hours.nodes.1.loop.times", "2"),
					resource.TestCheckResourceAttrSet("incident_escalation_path_beta.test", "id"),
				),
			},
			// The sequence names are ours to choose, and the API stores a nested
			// path that doesn't carry them: a read keeps the names already in
			// state, and an import has no state to keep them from, so the names
			// it comes back with are derived. Everything else has to round-trip.
			{
				ResourceName:            "incident_escalation_path_beta.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"start", "sequences"},
			},
			// Dropping the loop and slowing the acknowledgement window: an
			// update that rewrites the path rather than adding to it, which is
			// where a conversion bug in either direction would show.
			{
				Config: testAccEscalationPathBetaConfigWithoutLoop(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.in_hours.nodes.#", "1"),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.test", "sequences.in_hours.nodes.0.level.time_to_ack_seconds", "600"),
				),
			},
		},
	})
}

// The schedule the path pages. A target needs no schedule_mode unless it names a
// rotation, so this is the simplest thing an escalation can point at.
const escalationPathBetaAcceptanceTarget = `
resource "incident_schedule_beta" "target" {
  name     = {{ stableSuffix "acc-path-schedule" | quote }}
  timezone = "Europe/London"
}
`

func testAccEscalationPathBetaConfig() string {
	return testRunTemplate("escalation_path_beta_acc", escalationPathBetaAcceptanceTarget+`
resource "incident_escalation_path_beta" "test" {
  name  = {{ stableSuffix "acc-escalation-path" | quote }}
  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          # Named so the loop below can point back here.
          id = "start"
          branch = {
            if = {
              working_hours_active = "UK"
            }
            then = "in_hours"
            else = "out_of_hours"
          }
        }
      ]
    }

    in_hours = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "schedule"
              id      = incident_schedule_beta.target.id
              urgency = "high"
            }]
            time_to_ack_seconds = 300
          }
        },
        {
          loop = {
            back_to = "start"
            times   = 2
          }
        }
      ]
    }

    out_of_hours = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "schedule"
              id      = incident_schedule_beta.target.id
              urgency = "low"
            }]
            time_to_ack_seconds = 300
          }
        }
      ]
    }
  }

  working_hours = [
    {
      id       = "UK"
      name     = "UK"
      timezone = "Europe/London"
      weekday_intervals = [
        {
          weekday    = "monday"
          start_time = "09:00"
          end_time   = "17:00"
        }
      ]
    }
  ]
}
`, nil)
}

func testAccEscalationPathBetaConfigWithoutLoop() string {
	return testRunTemplate("escalation_path_beta_acc_no_loop", escalationPathBetaAcceptanceTarget+`
resource "incident_escalation_path_beta" "test" {
  name  = {{ stableSuffix "acc-escalation-path" | quote }}
  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          id = "start"
          branch = {
            if = {
              working_hours_active = "UK"
            }
            then = "in_hours"
            else = "out_of_hours"
          }
        }
      ]
    }

    in_hours = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "schedule"
              id      = incident_schedule_beta.target.id
              urgency = "high"
            }]
            time_to_ack_seconds = 600
          }
        }
      ]
    }

    out_of_hours = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "schedule"
              id      = incident_schedule_beta.target.id
              urgency = "low"
            }]
            time_to_ack_seconds = 300
          }
        }
      ]
    }
  }

  working_hours = [
    {
      id       = "UK"
      name     = "UK"
      timezone = "Europe/London"
      weekday_intervals = [
        {
          weekday    = "monday"
          start_time = "09:00"
          end_time   = "17:00"
        }
      ]
    }
  ]
}
`, nil)
}
