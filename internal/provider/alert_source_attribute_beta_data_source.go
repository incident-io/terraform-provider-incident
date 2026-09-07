package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ datasource.DataSource              = &AlertSourceAttributeBetaDataSource{}
	_ datasource.DataSourceWithConfigure = &AlertSourceAttributeBetaDataSource{}
)

func NewAlertSourceAttributeBetaDataSource() datasource.DataSource {
	return &AlertSourceAttributeBetaDataSource{}
}

type AlertSourceAttributeBetaDataSource struct {
	dataSourceConfigurer
}

func (d *AlertSourceAttributeBetaDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source_attribute_beta"
}

func (d *AlertSourceAttributeBetaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"alert_source_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceAttributeV3", "alert_source_id"),
		},
		"alert_attribute_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: apischema.Docstring("AlertSourceAttributeV3", "alert_attribute_id"),
		},
		"merge_strategy": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: EnumValuesDescription("AlertSourceAttributeV3", "merge_strategy"),
		},
	}
	for name, attr := range models.BindingDataSourceAttributes() {
		attributes[name] = attr
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up one attribute binding on an alert source by `alert_source_id` and `alert_attribute_id`.",
		Attributes:          attributes,
		Blocks: map[string]schema.Block{
			"expression":       models.ExpressionBlockDataSource(),
			"named_expression": models.NamedExpressionBlockDataSource(),
		},
	}
}

func (d *AlertSourceAttributeBetaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data alertSourceAttributeBetaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sourceID := data.AlertSourceID.ValueString()
	attributeID := data.AlertAttributeID.ValueString()

	result, err := d.client.AlertSourcesV3ShowAttributeWithResponse(ctx, sourceID, attributeID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert source attribute, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert source attribute, unexpected response: %s", result.Status()))
		return
	}

	model := alertSourceAttributeBetaFromAPI(result.JSON200.AlertSourceAttribute, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}
