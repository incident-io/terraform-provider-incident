# Create an Affected Teams multi-select field, required always, shown at all
# opportunities.
resource "incident_incident_role" "affected_teams" {
  name        = "Affected Teams"
  description = "The teams that are affected by this incident."
  field_type  = "multi_select"
  required    = "always"

  show_before_creation      = true
  show_before_closure       = true
  show_before_update        = true
  show_in_announcement_post = true
}
