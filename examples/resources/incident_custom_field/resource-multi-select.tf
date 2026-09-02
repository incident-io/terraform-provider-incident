# Multi-select: like single-select, but more than one option can be picked
# (e.g. Affected Teams). Options are managed separately with
# incident_custom_field_option.
resource "incident_custom_field" "affected_teams" {
  name        = "Affected Teams"
  description = "The teams that are affected by this incident."
  field_type  = "multi_select"
}
