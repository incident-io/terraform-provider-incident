package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// These tests walk the v6-to-v7 rename against a real account, using the `moved` blocks
// the movers exist for.
//
// Each one runs three steps:
//
//  1. Apply the configuration somebody is on before they rename: the `_beta` names.
//  2. Apply the rename: the same configuration under the new names, plus a `moved` block
//     for each resource.
//  3. Plan the settled configuration, with the `moved` blocks taken out, and expect no
//     changes.
//
// Step 2 asserts the moved resource plans as a no-op, which is the promise the mover
// makes: the object comes across rather than being created again. Step 3 asserts it
// settles, which is what somebody following the guide should see.
//
// The object's ID is compared across steps rather than checked for being set, because a
// rename that created a second object would set an ID too - just not the same one.
//
// The two configurations are one fixture with the names rewritten rather than two written
// out, because being the same configuration under a different name is the entire claim
// these tests exist to check.

// betaNames are the `_beta` type names, longest first so that rewriting one doesn't leave
// the tail of another behind.
var betaNames = []string{
	"incident_alert_source_attribute_beta",
	"incident_alert_source_beta",
	"incident_escalation_path_beta",
	"incident_schedule_rotation_beta",
	"incident_schedule_beta",
}

// renamed rewrites a configuration written against the `_beta` names to use the names
// those resources answer to now.
func renamed(config string) string {
	for _, name := range betaNames {
		config = strings.ReplaceAll(config, name, strings.TrimSuffix(name, "_beta"))
	}

	return config
}

// TestAccRenameScheduleAndRotations covers a schedule and the rotations on it, so the
// rename of a resource that references another is exercised alongside the resources
// themselves: the rotations' schedule_id has to keep pointing at the moved schedule.
func TestAccRenameScheduleAndRotations(t *testing.T) {
	scheduleID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testRunTemplate("rename_schedule_before", scheduleRenameFixture, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					scheduleID.AddStateValue("incident_schedule_beta.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config: testRunTemplate("rename_schedule_after",
					renamed(scheduleRenameFixture)+scheduleRenameMovedBlocks, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						// The move is the whole point: anything that plans as more than a
						// no-op here is being created or updated rather than carried across.
						plancheck.ExpectResourceAction("incident_schedule.moving", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("incident_schedule_rotation.primary", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("incident_schedule_rotation.secondary", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_rotation.primary", "name", "Primary"),
					resource.TestCheckResourceAttrPair(
						"incident_schedule_rotation.primary", "schedule_id",
						"incident_schedule.moving", "id",
					),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					scheduleID.AddStateValue("incident_schedule.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config:   testRunTemplate("rename_schedule_settled", renamed(scheduleRenameFixture), nil),
				PlanOnly: true,
			},
		},
	})
}

// The line-up is the NOBODY sentinel rather than real users, so the test needs no
// account-specific IDs.
const scheduleRenameFixture = `
resource "incident_schedule_beta" "moving" {
  name     = {{ stableSuffix "Renaming schedule" | quote }}
  timezone = "Europe/London"
}

resource "incident_schedule_rotation_beta" "primary" {
  schedule_id = incident_schedule_beta.moving.id
  name        = "Primary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "weekly"
  }]
}

resource "incident_schedule_rotation_beta" "secondary" {
  schedule_id = incident_schedule_beta.moving.id
  name        = "Secondary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "daily"
  }]
}
`

const scheduleRenameMovedBlocks = `
moved {
  from = incident_schedule_beta.moving
  to   = incident_schedule.moving
}

moved {
  from = incident_schedule_rotation_beta.primary
  to   = incident_schedule_rotation.primary
}

moved {
  from = incident_schedule_rotation_beta.secondary
  to   = incident_schedule_rotation.secondary
}
`

// TestAccRenameEscalationPath covers the resource whose state holds the most structure:
// a path's sequences are a map of nodes, and a mover that dropped them would show up here
// as the settled plan asking to rebuild the path.
func TestAccRenameEscalationPath(t *testing.T) {
	pathID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testRunTemplate("rename_escalation_path_before", escalationPathRenameFixture, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					pathID.AddStateValue("incident_escalation_path_beta.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config: testRunTemplate("rename_escalation_path_after",
					renamed(escalationPathRenameFixture)+`
moved {
  from = incident_escalation_path_beta.moving
  to   = incident_escalation_path.moving
}
`, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_escalation_path.moving", plancheck.ResourceActionNoop),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					pathID.AddStateValue("incident_escalation_path.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config:   testRunTemplate("rename_escalation_path_settled", renamed(escalationPathRenameFixture), nil),
				PlanOnly: true,
			},
		},
	})
}

const escalationPathRenameFixture = `
resource "incident_schedule_beta" "target" {
  name     = {{ stableSuffix "Renaming path schedule" | quote }}
  timezone = "Europe/London"
}

resource "incident_escalation_path_beta" "moving" {
  name  = {{ stableSuffix "Renaming path" | quote }}
  start = "main"

  sequences = {
    main = {
      nodes = [{
        level = {
          targets = [{
            id            = incident_schedule_beta.target.id
            type          = "schedule"
            urgency       = "high"
            schedule_mode = "currently_on_call"
          }]
          time_to_ack_seconds = 300
          ack_mode            = "all"
        }
      }]
    }
  }
}
`

// TestAccRenameAlertSourceAndAttribute covers the source and one of its attribute
// bindings, which is the binding's own rename as well as the source's. The binding is
// the one resource here with no `id`: it is keyed by the source and the attribute, so a
// mover that only carried an `id` would refuse the move outright.
func TestAccRenameAlertSourceAndAttribute(t *testing.T) {
	sourceID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testRunTemplate("rename_alert_source_before", alertSourceRenameFixture, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					sourceID.AddStateValue("incident_alert_source_beta.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config: testRunTemplate("rename_alert_source_after",
					renamed(alertSourceRenameFixture)+`
moved {
  from = incident_alert_source_beta.moving
  to   = incident_alert_source.moving
}

moved {
  from = incident_alert_source_attribute_beta.environment
  to   = incident_alert_source_attribute.environment
}
`, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_alert_source.moving", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("incident_alert_source_attribute.environment", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_attribute.environment", "value_literal", "production"),
					resource.TestCheckResourceAttrPair(
						"incident_alert_source_attribute.environment", "alert_source_id",
						"incident_alert_source.moving", "id",
					),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					sourceID.AddStateValue("incident_alert_source.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config:   testRunTemplate("rename_alert_source_settled", renamed(alertSourceRenameFixture), nil),
				PlanOnly: true,
			},
		},
	})
}

const alertSourceRenameFixture = `
resource "incident_alert_attribute" "environment" {
  name  = {{ stableSuffix "Renaming environment" | quote }}
  type  = "String"
  array = false
}

locals {
  alert_title = jsonencode({
    type = "doc"
    content = [{
      type    = "paragraph"
      content = [{ type = "text", text = "Renaming source alert" }]
    }]
  })
}

resource "incident_alert_source_beta" "moving" {
  name        = {{ stableSuffix "Renaming source" | quote }}
  source_type = "http"

  title       = { literal = local.alert_title }
  description = { literal = local.alert_title }
}

resource "incident_alert_source_attribute_beta" "environment" {
  alert_source_id    = incident_alert_source_beta.moving.id
  alert_attribute_id = incident_alert_attribute.environment.id

  value_literal  = "production"
  merge_strategy = "first_wins"
}
`
