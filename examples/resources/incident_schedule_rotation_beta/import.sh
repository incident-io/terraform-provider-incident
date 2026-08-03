#!/bin/bash

# Import a rotation using its schedule's ID and its own ID, separated by a colon.
# A rotation is only addressable through the schedule that holds it.
# Replace both IDs with real ones from your incident.io organization.
terraform import incident_schedule_rotation_beta.example 01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX
