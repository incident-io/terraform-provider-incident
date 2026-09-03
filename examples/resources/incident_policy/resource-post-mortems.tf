# The person to chase when a post-mortem falls due. Assignees can also be a
# reference, such as the incident lead; see `assignment_rules.bindings`.
data "incident_user" "postmortem_owner" {
  email = "quality@example.com"
}

# Look the timestamp up by name rather than pinning an ID, which differs between
# organisations.
data "incident_incident_timestamp" "closed" {
  name = "Closed at"
}

# A post-mortem policy: which incidents it covers, what they must satisfy, and
# when that falls due.
#
# There is no policy_type attribute to set. The post_mortem block below is what
# makes this a post-mortem policy, and policy_type is computed from it.
resource "incident_policy" "postmortems" {
  name        = "Post-mortems within 5 working days"
  description = "Major and above incidents need a post-mortem once they close."

  # Which incidents the policy applies to. An empty list applies it to every
  # incident, but the key is always required.
  condition_groups = [
    {
      conditions = [
        {
          subject        = "incident.severity"
          operation      = "gte"
          param_bindings = [{ value_literal = "01FCNDV6P870EA6S7TK1DSYD5H" }]
        }
      ]
    }
  ]

  # Who to chase, and when. Offsets are hours relative to the due date, so -24 is
  # a nudge the day before and 24 a chase the day after.
  assignment_rules = {
    bindings                       = [{ value_literal = data.incident_user.postmortem_owner.id }]
    reminder_due_date_offset_hours = [-24, 0, 24]
  }

  post_mortem = {
    # What a compliant incident looks like. This cannot be empty: a policy that
    # requires nothing could never find an incident non-compliant.
    requirements = [
      {
        conditions = [
          {
            subject        = "post_mortem.status"
            operation      = "one_of"
            param_bindings = [{ values = ["complete"] }]
          }
        ]
      }
    ]

    # Five working days after the incident closed.
    due_date_config = {
      incident_timestamp_id = data.incident_incident_timestamp.closed.id
      days                  = { value_literal = "5" }
      calculation_type      = "weekdays"
    }
  }
}
