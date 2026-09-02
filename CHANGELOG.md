## Unreleased

- Add `fixed_filter` to `incident_custom_field` and its data source, which restricts a catalog-backed field's options to a set of catalog entries chosen up front, rather than following another custom field's value the way `filter_by` does. A field has one filter or the other, so setting `fixed_filter` replaces any `filter_by`, and setting both is rejected at plan time. Removing the block from a field that has one isn't supported yet - clear it in the dashboard.
- Add the `incident_policy` resource, which manages the policies that encode how your organisation should handle incidents. There is no `policy_type` to set: a policy carries exactly one config block - `follow_up`, `debrief`, `post_mortem`, `schedule`, `on_call_readiness` or `vacation_conflict` - and which one it is makes it that type, with `policy_type` computed from it. Swapping which block is set replaces the policy; editing a block in place is an ordinary update. `on_call_readiness` and `vacation_conflict` assign the user in violation, so `assignment_rules` cannot be set alongside either.
- Add the `incident_policy` data source, which looks up an existing policy by `id` or by `name`, so you can reference a policy the dashboard owns without pinning an ID that differs between organisations. It reports `name`, `description`, `status` and `policy_type`. Set exactly one of the two lookup attributes; setting both, or neither, is rejected at plan time.
- Add the `incident_incident_timestamp` data source, which looks up an existing incident timestamp by `id` or by `name`. Timestamps can't be created from Terraform - some are set by incident.io on every incident, the rest are configured in your settings - so this is the way to get hold of one's ID, such as for a workflow that sets or reads it. Set exactly one of the two attributes; setting both is rejected at plan time.

## v6.9.0

- Add `value_literal`, `value_reference`, `expression_ref` and `values` as shorthands for a param binding's value, wherever bindings appear. `param_bindings = [{ value = { reference = "incident.url" } }]` becomes `param_bindings = [{ value_reference = "incident.url" }]`; `value` and `array_value` keep working, and setting more than one form is rejected at plan time.
- Add `concatenate` to expression operations, which adds the values behind another reference to the current value. Configs using it previously failed to apply.
- Fix `Provider produced inconsistent result after apply` on `incident_alert_source` when a template expression uses an operation alias such as `one_of`. The API returns the canonical name, and the provider stored that instead of what the config wrote.
- Expose `rank` on the `incident_status` data source. Statuses are ordered in the dashboard by this value, and the API already returns it; lookups by `id` or `name` now include it so you can sort or compare statuses without a second call.
- `incident_incident_role` can now be looked up by `name` as well as by `id`, so you no longer have to pin a role ID that differs between workspaces. Set exactly one of the two; setting both is rejected at plan time. The `incident_workflow` example now uses this to resolve the incident lead role instead of hardcoding its ID.

## v6.8.0

- Add `rate_limit_sharding` to `incident_alert_source` and `incident_alert_source_beta`, which splits a source's ingest rate limit into per-value buckets instead of applying one limit to the whole source. Set `rate_limit_sharding = { rate_limit_shard_key_path = "$.metadata.team" }` to give each distinct value at that JSON path its own allowance, so one noisy sender can't exhaust the source for everyone else. Remove the block to go back to a single limit. Not every source type supports it - the ones we fetch from over an API rather than receive a payload from don't - and a path set on one of those is rejected at plan time.

## v6.7.1

- Fix `Provider produced inconsistent result after apply` on `incident_schedule` when a rotation version's `effective_from` or `handover_start_at` is written as anything other than a UTC timestamp at second precision. The API re-renders timestamps its own way, and the provider stored what came back, so a config saying `2025-06-01T12:00:00.000Z` (which is what the dashboard's Terraform export writes) or `2026-04-10T19:00:00-04:00` read back as a different string for the same moment. Rotations and their versions are sets, which Terraform correlates by raw value, so that was enough to fail the post-apply consistency check with `planned set element ... does not correlate with any element in actual` - leaving the change applied in incident.io but absent from state, and a plan that never settled. The provider now keeps the timestamp exactly as your config writes it whenever it means the same moment as the one the API returns.
- Reject two `incident_schedule` rotations sharing an `id` at plan time. Versions of a rotation are entries sharing an `id` in the API, and the provider groups them back together by `id` when it reads a schedule, so a config that spells versions as separate `rotations` entries applied but read back as one rotation holding every version - failing the same post-apply consistency check, then planning a change on every run. The error now says so during `plan`, and points at listing the versions under the rotation's `versions` attribute.

## v6.7.0

