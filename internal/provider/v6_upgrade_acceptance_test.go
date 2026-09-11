package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// These tests walk the other half of the v7 migration: a configuration that never used
// the `_beta` resources at all, upgrading straight from the v6 schemas.
//
// state_move_acceptance_test.go covers the rename, which the provider carries with a
// state mover. This has no mover behind it and could not have one. The v6 resources kept
// their names, so `incident_schedule` in v7 reads state that v6's `incident_schedule`
// wrote: nothing is asked to move, and what makes it work is Terraform dropping the
// attributes the new schema no longer has - `rotations`, `path`, `template` - as it
// decodes prior state against the current schema.
//
// That is behaviour of Terraform rather than of this provider, which is exactly why it
// is worth a test. It is the load-bearing claim of the migration guide, it is invisible
// in our own code, and nothing in this repository would notice if a future change to the
// schemas broke it - a renamed attribute, or one whose type moved - until somebody's
// on-call schedule was recreated.
//
// Each test runs three steps:
//
//  1. Apply the v6 configuration with the real v6.13.0 provider from the registry. That
//     is the only way to get state in the old shape, because the schema that wrote it is
//     gone from this build.
//  2. Apply the rewritten configuration against the provider under test, and assert the
//     resource that kept its name carries its object across rather than making a new one.
//     The resources that split out of it - a rotation, an attribute binding - are claimed
//     with the `import` blocks the guide tells people to write.
//  3. Plan the settled configuration, with the `import` blocks taken out, and expect no
//     changes.
//
// The object's ID is compared across steps rather than checked for being set, because an
// upgrade that recreated the object would set an ID too - just not the same one. That
// comparison is the assertion these tests exist to make: everything else is detail.
//
// The two configurations are written out separately rather than derived from one
// another, unlike the rename tests, because here they genuinely are different
// configurations. Showing both in full is the point.

// v6ProviderVersion is the last v6 release, which is the version somebody upgrading from
// v6 is most likely to be on and the only one we claim a clean upgrade from.
const v6ProviderVersion = "6.13.0"

// v6Providers installs the released v6 provider for the step that writes the old state.
var v6Providers = map[string]resource.ExternalProvider{
	"incident": {
		Source:            "incident-io/incident",
		VersionConstraint: "= " + v6ProviderVersion,
	},
}

// useReleasedProviderNamespace makes the provider under test reattach at the address the
// released provider is installed at.
//
// The harness serves the provider under test at `<host>/hashicorp/incident` by default,
// while the released one installs at `<host>/incident-io/incident`. Every other test in
// the suite only ever talks to one of them, so the difference never shows; these talk to
// both, against one state file, and state records which provider an object belongs to.
// Left alone, step 2 would be asked to manage resources belonging to a provider its
// configuration no longer mentions, which Terraform refuses rather than guesses at.
//
// TF_ACC_PROVIDER_NAMESPACE is the harness's own knob for this. CI already sets it for
// the OpenTofu runs, so this only writes it when nothing else has.
func useReleasedProviderNamespace(t *testing.T) {
	if os.Getenv("TF_ACC_PROVIDER_NAMESPACE") == "" {
		t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "incident-io")
	}
}

// releasedProviderRequirement names the provider explicitly for the steps that run
// against the build under test.
//
// An unqualified provider name resolves to the `hashicorp` namespace, which is not where
// step 1 installed it, so the steps after it have to say which provider they mean. The
// host comes from the environment because the OpenTofu runs use a registry of their own.
func releasedProviderRequirement() string {
	host := os.Getenv("TF_ACC_PROVIDER_HOST")
	if host == "" {
		host = "registry.terraform.io"
	}

	return fmt.Sprintf(`
terraform {
  required_providers {
    incident = {
      source = %q
    }
  }
}
`, host+"/incident-io/incident")
}

