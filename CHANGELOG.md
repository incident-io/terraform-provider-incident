## Unreleased

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