- List the values every enum attribute accepts in the registry docs. Attributes backed by a fixed set of values in the API - `source_type` on `incident_alert_source`, `type` and `schedule_mode` on `incident_escalation_path` targets, `weekday`, `interval_type`, `merge_strategy`, `operation_type` and a couple of dozen others - now end their description with `Possible values are: ...`, taken from the API schema so the list can't go stale. Previously most of them read as a bare `(String)` with no hint of what to write, leaving you (or an LLM writing your config) to guess, and the few that did list values did it in hand-maintained prose that had already drifted from the API.
- Add `mark_imported_resources_as_managed` to the provider. Importing a resource claims it as managed by Terraform, which is what stops people editing it in the dashboard - and because Terraform runs imports during `plan` rather than apply, that claim is a write to your account during an operation you may expect to be read-only. Set `mark_imported_resources_as_managed = false` to skip it, so plans leave your account untouched. Creating or updating a resource claims it either way, so a resource imported with this off is claimed by the first apply that changes it, and stays editable in the dashboard until then. Defaults to `true`, leaving existing behaviour unchanged.
- Document `owning_team_ids` on `incident_workflow`: the resource example now shows a workflow owned by a team, with the team's ID resolved through the catalog by name rather than hardcoded. Teams are catalog entries, so this is a `incident_catalog_type` lookup for the Team type plus an `incident_catalog_entry` lookup for the team itself.
- Add the `incident_workflow` data source, which reads an existing workflow by `id`. It returns the workflow's full definition - `steps`, `expressions`, `condition_groups`, `form_fields`, `once_for` and `delay` included - not just its name and trigger, so you can reference the pieces of a workflow built in the dashboard from elsewhere in your config without importing it as a resource.
- Add the `incident_status` data source, which looks up an existing incident status by `id` or by `name`. This lets you reference the statuses incident.io manages for you, such as Triage or Closed, which can't be created as an `incident_status` resource.

## v6.6.0

- Add support for the `escalation_path` node type on `incident_escalation_path` and `incident_escalation_path_beta`. A node can now hand the escalation over to another escalation path, which continues from that path's first node - useful for a shared last-resort path, or for routing out of hours to a different team. Write it as `type = "escalation_path"` with `escalation_path = { escalation_path_id = ... }`. Paths that already use a reassignment node, such as one built in the dashboard, can now be imported and managed alongside the rest of your config.

## v6.5.0

- Add the `cast` operation to `expressions` on `incident_alert_route`, `incident_alert_source` and `incident_workflow`. An expression exported from the dashboard that casts a value could not be applied: the provider had no `cast` attribute, so it sent the operation with no options and the API rejected it with `expressions.N.operations.M.cast: Must be provided`. Write it as `cast = { returns = { type = "...", array = false } }`.
- Add `form_fields` to `incident_workflow`, letting you define the fields shown to someone who manually triggers a workflow. The value they provide is available in the workflow scope under `workflow_form.<key>`. Each field has a `key`, `title`, `type`, `array` and `required` setting, plus an optional `description`. Form fields only apply to manual triggers, and either `form_fields = []` or omitting the attribute clears any existing fields.

## v6.4.1

- Document `next_on_call` on `incident_schedule_sync_rule`: the resource example now shows syncing a Slack user group with the people on the next upcoming shift, and the schema lists it as a valid `sync_type`.
- Correct the documented description of `auto_resolve_incident_alerts` on `incident_alert_source` and `incident_alert_source_beta`. It controls whether alerts from the source keep counting down to auto-resolve while they're attached to an incident, and has no effect without `auto_resolve_timeout_minutes`.

## v6.4.0

- Add `incident_escalation_path_beta`, which writes an escalation path as a flat map of named sequences, with no limit on how deeply it branches. This resource is beta: its schema may change in ways that aren't backwards compatible while we settle the design. `incident_escalation_path` is unchanged and not deprecated - don't point both at the same escalation path, as each would plan to undo the other's changes.
- `incident_escalation_path` now validates its planned config against the API at plan time, so a config the API would reject - such as a target missing a required `selected_rota_id` - fails the plan with the API's own message instead of failing part way through an apply. A path that's valid but has no practical effect, like an `if_else` branch with no nodes, comes back as a plan warning rather than an error.

## v6.3.0

- `incident_user` lookups by `email` now resolve to the single active user when
  several users share that email, instead of failing with "Multiple users
  found". A deactivated leftover from a user merge alongside a live SSO account
  no longer breaks an apply. The plan warns when a lookup resolves this way, and
  names the user it picked. Set `id` or `slack_user_id` to choose the user
  yourself. Lookups still fail when several matches are active, or when none
  are. A user who was invited but never logged in counts as inactive.

## v6.2.1

- Fix a permanent diff after importing an `incident_alert_source_beta` that was created in the dashboard. Its `title` and `description` read back as raw JSON rather than the equivalent `{{ }}` template, so every plan showed a change on fields that hadn't changed.

## v6.2.0