// TestAccUpgradeScheduleFromV6 covers the resource that splits in two. A v6 schedule
// holds its rotations; a v7 one does not, so the schedule carries across on its name and
// each rotation is claimed by the compound ID the guide documents.
func TestAccUpgradeScheduleFromV6(t *testing.T) {
	useReleasedProviderNamespace(t)

	scheduleID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: v6Providers,
				Config:            testRunTemplate("upgrade_schedule_v6", scheduleUpgradeV6Fixture, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					scheduleID.AddStateValue("incident_schedule.upgrading", tfjsonpath.New("id")),
				},
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config: releasedProviderRequirement() + testRunTemplate("upgrade_schedule_v7",
					scheduleUpgradeV7Fixture+scheduleUpgradeImports, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						// The schedule kept its name, so its state carries over untouched:
						// anything more than a no-op here is the upgrade proposing to
						// rewrite a schedule people are on call on.
						plancheck.ExpectResourceAction("incident_schedule.upgrading", plancheck.ResourceActionNoop),
						// The rotation is a new address for an object that already exists,
						// which is what its `import` block is for. A create here means
						// Terraform made a second rotation rather than claiming the one
						// the schedule already has.
						plancheck.ExpectResourceAction("incident_schedule_rotation.primary", plancheck.ResourceActionNoop),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					scheduleID.AddStateValue("incident_schedule.upgrading", tfjsonpath.New("id")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					// The rotation's own ID is the one the v6 configuration chose, because
					// the old resource sent it rather than letting the API allocate one.
					// The guide tells people to import by it, so it had better be true.
					resource.TestCheckResourceAttr("incident_schedule_rotation.primary", "id", "primary"),
					resource.TestCheckResourceAttr("incident_schedule_rotation.primary", "name", "Primary"),
					resource.TestCheckResourceAttrPair(
						"incident_schedule_rotation.primary", "schedule_id",
						"incident_schedule.upgrading", "id",
					),
				),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config: releasedProviderRequirement() +
					testRunTemplate("upgrade_schedule_settled", scheduleUpgradeV7Fixture, nil),
				PlanOnly: true,
			},
		},
	})
}

// The line-up is the NOBODY sentinel rather than real users, so the test needs no
// account-specific IDs.
const scheduleUpgradeV6Fixture = `
resource "incident_schedule" "upgrading" {
  name     = {{ stableSuffix "Upgrading schedule" | quote }}
  timezone = "Europe/London"

  rotations = [{
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
  }]
}
`

// handover_start_at is first_interval_starts_at here, and the single layer is gone: one
// layer is a rotation with the default concurrent_shifts, so it has nothing to say.
const scheduleUpgradeV7Fixture = `
resource "incident_schedule" "upgrading" {
  name     = {{ stableSuffix "Upgrading schedule" | quote }}
  timezone = "Europe/London"
}

resource "incident_schedule_rotation" "primary" {
  schedule_id = incident_schedule.upgrading.id
  name        = "Primary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"

  handovers = [{
    interval      = 1
    interval_type = "weekly"
  }]
}
`

const scheduleUpgradeImports = `
import {
  to = incident_schedule_rotation.primary
  id = "${incident_schedule.upgrading.id}:primary"
}
`

// TestAccUpgradeEscalationPathFromV6 covers the resource that changes shape rather than
// splitting: the path itself imports nothing, and its whole structure is read back from
// the API in a shape the old state never held.
//
// It is written to reproduce every surprise the guide warns about, so the advice it gives
// is what this asserts rather than something we only believe:
//
//   - The sequence names are the ones the refresh derives, because the API does not store
//     them: `main`, and `main_then` and `main_else` for the sequences a branch leads to.
//   - Every level writes out the `ack_mode` the v6 default gave it, against a default
//     that is now `first`.
//   - The plan is an update rather than a no-op, because of the node ids v6 minted. See
//     the plan check for why, and the guide for what to tell people about it.
//
// The schedule the path targets is upgraded alongside it, because it has to be: the v2
// API will not create a schedule with no rotations, so there is no way to write a
// throwaway target for a v6 fixture.
func TestAccUpgradeEscalationPathFromV6(t *testing.T) {
	useReleasedProviderNamespace(t)

	pathID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: v6Providers,
				Config:            testRunTemplate("upgrade_escalation_path_v6", escalationPathUpgradeV6Fixture, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					pathID.AddStateValue("incident_escalation_path.upgrading", tfjsonpath.New("id")),
				},
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config: releasedProviderRequirement() + testRunTemplate("upgrade_escalation_path_v7",
					escalationPathUpgradeV7Fixture+escalationPathUpgradeImports, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						// An update, not a no-op, and the one resource here that cannot be
						// one. v6 minted a ULID for every node its configuration did not
						// name - `toPathPayload` filled an empty id with `ulid.Make()` -
						// and those IDs are what the API holds. A node id is derived here
						// instead, from the node's position, and the refresh only treats an
						// id as derived when it contains the separator we derive with. A
						// ULID doesn't, so it reads back as an id the author wrote, against
						// a configuration that never wrote one.
						//
						// So the first plan rewrites those ids to derived ones. It changes
						// no behaviour - the levels, targets and conditions are untouched -
						// and step 3 is what proves it settles in a single apply rather than
						// planning the same change forever.
						//
						// Pinned as an update rather than left unasserted because it is
						// documented in the guide: if this becomes a no-op, the guide is
						// telling people to expect a plan they will not get.
						plancheck.ExpectResourceAction("incident_escalation_path.upgrading", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("incident_schedule.target", plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction("incident_schedule_rotation.target_primary", plancheck.ResourceActionNoop),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					pathID.AddStateValue("incident_escalation_path.upgrading", tfjsonpath.New("id")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					// Spelled out because the guide tells people to expect exactly these
					// names, and a change to how the refresh derives them would otherwise
					// only show up as somebody's plan never going quiet.
					resource.TestCheckResourceAttr("incident_escalation_path.upgrading", "start", "main"),
					resource.TestCheckResourceAttr("incident_escalation_path.upgrading", "sequences.main_then.nodes.0.level.ack_mode", "all"),
					resource.TestCheckResourceAttr("incident_escalation_path.upgrading", "sequences.main_else.nodes.0.level.ack_mode", "all"),
					// The one node the v6 configuration named keeps the name it was given,
					// which is what a `loop` would have relied on. Only the ids v6 minted
					// are rewritten.
					resource.TestCheckResourceAttr("incident_escalation_path.upgrading", "sequences.main.nodes.0.id", "start"),
				),
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config: releasedProviderRequirement() +
					testRunTemplate("upgrade_escalation_path_settled", escalationPathUpgradeV7Fixture, nil),
				PlanOnly: true,
			},
		},
	})
}

