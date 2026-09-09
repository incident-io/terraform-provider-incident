package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// These tests walk the migration the v7 guide describes, against a real account,
// and they only make sense on this line: the migration is done on v6, where the
// resource being moved off and the resource being moved to are both registered.
// v7 removes the former, so nothing there can create the state these start from.
//
// Each one runs three steps, which are the three the guide asks for:
//
//  1. Apply the configuration somebody is on before they migrate.
//  2. Apply the migration: the new resources, an import block per resource
//     claiming the object that already exists, and a removed block that stops
//     managing the old resource without destroying it.
//  3. Plan the settled configuration, with the import and removed blocks taken
//     out, and expect no changes.
//
// Step 3 is the one that matters. A migration that recreated anything, or that
// mapped a field wrongly, shows up as a plan that isn't empty - which is exactly
// what someone following the guide would see, and what they should not.
//
// The import IDs come from data sources rather than being written out, because a
// test cannot know an ID that step 1 creates. Anyone doing this by hand reads
// them out of their own state, as the guide says.

// TestAccMigrateScheduleOffInlineRotations covers the case where one resource
// becomes several: a schedule with two rotations declared inline becomes three
// resources, and the schedule underneath is the same one throughout.
func TestAccMigrateScheduleOffInlineRotations(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMigrateScheduleBefore(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_schedule.migrating", "rotations.#", "2"),
					resource.TestCheckResourceAttrSet("incident_schedule.migrating", "id"),
				),
			},
			{
				Config: testAccMigrateScheduleAfter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// The same schedule, under the new resource: if the import
					// didn't claim it, this ID belongs to a second one.
					resource.TestCheckResourceAttrPair(
						"incident_schedule_beta.migrating", "id",
						"data.incident_schedule_beta.existing", "id",
					),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.primary", "name", "Primary"),
					resource.TestCheckResourceAttr("incident_schedule_rotation_beta.secondary", "name", "Secondary"),
					resource.TestCheckResourceAttrPair(
						"incident_schedule_rotation_beta.primary", "schedule_id",
						"incident_schedule_beta.migrating", "id",
					),
				),
			},
			{
				Config:   testAccMigrateScheduleSettled(),
				PlanOnly: true,
			},
		},
	})
}

// The schedule everyone has today: the rotation, its line-up and its handover
// cadence all inside the schedule resource.
//
// The line-up is the NOBODY sentinel rather than real users, so the test needs
// no account-specific IDs. If the older API rejects it, put a user ID here and
// in the migrated rotation - what matters is that the two agree, since a
// difference between them is a plan the migration shouldn't produce.
func testAccMigrateScheduleBefore() string {
	return testRunTemplate("migrate_schedule_before", `
resource "incident_schedule" "migrating" {
  name     = {{ stableSuffix "Migrating schedule" | quote }}
  timezone = "Europe/London"

  rotations = [
    {
      id   = "primary"
      name = "Primary"

      versions = [{
        handover_start_at = "2024-01-08T09:00:00Z"
        users             = ["NOBODY"]
        layers = [{
          id   = "primary"
          name = "Primary"
        }]
        handovers = [{
          interval      = 1
          interval_type = "weekly"
        }]
      }]
    },
    {
      id   = "secondary"
      name = "Secondary"

      versions = [{
        handover_start_at = "2024-01-08T09:00:00Z"
        users             = ["NOBODY"]
        layers = [{
          id   = "secondary"
          name = "Secondary"
        }]
        handovers = [{
          interval      = 1
          interval_type = "daily"
        }]
      }]
    },
  ]
}
`, nil)
}

