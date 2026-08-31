# Rich text fields hold a document, not a string, so build one from markdown
# rather than writing the JSON by hand. Variables written as {{ }} become
# references into the workflow scope when the message is sent - a plain string
# literal is stored as literal text and won't interpolate. The feature set must
# match the field it's going into: a Slack message is "mrkdwn".
data "incident_rich_text" "exec_page" {
  markdown    = "**Exec page for {{ incident.name }}**\n\n{{ form.reason }}\n\nJoin the incident channel: {{ incident.slack_channel }}"
  feature_set = "mrkdwn"
}

# This is a manually triggered workflow that collects information from the user
# via form fields when they run it. Each field's value is available in the
# workflow scope as `form.<key>`, for both conditions and steps to bind
# against - as the message below does with `form.reason`. That's the prefix the
# dashboard shows as "Form → <title>".
#
# The order of the list is the order the fields appear in the form. Either
# form_fields = [] or omitting the attribute clears any existing fields.
#
# A field's `id` is computed, never written: the provider correlates fields
# across applies by `key`, so retitling a field keeps its identity (and any
# in-flight runs), while changing its key replaces it. That's also why the key,
# not the title, is what the scope reference uses.
resource "incident_workflow" "page_execs" {
  name    = "Page execs"
  trigger = "manual"
  expressions = [
  ]
  condition_groups = [
  ]
  steps = [
    {
      # "Send direct message"
      id   = "01KG1326KD7DT1Z785SYVXXP98" # This is the ID of the step in the workflow, and must be a ULID
      name = "slack.send_message"
      param_bindings = [
        # "Recipient(s)", an array parameter: array-typed bindings always use
        # array_value, even when a single reference supplies every element.
        {
          array_value = [
            {
              reference = "form.execs"
            },
          ]
        },
        # "Message"
        {
          value = {
            literal = data.incident_rich_text.exec_page.json
          }
        },
        # "Timezone", left unset so the step falls back to UTC
        {},
      ]
    },
  ]
  form_fields = [
    {
      key         = "reason"
      title       = "Reason for paging"
      type        = "String"
      description = "Why are we paging the execs?"
      array       = false
      required    = true
    },
    {
      key         = "execs"
      title       = "Who should we page?"
      type        = "User"
      description = "The execs to message. Leave the on-call exec out, they're paged already."
      array       = true
      required    = true
    },
  ]
  once_for = [
    "incident",
  ]
  private_incident_scope = "none"
  continue_on_step_error = false
  runs_on_incidents      = "newly_created_and_active"
  runs_on_incident_modes = [
    "standard",
  ]
  state = "draft"
}
