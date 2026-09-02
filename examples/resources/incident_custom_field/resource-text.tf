# Text: a freeform text field (e.g. Customer ID).
resource "incident_custom_field" "customer_id" {
  name        = "Customer ID"
  description = "The customer identifier associated with this incident."
  field_type  = "text"
}
