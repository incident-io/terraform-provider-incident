#!/bin/bash

# An attribute is bound at most once per alert source, so the pair of IDs identifies it.
# The import ID is both IDs, separated by a colon.
# Replace the IDs with real IDs from your incident.io organization.
terraform import incident_alert_source_attribute_beta.example 01ABC123DEF456GHI789JKL:01XYZ987WVU654TSR321QPO
