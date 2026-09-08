#!/bin/bash

# Import an API key using its ID
# Replace the ID with a real ID from your incident.io organization
terraform import incident_api_key.example 01ABC123DEF456GHI789JKL

# An imported key has no token: the one it was issued with went to whoever created it and
# can't be read back, so `token` is null. It has no token_version either, so a config that
# sets one will rotate the key on the first apply - which is also the only way to get a
# token for a key Terraform has adopted. The plan says so before it happens.
