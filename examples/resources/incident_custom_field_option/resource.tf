# Create an Affected Teams custom field that we'll create options against.
resource "incident_custom_field" "affected_teams" {
  name        = "Affected Teams"
  description = "The teams that are affected by this incident."
  field_type  = "multi_select"
  required    = "always"

  show_before_creation      = true
  show_before_closure       = true
  show_before_update        = true
  show_in_announcement_post = true
}

# Create several teams against the parent custom field.
resource "incident_custom_field_option" "teams" {
  for_each = toset([
    "Payments",
    "Dashboard",
    "API",
  ])

  custom_field_id = incident_custom_field.affected_teams.id
  value           = each.value
}
