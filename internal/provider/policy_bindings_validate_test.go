package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestPolicyAssigneeBindingsValidation runs plan-only steps against the two rules that
// decide where assignee bindings may go: the types that assign the user the finding is
// about take reminders but no bindings, and every other type needs them.
func TestPolicyAssigneeBindingsValidation(t *testing.T) {
	cases := []struct {
		name   string
		config string
		errRe  string
	}{
		{
			name: "readiness takes reminders without bindings",
			config: `
resource "incident_policy" "test" {
  name             = "Responders can be reached"
  description      = "Test"
  condition_groups = []

  assignment_rules = {
    reminder_due_date_offset_hours = [24]
    reminder_cadence_after         = { interval = "daily" }
  }

  on_call_readiness = {
    high_urgency = [{ method_types = ["phone"], max_delay_seconds = 300 }]
  }
}
`,
		},
		{
			name: "readiness rejects bindings",
			config: `
resource "incident_policy" "test" {
  name             = "Responders can be reached"
  description      = "Test"
  condition_groups = []

  assignment_rules = {
    bindings                       = [{ value_literal = "01USER" }]
    reminder_due_date_offset_hours = []
  }

  on_call_readiness = {
    high_urgency = [{ method_types = ["phone"], max_delay_seconds = 300 }]
  }
}
`,
			errRe: `cannot be configured together: \[on_call_readiness,assignment_rules.bindings\]`,
		},
		{
			name: "vacation conflict rejects bindings",
			config: `
resource "incident_policy" "test" {
  name             = "No on-call during vacation"
  description      = "Test"
  condition_groups = []

  assignment_rules = {
    bindings                       = [{ value_literal = "01USER" }]
    reminder_due_date_offset_hours = []
  }

  vacation_conflict = {}
}
`,
			errRe: `cannot be configured together: \[vacation_conflict,assignment_rules.bindings\]`,
		},
		{
			name: "schedule requires bindings",
			config: `
resource "incident_policy" "test" {
  name             = "On-call schedules have no gaps"
  description      = "Test"
  condition_groups = []

  assignment_rules = {
    reminder_due_date_offset_hours = []
  }

  schedule = {
    requirement_type = "contiguous"
  }
}
`,
			errRe: "Missing assignee bindings",
		},
		{
			name: "schedule takes bindings",
			config: `
resource "incident_policy" "test" {
  name             = "On-call schedules have no gaps"
  description      = "Test"
  condition_groups = []

  assignment_rules = {
    bindings                       = [{ value_literal = "01USER" }]
    reminder_due_date_offset_hours = []
  }

  schedule = {
    requirement_type = "contiguous"
  }
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			step := resource.TestStep{
				Config:   tc.config,
				PlanOnly: true,
			}
			if tc.errRe == "" {
				step.ExpectNonEmptyPlan = true
			} else {
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
