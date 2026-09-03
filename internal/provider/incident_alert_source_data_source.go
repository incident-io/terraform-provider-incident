package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
)

var (
	_ datasource.DataSource              = &IncidentAlertSourceDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentAlertSourceDataSource{}
)

func NewIncidentAlertSourceDataSource() datasource.DataSource {
	return &IncidentAlertSourceDataSource{}
}

type IncidentAlertSourceDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentAlertSourceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source"
}

func (d *IncidentAlertSourceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := alertSourceDataSourceItemAttributes()
	attributes["id"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: apischema.Docstring("AlertSourceV2", "id"),
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve a single alert source by ID.",
		Attributes:          attributes,
	}
}

func (d *IncidentAlertSourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentAlertSourcesDataSourceItemModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.AlertSourcesV2ShowWithResponse(ctx, data.ID.ValueString())
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf("%s", result.Body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert source, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert source, unexpected response: %s", result.Status()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, alertSourceDataSourceItemFromAPI(result.JSON200.AlertSource))...)
}
