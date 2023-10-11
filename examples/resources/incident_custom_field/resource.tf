# Create an Affected Teams multi-select field, required always, shown at all
# opportunities.
resource "incident_custom_field" "affected_teams" {
  name        = "Affected Teams"
  description = "The teams that are affected by this incident."
  field_type  = "multi_select"
}
