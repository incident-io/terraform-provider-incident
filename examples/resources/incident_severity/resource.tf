# Create a Major severity with a default assigned rank.
resource "incident_severity" "trivial" {
  name        = "Trivial"
  description = "Issues causing no impact. No Immediate response is required."
}
