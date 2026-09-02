# Single-select: one value from a predefined list of options (e.g. Detection Method).
# Options are managed separately with incident_custom_field_option.
resource "incident_custom_field" "detection_method" {
  name        = "Detection Method"
  description = "How this incident was detected."
  field_type  = "single_select"
}
