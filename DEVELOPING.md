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

## Running tests

The best way to develop against this provider is to write tests. You can run
tests either with `make testacc` or targeting tests like so:

```
TF_ACC=1 dlv test ./internal/provider -- -test.run=TestAccIncidentCatalogEntriesResource
```

Tests run against a real API, either the production/staging incident.io API or
whatever you may be running locally (for incident.io staff). If you're staff,
it's best to use a localhost endpoint for lower latency if possible.

See the above for setting environment variables, otherwise configure your tests
just as you would for a normal environment.

## Releasing

When you want to cut a new release, you can:

1. Merge any of the changes you want in the release to master.
2. Ensure that terraform acceptance tests have passed.
3. Create a new commit on master that adjusts the CHANGELOG so all unreleased
   changes appear under the new version.
4. Push that commit and tag it with whatever your release version should be.

That will trigger the CI pipeline that will publish your provider version to the
terraform registry.
