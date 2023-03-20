## Unreleased

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
