# Numeric: an integer or fractional number (e.g. Customers Affected).
resource "incident_custom_field" "customers_affected" {
  name        = "Customers Affected"
  description = "How many customers are affected by this incident."
  field_type  = "numeric"
}
