#!/bin/bash

# Import an incident form using its ID, which is in the dashboard's URL when
# editing the form. The organisation's default forms can't be created, so this
# is how Terraform takes one over.
terraform import incident_incident_form.example 01ABC123DEF456GHI789JKL
