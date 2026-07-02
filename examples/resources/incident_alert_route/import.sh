#!/bin/bash

# Import an alert route using its ID. Replace the ID with a real ID from your
# incident.io organization.
#
# The provider automatically detects which schema to import: organisations
# migrated to the new alert grouping engine are imported using the v3 schema
# (grouping_config), and organisations that haven't migrated yet are imported
# using the v2 schema.
terraform import incident_alert_route.example 01ABC123DEF456GHI789JKL
