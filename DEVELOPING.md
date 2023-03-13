# Developing

This project runs acceptance tests against a separate incident-io-terraform
account, with its own isolated Slack workspace.

The account is empty and marked as a demo to enable all feature gates.

## incident.io staff

If you work at incident.io as an engineer, you may wish to develop this provider
against your own local instance of the product, such as when building terraform
resources for as-yet released features.

You can do this by:

1. Getting an API key with the relevant scopes from your local instance
2. Running tests with the following environment set:

```console
export INCIDENT_ENDPOINT="https://incident-io-name.eu.ngrok.io/api/public"
export INCIDENT_API_KEY="inc_development_<token>"
```

This points the provider at your local instance via ngrok.
