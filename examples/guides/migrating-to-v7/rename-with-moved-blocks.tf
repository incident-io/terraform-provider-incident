# Renaming off the _beta names, whenever you like. The old names still work in
# v7, so nothing here is urgent - they warn on every plan and go in v8.
#
# Renaming a resource changes its type as far as Terraform is concerned, so a
# moved block is what tells Terraform the state belongs to the new name rather
# than to something that has been destroyed and replaced. Both names are one
# resource behind one schema, so the provider carries every attribute across and
# Terraform plans no other changes.
#
# Drop the suffix from the references between resources at the same time:
# incident_schedule_beta.platform.id becomes incident_schedule.platform.id.
moved {
  from = incident_schedule_beta.platform
  to   = incident_schedule.platform
}

moved {
  from = incident_schedule_rotation_beta.primary
  to   = incident_schedule_rotation.primary
}

moved {
  from = incident_escalation_path_beta.urgent_support
  to   = incident_escalation_path.urgent_support
}

moved {
  from = incident_alert_source_beta.prometheus
  to   = incident_alert_source.prometheus
}

# One block covers every instance of a resource, so a resource built with
# for_each or count needs one, not one per key.
moved {
  from = incident_alert_source_attribute_beta.per_attribute
  to   = incident_alert_source_attribute.per_attribute
}
