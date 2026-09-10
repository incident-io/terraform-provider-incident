package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// These tests walk the migration onto the beta resources against a real account, using
// the `moved` blocks the movers exist for. They only make sense on this line, where the
// resource being moved off and the resource being moved to are both registered, and
// that is also where somebody doing this work runs it.
//
// Each one runs three steps:
//
//  1. Apply the configuration somebody is on before they migrate.
//  2. Apply the migration: the new resources, a `moved` block per resource that has one,
//     and an `import` block for each resource that split out of it.
//  3. Plan the settled configuration, with the blocks that drove the migration taken
//     out, and expect no changes.
//
// Step 2 asserts the moved resource plans as a no-op, which is the promise the mover
// makes: the object comes across rather than being created again, and the refresh that
// follows the move fills in what the mover left null. Step 3 asserts it settles, which
// is what somebody following the guide should see.
//
// The object's ID is compared across steps rather than checked for being set, because a
// migration that created a second object would set an ID too - just not the same one.

// TestAccMoveScheduleOntoBetaResources covers the case where one resource becomes
// several: the schedule moves, and its two rotations are imported, because a `moved`
// block has one target and there is nowhere to take a second from.
func TestAccMoveScheduleOntoBetaResources(t *testing.T) {
	scheduleID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMoveScheduleBefore(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule.moving", "rotations.#", "2"),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					scheduleID.AddStateValue("incident_schedule.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config: testAccMoveScheduleAfter(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						// The move is the whole point: a schedule that plans as anything
						// else here is one being created or updated rather than carried
						// across.
						plancheck.ExpectResourceAction("incident_schedule_beta.moving", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.primary", "name", "Primary"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.secondary", "name", "Secondary"),
					resource.TestCheckResourceAttrPair(
						"incident_schedule_rotation_beta.primary", "schedule_id",
						"incident_schedule_beta.moving", "id",
					),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					scheduleID.AddStateValue("incident_schedule_beta.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config:   testAccMoveScheduleSettled(),
				PlanOnly: true,
			},
		},
	})
}

// The schedule everyone has today: the rotation, its line-up and its handover cadence
// all inside the schedule resource.
//
// The line-up is the NOBODY sentinel rather than real users, so the test needs no
// account-specific IDs. What matters is that the old and new configurations agree on it,
// since a difference between them is a plan the migration shouldn't produce.
func testAccMoveScheduleBefore() string {
	return testRunTemplate("move_schedule_before", `
resource "incident_schedule" "moving" {
  name     = {{ stableSuffix "Moving schedule" | quote }}
  timezone = "Europe/London"

  rotations = [
    {
      id   = "primary"
      name = "Primary"

      versions = [{
        handover_start_at = "2024-01-08T09:00:00Z"
        users             = ["NOBODY"]
        layers            = [{ id = "primary", name = "Primary" }]
        handovers         = [{ interval = 1, interval_type = "weekly" }]
      }]
    },
    {
      id   = "secondary"
      name = "Secondary"

      versions = [{
        handover_start_at = "2024-01-08T09:00:00Z"
        users             = ["NOBODY"]
        layers            = [{ id = "secondary", name = "Secondary" }]
        handovers         = [{ interval = 1, interval_type = "daily" }]
      }]
    },
  ]
}
`, nil)
}

// scheduleMoveFixture is the configuration either side of the schedule's move, which is
// everything except the blocks that drive it: writing it once is what makes step 3 the
// same configuration as step 2 with those blocks deleted.
const scheduleMoveFixture = `
resource "incident_schedule_beta" "moving" {
  name     = {{ stableSuffix "Moving schedule" | quote }}
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

// The migration: the schedule moves, and each rotation is imported.
//
// A rotation's import ID is the schedule's ID and the rotation's, joined by a colon. The
// rotation IDs are the ones the old configuration chose, because the old resource sent
// them, so they are "primary" and "secondary" here rather than something looked up. The
// schedule's own ID is read from a data source: a test cannot know an ID that step 1
// created, and whoever does this by hand has it in their state already.
func testAccMoveScheduleAfter() string {
	return testRunTemplate("move_schedule_after", scheduleMoveFixture+`
data "incident_schedule_beta" "existing" {
  name = {{ stableSuffix "Moving schedule" | quote }}
}

moved {
  from = incident_schedule.moving
  to   = incident_schedule_beta.moving
}

import {
  to = incident_schedule_rotation_beta.primary
  id = "${data.incident_schedule_beta.existing.id}:primary"
}

import {
  to = incident_schedule_rotation_beta.secondary
  id = "${data.incident_schedule_beta.existing.id}:secondary"
}
`, nil)
}

func testAccMoveScheduleSettled() string {
	return testRunTemplate("move_schedule_settled", scheduleMoveFixture, nil)
}

// TestAccMoveEscalationPathOntoBetaResource covers the case a `moved` block handles on
// its own: one resource becomes one, with no field left in place. Nothing is imported
// here, so an empty plan at the end is the mover's work and the refresh's alone.
func TestAccMoveEscalationPathOntoBetaResource(t *testing.T) {
	pathID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMoveEscalationPathBefore(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_escalation_path.moving", "path.#", "1"),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					pathID.AddStateValue("incident_escalation_path.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config: testAccMoveEscalationPathAfter(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_escalation_path_beta.moving", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_escalation_path_beta.moving", "start", "main"),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					pathID.AddStateValue("incident_escalation_path_beta.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config:   testAccMoveEscalationPathSettled(),
				PlanOnly: true,
			},
		},
	})
}

// escalationPathMoveTarget is the schedule the path escalates to, which stays on the old
// resource throughout: migrating one thing at a time is what the guide suggests, and it
// keeps a failure here legible.
const escalationPathMoveTarget = `
resource "incident_schedule" "target" {
  name     = {{ stableSuffix "Moving path target" | quote }}
  timezone = "Europe/London"

  rotations = [{
    id   = "primary"
    name = "Primary"

    versions = [{
      handover_start_at = "2024-01-08T09:00:00Z"
      users             = ["NOBODY"]
      layers            = [{ id = "primary", name = "Primary" }]
      handovers         = [{ interval = 1, interval_type = "weekly" }]
    }]
  }]
}
`

func testAccMoveEscalationPathBefore() string {
	return testRunTemplate("move_escalation_path_before", escalationPathMoveTarget+`
