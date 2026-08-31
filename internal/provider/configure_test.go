package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// Configure is promoted from an embedded configurer rather than declared on each type, so
// assert every registered resource and data source still satisfies the framework interface
// and actually takes the client. A missing Configure fails at apply with a nil client, not at
// compile time.
func TestEveryResourceIsConfigurable(t *testing.T) {
	p := &IncidentProvider{}
	api := &client.ClientWithResponses{}
	providerData := &IncidentProviderData{
		Client:                api,
		TerraformVersion:      "1.14.0",
		MarkImportedAsManaged: true,
	}

	for _, newResource := range p.Resources(context.Background()) {
		r := newResource()
		name := fmt.Sprintf("%T", r)

		t.Run(name, func(t *testing.T) {
			configurable, ok := r.(resource.ResourceWithConfigure)
			require.True(t, ok, "%s does not implement resource.ResourceWithConfigure", name)

			resp := &resource.ConfigureResponse{}
			configurable.Configure(context.Background(),
				resource.ConfigureRequest{ProviderData: providerData}, resp)

			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			assert.Same(t, api, clientOf(t, r), "%s did not take the client", name)
		})
	}

	for _, newDataSource := range p.DataSources(context.Background()) {
		d := newDataSource()
		name := fmt.Sprintf("%T", d)

		t.Run(name, func(t *testing.T) {
			configurable, ok := d.(datasource.DataSourceWithConfigure)
			require.True(t, ok, "%s does not implement datasource.DataSourceWithConfigure", name)

			resp := &datasource.ConfigureResponse{}
			configurable.Configure(context.Background(),
				datasource.ConfigureRequest{ProviderData: providerData}, resp)

			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			assert.Same(t, api, clientOf(t, d), "%s did not take the client", name)
		})
	}
}

// The framework calls Configure on everything in the graph before the provider itself is
// configured, handing it a nil ProviderData. That has to be a no-op, not an error.
func TestConfigureToleratesNilProviderData(t *testing.T) {
	var c providerConfig
	var diags diag.Diagnostics

	c.configure(nil, &diags)

	assert.False(t, diags.HasError())
	assert.Nil(t, c.client)
}

func TestConfigureRejectsUnexpectedProviderData(t *testing.T) {
	var c providerConfig
	var diags diag.Diagnostics

	c.configure("not the provider data", &diags)

	require.True(t, diags.HasError())
	assert.Contains(t, diags.Errors()[0].Detail(), "Expected *IncidentProviderData")
}

// configuredClient is declared here rather than in configure.go because only the tests need
// it. Embedding promotes it, so it reaches the client of any resource or data source.
func (c providerConfig) configuredClient() *client.ClientWithResponses {
	return c.client
}

// clientOf reaches the promoted client field, whichever configurer the type embeds.
func clientOf(t *testing.T, v any) *client.ClientWithResponses {
	t.Helper()

	c, ok := v.(interface {
		configuredClient() *client.ClientWithResponses
	})
	require.True(t, ok, "%T embeds no configurer", v)

	return c.configuredClient()
}

// configure sets everything the provider hands over, not just the client.
func TestConfigureTakesAllProviderData(t *testing.T) {
	var c providerConfig
	var diags diag.Diagnostics
	api := &client.ClientWithResponses{}

	c.configure(&IncidentProviderData{
		Client:                api,
		TerraformVersion:      "1.15.2",
		MarkImportedAsManaged: false,
	}, &diags)

	require.False(t, diags.HasError())
	assert.Same(t, api, c.client)
	assert.Equal(t, "1.15.2", c.terraformVersion)
	assert.False(t, c.markImportedAsManaged)
}
