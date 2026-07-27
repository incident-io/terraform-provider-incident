#!/bin/bash

# Import a schedule sync rule using the format schedule_id:rule_id
# Sync rules belong to a schedule, so both IDs are required: use the
# ListScheduleSyncRules API endpoint for a schedule to find its rules' IDs
# Replace the IDs with real IDs from your incident.io organization
terraform import incident_schedule_sync_rule.example 01ABC123DEF456GHI789JKL:01MNO456PQR789STU012VWX