// The migration: the new resources, the imports that claim what already exists,
// and the removed block that lets go of the old resource without deleting the
// schedule under it.
//
// A rotation's import ID is the schedule's ID and the rotation's, joined by a
// colon. The rotation's ID is the one the old configuration chose - the old
// resource sent it - so it is "primary" here rather than something looked up.
func testAccMigrateScheduleAfter() string {
	return testRunTemplate("migrate_schedule_after", `
data "incident_schedule_beta" "existing" {
  name = {{ stableSuffix "Migrating schedule" | quote }}
}

resource "incident_schedule_beta" "migrating" {
  name     = {{ stableSuffix "Migrating schedule" | quote }}
  timezone = "Europe/London"
}

resource "incident_schedule_rotation_beta" "primary" {
  schedule_id = incident_schedule_beta.migrating.id
  name        = "Primary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "weekly"
  }]
}

resource "incident_schedule_rotation_beta" "secondary" {
  schedule_id = incident_schedule_beta.migrating.id
  name        = "Secondary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "daily"
  }]
}

import {
  to = incident_schedule_beta.migrating
  id = data.incident_schedule_beta.existing.id
}

import {
  to = incident_schedule_rotation_beta.primary
  id = "${data.incident_schedule_beta.existing.id}:primary"
}

import {
  to = incident_schedule_rotation_beta.secondary
  id = "${data.incident_schedule_beta.existing.id}:secondary"
}

removed {
  from = incident_schedule.migrating

  lifecycle {
    destroy = false
  }
}
`, nil)
}

// What the configuration looks like once the migration is done and the blocks
// that drove it are deleted. Planning this has to report no changes.
func testAccMigrateScheduleSettled() string {
	return testRunTemplate("migrate_schedule_settled", `
data "incident_schedule_beta" "existing" {
  name = {{ stableSuffix "Migrating schedule" | quote }}
}

resource "incident_schedule_beta" "migrating" {
  name     = {{ stableSuffix "Migrating schedule" | quote }}
  timezone = "Europe/London"
}

resource "incident_schedule_rotation_beta" "primary" {
  schedule_id = incident_schedule_beta.migrating.id
  name        = "Primary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "weekly"
  }]
}

resource "incident_schedule_rotation_beta" "secondary" {
  schedule_id = incident_schedule_beta.migrating.id
  name        = "Secondary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"
  handovers = [{
    interval      = 1
    interval_type = "daily"
  }]
}
`, nil)
}

// TestAccMigrateEscalationPathOffNestedPath covers the case where one resource
// becomes one: the same escalation path, written as sequences instead of a
// nested path. The state moves by import rather than by a moved block, because
// the two resources hold their nodes in shapes nothing can translate between.
func TestAccMigrateEscalationPathOffNestedPath(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMigrateEscalationPathBefore(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("incident_escalation_path.migrating", "id"),
					resource.TestCheckResourceAttr("incident_escalation_path.migrating", "path.#", "1"),
				),
			},
			{
				Config: testAccMigrateEscalationPathAfter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"incident_escalation_path_beta.migrating", "id",
						"data.incident_escalation_path_beta.existing", "id",
					),
					resource.TestCheckResourceAttr("incident_escalation_path_beta.migrating", "start", "main"),
				),
			},
			{
				Config:   testAccMigrateEscalationPathSettled(),
				PlanOnly: true,
			},
		},
	})
}

func testAccMigrateEscalationPathBefore() string {
	return testRunTemplate("migrate_escalation_path_before", `
resource "incident_schedule" "target" {
  name     = {{ stableSuffix "Migrating path target" | quote }}
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

resource "incident_escalation_path" "migrating" {
  name = {{ stableSuffix "Migrating path" | quote }}

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

// The same path as sequences: one sequence, named by start, holding the level
// the old path held. The schedule it targets is left on the old resource, so
// this step migrates one thing at a time - which is what the guide suggests,
// and what keeps a failure legible.
func testAccMigrateEscalationPathAfter() string {
	return testRunTemplate("migrate_escalation_path_after", `
resource "incident_schedule" "target" {
  name     = {{ stableSuffix "Migrating path target" | quote }}
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

data "incident_escalation_path_beta" "existing" {
  name = {{ stableSuffix "Migrating path" | quote }}
}

resource "incident_escalation_path_beta" "migrating" {
  name  = {{ stableSuffix "Migrating path" | quote }}
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

import {
  to = incident_escalation_path_beta.migrating
  id = data.incident_escalation_path_beta.existing.id
}

removed {
  from = incident_escalation_path.migrating

  lifecycle {
    destroy = false
  }
}
`, nil)
}

func testAccMigrateEscalationPathSettled() string {
	return testRunTemplate("migrate_escalation_path_settled", `
resource "incident_schedule" "target" {
  name     = {{ stableSuffix "Migrating path target" | quote }}
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

data "incident_escalation_path_beta" "existing" {
  name = {{ stableSuffix "Migrating path" | quote }}
}

resource "incident_escalation_path_beta" "migrating" {
  name  = {{ stableSuffix "Migrating path" | quote }}
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
`, nil)
}

// TestAccMigrateAlertSourceOffTemplateAttributes covers the other one-becomes-
// several case: a source that declared its attribute bindings under
// template.attributes becomes a source plus one resource per binding.
//
// The title and description carry the same document across unchanged. They are
// rich text either side, and a template compares equal to the document it
// produces, so there is nothing to translate - which is worth a test of its own,
// because the alternative would be a plan that never settles.
func TestAccMigrateAlertSourceOffTemplateAttributes(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMigrateAlertSourceBefore(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("incident_alert_source.migrating", "id"),
					resource.TestCheckResourceAttr("incident_alert_source.migrating", "template.attributes.#", "1"),
				),
			},
			{
				Config: testAccMigrateAlertSourceAfter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"incident_alert_source_attribute_beta.environment", "alert_source_id",
						"incident_alert_source_beta.migrating", "id",
					),
					resource.TestCheckResourceAttr("incident_alert_source_attribute_beta.environment", "value_literal", "production"),
				),
			},
			{
				Config:   testAccMigrateAlertSourceSettled(),
				PlanOnly: true,
			},
		},
	})
}

// alertSourceMigrationFixture is the part of the configuration that does not
// change across the migration: the attribute being bound, and the document the
// source titles its alerts with.
const alertSourceMigrationFixture = `
resource "incident_alert_attribute" "environment" {
  name  = {{ stableSuffix "Migrating environment" | quote }}
  type  = "String"
  array = false
}

locals {
  alert_title = jsonencode({
    type = "doc"
    content = [{
      type    = "paragraph"
      content = [{ type = "text", text = "Migrating source alert" }]
    }]
  })
}
`

func testAccMigrateAlertSourceBefore() string {
	return testRunTemplate("migrate_alert_source_before", alertSourceMigrationFixture+`
resource "incident_alert_source" "migrating" {
  name        = {{ stableSuffix "Migrating source" | quote }}
  source_type = "http"

  template = {
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

// The source's ID has to be found by name, because the singular data source
// looks a source up by ID and the plural one filters by type: hence the
// comprehension. Whoever does this by hand has the ID in their state already.
func testAccMigrateAlertSourceAfter() string {
	return testRunTemplate("migrate_alert_source_after", alertSourceMigrationFixture+`
data "incident_alert_sources" "all" {}

data "incident_alert_attribute" "environment" {
  name = {{ stableSuffix "Migrating environment" | quote }}
}

locals {
  migrating_source_id = one([
    for source in data.incident_alert_sources.all.alert_sources :
    source.id if source.name == {{ stableSuffix "Migrating source" | quote }}
  ])
}

resource "incident_alert_source_beta" "migrating" {
  name        = {{ stableSuffix "Migrating source" | quote }}
  source_type = "http"

  title       = { literal = local.alert_title }
  description = { literal = local.alert_title }
}

resource "incident_alert_source_attribute_beta" "environment" {
  alert_source_id    = incident_alert_source_beta.migrating.id
  alert_attribute_id = incident_alert_attribute.environment.id

  value_literal  = "production"
  merge_strategy = "first_wins"
}

import {
  to = incident_alert_source_beta.migrating
  id = local.migrating_source_id
}

import {
  to = incident_alert_source_attribute_beta.environment
  id = "${local.migrating_source_id}:${data.incident_alert_attribute.environment.id}"
}

removed {
  from = incident_alert_source.migrating

  lifecycle {
    destroy = false
  }
}
`, nil)
}

func testAccMigrateAlertSourceSettled() string {
	return testRunTemplate("migrate_alert_source_settled", alertSourceMigrationFixture+`
resource "incident_alert_source_beta" "migrating" {
  name        = {{ stableSuffix "Migrating source" | quote }}
  source_type = "http"

  title       = { literal = local.alert_title }
  description = { literal = local.alert_title }
}

resource "incident_alert_source_attribute_beta" "environment" {
  alert_source_id    = incident_alert_source_beta.migrating.id
  alert_attribute_id = incident_alert_attribute.environment.id

  value_literal  = "production"
  merge_strategy = "first_wins"
}
`, nil)
}
