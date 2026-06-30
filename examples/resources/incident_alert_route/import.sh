#!/bin/bash

# Import an alert route using its ID. Replace the ID with a real ID from your
# incident.io organization.

# A bare ID imports a route managed with the v2 schema (the default):
terraform import incident_alert_route.example 01ABC123DEF456GHI789JKL

# Prefix the ID with "v3:" to import a route you manage with the v3 schema
# (i.e. one whose configuration sets the grouping_config block). This is needed
# because the same route is readable through both APIs, so the ID alone can't
# tell Terraform which schema your configuration uses.
terraform import incident_alert_route.example v3:01ABC123DEF456GHI789JKL
