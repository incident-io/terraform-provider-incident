package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
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

func (d *AlertSourceAttributeBetaDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source_attribute_beta"
}

func (d *AlertSourceAttributeBetaDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Use this data source to retrieve information about an alert source attribute binding.",
		Attributes: map[string]schema.Attribute{
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
			"value_literal": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A fixed value, shorthand for `value = { literal = ... }`.",
			},
			"value_reference": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "A reference into the scope, shorthand for `value = { reference = ... }`.",
			},
			"expression_ref": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of an expression on this resource, whose result becomes the value.",
			},
			"values": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Several fixed values, shorthand for an `array_value` of literals.",
			},
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

	// Fetch the specific attribute binding
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