- Add beta support for splitting an alert source from the attributes it populates, as `incident_alert_source_beta` and `incident_alert_source_attribute_beta` resources. `incident_alert_source_beta` manages just the source itself (name, type, title, description, priority, options); each attribute binding is its own `incident_alert_source_attribute_beta` resource with its own lifecycle, so filling in one more attribute doesn't mean rewriting every other attribute on the same source. These resources are beta: their schemas may change in ways that aren't backwards compatible while we settle the design. The existing `incident_alert_source` resource is unchanged and not deprecated.
- Add `data.incident_rich_text`, for building a rich text document from markdown for fields — such as the new resources' `title` and `description` — that store a document rather than a `{{ }}` template.

## v6.1.2

- Check an `incident_alert_source`'s template at plan time. A reference that doesn't resolve, or a merge strategy the attribute doesn't support, now fails the plan. Previously it failed part way through an apply, after other resources had been created. Templates whose values aren't known until apply are left alone, and when the check can't run the plan warns rather than failing.

## v6.1.1

- Fix `incident_alert_route` wrongly requiring deprecated v2 attributes
  (`incident_template` etc.) when the resource is driven by `for_each`/`count`.

## v6.1.0

- Add optional `retry_config` to `incident_escalation_path` levels, allowing you
  to configure repeated notifications on a single level.

## v6.0.1

- Warn at plan time when an edit to an `incident_schedule_rotation_beta` lands on a change already scheduled on that rotation: a rollout replaces that change, and an edit without one rewrites it rather than changing who is on call now. Easy to miss after importing a rotation.
- Document `import` blocks on every resource's registry page, above the existing `terraform import` command. Config-driven import can be reviewed and applied like any other change, rather than needing a command run by hand.

## v6.0.0

#### Breaking changes

- **Terraform 1.14 is now the minimum supported version.** HashiCorp only patch the latest two Terraform minors, and until now this provider's acceptance tests ran solely against Terraform 1.2, which has been end-of-life since 2022. We now test against the Terraform versions still receiving patches, and no longer test anything older. Nothing enforces this at install time — the provider still advertises protocol 6, so it may keep working on older CLIs — but we won't be fixing issues that only reproduce below 1.14. Pin to v5.x if you need to stay on an older Terraform.

#### Other changes

- Add beta support for the v3 schedules API, as `incident_schedule_beta` and `incident_schedule_rotation_beta` resources with matching data sources. A schedule and its rotations are separate resources, so a rotation can be managed on its own. Note that a new `incident_schedule_beta` has no rotations, so nobody is on call until one is added.
- These resources are beta: their schemas may change in ways that aren't backwards compatible while we settle the design, which is why they're named `_beta` and versioned separately from the rest of the provider. The existing `incident_schedule` resource and data source are unchanged, continue to use the v2 API, and are not deprecated.
- The provider is now tested directly against OpenTofu on every change, rather than relying on Terraform compatibility to cover it.
- Keep `Computed` attributes nested inside collections planning as unknown rather than null when a new element is added, which the update to `terraform-plugin-framework` 1.19.0 would otherwise have changed. Without this, adding an entry to an existing `incident_catalog_entries`, a node to an `incident_escalation_path`, or a template to an `incident_alert_source` could fail the apply with `Provider produced inconsistent result after apply`.

## v5.46.1

- Fix `Provider produced inconsistent result after apply` on `incident_workflow` when a workflow step gains new parameters, which failed the apply with `.steps[0].param_bindings: new element N has appeared`. Step `param_bindings` are positional, so when a step grows the API pads the bindings out to the new parameter count and creating a workflow returned more bindings than were configured. The provider now drops those trailing empty bindings. Bindings that carry data are kept, so a genuine change made outside Terraform still shows up as a diff.
- Return a diagnostic instead of panicking when the incident.io API client cannot be created, for example when `INCIDENT_ENDPOINT` is set to an invalid URL. Previously this crashed the provider with a raw Go stack trace; it now reports `Unable to Create incident.io API Client` with the underlying error, like every other failure in the provider.

## v5.46.0

- Add optional `permanent_member_user_ids` to `incident_schedule_sync_rule`, naming users who stay in the synced Slack user group regardless of who is on call. Omit the attribute to leave existing members unchanged on update; set it to `[]` to clear them.
- Stop `terraform apply` breaking when a user referenced by `email` or `slack_user_id` in the `incident_user` data source has been deactivated (e.g. offboarded). These lookups now include inactive users, so an already-scheduled user still resolves instead of failing the apply. The data source also exposes a computed `is_active` attribute and emits a plan-time warning (rather than an error) when it resolves an inactive user, nudging authors to move on-call responsibilities to an active user.
- Document import support for `incident_schedule_sync_target` and `incident_schedule_sync_rule`, the last two resources whose registry pages had no import example. Targets import by their ID; rules are nested under a schedule in the API, so they import by `schedule_id:rule_id`.
- Make importing either resource read it from the API up front, so the imported state is fully populated and an ID that doesn't exist fails with a clear message rather than silently importing an empty resource. An import ID with an empty schedule or rule ID is now rejected too, and an unexpected successful API response without a body returns a diagnostic instead of panicking.

