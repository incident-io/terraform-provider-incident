package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/samber/lo"
)

var (
	_ datasource.DataSource              = &IncidentCatalogTypeDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentCatalogTypeDataSource{}
)

func NewIncidentCatalogTypeDataSource() datasource.DataSource {
	return &IncidentCatalogTypeDataSource{}
}

type IncidentCatalogTypeDataSource struct {
	client *client.ClientWithResponses
}

func (i *IncidentCatalogTypeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides information about a catalog type.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogTypeV2ResponseBody", "id"),
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogV2CreateTypeRequestBody", "name"),
				Optional:            true,
				Computed:            true,
			},
			"type_name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogV2CreateTypeRequestBody", "type_name"),
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogTypeV2ResponseBody", "description"),
				Computed:            true,
			},
			"source_repo_url": schema.StringAttribute{
				MarkdownDescription: "The url of the external repository where this type is managed. When set, users will not be able to edit the catalog type (or its entries) via the UI, and will instead be provided a link to this URL.",
				Computed:            true,
			},
		},
	}
}

func (i *IncidentCatalogTypeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source User",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client.Client
}

func (i *IncidentCatalogTypeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_type"
}

func (i *IncidentCatalogTypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentCatalogTypeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := i.client.CatalogV2ListTypesWithResponse(ctx)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog types, got error: %s", err))
		return
	}

	if data.Name.IsNull() && data.TypeName.IsNull() {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find catalog type, got error: %s", "Either name or type_name must be set"))
		return
	}

	catalogTypes := result.JSON200.CatalogTypes
	if !data.Name.IsNull() {
		catalogTypes = lo.Filter(catalogTypes, func(ct client.CatalogTypeV2, _ int) bool {
			return ct.Name == data.Name.ValueString()
		})
	}
	if !data.TypeName.IsNull() {
		catalogTypes = lo.Filter(catalogTypes, func(ct client.CatalogTypeV2, _ int) bool {
			return ct.TypeName == data.TypeName.ValueString()
		})
	}

	var catalogType *client.CatalogTypeV2
	if len(catalogTypes) > 0 {
		catalogType = lo.ToPtr(catalogTypes[0])
	}

	if catalogType == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find catalog type, got error: %s", "Catalog type not found"))
		return
	}

	modelResp := new(IncidentCatalogTypeResource).buildModel(*catalogType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}
