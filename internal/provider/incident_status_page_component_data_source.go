package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentStatusPageComponentDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentStatusPageComponentDataSource{}
)

func NewIncidentStatusPageComponentDataSource() datasource.DataSource {
	return &IncidentStatusPageComponentDataSource{}
}

type IncidentStatusPageComponentDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentStatusPageComponentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_component"
}

func (d *IncidentStatusPageComponentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a status page component by `id`, such as one created in the incident.io " +
			"dashboard, without managing it as an `incident_status_page_component` resource.\n\n" +
			"Only live components can be found; the API does not return archived ones. Any valid API key can " +
			"read components, and no specific permission is needed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("StatusPageComponentV2", "id"),
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("StatusPageComponentV2", "name"),
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("StatusPageComponentV2", "description") + ". Null when the component has none.",
				Computed:            true,
			},
		},
	}
}

func (d *IncidentStatusPageComponentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.StatusPageComponentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.StatusPageComponentsV2ShowWithResponse(ctx, data.ID.ValueString())
	if err == nil && result.JSON200 == nil {
		err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read status page component, got error: %s", err))
		return
	}

	model := models.StatusPageComponentDataSourceModel{}.FromAPI(result.JSON200.StatusPageComponent)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