## v5.45.0

- Add `group_alerts_summary` to the v3 `incident_alert_route` resource, on Slack and MS Teams channel targets under `message_config.destinations`. When enabled, grouped alerts render as a single editable group-summary message per channel instead of one message per alert.

## v5.44.2

- Fix `Provider produced inconsistent result after apply` on `incident_alert_route` when `escalation_config.when_alert_joins_group.mode` is `on_priority_increase` (or any mode other than `on_each_new_alert`) and `grace_period_seconds` is not set. The API echoes back a zero `grace_period_seconds` for these modes, but the field is only configurable with `on_each_new_alert` (it is rejected by validation and never sent otherwise), so the plan holds null. The provider now only reads `grace_period_seconds` into state when the mode is `on_each_new_alert`, matching how it is validated and sent.

## v5.44.1

- Fix `Provider produced inconsistent result after apply` on `incident_alert_route` when `escalation_config.escalation_targets` sets an escalation path. For escalation-path targets the API echoes the same binding back in `users` for legacy compatibility; the provider surfaced that duplicate, leaving the read-back set element (`users` non-null) unable to correlate with the planned element (`users = null`). The provider now keeps only `escalation_paths` for escalation-path targets.

## v5.44.0

- Add `owning_team_ids` to catalog type resource and data source

## v5.43.1

