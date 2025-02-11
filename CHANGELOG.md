## Unreleased

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
