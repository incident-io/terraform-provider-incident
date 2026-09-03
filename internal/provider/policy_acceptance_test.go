package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestAccIncidentPolicyPostMortem covers the shape with the most to get wrong: an
// incident-scoped policy carrying requirements, a due date and an assignee.
//
// The empty plan after apply is the point: the API stores a scalar assignee binding as a
// one-element array and answers with the array, which only a real API produces.
func TestAccIncidentPolicyPostMortem(t *testing.T) {
	// The lookups below run before resource.Test, so honour TF_ACC ourselves rather than
	// calling the API during a unit test run, then initialise testClient.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}
	testAccPreCheck(t)

	// Neither a user nor a timestamp can be created from Terraform, so the test asks the
	// account what it has rather than hardcoding one.
	email := testAccUniqueActiveUserEmail(t)
	timestampID := testAccIncidentTimestamp(t).Id

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyPostMortemConfig(email, timestampID, 5),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("incident_policy.post_mortems", "id"),
					// Computed from the block rather than written, so a read that stops
					// deriving it shows up here.
					resource.TestCheckResourceAttr("incident_policy.post_mortems", "policy_type", "post_mortem"),
					// Defaulted, not configured.
					resource.TestCheckResourceAttr("incident_policy.post_mortems", "status", "enabled"),
					// The shorthand the config wrote, not the long form the API answers
					// with: this is ReconcileScalarBinding doing its job.
					resource.TestCheckResourceAttrPair(
						"incident_policy.post_mortems", "assignment_rules.bindings.0.value_literal",
						"data.incident_user.assignee", "id"),
					resource.TestCheckResourceAttr("incident_policy.post_mortems",
						"post_mortem.due_date_config.days.value_literal", "5"),
					// Direction is which field holds the cadence, so a read that swapped
					// them would still look valid without these.
					resource.TestCheckResourceAttr("incident_policy.post_mortems",
						"assignment_rules.reminder_cadence_before.interval", "weekly"),
					resource.TestCheckResourceAttr("incident_policy.post_mortems",
						"assignment_rules.reminder_cadence_after.interval", "daily"),
				),
			},
			{
				ResourceName:      "incident_policy.post_mortems",
				ImportState:       true,
				ImportStateVerify: true,
				// An import has no config to match, so bindings come back in the long
				// form rather than the shorthand. The first apply puts the spelling back.
				ImportStateVerifyIgnore: []string{
					"assignment_rules.bindings.0.value",
					"assignment_rules.bindings.0.array_value",
					"post_mortem.due_date_config.days.value",
					"post_mortem.requirements.0.conditions.0.param_bindings.0.value",
					"post_mortem.requirements.0.conditions.0.param_bindings.0.array_value",
				},
			},
			{
				// Editing a block in place is an ordinary update, not a replacement: only
				// adding or removing one changes the policy type.
				Config: testAccPolicyPostMortemConfig(email, timestampID, 10),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_policy.post_mortems",
						"post_mortem.due_date_config.days.value_literal", "10"),
				),
			},
		},
	})
}

// TestAccIncidentPolicyOnCallReadiness covers the type whose assignee the API fills in
// itself. A read that kept those rules would return assignment_rules the config never asked
// for and fail the apply as an inconsistent result, which the empty plan is what catches.
func TestAccIncidentPolicyOnCallReadiness(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyOnCallReadinessConfig(300),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_policy.readiness", "policy_type", "on_call_readiness"),
					// The API assigns the user in violation, and the resource drops the
					// rules it invents so they never reach state.
					resource.TestCheckNoResourceAttr("incident_policy.readiness", "assignment_rules.bindings.#"),
					resource.TestCheckResourceAttr("incident_policy.readiness",
						"on_call_readiness.high_urgency.0.max_delay_seconds", "300"),
				),
			},
			{
				ResourceName:      "incident_policy.readiness",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccPolicyOnCallReadinessConfig(600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_policy.readiness",
						"on_call_readiness.high_urgency.0.max_delay_seconds", "600"),
				),
			},
		},
	})
}

