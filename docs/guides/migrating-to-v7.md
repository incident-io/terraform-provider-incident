---
page_title: "Migrating to v7"
subcategory: ""
description: |-
  What changed in v7, and how to get there without recreating anything in your
  incident.io account.
---

# Migrating to v7

v7 promotes the beta resources to the names they keep. Schedules, escalation
paths and alert sources are now managed through the schemas that were in beta
through v6, and the schemas they replace are gone.

Nothing in your incident.io account has to be recreated to get there. Every
migration in this guide is a state move or an import: your schedules keep their
history, your escalation paths keep escalating, and nobody's on-call shifts
change hands.

Two things to know before you start:

- **A name you already use may now mean a different schema.** `incident_schedule`
  in v7 is the resource that was `incident_schedule_beta` in v6. If your
  configuration uses the v6 schema, upgrading the provider without migrating
  first leaves state that v7 cannot read - see
  [I upgraded before migrating](#i-upgraded-before-migrating) for the way out.
- **Do the schema work on v6.** Both schemas exist side by side there, which is
  what lets you import the new resources and check the result before the old
  ones go away.

## What changed

| v6 | v7 | What it takes |
| --- | --- | --- |
| `incident_schedule_beta` | `incident_schedule` | A rename |
| `incident_schedule_rotation_beta` | `incident_schedule_rotation` | A rename |
| `incident_escalation_path_beta` | `incident_escalation_path` | A rename |
| `incident_alert_source_beta` | `incident_alert_source` | A rename |
| `incident_alert_source_attribute_beta` | `incident_alert_source_attribute` | A rename |
| `incident_schedule`, rotations declared inline | Removed | A rewrite, then an import |
| `incident_escalation_path`, with a nested `path` | Removed | A rewrite, then an import |
| `incident_alert_source`, with `template.attributes` | Removed | A rewrite, then an import |

The data sources are renamed the same way. A data source holds no state, so
renaming one is an edit to your configuration and nothing else.

`incident_schedule_replica`, `incident_schedule_sync_rule` and
`incident_schedule_sync_target` are unchanged. They reference a schedule by ID,
so the only edit they need is to the address they read that ID from. Two things
worth knowing about them:

- A sync rule scoped to a rotation used to name it with an ID you chose. A
  rotation's ID comes from us now, so point `rotation_id` at the rotation
  resource: `rotation_id = incident_schedule_rotation.primary.id`.
- A replica source names a rotation *and a layer*. Layers are not something
  `incident_schedule_rotation` declares - `concurrent_shifts` replaced them - so
  replicating a schedule Terraform manages needs a layer ID from elsewhere.

The `incident_alert_source` and `incident_alert_sources` data sources are also
unchanged, which means the plural one still reports a source's bindings as a
`template`: that is the shape our API returns, rather than the shape the
resources now take.

## Which path you are on

- **You only use the beta resources.** Do [stage two](#stage-two-rename-the-resources-on-v7).
  It is a rename.
- **You use the v6 schemas.** Do [stage one](#stage-one-move-off-the-v6-schemas-on-v6)
  first, on v6, then stage two.
- **You use both**, which is normal if you adopted the beta resources for new
  configuration and left the rest alone. Do stage one for what is still on the
  v6 schemas, then one pass of stage two over everything.

## Before you start

1. Pin the provider so nothing moves under you while you work:
   `version = "~> 6.12"`.
2. Get to an empty plan. A migration is verified by a plan that reports no
   changes, which only means something if it reported none to begin with.
3. Back up your state: `terraform state pull > backup.tfstate`. Nothing here
   destroys anything, but a state file you can put back is what makes that
   claim easy to test.
4. Check your Terraform version if you have overridden it. `moved`, `removed`
   and `import` blocks are all used below; the provider has required Terraform
   1.14 or later since v6.0, so a supported version already has them.

If you build configuration by exporting from the dashboard, re-export the
objects you are migrating. The export writes the new schema, which saves doing
the rewrite below by hand.

## Stage one: move off the v6 schemas, on v6

Three edits per object, all in configuration, and one apply:

1. Write the new resources alongside the old one.
2. Add an `import` block per new resource, with the ID of the object that
   already exists. This is what stops Terraform creating a second schedule
   beside the one you have.
3. Add a `removed` block for the old resource, with `destroy = false`, so
   Terraform stops managing it without deleting what it points at.

Then `terraform plan`. It should report imports and one resource forgotten, with
nothing created and nothing destroyed. Apply it, and delete the `import` and
`removed` blocks in a follow-up commit.

~> **Never let Terraform destroy the old resource.** Both resources point at the
same object in incident.io, so a destroy is not bookkeeping: it deletes the
schedule, escalation path or alert source, and everything attached to it. That
is what `destroy = false` is for. If a plan proposes a destroy, stop and find
out why before applying it.

Finding the IDs: the object's ID is in the state you already have, which is
usually quicker than the dashboard. `terraform state show incident_schedule.platform`
prints the schedule's `id` and the `id` of each of its rotations, which is
where the compound rotation IDs below come from.

| What you are importing | ID to use |
| --- | --- |
| Schedule | `<schedule_id>` |
| Schedule rotation | `<schedule_id>:<rotation_id>` |
| Escalation path | `<escalation_path_id>` |
| Alert source | `<alert_source_id>` |
| Alert source attribute | `<alert_source_id>:<alert_attribute_id>` |

### Schedules

One `incident_schedule` becomes one schedule plus one rotation resource per
rotation.

- `rotations[*].versions[*].handover_start_at` becomes the rotation's
  `first_interval_starts_at`. It means the same moment.
- `rotations[*].versions[*].layers` is gone. A rotation with two layers is one
  rotation with `concurrent_shifts = 2`, which is how many people it puts on
  call at once.
- A rotation's versions are no longer written out. `effective_from` and
  `rollout` on the rotation are how a change to the line-up is introduced.
- `timezone`, `team_ids` and `holidays_public_config` are unchanged, and stay on
  the schedule.
- `users = []` is rejected where the old schema took it. A rotation nobody is in
  is `users = ["NOBODY"]`, which schedules the shifts and leaves them uncovered
  until an override fills one.

```terraform
# Before: one resource holds the schedule and every rotation on it, with each
# rotation's line-up under versions.
resource "incident_schedule" "platform" {
  name     = "Platform on-call"
  timezone = "Europe/London"

  team_ids = [data.incident_catalog_entry.platform_team.id]

  rotations = [{
    id   = "primary"
    name = "Primary"

    versions = [
      {
        handover_start_at = "2024-01-08T09:00:00Z"
        users = [
          data.incident_user.alice.id,
          data.incident_user.bob.id,
        ]
        layers = [{
          id   = "primary"
          name = "Primary"
        }]
        handovers = [{
          interval      = 1
          interval_type = "weekly"
        }]
      },
    ]
  }]
}
```

```terraform
# After: the schedule and each of its rotations are separate resources, so a
# change to one rotation leaves the rest of the schedule alone.
#
# Written here with the v6 names, because this is the stage you do while still
# on v6. Stage two drops the _beta suffix from all of them.
resource "incident_schedule_beta" "platform" {
  name     = "Platform on-call"
  timezone = "Europe/London"

  team_ids = [data.incident_catalog_entry.platform_team.id]
}

resource "incident_schedule_rotation_beta" "primary" {
  schedule_id = incident_schedule_beta.platform.id
  name        = "Primary"

  users = [
    data.incident_user.alice.id,
    data.incident_user.bob.id,
  ]

  # handover_start_at is now first_interval_starts_at. It means the same thing:
  # the moment handover intervals are counted from.
  first_interval_starts_at = "2024-01-08T09:00:00Z"

  handovers = [{
    interval      = 1
    interval_type = "weekly"
  }]

  # layers are gone. A rotation with two layers is one rotation with
  # concurrent_shifts = 2, which is how many people it puts on call at once.
}

# Claim the schedule that already exists, rather than creating a second one.
import {
  to = incident_schedule_beta.platform
  id = "01ABC123DEF456GHI789JKL"
}

# A rotation is identified by the schedule it belongs to as well as itself.
import {
  to = incident_schedule_rotation_beta.primary
  id = "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX"
}

# Stop managing the old resource without deleting what it points at. Destroying
# it would delete the schedule, its rotations, and everyone's shifts.
removed {
  from = incident_schedule.platform

  lifecycle {
    destroy = false
  }
}
```

### Escalation paths

The nesting goes away: a branch names the sequences to continue down instead of
holding their nodes.

- `path` becomes `sequences`, a map you key yourself, plus `start` naming the
  sequence the escalation begins with.
- An `if_else` node becomes a `branch`, with `then_path` and `else_path`
  becoming sequences of their own that `then` and `else` name.
- `if_else.conditions`, which was a raw engine condition, becomes `branch.if`:
  one attribute per thing an escalation can be tested on.
- A `repeat` node becomes a `loop`, which names the node to go `back_to`.
- `working_hours`, `repeat_config` and `team_ids` are unchanged.
- The five-level limit on branching is gone, so a path too deep to write before
  can be written now.

```terraform
# Before: a branch holds the nodes that follow it, nested under then_path and
# else_path. Terraform schemas cannot recurse indefinitely, so this shape stops
# at five levels of branching.
resource "incident_escalation_path" "urgent_support" {
  name = "Urgent support"

  path = [
    {
      id   = "start"
      type = "if_else"
      if_else = {
        conditions = [
          {
            operation      = "is_active"
            param_bindings = []
            subject        = "escalation.working_hours[\"UK\"]"
          }
        ]
        then_path = [
          {
            type = "level"
            level = {
              targets = [{
                type    = "user"
                id      = data.incident_user.on_call.id
                urgency = "high"
              }]
              time_to_ack_seconds = 300
            }
          },
          {
            type = "repeat"
            repeat = {
              repeat_times = 3
              to_node      = "start"
            }
          }
        ]
        else_path = [
          {
            type = "level"
            level = {
              targets = [{
                type    = "user"
                id      = data.incident_user.on_call.id
                urgency = "low"
              }]
              time_to_ack_seconds = 300
            }
          }
        ]
      }
    }
  ]

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
```

```terraform
# After: the same escalation path, written as a flat map of named sequences. A
# branch says which sequence to continue down rather than holding its nodes, so
# every sequence sits at the same depth and there is no nesting limit.
#
# Written here with the v6 name, because this is the stage you do while still on
# v6. Stage two drops the _beta suffix.
resource "incident_escalation_path_beta" "urgent_support" {
  name = "Urgent support"

  # Which sequence the escalation begins with. There is no equivalent in the old
  # shape, where the first element of path was the start.
  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          # Keep an id on the nodes something loops back to, and leave it off the rest.
          id = "start"
          branch = {
            # The raw engine condition becomes one attribute per thing an
            # escalation can be tested on: working hours, or the priority it
            # came in at.
            if = {
              working_hours_active = "UK"
            }
            then = "in_hours"
            else = "out_of_hours"
          }
        }
      ]
    }

    # then_path becomes a sequence of its own, named by the branch above.
    in_hours = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "user"
              id      = data.incident_user.on_call.id
              urgency = "high"
            }]
            time_to_ack_seconds = 300
          }
        },
        {
          # repeat is now loop, which names the node to go back to.
          loop = {
            back_to = "start"
            times   = 3
          }
        }
      ]
    }

    out_of_hours = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "user"
              id      = data.incident_user.on_call.id
              urgency = "low"
            }]
            time_to_ack_seconds = 300
          }
        }
      ]
    }
  }

  # working_hours, repeat_config and team_ids are unchanged.
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

# Both resources manage the same escalation path through the same API, so claim
# the existing one rather than creating a second.
import {
  to = incident_escalation_path_beta.urgent_support
  id = "01ABC123DEF456GHI789JKL"
}

removed {
  from = incident_escalation_path.urgent_support

  lifecycle {
    destroy = false
  }
}
```

### Alert sources

One `incident_alert_source` becomes one source plus one resource per attribute
it populates.

- `template.title`, `template.description`, `template.is_private` and
  `template.visible_to_teams` move to the top level of the source.
- `title` and `description` are required for every source type but `heartbeat`,
  which writes its own. A configuration that left them out and took the API's
  default has to say what it wants now, because an attribute the config omits is
  one the provider has nowhere to keep.
- Each entry of `template.attributes` becomes an
  `incident_alert_source_attribute` resource. The binding's value keeps its
  shape, and `merge_strategy` comes with it.
- `template.expressions` becomes `expression` and `named_expression` blocks, on
  whichever resource uses the expression. One feeding a single attribute belongs
  on that attribute's resource.
- The source's own options - `source_type`, `jira_options`,
  `http_custom_options`, `rate_limit_sharding`, `filter_condition_groups`,
  `fixed_team_id`, the auto-resolve settings - are unchanged.

A title or description imported from an existing source reads back as the
`{{ }}` template equivalent to whatever document it holds, so let the
import tell you the canonical form rather than hand-copying a JSON literal
across.

```terraform
# Before: the source and every attribute it populates are declared together,
# under one template.attributes list. Filling in one more attribute means
# rewriting that list, and two people editing different attributes are editing
# the same resource.
resource "incident_alert_source" "prometheus" {
  name        = "Prometheus"
  source_type = "http"

  owning_team_ids = [data.incident_catalog_entry.platform_team.id]

  template = {
    title = {
      literal = "{{payload.labels.alertname}} on {{payload.labels.service}}"
    }

    description = {
      literal = data.incident_rich_text.prometheus_alert.json
    }

    is_private = false

    attributes = [
      {
        alert_attribute_id = incident_alert_attribute.environment.id
        binding = {
          value = {
            literal = "production"
          }
          merge_strategy = "first_wins"
        }
      },
      {
        alert_attribute_id = incident_alert_attribute.regions.id
        binding = {
          array_value = [
            { literal = "eu-west-1" },
            { literal = "eu-west-2" },
          ]
          merge_strategy = "append"
        }
      },
    ]
  }
}
```

```terraform
# After: the source is one resource and each attribute binding is another, with
# its own lifecycle. Filling in one more attribute is an add rather than an edit
# of everything else.
#
# Written here with the v6 names, because this is the stage you do while still
# on v6. Stage two drops the _beta suffix from all of them.
resource "incident_alert_source_beta" "prometheus" {
  name        = "Prometheus"
  source_type = "http"

  owning_team_ids = [data.incident_catalog_entry.platform_team.id]

  # title, description and is_private come out of template to the top level.
  # A title takes a {{ }} template; a description that needs formatting, links
  # or lists is built from markdown with data.incident_rich_text.
  title = {
    literal = "{{payload.labels.alertname}} on {{payload.labels.service}}"
  }

  description = {
    literal = data.incident_rich_text.prometheus_alert.json
  }

  is_private = false

  # template.expressions becomes expression and named_expression blocks, on
  # whichever resource uses them: an expression feeding one attribute belongs on
  # that attribute's resource.
}

# Each entry of template.attributes becomes one of these. The binding's value
# keeps its shape, and merge_strategy comes with it.
resource "incident_alert_source_attribute_beta" "environment" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.environment.id

  # value = { literal = "..." } is still accepted; value_literal is shorthand.
  value_literal  = "production"
  merge_strategy = "first_wins"
}

resource "incident_alert_source_attribute_beta" "regions" {
  alert_source_id    = incident_alert_source_beta.prometheus.id
  alert_attribute_id = incident_alert_attribute.regions.id

  # An array of fixed values is values; array_value is for a mix of fixed
  # values and references.
  values         = ["eu-west-1", "eu-west-2"]
  merge_strategy = "append"
}

import {
  to = incident_alert_source_beta.prometheus
  id = "01ABC123DEF456GHI789JKL"
}

# An attribute binding is identified by its source and the attribute it binds.
import {
  to = incident_alert_source_attribute_beta.environment
  id = "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX"
}

import {
  to = incident_alert_source_attribute_beta.regions
  id = "01ABC123DEF456GHI789JKL:01STU012VWX345YZA678BCD"
}

removed {
  from = incident_alert_source.prometheus

  lifecycle {
    destroy = false
  }
}
```

## Stage two: rename the resources, on v7

Now that every resource is on the new schema, the rest is a rename.

1. Bump the provider: `version = "~> 7.0"`.
2. Drop `_beta` from every resource and data source, including the references
   between them: `incident_schedule_beta.platform.id` becomes
   `incident_schedule.platform.id`.
3. Add a `moved` block per resource, so Terraform knows the state belongs to the
   new name rather than to something that has been replaced.
4. `terraform plan`. It should report the moves and no other changes. Apply it,
   and delete the `moved` blocks once every workspace using this configuration
   has applied.

```terraform
# Stage two, on v7: the resources keep their configuration and change their name.
# Renaming one changes its resource type as far as Terraform is concerned, so a
# moved block is what tells Terraform the state belongs to the new name rather
# than to something that has been destroyed and replaced.
#
# Terraform reports these as moves and plans no other changes.
moved {
  from = incident_schedule_beta.platform
  to   = incident_schedule.platform
}

moved {
  from = incident_schedule_rotation_beta.primary
  to   = incident_schedule_rotation.primary
}

moved {
  from = incident_escalation_path_beta.urgent_support
  to   = incident_escalation_path.urgent_support
}

moved {
  from = incident_alert_source_beta.prometheus
  to   = incident_alert_source.prometheus
}

# One block covers every instance of a resource, so a resource built with
# for_each or count needs one, not one per key.
moved {
  from = incident_alert_source_attribute_beta.per_attribute
  to   = incident_alert_source_attribute.per_attribute
}
```

## Doing it with an agent

Stage one is a mechanical rewrite, and the mapping above is written to be handed
to a coding agent along with your configuration. Something like:

```text
Migrate this Terraform configuration to the incident.io provider's new schemas,
following https://registry.terraform.io/providers/incident-io/incident/latest/docs/guides/migrating-to-v7

- Rewrite each incident_schedule as incident_schedule_beta plus one
  incident_schedule_rotation_beta per rotation, each incident_escalation_path as
  incident_escalation_path_beta, and each incident_alert_source as
  incident_alert_source_beta plus one incident_alert_source_attribute_beta per
  entry of template.attributes.
- Add an import block for every new resource, taking IDs from the existing
  state, and a removed block with destroy = false for every old one.
- Do not remove or reword anything else, and do not run terraform apply.
```

Read the diff and the plan yourself before applying: the plan proposing a create
where you expected an import, or any destroy at all, is the thing to catch.

## Verifying

- `terraform plan` reports no changes.
- Your schedules still show the same people on call, in the dashboard.
- The resources still show as managed by Terraform in the dashboard. Creating,
  updating and importing all claim a resource, so a migrated resource stays
  claimed - unless you set `mark_imported_resources_as_managed = false`, in
  which case an imported resource is claimed by the first apply that changes it.

## Troubleshooting

**The plan wants to create a resource I am importing.** The `import` block's
`to` address does not match the resource it is meant to claim, or the ID is
wrong. Compare both against `terraform state show` on the old resource.

**The plan wants to destroy the old resource.** The `removed` block is missing,
or missing its `lifecycle { destroy = false }`. Do not apply: that plan deletes
the object in incident.io.

**The plan wants to replace, not move.** The `moved` block's `to` address does
not match what you renamed the resource to. The two have to agree exactly,
including the local name.

**`Unable to Move Resource State`.** The provider being asked to take the move
does not know the name you are moving from. Check you are on v7, and that the
`from` address is the `_beta` name exactly as it appeared in your configuration.

**`Provider produced inconsistent result after apply`, or a plan that never
settles.** Upgrade to the latest v6 before migrating. Several of these were
fixed there, and they are easier to tell apart from a migration problem when
they are not happening at the same time.

**Both resources are fighting.** If the old and new resource both manage the
same object, each plans to undo the other's changes. Finish stage one for an
object in a single apply rather than leaving it half done.

### I upgraded before migrating

If you moved to v7 with configuration still on a v6 schema, the state under that
resource name was written with a schema the provider no longer has, and it
cannot be read - which is also why it cannot be refreshed or forgotten in the
usual way. Recover it like this:

1. Take the resource out of state: `terraform state rm incident_schedule.platform`.
   This is a local operation and does not touch your incident.io account, which
   still has the schedule.
2. Rewrite the configuration into the new resources, as in stage one, but under
   the v7 names, with no `_beta` suffix.
3. Add the `import` blocks from stage one, plan, and apply.

You can also pin back to `~> 6.12`, restore your state backup, and do the
migration in the order above.

## Rolling back

Pin the provider back to `~> 6.12` and revert the configuration. If you have
already applied stage two, restore the state you backed up: a `moved` block in
the other direction asks v6 to accept a move it has no support for, so the
backup is what gets you back rather than another plan.
