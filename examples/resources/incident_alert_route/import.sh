#!/bin/bash

# Import an alert route using its ID. Replace the ID with a real ID from your
# incident.io organization.
#
# The provider automatically detects which configuration format to import:
# alert routes where the current format is available import using it
# (grouping_config), and routes where it isn't available yet import using the
# deprecated format.
terraform import incident_alert_route.example 01ABC123DEF456GHI789JKL
