package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ datasource.DataSource              = &IncidentAlertAttributeDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentAlertAttributeDataSource{}
)

func NewIncidentAlertAttributeDataSource() datasource.DataSource {
	return &IncidentAlertAttributeDataSource{}
}

type IncidentAlertAttributeDataSource struct {
	client *client.ClientWithResponses
}

func (i *IncidentAlertAttributeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides information about an alert attribute.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "id"),
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "name"),
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "type"),
				Computed:            true,
			},
			"array": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "array"),
				Computed:            true,
			},
			"required": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("AlertAttributeV2", "required"),
				Computed:            true,
			},
		},
	}
}

func (i *IncidentAlertAttributeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Alert Attribute Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client.Client
}

func (i *IncidentAlertAttributeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_attribute"
}

func (i *IncidentAlertAttributeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentAlertAttributeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := i.client.AlertAttributesV2ListWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read alert attributes, got error: %s", err))
		return
	}

	alertAttributes := result.JSON200.AlertAttributes
	alertAttributes = lo.Filter(alertAttributes, func(aa client.AlertAttributeV2, _ int) bool {
		return aa.Name == data.Name.ValueString()
	})

	var alertAttribute *client.AlertAttributeV2
	if len(alertAttributes) > 0 {
		alertAttribute = lo.ToPtr(alertAttributes[0])
	}

	if alertAttribute == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find alert attribute with name: %s", data.Name.ValueString()))
		return
	}

	modelResp := new(IncidentAlertAttributeResource).buildModel(*alertAttribute, types.BoolValue(alertAttribute.Required))

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}
