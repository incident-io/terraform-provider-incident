# Read an existing incident form by its ID, which is in the dashboard's URL when
# editing the form.
data "incident_incident_form" "default_declare" {
  id = "01FCNDV6P870EA6S7TK1DSYDG0"
}

# The full definition is available, so you can (for example) check which custom
# fields a form built in the dashboard asks for.
output "default_declare_custom_fields" {
  value = [
    for element in data.incident_incident_form.default_declare.lifecycle_elements :
    element.custom_field_id if element.element_type == "custom_field"
  ]
}
