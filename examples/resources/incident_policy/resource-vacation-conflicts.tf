# A vacation conflict policy, which flags responders rota'd on while they are
# away. The type has nothing to configure, so its block is empty: it is only
# there to say which type this is.
#
# Like on-call readiness, it takes no assignment_rules: the API assigns the user
# the finding is about.
resource "incident_policy" "vacation_conflicts" {
  name        = "No on-call during vacation"
  description = "Flag anyone scheduled on call while they are on leave."

  condition_groups = []

  vacation_conflict = {}
}