- Fix `Value Conversion Error` (`Path: path`) when importing an `incident_catalog_type_attribute` via an `import {}` block. When the API response for the catalog type did not contain the imported attribute, `buildModel` produced an untyped `path` list (`types.List[DynamicPseudoType]`) that the framework rejected while writing state. `buildModel` now always writes a fully-typed `path`, `ImportState` populates the full model (and returns a clear diagnostic if the catalog type or attribute can't be found), and `Read` removes the resource from state if the attribute no longer exists.

## v5.43.0

- Add `private_incident_scope` to workflow resource (`all` / `owning_teams` / `none`), deprecating `include_private_incidents`. The two may be set together as long as they agree (`include_private_incidents` is `true` for the `all` and `owning_teams` scopes, `false` for `none`)

## v5.42.0

- Add `owning_team_ids` to workflow resource

# v5.41.1

- Fix panic when importing or refreshing an `incident_escalation_path` if the API returns an unexpected successful response (e.g. a 2xx without the expected JSON body). The provider now returns a diagnostic with the response status instead of crashing on a nil pointer dereference.

## v5.41.0

This release adds support for a revised alert route configuration schema. Existing alert routes using the previous schema remain supported, though users are advised to migrate to the new schema, which better corresponds to the configuration options offered in incident.io.

#### Changes

The following changes are made to the `incident_alert_route` resource:

- Alert grouping configuration is moved out from `incident_config` to a new `grouping_config` object at the top level.
- The incident template is moved from `incident_template` to be nested under `incident_config`.
- Config for sending messages to Slack or Microsoft Teams is combined under `message_config`, instead of the separate `message_template` and `channel_config`.

Support for the previous configuration schema is retained, but using them will show deprectation warnings advising updating to the latest schema.

#### Migration

In simple cases, the easiest way to migrate will be to open each alert route in incident.io and re-export the Terraform configuration.

For other cases, here is a more detailed description of the changes you'll need to make -- you may wish to feed these into to your coding agent of choice:

- Setting the top-level `grouping_config` block is what selects the new schema — a route is on the new schema if and only if `grouping_config` is present. All of the changes below therefore have to be made together in a single edit; a half-migrated resource won't validate.
- Add a top-level `grouping_config` block and move the grouping settings out of `incident_config` into it:
  - `incident_config.grouping_keys` → `grouping_config.default.grouping_keys`.
  - `incident_config.grouping_window_seconds` → `grouping_config.default.window_seconds`.
  - Add `grouping_config.default.enabled = true` (new, required).
  - Add `grouping_config.default.window_type` (new, required when enabled): `"rolling"` extends the window each time a new alert joins the group; `"fixed"` holds the window from the first alert. `"rolling"` matches the previous grouping behaviour — if unsure, re-export the route from incident.io to see which value it uses.
  - If the route was not grouping before (no grouping fields set), still add `grouping_config` with `default = { enabled = false }`, and leave `grouping_keys`, `window_seconds`, and `window_type` unset — they are rejected when `enabled = false`.
- Translate `defer_time_seconds` and `auto_relate_grouped_alerts` into `escalation_config.when_alert_joins_group`. Both fields are removed from `incident_config`; the behaviour is now an escalation mode applied when a subsequent alert joins an existing group:
  - `auto_relate_grouped_alerts = true` → `when_alert_joins_group = { mode = "on_priority_increase" }` (later alerts join the existing incident and only re-escalate when they raise the priority).
  - `auto_relate_grouped_alerts = false` → `when_alert_joins_group = { mode = "on_each_new_alert" }` (every alert joining the group escalates).
  - `defer_time_seconds = N` → `when_alert_joins_group.grace_period_seconds = N`. This is only valid with `mode = "on_each_new_alert"` and cannot be combined with `on_priority_increase`. If you previously set a defer time alongside `auto_relate_grouped_alerts = true`, the grace period no longer applies — drop it.
  - `when_alert_joins_group` is only valid when `grouping_config.default.enabled = true`; omit it otherwise.
- Replace `channel_config` and the top-level `message_template` with a single `message_config` block:
  - Each `channel_config` element becomes an element of `message_config.destinations`.
  - The top-level `message_template` moves to `message_config.template`.
  - `message_config` is required on the new schema. If the route had neither, add `message_config = { destinations = [] }`.
- Move the top-level `incident_template` under `incident_config.template`:
  - Relocate the whole block; all fields keep the same shape.
  - Remove the `workspace` binding if present — it is not supported on the new schema. Use [incident channel workspaces](https://docs.incident.io/getting-started/slack-enterprise-grid#configuring-incident-channel-workspaces) to configure workspaces instead.
  - `incident_config.template` is only valid when `incident_config.enabled = true`. If incident creation is disabled, omit the template, leave `condition_groups` empty, and leave `auto_decline_enabled` unset.
  - Delete the now-empty top-level `incident_template` block.
- Remove every deprecated attribute once its replacement is in place. With `grouping_config` set, the provider rejects `channel_config`, `message_template`, `incident_template`, and `incident_config`'s `grouping_keys` / `grouping_window_seconds` / `defer_time_seconds` / `auto_relate_grouped_alerts`.

As with any Terraform changes, we strongly recommend running `terraform plan` and inspecting the changes it would make before applying your updated configuration.


## v5.40.0

- Add an optional `rotation_id` to `incident_schedule_sync_rule`, scoping a sync rule to a single rotation on the schedule. Omit it to sync all rotations (the previous behaviour); to feed a Slack user group from several rotations, create one rule per rotation pointing at the same target.
- Validate at plan time that `http_custom_options` is only set when `source_type = "http_custom"` (and is required for it). Previously these options were accepted but silently dropped by the API for other source types — including the legacy `http` source — which surfaced as a `Provider produced inconsistent result after apply` error.

## v5.39.0

- Fix `Provider produced inconsistent result after apply` and perpetual diffs on engine param-binding `literal` fields when JSON containing HTML characters (`>`, `<`, `&`) is supplied by an encoder that does not HTML-escape (e.g. CDKTF `JSON.stringify`, `file()`, heredocs). The `literal` field now uses a semantic-equality string type that treats byte-different-but-equivalent JSON as equal.
- Fix plan-time crash on `incident_escalation_path` when `path`, `targets`, or `working_hours` derive from unknown values (e.g. a `local` indexed by a variable, or another resource's computed attribute)
- Fix crash on `incident_escalation_path` when `if_else` nodes are nested at the maximum supported depth, and reject nesting beyond it at plan time with a clear error instead of an opaque API failure

## v5.38.1

- Mark `incident_schedule_sync_target` and `incident_schedule_sync_rule` resources as managed by Terraform

## v5.38.0

- Add `incident_schedule_sync_target` and `incident_schedule_sync_rule` resources for syncing schedules to Slack user groups

## v5.37.0

- Support new escalation path schedule modes (`currently_on_call_for_rota`, `next_on_call_for_rota`, `next_on_call`) and the associated `selected_rota_id` target field

## v5.36.0

- Add support for transform expressions on email alert source

## v5.35.2

- Support escalation path nesting up to a depth of 4

## v5.35.1

- Fix inconsistent state warning for `auto_resolve_incident_alerts`

## v5.35.0

- Add heartbeat monitoring options to alert source resource (`interval`, `grace_period_seconds`, `failure_threshold`)

## v5.34.0

- Add support for `delay` nodes on escalation paths

## v5.33.0

- Add `alert_events_url` to alert source resource and data source

## v5.32.0

- Add `incident_maintenance_window` resources to create maintenance windows

## v5.31.0

- Add support for `repeat_config` on escalation paths

## v5.30.0

- Add support for `emoji` on alert attributes

## v5.29.0

- Add auto_resolve_incident_alerts and auto_resolve_timeout_minutes to alert source provider

## v5.28.0

- Added support for `merge_strategy` on alert attributes


## v5.27.0

- Add support for `message_template` on alert routes

## v5.26.2

- Add plan-time validation to the `branches` operations in `expressions`

## v5.26.1

- Fix error when workflow steps are upgraded

## v5.26.0

- Fixes an issue where you couldn't destroy catalog entries where the type was managed and you were only managing a subset of its attributes
- Fixes an issue where `incident_template.severity` was being required, when it's actually optional

## v5.25.0

- Add support for `owning_team_ids` for alert sources and alert routes

## v5.24.1

- Fix `schema_only` drift when catalog type attribute mode changes

## v5.24.0

- Add support for private alert sources

## v5.23.1
- Fix handling of empty `external_id` in catalog entries

## v5.23.0
- Add "include_private_escalations" option to workflows

## v5.22.0

- Add "incident_escalation_path" data source for getting escalation paths by id or name

## v5.21.1

- Revert `expressions` to use a set type. There order isn't consistent when coming back from the server.

## v5.21.0

- Improve terraform plan performance by using a list type rather than set for `conditions`, `condition_groups` and `expressions`. This may cause a one-time ordering changes in plans, this is expected and will resolve after applying.

## v5.20.0

- Improve terraform plan performance by using a list type rather than set for `array_values`. This may cause a one-time ordering changes in plans, this is expected and will resolve after applying.
- Add support for `auto_relate_grouped_alerts` for Alert Routes

## v5.19.1

- Performance improvement: `incident_catalog_entries` now uses bulk update API to batch updates in groups of 100, significantly reducing API calls for large catalog syncs

## v5.19.0

- Automatically remove resources from Terraform state if they're not found remotely (contribution from @maxtacu)

## v5.18.0

- We will now allow you to use dynamic variables for `working_intervals` in `incident_schedule`.

## v5.17.1

- Handle 404s when an alert attribute has been deleted outside of Terraform (contribution from @maxtacu)

## v5.17.0

- Fix rate limiting by adding missing unit to backoff

## v5.16.0

- Add error logging for panics

## v5.15.0

- Add debug logging for outbound HTTP requests
- Fixed a bug where catalog type attributes weren't defaulting to being marked as managed via Terraform, unless explicitly marked. You may see a change in the state of schema_only in your next plan if you hadn't set it previously.

## v5.14.0

- Fix issue with HTML-like characters (e.g. `>`) in JSON engine values being incorrectly encoded
- Clarify that `source_repo_url` is required for catalog types in documentation
- Update dependencies to latest versions

## v5.13.0

- Support for custom HTTP alert source configuration
- Allow configuration of `ack_mode` for escalation paths
- Improve description for `use_name_as_identifier` field in catalog types to be clearer
- Improve escalation path resource description to be more focused

## v5.12.0

- Fix `incident_catalog_type` to be able to better handle undefined or empty category lists
- Expose `incident_catalog_entries` in the documentation
- No longer escape HTML characters in engine JSON strings
- `incident_alert_attribute` now has an optional `required` property. Set this
  to true for attributes which you expect to be present on all alerts.

## v5.11.0

- Add `incident_catalog_entries` data source to get catalog entries for a specific catalog type. This is useful for
  building up a list of catalog entries which you might be managing via catalog-importer.

## v5.10.0

- Enforce consistent ordering of keys in engine literal values which are JSON objects.
- Made `handovers` on `incident_schedule.config.rotations.*` required, as it
  was always required by the API, but was not marked as such in the provider.
- Switch `runs_on_incident_modes` on workflow resources to be a set and not a
  list, so it's not sensitive to ordering.
- Add `incident_schedule` data source for retrieving existing schedules by ID or name.

## v5.9.1

- Improve documentation to reflect potential values of enumerated values.
- Improve example for `incident_alert_route` documentation.

## v5.9.0

- Update the documentation for `incident_alert_source`, `incident_alert_route`, `incident_escalation_path` and `incident_schedule` to reference the 'Export' flow
  in the dashboard
- Adds `incident_alert_sources` as a plural data source, to retrieve a list of your
  alert sources

## v5.8.0

- `incident_schedule_resource` now uses sets for rotations as the ordering of them does not matter.
- Alert sources and alert routes created by Terraform or imported to Terraform will be tagged
  as such and won't be editable in the incident.io dashboard.

## v5.7.1

- Support up to 3 levels of branch nesting on escalation paths

## v5.7.0

- Improve the documentation for `team_ids` in `incident_escalation_path`
- `incident_alert_source` supports dynamic values for all attributes - for example initialising attributes from local variables.

## v5.6.0

- Improve the documentation for `channel_config` in `incident_alert_route`
- Fix a bug where empty slices of `team_ids` would be sent to the API as `null`
- Fix a bug where empty slices of `managed_attributes` would mean we mark every attribute as managed in Terraform, whereas it should mean every attribute is managed in the dashboard.

## v5.5.0

- Add `grouping_window_seconds` to alert route incident config. This is a required field
  that was being defaulted to 0, meaning any alert route created through terraform ended
  up with grouping disabled.
- Fix a bug where custom fields would show a diff when specified in a different order to
  when the custom field itself was created. As ordering does not matter, this now uses a
  set rather than a list.
- Make `incident_template` required on alert routes. This was previously marked optional,
  but our provider would crash if it was not supplied. This is also required by our API,
  so we have made it required in the provider.

Note that we've decided to release this as a minor version despite the breaking change of
`grouping_window_seconds` being required. This is because the field was previously
defaulted to 0, and so any alert route created through terraform would have had
grouping disabled. As such, we consider this a bug fix and encourage all users to upgrade.

If you want to leave grouping disabled, set `grouping_window_seconds` to 0.

## v5.4.2

- Add validation for RFC3339 timestamp format in `handover_start_at` and `effective_from` fields to prevent invalid dates
- `incident_alert_route` supports dynamic values for all attributes - for example enabling `channel_config` using a variable.
- Fixed a bug where the plan for `incident_alert_route` would always show a diff for `incident_template.name.array_value` and `incident_template.summary.array_value`.

## v5.4.1

- Allow `terraform import` for `incident_catalog_type_attribute`
- Add `terraform import` support to the documentation

## v5.4.0

- Adds `incident_alert_route` resource for managing alert routes.

## v5.3.2

- In the catalog entry resource, we now guard against cases where the type of
  `attribute_values` is inferred to be unknown during the validation of managed
  attributes.

## v5.3.1

When loading workflows, ensure that any additional parameter bindings are
skipped, so that `apply` does not see these as differences.

## v5.3.0

The `incident_schedule` and `incident_escalation_path` resources now support a
`team_ids` attribute to associate those resources with a team.

## v5.3.0-beta1

The `incident_schedule` and `incident_escalation_path` resources now support a
`team_ids` attribute to associate those resources with a team.

## v5.2.0

Temporarily remove `team_ids` support.

## v5.1.0

The `incident_schedule` and `incident_escalation_path` resources now support a
`team_ids` attribute to associate those resources with a team.

## v5.0.0

#### Breaking changes

`incident_catalog_type`'s `source_repo_url` attribute is now required.

This prevents the catalog type from being edited manually, and ensures
there is a link from the incident.io dashboard to the configuration that defines
the catalog type.

#### Schema-only attributes

Sometimes you want to define most of a catalog entry's attributes in Terraform,
but allow other attributes to be edited in the incident.io dashboard.

This is now possible with **schema-only attributes**:

- add `schema_only = true` to the `incident_catalog_type_attribute` resource to
  mark the attribute as schema-only: it will be created by Terraform, but values
  can be edited in the incident.io dashboard.
- add `managed_attributes` to the `incident_catalog_entry` or
  `incident_catalog_entries`, and specify only the attributes that should be
  managed in Terraform.

  By excluding schema-only attributes from this list, changes to those attribute
  values made in the dashboard will not cause unnecessary diffs when you next
  run `terraform plan`.

#### New data sources

There are now data sources for `incident_catalog_type_attribute` and `incident_catalog_entry`.

These allow you to look up an attribute by its catalog type ID and name, and an
entry by its catalog type ID and name, external ID, or alias.

This is useful for:

- managing entries of a catalog type across multiple modules: you can use the
  `incident_catalog_type_attribute` data source to get the ID of attributes,
  without needing to pass the ID between modules.
- using data from your catalog elsewhere in Terraform: for example attributes of
  your `Team` catalog type.

## v4.3.3

- Fixed a panic when using an `incident_escalation_path` with a `notify_channel`
  node that has a `time_to_ack_interval_condition` set.

## v4.3.2

- The order of attributes set within an alert source's `template` block is now
  ignored when planning and applying changes.

## v4.3.1

- Mark the 'email_address' and 'secret_token' fields on incident_alert_source as
  remaining the same for the lifetime of an alert source, to avoid misleading
  plans.

## v4.3.0

- `incident_custom_field` resource now supports catalog-powered custom fields,
  including controlling which attribute is used to group options, add helptext,
  and filter the available options by another field's value.
- `incident_custom_field` data source exposes extra attributes for
  catalog-powered custom fields.

## 4.2.0

- `incident_alert_attribute` resource and data source, for managing your alert attributes
- `incident_alert_source` resource for managing your alert sources
- A number of dependencies have been updated.

## 4.1.0

- Escalation paths created by Terraform or imported to Terraform will be tagged
  as such and won't be editable in the incident.io dashboard.

## 4.0.4

- Updates the documentation for custom fields

## 4.0.3

- Updates to documentation

## 4.0.2

- Adds support for adding slack_channel nodes to escalation paths

## 4.0.1

- Ensures that client operations will fail with errors when an endpoint would
  otherwise have returned 204 no content for a successful operation.

## 4.0.0

- Fixes an issue where the provider might fail to import Terraform state for a schedule with working hours applied

To upgrade to v4.0.0, if you've got on-call schedules with working hours specified in your Terraform code, you'll need to rename the following properties of your `working_intervals`:

- `day` -> `weekday`
- `start` -> `start_time`
- `end` -> `end_time`

## 3.8.11

- Add `external_id` to `resource_catalog_entry`

## 3.8.10

- Fix another issue with condition group arrays that was producing inconsistent apply results

## 3.8.9

- Fix a bug with the serialisation of empty condition group arrays that caused validation errors

## 3.8.8

- Support workflow shortforms for triggering manual workflows.
- Fix regression from 3.8.6 that impacted creating and updating schedules with working intervals

## 3.8.7

- Migrate to a new internal client, no functional changes.

## 3.8.6

- Add support for the `incident_incident_role` data source.

## 3.8.5

- Fixed an issue (#99) where the provider crashed if a round robin config with no minutes was provided

## 3.8.4

- Add support for `holidays_public_config` on the `incident_schedule` resource

## 3.8.3

- Retry on 429 responses from the API, respecting the Retry-After header

## 3.8.2

- Fixed incorrect `produced an unexpected new value` errors when configuring escalation paths

## 3.8.1

- Improved handling of HTTP errors

## 3.8.0

- Add support for `schedule_mode` on the `incident_escalation_path` resource target parameter
- Add support for `round_robin_config` on the `incident_escalation_path` resource level parameter

## 3.7.0

- Add support for path attributes on the `incident_catalog_type_attribute` resource
- Add support for categories on the `incident_catalog_type` resource

## 3.6.0

- `incident_escalation_path` for configuring escalation paths.

## 3.5.0

- data sources for `incident_custom_field` and `incident_custom_field_option`, contributed
  by @mdb

## 3.4.0

- data source for `incident_catalog_type` to allow for lookups of catalog types

## 3.3.1

- Docs update to include examples of `incident_workflow` resource

## 3.3.0

- Add support for workflows using the `incident_workflow` resource

## 3.2.3

- Docs update to include examples of `incident_schedule` resource

## 3.2.2

- Adds supports for on-call schedules using the `incident_schedule` resource
- Adds support for user looksups using the `incident_user` data source

## 3.2.1

- Add support for setting the source_repo_url on catalog types
- Fix a bug where we'd panic if we received a specific kind of error when updating catalog entries

## 3.2.0

- Add support for backlink attributes on catalog types

## 3.1.2

- Marks type_name as requiring a replace, as it is immutable
- Updates our docs so they are a lot clearer on how to connect attributes

## 3.1.1

- Handle 404 for all resources without panicking, and remove resource from state

## 3.1.0

- Add support for setting the `type_name` of a catalog type. This allows
  other catalog attributes to refer to this type by a friendly name, rather than
  the randomly generated ID

## 3.0.0

- Remove SemanticType from catalog types (This has never been used by our
  application, so we've decided to remove it from the provider as we have no
  plans to use it.)
- Move to CustomFieldsV2 API as we are deprecating a number of fields from the
  CustomFieldsV1 API (required, show_before_closure, show_before_creation,
  show_before_update, show_in_announcement_post). These will now be controlled
  via 'Incident Forms' which (for now) will only be available via the web
  dashboard. This will enable users to have much more control over the way they
  configure their incident forms.
- Move to IncidentRolesV2 API as we are deprecating the `required` field from the
  IncidentRolesV1 API. This will now be controlled via 'Incident Forms' which
  (for now) will only be available via the web dashboard. This will enable users
  to have much more control over the way they configure their incident forms.

To upgrade to v3, you will need to remove the deprecated fields from any `custom_field` and `incident_role` resources.
You'll also need to remove any references to `semantic_type`

## 2.0.2

- Handle omission of empty list or null array_value in catalog entries (#36)

## 2.0.1

- Update client to latest API schema
- Remove any disclaimers about the catalog being in beta ahead of launch

## 2.0.0

- Rename `alias` in catalog_entries and catalog_entry to `aliases` in support
  for multiple alias entries
- Handle catalog types having been removed without panicking

## 1.4.3

- Handle 404 for catalog types without panicking

## 1.4.2

- Fix bug in framework patch that meant we never defaulted our log level

## 1.4.1

- Pin the correct dependency to include our logging patch

## 1.4.0

- incident_catalog_entries for large entry counts

## 1.3.1

- Fix bug around omitted empty arrays

## 1.3.0

- Support alias and rank for catalog_entry

## 1.2.0

- Technically new feature, this represents attribute values on catalog entries
  as sets to avoid unnecessary diffs when reordering the attributes

## 1.1.0

- Adds support for catalog types, attributes and entries

## 1.0.3

- Bugfix for terraform provider variables

## 1.0.2

- Fix API key setting via provider attribute
- Provide user-agent of terraform-provider-incident/version for all requests
- Fix creating severities without providing a rank

## 1.0.1

- Severity rank is computed (https://github.com/incident-io/terraform-provider-incident/pull/2)

## 1.0.0

Initial release, including support for:

- Custom fields (`incident_custom_field`)
- Custom field options (`incident_custom_field_option`)
- Incident roles (`incident_incident_role`)
- Severities (`incident_severity`)
- Statuses (`incident_status`)
