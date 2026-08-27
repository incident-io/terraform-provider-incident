package provider

import (
	"context"
	"fmt"
	"os"

	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var _ provider.Provider = &IncidentProvider{}

type IncidentProvider struct {
	version string
}

type IncidentProviderModel struct {
	Endpoint                       types.String `tfsdk:"endpoint"`
	APIKey                         types.String `tfsdk:"api_key"`
	MarkImportedResourcesAsManaged types.Bool   `tfsdk:"mark_imported_resources_as_managed"`
}

type IncidentProviderData struct {
	Client           *client.ClientWithResponses
	TerraformVersion string
	// MarkImportedAsManaged is the resolved value of the provider's
	// mark_imported_resources_as_managed attribute, defaulting to true when it
	// isn't set.
	MarkImportedAsManaged bool
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &IncidentProvider{
			version: version,
		}
	}
}

func (p *IncidentProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "incident"
	resp.Version = p.version
}

func (p *IncidentProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "This project is the official Terraform provider for incident.io.",
		MarkdownDescription: `
This project is the official Terraform provider for incident.io.

With this provider you manage configuration such as incident severities, roles,
custom fields and more inside of your incident.io account.

To view the full documentation of this provider, we recommend reading the
documentation on the [Terraform
Registry](https://registry.terraform.io/providers/incident-io/incident/latest).

## Supported Terraform versions

From v6.0.0 this provider supports Terraform 1.14 and above, and is tested
against the Terraform versions HashiCorp still patch. It is also tested directly
against OpenTofu. Older Terraform releases are no longer tested, and while the
provider may continue to work with them we won't be fixing issues that only
reproduce there. Pin to v5.x if you need to stay on an older CLI.
`,
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "URL of the incident.io API",
				Optional:            true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "API key for incident.io (https://app.incident.io/settings/api-keys). Sourced from the `INCIDENT_API_KEY` environment variable, if set.",
				Optional:            true,
				Sensitive:           true,
			},
			"mark_imported_resources_as_managed": schema.BoolAttribute{
				MarkdownDescription: "Whether importing a resource claims it as managed by Terraform, which is what stops people editing it in the incident.io dashboard. Defaults to `true`. Terraform runs imports during `plan` rather than apply, so this claim is a write to your account during an operation you may expect to be read-only: set this to `false` if plans must leave your account untouched. Creating or updating a resource claims it regardless of this setting, so a resource imported with this off is claimed by the first apply that changes it. It stays editable in the dashboard until then, and indefinitely if its configuration already matches the account and so never produces a change to apply.",
				Optional:            true,
			},
		},
	}
}

func (p *IncidentProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data IncidentProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var endpoint string
	if override := os.Getenv("INCIDENT_ENDPOINT"); override != "" {
		endpoint = override
	} else if data.Endpoint.IsNull() || data.Endpoint.IsUnknown() {
		endpoint = "https://api.incident.io"
	} else {
		endpoint = data.Endpoint.ValueString()
	}

	var apiKey string
	if data.APIKey.IsNull() || data.APIKey.IsUnknown() {
		apiKey = os.Getenv("INCIDENT_API_KEY")
	} else {
		apiKey = data.APIKey.ValueString()
	}

	// Unset means claim on import, which is how the provider behaved before this
	// was configurable.
	markImportedAsManaged := true
	if !data.MarkImportedResourcesAsManaged.IsNull() && !data.MarkImportedResourcesAsManaged.IsUnknown() {
		markImportedAsManaged = data.MarkImportedResourcesAsManaged.ValueBool()
	}

	c, err := client.New(ctx, apiKey, endpoint, p.version)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create incident.io API Client",
			fmt.Sprintf("An error occurred when creating the incident.io API client: %s", err),
		)
		return
	}

	resp.DataSourceData = &IncidentProviderData{
		Client:                c,
		TerraformVersion:      req.TerraformVersion,
		MarkImportedAsManaged: markImportedAsManaged,
	}
	resp.ResourceData = &IncidentProviderData{
		Client:                c,
		TerraformVersion:      req.TerraformVersion,
		MarkImportedAsManaged: markImportedAsManaged,
	}
}

func (p *IncidentProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewIncidentAlertSourceResource,
		NewIncidentCatalogEntriesResource,
		NewIncidentCatalogEntryResource,
		NewIncidentCatalogTypeAttributesResource,
		NewIncidentCatalogTypeResource,
		NewIncidentCustomFieldOptionResource,
		NewIncidentCustomFieldResource,
		NewIncidentEscalationPathResource,
		NewEscalationPathBetaResource,
		NewIncidentRoleResource,
		NewIncidentSeverityResource,
		NewIncidentStatusResource,
		NewIncidentScheduleResource,
		NewIncidentScheduleBetaResource,
		NewIncidentScheduleRotationBetaResource,
		NewIncidentScheduleSyncTargetResource,
		NewIncidentScheduleSyncRuleResource,
		NewIncidentWorkflowResource,
		NewIncidentAlertAttributeResource,
		NewIncidentAlertRouteResource,
		NewIncidentMaintenanceWindowResource,
		NewAlertSourceBetaResource,
		NewAlertSourceAttributeBetaResource,
	}
}

func (p *IncidentProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewIncidentCatalogTypeDataSource,
		NewIncidentCatalogTypeAttributeDataSource,
		NewIncidentCatalogEntryDataSource,
		NewIncidentCatalogEntriesDataSource,
		NewIncidentCustomFieldDataSource,
		NewIncidentCustomFieldOptionDataSource,
		NewIncidentUserDataSource,
		NewIncidentRoleDataSource,
		NewIncidentStatusDataSource,
		NewIncidentAlertAttributeDataSource,
		NewIncidentAlertSourcesDataSource,
		NewIncidentScheduleDataSource,
		NewIncidentScheduleBetaDataSource,
		NewIncidentScheduleRotationBetaDataSource,
		NewIncidentIncidentTypesDataSource,
		NewIncidentEscalationPathDataSource,
		NewIncidentWorkflowDataSource,
		NewRichTextDataSource,
	}
}
