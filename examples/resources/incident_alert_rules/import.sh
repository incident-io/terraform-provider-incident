#!/bin/bash

# Import a set of alert rules using its ID. Replace the ID with a real ID from your
# incident.io organization: the public API's list endpoint returns each set with its kind
# and team. Creating the organization's required or default rules fails once they exist,
# so this is how Terraform takes them over. Copy each action's id from state into your
# configuration after the import, or the next apply updates the set once to drop them.
terraform import incident_alert_rules.example 01ABC123DEF456GHI789JKL
