#!/bin/bash

# Import a pay config using its ID
# Replace the ID with a real ID from your incident.io organization
terraform import incident_pay_config.example 01ABC123DEF456GHI789JKL

# The rules come with it, in the order the API holds them. Write weekly_rules in your
# configuration in that same order, or the first plan updates every rule that moved.
