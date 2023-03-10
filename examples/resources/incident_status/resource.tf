# Create an additional closed status called "Clean-up".
resource "incident_status" "clean_up" {
  name        = "Clean-up"
  description = "Not yet fully finished, but isn't a live incident anymore."
  category    = "closed"
}
