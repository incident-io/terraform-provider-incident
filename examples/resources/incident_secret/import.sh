#!/bin/bash

# Import a secret using its ID
# Replace the ID with a real ID from your incident.io organization
terraform import incident_secret.example 01ABC123DEF456GHI789JKL

# An imported secret has no value_wo_version, because the value it already holds didn't
# come from your configuration. If your config sets value_wo and value_wo_version, the
# first apply after importing will rotate the secret to that value - the plan says so.
# To adopt a secret without touching its value, leave both attributes unset.