// The schedule the path targets carries a rotation because it has to: `rotations` is
// required by the v6 schema, and the v2 API behind it rejects a schedule that has none
// with "Rotations are required". So this fixture upgrades a schedule as well as a path,
// and the rotation is imported the way the schedule test imports its own.
//
// Neither level sets ack_mode, which is the case the guide is about: v6 defaults it to
// `all`, and that is the behaviour the rewritten configuration has to keep.
const escalationPathUpgradeV6Fixture = `
resource "incident_schedule" "target" {
  name     = {{ stableSuffix "Upgrading path schedule" | quote }}
  timezone = "Europe/London"

  rotations = [{
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
  }]
}

resource "incident_escalation_path" "upgrading" {
  name     = {{ stableSuffix "Upgrading path" | quote }}
  team_ids = []

  path = [{
    id   = "start"
    type = "if_else"
    if_else = {
      conditions = [{
        operation      = "is_active"
        param_bindings = []
        subject        = "escalation.working_hours[\"UK\"]"
      }]

      then_path = [{
        type = "level"
        level = {
          targets = [{
            id            = incident_schedule.target.id
            type          = "schedule"
            urgency       = "high"
            schedule_mode = "currently_on_call"
          }]
          time_to_ack_seconds = 300
        }
      }]

      else_path = [{
        type = "level"
        level = {
          targets = [{
            id            = incident_schedule.target.id
            type          = "schedule"
            urgency       = "low"
            schedule_mode = "currently_on_call"
          }]
          time_to_ack_seconds = 300
        }
      }]
    }
  }]

  working_hours = [{
    id       = "UK"
    name     = "UK"
    timezone = "Europe/London"

    weekday_intervals = [{
      weekday    = "monday"
      start_time = "09:00"
      end_time   = "17:00"
    }]
  }]
}
`

// main, main_then and main_else are not a style choice: the API does not store sequence
// names, so those are the ones the refresh derives, and a configuration that calls them
// anything else plans a change. ack_mode is written out on both levels to hold the
// behaviour v6 gave them by default.
const escalationPathUpgradeV7Fixture = `
resource "incident_schedule" "target" {
  name     = {{ stableSuffix "Upgrading path schedule" | quote }}
  timezone = "Europe/London"
}

resource "incident_schedule_rotation" "target_primary" {
  schedule_id = incident_schedule.target.id
  name        = "Primary"

  users = ["NOBODY"]

  first_interval_starts_at = "2024-01-08T09:00:00Z"

  handovers = [{
    interval      = 1
    interval_type = "weekly"
  }]
}

resource "incident_escalation_path" "upgrading" {
  name     = {{ stableSuffix "Upgrading path" | quote }}
  team_ids = []
  start    = "main"

  sequences = {
    main = {
      nodes = [{
        id = "start"
        branch = {
          if = {
            working_hours_active = "UK"
          }
          then = "main_then"
          else = "main_else"
        }
      }]
    }

    main_then = {
      nodes = [{
        level = {
          targets = [{
            id            = incident_schedule.target.id
            type          = "schedule"
            urgency       = "high"
            schedule_mode = "currently_on_call"
          }]
          time_to_ack_seconds = 300
          ack_mode            = "all"
        }
      }]
    }

    main_else = {
      nodes = [{
        level = {
          targets = [{
            id            = incident_schedule.target.id
            type          = "schedule"
            urgency       = "low"
            schedule_mode = "currently_on_call"
          }]
          time_to_ack_seconds = 300
          ack_mode            = "all"
        }
      }]
    }
  }

  working_hours = [{
    id       = "UK"
    name     = "UK"
    timezone = "Europe/London"

    weekday_intervals = [{
      weekday    = "monday"
      start_time = "09:00"
      end_time   = "17:00"
    }]
  }]
}
`