resource "incident_escalation_path" "moving" {
  name = {{ stableSuffix "Moving path" | quote }}

  path = [
    {
      id   = "start"
      type = "level"
      level = {
        targets = [{
          type    = "schedule"
          id      = incident_schedule.target.id
          urgency = "high"
        }]
        time_to_ack_seconds = 300
      }
    }
  ]
}
`, nil)
}

// The same path as sequences.
//
// The sequence is named `main` and the node keeps the ID the old configuration gave it,
// because those are what the move settles on: the API does not store sequence names, so
// the refresh names the start sequence with the fallback, and it reports the node under
// the ID the old resource sent. A configuration naming either of them differently plans
// a change, which is what the settled step would catch.
const escalationPathMoveFixture = escalationPathMoveTarget + `
resource "incident_escalation_path_beta" "moving" {
  name  = {{ stableSuffix "Moving path" | quote }}
  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          id = "start"
          level = {
            targets = [{
              type    = "schedule"
              id      = incident_schedule.target.id
              urgency = "high"
            }]
            time_to_ack_seconds = 300
          }
        }
      ]
    }
  }
}
`

func testAccMoveEscalationPathAfter() string {
	return testRunTemplate("move_escalation_path_after", escalationPathMoveFixture+`
moved {
  from = incident_escalation_path.moving
  to   = incident_escalation_path_beta.moving
}
`, nil)
}

func testAccMoveEscalationPathSettled() string {
	return testRunTemplate("move_escalation_path_settled", escalationPathMoveFixture, nil)
}

// TestAccMoveAlertSourceOntoBetaResources covers the other one-becomes-several case: the
// source moves, and the binding it declared under template.attributes is imported.
//
// The title and description are the interesting part. They do not move - the v6 resource
// holds them under `template` - so the refresh reads them back from the API, and a
// template compares equal to the document it produces, which is what makes the settled
// plan empty rather than a diff nobody can get rid of.
func TestAccMoveAlertSourceOntoBetaResources(t *testing.T) {
	sourceID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMoveAlertSourceBefore(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source.moving", "template.attributes.#", "1"),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					sourceID.AddStateValue("incident_alert_source.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config: testAccMoveAlertSourceAfter(),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_alert_source_beta.moving", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_attribute_beta.environment", "value_literal", "production"),
					resource.TestCheckResourceAttrPair(
						"incident_alert_source_attribute_beta.environment", "alert_source_id",
						"incident_alert_source_beta.moving", "id",
					),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					sourceID.AddStateValue("incident_alert_source_beta.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config:   testAccMoveAlertSourceSettled(),
				PlanOnly: true,
			},
		},
	})
}

// alertSourceMoveAttribute is the attribute being bound and the document the source
// titles its alerts with: the part of the configuration the migration doesn't change.
const alertSourceMoveAttribute = `
resource "incident_alert_attribute" "environment" {
  name  = {{ stableSuffix "Moving environment" | quote }}
  type  = "String"
  array = false
}

locals {
  alert_title = jsonencode({
    type = "doc"
    content = [{
      type    = "paragraph"
      content = [{ type = "text", text = "Moving source alert" }]
    }]
  })
}
`

func testAccMoveAlertSourceBefore() string {
	return testRunTemplate("move_alert_source_before", alertSourceMoveAttribute+`
resource "incident_alert_source" "moving" {
  name        = {{ stableSuffix "Moving source" | quote }}
  source_type = "http"

  template = {
    # Required on the v6 resource, which keeps a source's named expressions here
    # rather than as blocks of their own.
    expressions = []

    title       = { literal = local.alert_title }
    description = { literal = local.alert_title }

    attributes = [{
      alert_attribute_id = incident_alert_attribute.environment.id
      binding = {
        value          = { literal = "production" }
        merge_strategy = "first_wins"
      }
    }]
  }
}
`, nil)
}

const alertSourceMoveFixture = alertSourceMoveAttribute + `
resource "incident_alert_source_beta" "moving" {
  name        = {{ stableSuffix "Moving source" | quote }}
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

// The binding's import ID is the source's ID and the attribute's, joined by a colon. The
// source's has to be found by name, because the singular data source looks a source up
// by ID and the plural one filters by type: hence the comprehension.
func testAccMoveAlertSourceAfter() string {
	return testRunTemplate("move_alert_source_after", alertSourceMoveFixture+`
data "incident_alert_sources" "all" {}

locals {
  moving_source_id = one([
    for source in data.incident_alert_sources.all.alert_sources :
    source.id if source.name == {{ stableSuffix "Moving source" | quote }}
  ])
}

moved {
  from = incident_alert_source.moving
  to   = incident_alert_source_beta.moving
}

import {
  to = incident_alert_source_attribute_beta.environment
  id = "${local.moving_source_id}:${incident_alert_attribute.environment.id}"
}
`, nil)
}

func testAccMoveAlertSourceSettled() string {
	return testRunTemplate("move_alert_source_settled", alertSourceMoveFixture, nil)
}
