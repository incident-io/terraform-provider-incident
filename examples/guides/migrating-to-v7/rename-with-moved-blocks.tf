# Stage two, on v7: the resources keep their configuration and change their name.
# Renaming one changes its resource type as far as Terraform is concerned, so a
# moved block is what tells Terraform the state belongs to the new name rather
# than to something that has been destroyed and replaced.
#
# Terraform reports these as moves and plans no other changes.
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