// The schedule this path targets splits the same way any other does, so its rotation is
// claimed rather than created. Nothing about the path itself is imported: it changes
// shape without splitting, so its whole configuration is read back onto the state that
// carried over.
const escalationPathUpgradeImports = `
import {
  to = incident_schedule_rotation.target_primary
  id = "${incident_schedule.target.id}:primary"
}
`

// TestAccUpgradeAlertSourceFromV6 covers the source and the one attribute it populates,
// which is the third shape of the migration: part of the resource splits out, and the
// part that stays is read back in the API's own spelling rather than the author's.
func TestAccUpgradeAlertSourceFromV6(t *testing.T) {
	useReleasedProviderNamespace(t)

	sourceID := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ExternalProviders: v6Providers,
				Config:            testRunTemplate("upgrade_alert_source_v6", alertSourceUpgradeV6Fixture, nil),
				ConfigStateChecks: []statecheck.StateCheck{
					sourceID.AddStateValue("incident_alert_source.upgrading", tfjsonpath.New("id")),
				},
			},
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config: releasedProviderRequirement() + testRunTemplate("upgrade_alert_source_v7",
					alertSourceUpgradeV7Fixture+alertSourceUpgradeImports, nil),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						// The binding is a new address for a binding that already exists,
						// claimed by its `import` block. Only the binding is asserted here:
						// the source itself may plan an update, because its title and
						// description come back in the API's spelling of the document
						// rather than the one the configuration wrote, which the guide says
						// to expect. That it is an update and not a replacement is what the
						// ID comparison below is for.
						plancheck.ExpectResourceAction("incident_alert_source_attribute.environment", plancheck.ResourceActionNoop),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					sourceID.AddStateValue("incident_alert_source.upgrading", tfjsonpath.New("id")),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("incident_alert_source_attribute.environment", "value_literal", "production"),
					resource.TestCheckResourceAttrPair(
						"incident_alert_source_attribute.environment", "alert_source_id",
						"incident_alert_source.upgrading", "id",
					),
					resource.TestCheckResourceAttrPair(
						"incident_alert_source_attribute.environment", "alert_attribute_id",
						"incident_alert_attribute.environment", "id",
					),
				),
			},
			{
				// The guide promises the title and description settle after one apply.
				// This is where that is either true or it isn't.
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config: releasedProviderRequirement() +
					testRunTemplate("upgrade_alert_source_settled", alertSourceUpgradeV7Fixture, nil),
				PlanOnly: true,
			},
		},
	})
}

const alertSourceUpgradeV6Fixture = `
resource "incident_alert_attribute" "environment" {
  name  = {{ stableSuffix "Upgrading environment" | quote }}
  type  = "String"
  array = false
}

locals {
  upgrading_alert_doc = jsonencode({
    type = "doc"
    content = [{
      type    = "paragraph"
      content = [{ type = "text", text = "Upgrading source alert" }]
    }]
  })
}

resource "incident_alert_source" "upgrading" {
  name        = {{ stableSuffix "Upgrading source" | quote }}
  source_type = "http"

  template = {
    title       = { literal = local.upgrading_alert_doc }
    description = { literal = local.upgrading_alert_doc }

    # Required in v6 even when there are none. v7 has no template at all, and an
    # expression is a block on whichever resource uses it.
    expressions = []

    attributes = [{
      alert_attribute_id = incident_alert_attribute.environment.id
      binding = {
        value          = { literal = "production" }
        merge_strategy = "first_wins"
      }
    }]
  }
}
`

// title and description come out of template to the top level, and the one entry in
// template.attributes becomes a resource of its own.
const alertSourceUpgradeV7Fixture = `
resource "incident_alert_attribute" "environment" {
  name  = {{ stableSuffix "Upgrading environment" | quote }}
  type  = "String"
  array = false
}

locals {
  upgrading_alert_doc = jsonencode({
    type = "doc"
    content = [{
      type    = "paragraph"
      content = [{ type = "text", text = "Upgrading source alert" }]
    }]
  })
}

resource "incident_alert_source" "upgrading" {
  name        = {{ stableSuffix "Upgrading source" | quote }}
  source_type = "http"

  title       = { literal = local.upgrading_alert_doc }
  description = { literal = local.upgrading_alert_doc }
}

resource "incident_alert_source_attribute" "environment" {
  alert_source_id    = incident_alert_source.upgrading.id
  alert_attribute_id = incident_alert_attribute.environment.id

  value_literal  = "production"
  merge_strategy = "first_wins"
}
`

// A binding is addressed by its source and the attribute it binds, which is the compound
// ID the guide documents.
const alertSourceUpgradeImports = `
import {
  to = incident_alert_source_attribute.environment
  id = "${incident_alert_source.upgrading.id}:${incident_alert_attribute.environment.id}"
}
`
