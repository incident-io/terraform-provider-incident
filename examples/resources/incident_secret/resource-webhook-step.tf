# A secret exists to be used, and a workflow's "Send a webhook" step is the main place
# that happens. It can take a secret two different ways, and this workflow uses both.
variable "partner_webhook_token" {
  type      = string
  ephemeral = true
}

resource "incident_secret" "partner_webhook_token" {
  name        = "Partner webhook token"
  description = "Bearer token for the partner's incident webhook"

  value_wo         = var.partner_webhook_token
  value_wo_version = 1
}

# The first way: put the secret's value inside a field's text.
#
# A field whose feature set ends in `_with_secrets` - which the webhook step's headers do -
# accepts a variable naming a secret by ID, and incident.io substitutes the value when the
# workflow runs. Write it as `{{ <secret id> }}` and reference the secret's `id`, never its
# value: Terraform is arranging for the value to be used, not handling it.
data "incident_rich_text" "authorization_header" {
  markdown    = "Authorization: Bearer {{ ${incident_secret.partner_webhook_token.id} }}"
  feature_set = "plain_single_line_with_secrets"
}

data "incident_rich_text" "content_type_header" {
  markdown    = "Content-Type: application/json"
  feature_set = "plain_single_line_with_secrets"
}

data "incident_rich_text" "webhook_endpoint" {
  markdown    = "https://partner.example.com/hooks/incident"
  feature_set = "plain_single_line"
}

data "incident_rich_text" "webhook_body" {
  markdown    = <<-EOT
    {{ incident.name }} is now {{ incident.status }} at severity {{ incident.severity }}.
  EOT
  feature_set = "rich"
}

resource "incident_workflow" "notify_partner" {
  name        = "Notify our partner of incident updates"
  trigger     = "incident.updated"
  expressions = []
  condition_groups = [
  ]
  steps = [
    {
      # "Send a webhook"
      id   = "01K5TSNDBF7KBZY9AFDKYXJM3N" # This is the ID of the step in the workflow, and must be a ULID
      name = "webhook.send"
      param_bindings = [
        # "Endpoint URL"
        { value_literal = data.incident_rich_text.webhook_endpoint.json },
        # "HTTP Method"
        { value_literal = "POST" },
        # "Headers", which are a list: each entry is one "key: value" line. The first of
        # these is the header carrying the secret.
        { values = [
          data.incident_rich_text.authorization_header.json,
          data.incident_rich_text.content_type_header.json,
        ] },
        # "Body"
        { value_literal = data.incident_rich_text.webhook_body.json },
        # The second way: "Signing secret". This one takes the secret's ID directly rather
        # than through a document, because the step signs the request with it (HMAC-SHA256)
        # instead of writing it into the payload. The partner verifies the signature using
        # the same value.
        { value_literal = incident_secret.partner_webhook_token.id },
        # "Generated signing secret", which incident.io generates for you when you haven't
        # chosen a secret of your own. Left empty, because the secret above is the one
        # signing this request.
        {},
        # "Signature header", empty for the default of webhook-signature.
        {},
      ]
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
