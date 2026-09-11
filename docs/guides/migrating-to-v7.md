---
page_title: "Migrating to v7"
subcategory: ""
description: |-
  What changed in v7, and how to get there without recreating anything in your
  incident.io account.
---

# Migrating to v7

v7 promotes the beta resources to the names they keep. Schedules, schedule
rotations, escalation paths, alert sources and alert source attributes are now
managed through the schemas that were in beta through v6, and the schemas they
replace are gone.

Nothing in your incident.io account is recreated to get there. Every migration
in this guide is state you already hold, a move Terraform asks the provider to
carry, or an import of something that exists: your schedules keep their history,
your escalation paths keep escalating, and nobody's on-call shifts change hands.

## Which path you are on

One question, and for most configurations the answer is the end of it.

- **Your configuration uses the `_beta` resources.** Upgrading is the version
  bump and nothing else. Those names still work in v7, backed by the same
  schemas and the same API, so a configuration written against them plans and
  applies exactly as it did. They are deprecated and go in v8, so
  [rename them](#renaming-off-the-beta-names) whenever it suits you.
- **Your configuration uses the v6 schemas.** Your state comes across, but your
  configuration has to be rewritten, because `rotations`, `path` and `template`
  are no longer attributes these resources have. See
  [Moving off the v6 schemas](#moving-off-the-v6-schemas).
- **Both**, which is normal if you adopted the beta resources for new
  configuration and left the rest alone. Do each for the resources it applies
  to; they do not interact.

## What changed

| v6 | v7 | What it takes |
| --- | --- | --- |
| `incident_schedule_beta` | `incident_schedule` | Nothing. The old name still works, deprecated, until v8 |
| `incident_schedule_rotation_beta` | `incident_schedule_rotation` | Nothing, as above |
| `incident_escalation_path_beta` | `incident_escalation_path` | Nothing, as above |
| `incident_alert_source_beta` | `incident_alert_source` | Nothing, as above |
| `incident_alert_source_attribute_beta` | `incident_alert_source_attribute` | Nothing, as above |
| `incident_schedule`, rotations declared inline | The schema that was `incident_schedule_beta` | A rewrite, and an import per rotation |
| `incident_escalation_path`, with a nested `path` | The schema that was `incident_escalation_path_beta` | A rewrite |
| `incident_alert_source`, with `template.attributes` | The schema that was `incident_alert_source_beta` | A rewrite, and an import per attribute |

The data sources changed the same way, and a data source holds no state, so
there is nothing to move: renaming one is an edit to your configuration and
nothing else.

`incident_schedule_replica`, `incident_schedule_sync_rule` and
`incident_schedule_sync_target` are unchanged. They reference a schedule by ID,
so the only edit they need is to the address they read that ID from.

If you depend on this provider as a Go module rather than through Terraform, its
path is now `github.com/incident-io/terraform-provider-incident/v7`.

## Renaming off the beta names

Bump the provider to `~> 7.0` and you are done: this section is optional until
v8. Every plan touching a `_beta` name warns, naming the resource it became and
the block that gets there, so there is nothing to go looking for - and nothing
breaks while you leave it.

When you do rename:

1. Drop `_beta` from every resource and data source, including the references
   between them: `incident_schedule_beta.platform.id` becomes
   `incident_schedule.platform.id`.
2. Add a `moved` block per resource.
3. `terraform plan`. It reports the moves and nothing else. Apply it, and delete
   the `moved` blocks once every workspace using this configuration has applied.

```terraform
# Renaming off the _beta names, whenever you like. The old names still work in
# v7, so nothing here is urgent - they warn on every plan and go in v8.
#
# Renaming a resource changes its type as far as Terraform is concerned, so a
# moved block is what tells Terraform the state belongs to the new name rather
# than to something that has been destroyed and replaced. Both names are one
# resource behind one schema, so the provider carries every attribute across and
# Terraform plans no other changes.
#
# Drop the suffix from the references between resources at the same time:
# incident_schedule_beta.platform.id becomes incident_schedule.platform.id.
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

The provider takes these moves itself, which is why the plan after one is empty:
both names are one registration of one resource, so every attribute in state is
carried across rather than being read back and reconciled. Nothing is destroyed
and nothing is created.

## Moving off the v6 schemas

Your state comes across on the upgrade, because these resources kept their
names: `incident_schedule` in v7 is the schema that was `incident_schedule_beta`
in v6, holding the same object. What your state holds under `rotations`, `path`
and `template` is dropped, because those are not attributes any more, and the
rest is read back from the API by the refresh Terraform runs before it plans.

What does not come across is your configuration. `rotations`, `path` and
`template` have to be rewritten before Terraform will accept the file at all, so
the rewrite and the version bump land together. The rewrites are below, one per
resource, and the resources that split out of these - a schedule's rotations, an
alert source's attribute bindings - are imported, because nothing in state is
addressed by their new names yet.

If you would rather separate the two, there is a route that does the rewrite on
v6.13 and the upgrade afterwards: [see below](#doing-the-rewrite-on-v613-first).

### Before you start

1. Get to an empty plan on v6. A migration is verified by a plan that reports no
   changes, which only means something if it reported none to begin with.
2. Back up your state: `terraform state pull > backup.tfstate`. Nothing here
   destroys anything, but a state file you can put back is what makes that claim
   easy to test.
3. Check your Terraform version if you have overridden it. `moved` and `import`
   blocks are both used below; the provider has required Terraform 1.14 or later
   since v6.0, so a supported version already has them.

If you build configuration by exporting from the dashboard, re-export the
objects you are migrating. The export writes the new schema, which saves doing
the rewrite by hand.

~> **Read the plan before you apply it.** Nothing in this migration destroys
anything, so a plan proposing a destroy is a plan that has misunderstood
something - and a destroyed schedule, escalation path or alert source takes
everything attached to it. Stop and find out why rather than applying it.

The IDs to import by, all of which are in the state you already have:
`terraform state show incident_schedule.platform` on v6 prints the schedule's
`id` and the `id` of each of its rotations.

| What you are importing | ID to use |
| --- | --- |
| Schedule rotation | `<schedule_id>:<rotation_id>` |
| Alert source attribute | `<alert_source_id>:<alert_attribute_id>` |

### Schedules

One `incident_schedule` becomes one schedule plus one rotation resource per
rotation, so a change to one rotation no longer rewrites the whole schedule and
disturbs who is on call elsewhere on it.

- `rotations[*].versions[*].handover_start_at` becomes the rotation's
  `first_interval_starts_at`. It means the same moment.
- `rotations[*].versions[*].layers` is gone. A rotation with two layers is one
  rotation with `concurrent_shifts = 2`, which is how many people it puts on
  call at once.
- A rotation's versions are no longer written out. `effective_from` and
  `rollout` on the rotation are how a change to the line-up is introduced.
- `timezone`, `team_ids` and `holidays_public_config` are unchanged, and stay on
  the schedule.
- The split brings controls the old shape had no way to express:
  `scheduling_mode` decides how people are allocated across shifts of differing
  length, and `working_intervals` restricts a rotation to given hours.

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
# The resource keeps its address. incident_schedule is the new schema in v7, so
# the schedule's state carries over as it stands, minus the rotations it used to
# hold. Nothing is created or destroyed, and nobody's shifts move.
resource "incident_schedule" "platform" {
  name     = "Platform on-call"
  timezone = "Europe/London"

  team_ids = [data.incident_catalog_entry.platform_team.id]
}

resource "incident_schedule_rotation" "primary" {
  schedule_id = incident_schedule.platform.id
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

# The rotation is the one thing that needs claiming: it was part of the schedule
# in v6 and is its own resource now, so there is no state under this address to
# carry over. A rotation is identified by its schedule as well as itself, and its
# own ID is the one the old configuration chose - "primary", here - because the
# old resource sent it.
import {
  to = incident_schedule_rotation.primary
  id = "01ABC123DEF456GHI789JKL:primary"
}
```

The plan should report the schedule unchanged and each rotation an import, with
nothing created and nothing destroyed. Delete the `import` blocks in a follow-up
commit once applied.

### Escalation paths

The nesting goes away: a branch names the sequences to continue down instead of
holding their nodes, so every sequence sits at the same depth and the five-level
limit on branching is gone. A path too deep to write before can be written now.

- `path` becomes `sequences`, a map you key yourself, plus `start` naming the
  sequence the escalation begins with.
- An `if_else` node becomes a `branch`, with `then_path` and `else_path`
  becoming sequences of their own that `then` and `else` name.
- `if_else.conditions`, which was a raw engine condition, becomes `branch.if`:
  one attribute per thing an escalation can be tested on.
- A `repeat` node becomes a `loop`, which names the node to go `back_to`.
- `working_hours`, `repeat_config` and `team_ids` are unchanged.

Three things to expect in the first plan. Two you can avoid, as the example below
does; the third you cannot, and it is the one case in this guide where the first
plan after an upgrade is not empty.

The API does not store sequence names, so the refresh names them: the sequence
the path starts with is `main`, and the sequences a branch leads to are
`main_then` and `main_else`, after the sequence the branch sits in. A
configuration that calls them something else plans a change - harmless, but
noise you can do without on the plan you are reading carefully.

`ack_mode` on a level now defaults to `first` where the v6 resource defaulted it
to `all`. A default applies wherever the configuration is silent, so a level that
never set `ack_mode` plans a change from `all` to `first`, and applying it
changes how the level pages: with `first`, the first person to acknowledge
cancels everyone else's escalation on that level. Write `ack_mode = "all"` on
every level whose behaviour you mean to keep.

~> **Expect an update on the path itself, and read it before applying.** A node
id is now derived from the node's position, and only ids in that form are treated
as derived when the path is read back. v6 minted a random id for every node your
configuration did not name, and those are what your escalation path holds, so
they read back as ids you wrote against a configuration that never wrote one -
and the plan proposes rewriting them. It changes nothing about how the path
escalates: the levels, targets, conditions and working hours are untouched, and
the nodes you *did* name keep their names, so a `loop` still finds what it loops
back to. Apply it once and it settles. What to check before you do is that the
plan touches ids and nothing else.

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
# The resource keeps its address. incident_escalation_path is the new schema in
# v7, so the path's state carries over, minus the path it used to hold, and the
# refresh reads its nodes back in the new shape.
resource "incident_escalation_path" "urgent_support" {
  name = "Urgent support"

  # Which sequence the escalation begins with. There is no equivalent in the old
  # shape, where the first element of path was the start.
  #
  # The API does not store sequence names, so the refresh after an upgrade names
  # them: the one the path starts with is main, and the sequences a branch leads
  # to are main_then and main_else, after the sequence the branch sits in. These
  # are written to match, so the names are not something the first plan has to
  # reconcile. Rename them afterwards if you would rather they read better -
  # that is an update to your own state, not a change to the path.
  #
  # The first plan will still propose an update, for the node ids v6 minted for
  # the nodes it wasn't told the names of. That one can't be written around: apply
  # it once and it settles. See the guide.
  start = "main"

  sequences = {
    main = {
      nodes = [
        {
          # Keep an id on the nodes something loops back to, and leave it off the
          # rest: an unnamed node gets one derived from its position, which is
          # stable across applies. A node your v6 config named keeps its name, so
          # this is the id a loop can still rely on after the upgrade.
          id = "start"
          branch = {
            # The raw engine condition becomes one attribute per thing an
            # escalation can be tested on: working hours, or the priority it
            # came in at.
            if = {
              working_hours_active = "UK"
            }
            then = "main_then"
            else = "main_else"
          }
        }
      ]
    }

    # then_path becomes a sequence of its own, named by the branch above.
    main_then = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "user"
              id      = data.incident_user.on_call.id
              urgency = "high"
            }]
            time_to_ack_seconds = 300

            # Write out ack_mode on every level you mean to keep as it is. This
            # resource defaults it to first where v6 defaulted it to all, and a
            # default applies wherever the configuration is silent - so a level
            # that never set it plans a change from all to first, after which
            # the first person to acknowledge cancels everyone else's
            # escalation on the level.
            ack_mode = "all"
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

    main_else = {
      nodes = [
        {
          level = {
            targets = [{
              type    = "user"
              id      = data.incident_user.on_call.id
              urgency = "low"
            }]
            time_to_ack_seconds = 300
            ack_mode            = "all"
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
```

### Alert sources

One `incident_alert_source` becomes one source plus one resource per attribute
it populates, so filling in one more attribute is an add rather than an edit of
everything else.

- `template.title`, `template.description` and `template.is_private` move to the
  top level of the source, and `template.visible_to_teams` becomes
  `visible_to_teams`.
- Each entry of `template.attributes` becomes an
  `incident_alert_source_attribute` resource. The binding's value keeps its
  shape, and `merge_strategy` comes with it.
- `template.expressions` becomes `expression` and `named_expression` blocks, on
  whichever resource uses the expression. One feeding a single attribute belongs
  on that attribute's resource.
- The source's own options - `source_type`, `jira_options`,
  `http_custom_options`, `rate_limit_sharding`, `filter_condition_groups`,
  `fixed_team_id`, the auto-resolve settings - are unchanged.

`title` and `description` are read back from the API in its spelling of the
document rather than yours - equivalent JSON, different bytes - so expect the
first plan to ask to rewrite both. Applying it writes what your configuration
already says, and it settles. Let the read tell you the canonical form rather
than hand-copying a JSON literal across.

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

    # Required, even when there are none. In v7 there is no template, and an
    # expression is a block on whichever resource uses it.
    expressions = []

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
# The source keeps its address. incident_alert_source is the new schema in v7,
# so its state carries over, minus the template it used to hold.
resource "incident_alert_source" "prometheus" {
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

  # template.expressions becomes expression and named_expression blocks, and
  # template.visible_to_teams becomes visible_to_teams. An expression feeding a
  # single attribute belongs on that attribute's resource.
}

# Each entry of template.attributes becomes one of these. The binding's value
# keeps its shape, and merge_strategy comes with it. These are the resources
# that need claiming: the bindings lived inside the source in v6, so there is no
# state under these addresses to carry over.
resource "incident_alert_source_attribute" "environment" {
  alert_source_id    = incident_alert_source.prometheus.id
  alert_attribute_id = incident_alert_attribute.environment.id

  # value = { literal = "..." } is still accepted; value_literal is shorthand.
  value_literal  = "production"
  merge_strategy = "first_wins"
}

resource "incident_alert_source_attribute" "regions" {
  alert_source_id    = incident_alert_source.prometheus.id
  alert_attribute_id = incident_alert_attribute.regions.id

  # An array of fixed values is values; array_value is for a mix of fixed
  # values and references.
  values         = ["eu-west-1", "eu-west-2"]
  merge_strategy = "append"
}

# A binding is identified by its source and the attribute it binds.
import {
  to = incident_alert_source_attribute.environment
  id = "01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX"
}

import {
  to = incident_alert_source_attribute.regions
  id = "01ABC123DEF456GHI789JKL:01STU012VWX345YZA678BCD"
}
```

### Doing the rewrite on v6.13 first

The rewrite above lands with the version bump, because a configuration on the v6
schemas stops parsing the moment the provider changes under it. If you would
rather do the two separately - rewrite and verify against a provider you can
roll back with a version pin, then upgrade with nothing outstanding - v6.13 lets
you, because both schemas exist side by side there.

On `~> 6.13`, do the rewrites above under the `_beta` names, and move each
resource onto its replacement rather than importing it:

```terraform
moved {
  from = incident_schedule.platform
  to   = incident_schedule_beta.platform
}
```

The rotations and attribute bindings are imported exactly as above, because a
`moved` block has a single target. Everything this guide says about sequence
names, `ack_mode` and the spelling of a title applies here too: it is the same
pair of schemas either side of the move.

Once that has applied and the plan is empty, you are on the beta resources,
which makes the upgrade to v7 the version bump and nothing else. Then
[rename off the `_beta` names](#renaming-off-the-beta-names) whenever you like.

## Doing it with an agent

The rewrite is mechanical, and the mapping above is written to be handed to a
coding agent along with your configuration. Something like:

```text
Migrate this Terraform configuration to the incident.io provider's v7 schemas,
following https://registry.terraform.io/providers/incident-io/incident/latest/docs/guides/migrating-to-v7

- Rewrite each incident_schedule as a schedule plus one
  incident_schedule_rotation per rotation, each incident_escalation_path's path
  as sequences, and each incident_alert_source's template.attributes as one
  incident_alert_source_attribute apiece.
- Keep every resource at the address it already has. Add an import block only
  for the resources that are new addresses: the rotations and the alert source
  attributes, with IDs taken from the existing state.
- Write out ack_mode on every escalation path level, keeping the behaviour the
  v6 default gave it.
- Do not remove or reword anything else, and do not run terraform apply.
```

Read the diff and the plan yourself before applying: a plan proposing a create
where you expected an import, or any destroy at all, is the thing to catch.

## Verifying

- `terraform plan` reports no changes.
- Your schedules still show the same people on call, in the dashboard.
- Your escalation paths still page the same people in the same order, and each
  level acknowledges the way it used to.
- The resources still show as managed by Terraform in the dashboard. Creating,
  updating and importing all claim a resource, so a migrated resource stays
  claimed - unless you set `mark_imported_resources_as_managed = false`, in
  which case an imported resource is claimed by the first apply that changes it.

## Troubleshooting

**`Unsupported argument` on `rotations`, `path` or `template`.** The provider has
been upgraded and the configuration has not. These are not attributes in v7; do
the rewrite for [the resource in question](#moving-off-the-v6-schemas), or pin
back to `~> 6.13` while you write it.

**The plan wants to create a resource I am importing.** The `import` block's
`to` address does not match the resource it is meant to claim, or the ID is
wrong. Compare both against `terraform state show` on the old resource - a
rotation's ID is the one your v6 configuration chose, not an ID incident.io
generated.

**The plan wants to replace, not move.** The `moved` block's `to` address does
not match what you renamed the resource to. The two have to agree exactly,
including the local name.

**`Unable to Move Resource State`.** The provider being asked to take the move
does not know the name you are moving from. Check the `from` address is the
`_beta` name exactly as it appeared in your configuration, and that you are on
the version that takes that move: v7 takes `_beta` to the new name, and v6.13
takes a v6 resource to its `_beta` replacement. Neither takes a move in the
other direction.

**A plan that will not settle, or `Provider produced inconsistent result after
apply`.** Upgrade to v6.13 before migrating. Several of these were fixed there,
and they are easier to tell apart from a migration problem when they are not
happening at the same time.

**A plan asking to rewrite a title or description you have not touched.** Expected
once, on an alert source: the API returns its own spelling of the document. Apply
it and it settles. A plan that asks again after applying is a bug - please report
it.

**A plan asking to rewrite an escalation path's node ids.** Expected once, and
covered above: v6 minted those ids, and they are not the form this resource
derives. Applying it changes no behaviour.

**`Failed to marshal state to json: unsupported attribute "path"`**, or the same
for `rotations` or `template`. `terraform show -json` renders state against the
provider's current schema, and until the first apply rewrites it your state still
holds the attribute the old schema had. `terraform plan` and `terraform apply`
are unaffected - it is only the JSON rendering that fails - so the fix is to
apply the migration. It is worth knowing about if your pipeline reads plans as
JSON, because that is the step that breaks between upgrading the provider and
applying.

## Rolling back

Pin the provider back to `~> 6.13` and revert the configuration. If you have
already applied, restore the state you backed up: a `moved` block in the other
direction asks the older provider to accept a move it has no support for, so the
backup is what gets you back rather than another plan.
