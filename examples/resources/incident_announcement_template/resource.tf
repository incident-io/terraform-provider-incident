# A template for major incidents: severity and status first, then a note asking people
# to join. Fields and actions appear on the post in the order they're listed.
resource "incident_announcement_template" "major_incidents" {
  name = "Major incidents"

  fields = [
    {
      field_type = "announcement_post_fields_severity"
      emoji      = "fire"
    },
    {
      field_type = "announcement_post_fields_status"
    },
    {
      field_type = "announcement_post_fields_rich_text"
      rich_text = {
        type = "markdown"
        # Incident variables are written as {{name}}.
        contents = "If you can help, please join {{incident.reference}}"
      }
    },
  ]

  actions = [
    { action_type = "announcement_post_actions_join_call" },
    { action_type = "announcement_post_actions_homepage" },
  ]
}
