#!/bin/bash

# Import a team grouping preference using its ID. Replace the ID with a real ID from your
# incident.io organization: the Grouping tab of alert routing lists each team's preference,
# and the public API's list endpoint filters them by team_id.
terraform import incident_team_grouping_preference.example 01ABC123DEF456GHI789JKL
