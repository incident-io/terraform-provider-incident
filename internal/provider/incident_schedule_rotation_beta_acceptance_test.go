package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccIncidentScheduleRotationBeta covers a rotation's lifecycle against the
// API: create, read back unchanged, import by its compound ID, and update.
//
// The rotation is the resource that made splitting schedules worth doing, and
// the controls it brought - concurrent_shifts, working_intervals, rank,
// scheduling_mode - are the ones the inline shape had no way to express, so they
// are what this exercises. Like the schedule, it had no acceptance test before
// now.
//
// The line-up is the NOBODY sentinel throughout: it needs no account-specific
// user IDs, and a rotation nobody is in still schedules its shifts.
func TestAccIncidentScheduleRotationBeta(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccIncidentScheduleRotationBetaConfig(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "name", "Primary"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "users.#", "1"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "users.0", "NOBODY"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "handovers.#", "1"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "handovers.0.interval_type", "weekly"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "rank", "1"),
					resource.TestCheckResourceAttrPair(
						"incident_schedule_rotation_beta.test", "schedule_id",
						"incident_schedule_beta.test", "id",
					),
					resource.TestCheckResourceAttrSet("incident_schedule_rotation_beta.test", "id"),
				),
			},
			// A rotation is nested under its schedule in the API, so it imports
			// by both IDs rather than by its own.
			//
			// rank is Optional and not Computed, so it is only tracked when the
			// config asks for it, and an import has no config to ask: the
			// rotation comes back unordered rather than adopting the position it
			// happens to hold, which is the point of not computing it.
			{
				ResourceName:            "incident_schedule_rotation_beta.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"rank"},
				ImportStateIdFunc:       importScheduleRotationBetaStateIDFunc("incident_schedule_rotation_beta.test"),
			},
			// The controls the inline shape had no way to express: two people on
			// call at once, restricted to weekday working hours, allocated in a
			// running order obvious to the people in it.
			{
				Config: testAccIncidentScheduleRotationBetaConfigWithWorkingIntervals(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "concurrent_shifts", "2"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "scheduling_mode", "sequential"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "working_intervals.#", "2"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "working_intervals.0.weekday", "monday"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "users.#", "3"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "handovers.0.interval_type", "daily"),
				),
			},
			// Omitting working_intervals puts the rotation back on call around
			// the clock, which an empty list is explicitly not a way to say.
			{
				Config: testAccIncidentScheduleRotationBetaConfig(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("incident_schedule_rotation_beta.test", "working_intervals.#"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.test", "users.#", "1"),
				),
			},
		},
	})
}

// importScheduleRotationBetaStateIDFunc builds the <schedule_id>:<rotation_id>
// an import takes, out of the state the previous step left.
func importScheduleRotationBetaStateIDFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("%s is not in state", resourceName)
		}

		return rs.Primary.Attributes["schedule_id"] + ":" + rs.Primary.Attributes["id"], nil
	}
}

const scheduleRotationBetaAcceptanceSchedule = `
resource "incident_schedule_beta" "test" {
  name     = {{ stableSuffix "acc-rotation-schedule" | quote }}
  timezone = "Europe/London"
}
`

// rank is set rather than left out so that the configuration and what the API
// reports agree: it is Optional and not Computed, so a rotation that has never
// been ordered reads back unset, and the two only match if the config says
// which it is.
func testAccIncidentScheduleRotationBetaConfig() string {
	return testRunTemplate("incident_schedule_rotation_beta_acc", scheduleRotationBetaAcceptanceSchedule+`
resource "incident_schedule_rotation_beta" "test" {
  schedule_id = incident_schedule_beta.test.id
  name        = "Primary"
  rank        = 1

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "weekly"
  }]
}
`, nil)
}

func testAccIncidentScheduleRotationBetaConfigWithWorkingIntervals() string {
	return testRunTemplate("incident_schedule_rotation_beta_acc_intervals", scheduleRotationBetaAcceptanceSchedule+`
resource "incident_schedule_rotation_beta" "test" {
  schedule_id = incident_schedule_beta.test.id
  name        = "Primary"
  rank        = 1

  users = ["NOBODY", "NOBODY", "NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "daily"
  }]

  concurrent_shifts = 2
  scheduling_mode   = "sequential"

  working_intervals = [
    { weekday = "monday", start_time = "09:00", end_time = "17:00" },
    { weekday = "tuesday", start_time = "09:00", end_time = "17:00" },
  ]
}
`, nil)
}
