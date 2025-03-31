# Developing

This project runs acceptance tests against a separate incident-io-terraform
account, with its own isolated Slack workspace.

The account is empty and marked as a demo to enable all feature gates.

## incident.io staff

If you work at incident.io as an engineer, you may wish to develop this provider
against your own local instance of the product, such as when building Terraform
resources for as-yet released features.

You can do this by:

1. Getting an API key with the relevant scopes from your local instance
2. Running tests with the following environment set:

```console
export INCIDENT_ENDPOINT="https://incident-io-name.eu.ngrok.io/api/public"
export INCIDENT_API_KEY="inc_development_<token>"
```

This points the provider at your local instance via ngrok.

If you need to regenerate the client, you first need to copy the following file from `core`:

```
# If your core repository is one level up, this would be:
cp ../core/server/lib/openapi/public-schema-v3-including-secret-endpoints.json internal/apischema
```

And then run `go generate ./internal/client`

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

> [!NOTE]
> In CI, we do not run tests that require integrations to be installed
> on the test account, to minimise flakiness. To run these tests locally, add
> extra environment variables (e.g. `TF_ACC_JIRA=1`).

## Running the provider locally

There may be changes where you want to be running the provider itself, rather than
just running tests. To do that you need to:

1. Install Terraform:

```sh
brew install hashicorp/tap/terraform
```

2. Create yourself a project

```
mkdir tmp/project

```

3. Create a `dev.tfrc` file, inside `tmp/project` so that `terraform` uses your local version of the provider:

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
      version = "~> 4"
    }
  }
}

provider "incident" {}

resource "incident_catalog_type" "service_tier" {
  name            = "Service Tier"
  description     = "Level of importance for each service"
  source_repo_url = "https://github.com/incident-io/terraform-provider-incident"
}

```

5. Build the provider binary

```sh
make build
```

6. Start your `terraform` server:

```
TF_CLI_CONFIG_FILE=./dev.tfrc terraform init
```

7. You can now plan and apply your Terraform configuration:

```
TF_CLI_CONFIG_FILE=./dev.tfrc terraform plan

TF_CLI_CONFIG_FILE=./dev.tfrc terraform apply
```

## Schema Attributes - What to use when

### `Optional`

Use `Optional: true` when the user doesn't need to specify an attribute in their config. For example,
`external_id` on `incident_catalog_entry_resource`. Catalog entries don't have to have an external ID,
so we don't make users set one in their terraform config.

### `Computed`

Use `Computed: true` when we generate a value for this attribute if the user doesn't specify one. For
example, `type_name` on `incident_catalog_type_resource`. If the user specifies a type name for a catalog
type, we'll use it. If they don't, we'll generate one based on the name of the type.

### `Default`

Use `Default` when you want to use a particular value for the attribute during planning _when the user
hasn't specified a value_. For example, `team_ids` on `incident_schedule_resource` defaults to an empty set:

```
Default: setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
```

This means when planning, we compare the team IDs that the API returns against the empty set if no team IDs
are specified. This is useful for avoiding any state mismatch problems when you introduce a new attribute
to an existing resource.

### Plan modifier: `UseStateForUnknown`

Use the `UseStateForUnknown` plan modifier when there's an attribute value that the user can't specify,
but you want to avoid the plan being littered with "Known after apply". For example, this is very useful
for ID attributes like on `incident_schedule_resource`:

```
"id": schema.StringAttribute{
  Computed: true,
  PlanModifiers: []planmodifier.String{
    stringplanmodifier.UseStateForUnknown(),
  },
  MarkdownDescription: apischema.Docstring("ScheduleV2", "id"),
},
```

When planning, we'll have an ID in state for each schedule, and this plan modifier will copy the ID
from the state in to the planned value, avoiding the "Known after apply" warning.
