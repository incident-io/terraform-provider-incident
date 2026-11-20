#!/bin/bash

# Import a team's grouping preference or the organisation default using its ID. Replace the
# ID with a real ID from your incident.io organization: the public API's team grouping
# preferences endpoint lists each team's, filtered by team_id, and its default grouping
# preferences endpoint returns the organisation default. Creating the default fails once it
# exists, so this is how Terraform takes it over.
terraform import incident_grouping_preference.example 01ABC123DEF456GHI789JKL
