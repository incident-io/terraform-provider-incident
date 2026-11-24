#!/bin/bash

# Import a team's grouping preference or the organisation default using its ID. Replace the
# ID with a real ID from your incident.io organization: the public API's alert grouping
# preferences endpoint lists them, filtered by kind or team_id. Creating the default fails
# once it exists, so this is how Terraform takes it over.
terraform import incident_alert_grouping_preference.example 01ABC123DEF456GHI789JKL
