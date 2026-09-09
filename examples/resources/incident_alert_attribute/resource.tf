# Create an alert attribute that points at a single Github user in the catalog.
#
# Take the engine type from the catalog type itself rather than writing it out: a type one
# of our integrations owns is referenced by its registry name, and a type you manage by its
# ID, so the two don't have the same shape.
data "incident_catalog_type" "github_user" {
  type_name = "GithubUser"
}

resource "incident_alert_attribute" "github_user" {
  name     = "Github user"
  type     = data.incident_catalog_type.github_user.attribute_type
  array    = false
  required = true
}

# Create an optional alert attribute for severity information
resource "incident_alert_attribute" "severity" {
  name     = "Severity"
  type     = "String"
  array    = false
  required = false
  emoji    = "warning"
}

# An attribute is account-wide, so declare it once. What differs between environments is the
# binding that fills it in, which belongs to an alert source.
resource "incident_alert_attribute" "gcp_service" {
  name  = "GCP service"
  type  = "String"
  array = false
}

# Staging and production each parse the same attribute out of their own source, so both
# environments' alerts are labelled with one attribute that routes can match on.
resource "incident_alert_source_attribute_beta" "gcp_service_staging" {
  alert_source_id    = incident_alert_source_beta.gcp_staging.id
  alert_attribute_id = incident_alert_attribute.gcp_service.id

  # The payload is opaque JSON, so `payload` as a whole is the only part of it in scope: a
  # reference to "payload.resource.labels.service_name" resolves to nothing. A parse reaches
  # inside it.
  expression {
    start_from = "payload"

    operation {
      parse = {
        # JavaScript, evaluated with the payload bound to `$`.
        function = "$.resource.labels.service_name"
        as       = "String"
      }
    }
  }
}

resource "incident_alert_source_attribute_beta" "gcp_service_production" {
  alert_source_id    = incident_alert_source_beta.gcp_production.id
  alert_attribute_id = incident_alert_attribute.gcp_service.id

  # The payload is opaque JSON, so `payload` as a whole is the only part of it in scope: a
  # reference to "payload.resource.labels.service_name" resolves to nothing. A parse reaches
  # inside it.
  expression {
    start_from = "payload"

    operation {
      parse = {
        # JavaScript, evaluated with the payload bound to `$`.
        function = "$.resource.labels.service_name"
        as       = "String"
      }
    }
  }
}

# Where a source lives in a different workspace to the attribute it binds, read the attribute
# by name rather than declaring it again. Two workspaces declaring the same name will both plan
# cleanly, and then the second one to apply will fail.
data "incident_alert_attribute" "existing_gcp_service" {
  name = "GCP service"
}

resource "incident_alert_source_attribute_beta" "gcp_service_other_workspace" {
  alert_source_id    = incident_alert_source_beta.gcp_other.id
  alert_attribute_id = data.incident_alert_attribute.existing_gcp_service.id

  # The payload is opaque JSON, so `payload` as a whole is the only part of it in scope: a
  # reference to "payload.resource.labels.service_name" resolves to nothing. A parse reaches
  # inside it.
  expression {
    start_from = "payload"

    operation {
      parse = {
        # JavaScript, evaluated with the payload bound to `$`.
        function = "$.resource.labels.service_name"
        as       = "String"
      }
    }
  }
}
