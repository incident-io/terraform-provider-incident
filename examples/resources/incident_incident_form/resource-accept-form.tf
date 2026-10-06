# An accept form for one incident type: what responders fill in when they accept
# a triage incident as real. Everything a declare form asks for is available here
# too, except triage and visibility, which the accepted incident already has.
data "incident_incident_type" "customer_report" {
  name = "Customer report"
}

data "incident_incident_role" "comms_lead" {
  name = "Communications Lead"
}

data "incident_incident_timestamp" "impact_started" {
  name = "Impact started"
}

resource "incident_incident_form" "customer_report_accept" {
  form_type        = "accept"
  incident_type_id = data.incident_incident_type.customer_report.id

  lifecycle_elements = [
    { element_type = "name" },
    {
      element_type = "severity"
      required_if  = "always_require"
    },
    {
      element_type     = "incident_role"
      incident_role_id = data.incident_incident_role.comms_lead.id
      description      = "Customer reports need someone owning the reply."
    },
    {
      element_type          = "timestamp"
      incident_timestamp_id = data.incident_incident_timestamp.impact_started.id
      description           = "When the customer first saw the problem, if they said."
    },
    { element_type = "summary" },
  ]
}
