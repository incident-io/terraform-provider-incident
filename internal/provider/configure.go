package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// providerConfig is everything a resource or data source takes from the provider. Embed
// resourceConfigurer or dataSourceConfigurer rather than this, and read the fields directly:
// they promote, so r.client keeps working.
type providerConfig struct {
	client                *client.ClientWithResponses
	terraformVersion      string
	markImportedAsManaged bool
}

// configure reads the provider data, which is nil until the provider itself is configured —
// the framework calls Configure on every resource and data source in the graph regardless.
func (c *providerConfig) configure(providerData any, diags *diag.Diagnostics) {
	if providerData == nil {
		return
	}

	data, ok := providerData.(*IncidentProviderData)
	if !ok {
		diags.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *IncidentProviderData, got: %T. Please report this issue to the provider developers.", providerData),
		)

		return
	}

	c.client = data.Client
	c.terraformVersion = data.TerraformVersion
	c.markImportedAsManaged = data.MarkImportedAsManaged
}

// resourceConfigurer implements Configure for a resource. Embed it in the resource struct.
type resourceConfigurer struct {
	providerConfig
}

func (c *resourceConfigurer) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c.configure(req.ProviderData, &resp.Diagnostics)
}

// dataSourceConfigurer implements Configure for a data source. Embed it in the data source
// struct. It's separate from resourceConfigurer only because the framework gives the two a
// different request and response type, so one method can't serve both.
type dataSourceConfigurer struct {
	providerConfig
}

func (c *dataSourceConfigurer) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c.configure(req.ProviderData, &resp.Diagnostics)
}

// withClient and withClientDataSource build a configurer around a client, for tests that
// construct a resource or data source directly rather than through the provider.
func withClient(c *client.ClientWithResponses) resourceConfigurer {
	return resourceConfigurer{providerConfig{client: c}}
}

func withClientDataSource(c *client.ClientWithResponses) dataSourceConfigurer {
	return dataSourceConfigurer{providerConfig{client: c}}
}
