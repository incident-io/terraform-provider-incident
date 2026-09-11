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
	_ datasource.DataSource              = &AlertSourceAttributeDataSource{}
	_ datasource.DataSourceWithConfigure = &AlertSourceAttributeDataSource{}
)

func NewIncidentAlertSourceAttributeDataSource() datasource.DataSource {
	return &AlertSourceAttributeDataSource{}
}

// NewIncidentAlertSourceAttributeBetaDataSource registers this data source under the name
// it had in v6. See aliases.go.
func NewIncidentAlertSourceAttributeBetaDataSource() datasource.DataSource {
	return &AlertSourceAttributeDataSource{betaAlias: alertSourceAttributeBetaAlias}
}

type AlertSourceAttributeDataSource struct {
	dataSourceConfigurer
	betaAlias
}

func (d *AlertSourceAttributeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = d.typeName(req.ProviderTypeName, "_alert_source_attribute")
}

func (d *AlertSourceAttributeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
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
		DeprecationMessage:  d.deprecationMessage(),
		MarkdownDescription: d.description("Look up one attribute binding on an alert source by `alert_source_id` and `alert_attribute_id`."),
		Attributes:          attributes,
		Blocks: map[string]schema.Block{
			"expression":       models.ExpressionBlockDataSource(),
			"named_expression": models.NamedExpressionBlockDataSource(),
		},
	}
}

func (d *AlertSourceAttributeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data alertSourceAttributeModel
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

	model := alertSourceAttributeFromAPI(result.JSON200.AlertSourceAttribute, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}