// TestAccIncidentPolicyTypeChangeReplaces asserts the replacement rule: swapping which
// block is set is the only way a policy's type can change, and the API refuses to change
// the type of an existing policy, so Terraform has to make a new one.
func TestAccIncidentPolicyTypeChangeReplaces(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyOnCallReadinessConfig(300),
				Check: resource.TestCheckResourceAttr(
					"incident_policy.readiness", "policy_type", "on_call_readiness"),
			},
			{
				Config: testAccPolicyVacationConflictConfig(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_policy.readiness", plancheck.ResourceActionReplace),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(
					"incident_policy.readiness", "policy_type", "vacation_conflict"),
			},
		},
	})
}

func testAccPolicyPostMortemConfig(email, timestampID string, days int) string {
	return testRunTemplate("incident_policy_post_mortem", `
data "incident_user" "assignee" {
  email = {{ quote .Email }}
}

resource "incident_policy" "post_mortems" {
  name        = {{ quote .Name }}
  description = "Closed incidents need a post-mortem before the due date passes."

  # Empty rather than absent: the API requires the key, and an empty list applies the
  # policy to every incident.
  condition_groups = []

  # value_literal is a provider-only shorthand. The API stores this scalar as a
  # one-element array and answers with the long form, so it reading back unchanged is
  # what proves the reconcile works.
  assignment_rules = {
    bindings                       = [{ value_literal = data.incident_user.assignee.id }]
    reminder_due_date_offset_hours = [-24, 0, 24]

    # Recurring reminders, in both directions. The empty plan below is what proves
    # each reads back as the direction it was written in.
    reminder_cadence_before = { interval = "weekly" }
    reminder_cadence_after  = { interval = "daily" }
  }

  # The API rejects an empty requirements list: a policy that requires nothing could
  # never find an incident non-compliant.
  post_mortem = {
    requirements = [
      {
        conditions = [
          {
            subject        = "post_mortem.status"
            operation      = "one_of"
            param_bindings = [{ values = ["complete"] }]
          }
        ]
      }
    ]

    due_date_config = {
      incident_timestamp_id = {{ quote .TimestampID }}
      days                  = { value_literal = {{ quote .Days }} }
      calculation_type      = "weekdays"
    }
  }
}
`, struct {
		Name        string
		Email       string
		TimestampID string
		Days        int
	}{
		Name:        StableSuffix("Post-mortems within a few working days"),
		Email:       email,
		TimestampID: timestampID,
		Days:        days,
	})
}

func testAccPolicyOnCallReadinessConfig(maxDelaySeconds int) string {
	return testRunTemplate("incident_policy_on_call_readiness", `
resource "incident_policy" "readiness" {
  name        = {{ quote .Name }}
  description = "Anyone on call needs a notification method that reaches them quickly."

  # No policy_type: the block below is what makes this an on-call readiness policy.

  # Empty rather than absent: an empty list means the policy applies to everyone.
  condition_groups = []

  # No assignment_rules: this type always assigns the user in violation, and the API
  # fills the binding in itself. The Conflicting validator rejects one here.
  on_call_readiness = {
    high_urgency = [
      {
        method_types      = ["email"]
        max_delay_seconds = {{ .MaxDelaySeconds }}
      }
    ]
  }
}
`, struct {
		Name            string
		MaxDelaySeconds int
	}{
		Name:            StableSuffix("Responders can be reached"),
		MaxDelaySeconds: maxDelaySeconds,
	})
}

// testAccPolicyVacationConflictConfig keeps the same resource address as the readiness
// config, so swapping between the two is a type change rather than a second policy.
func testAccPolicyVacationConflictConfig() string {
	return testRunTemplate("incident_policy_vacation_conflict", `
resource "incident_policy" "readiness" {
  name        = {{ quote .Name }}
  description = "Flag responders rota'd on while they are away."

  condition_groups = []

  # Empty because the type has nothing to configure: the block is only here to say
  # which type this is.
  vacation_conflict = {}
}
`, struct{ Name string }{Name: StableSuffix("Responders can be reached")})
}
