# Create a Major severity with a default assigned rank.
resource "incident_severity" "major" {
  name        = "Major"
  description = "Issues causing significant impact. Immediate response is usually required."
}
