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

## Running the provider locally

There may be changes where you want to be running the provider itself, rather than
just running tests. To do that you need to:

1. Install terraform:
```sh
brew install terraform
```
2. Create yourself a project
```
mkdir tmp/project

```
3. Create a `dev.tfrc` file, inside `tmp/project` so that terraform uses your local version of the provider:
```
provider_installation {
  dev_overrides {
    "incident-io/incident" = "/<PATH-TO-REPO>/terraform-provider-incident/bin"
  }

  direct {}
}
```
4. Create a `main.tf` file including any resources that you want to create (look in `examples/resources` for some nice examples)
```
terraform {
  required_providers {
    incident = {
      source  = "incident-io/incident"
      version = "3.0.0"
    }
  }
}

provider "incident" {}

resource "incident_catalog_type" "service_tier" {
  name        = "Service Tier"
  description = "Level of importance for each service"
}

```
5. Build the provider binary
```sh
make build
```
6. Start your terraform server:
```
TF_CLI_CONFIG_FILE=./dev.tfrc terraform init
```
7. You can now plan and apply your terraform configuration:
```
TF_CLI_CONFIG_FILE=./dev.tfrc terraform plan

TF_CLI_CONFIG_FILE=./dev.tfrc terraform apply
```