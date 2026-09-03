# Link: a URL synced to Slack bookmarks on the incident channel
# (e.g. External Status Page).
resource "incident_custom_field" "external_status_page" {
  name        = "External Status Page"
  description = "Link to the external status page for this incident."
  field_type  = "link"
}
