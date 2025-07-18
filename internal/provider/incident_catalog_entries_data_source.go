package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ datasource.DataSource              = &IncidentCatalogEntriesDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentCatalogEntriesDataSource{}
)

func NewIncidentCatalogEntriesDataSource() datasource.DataSource {
	return &IncidentCatalogEntriesDataSource{}
}

type IncidentCatalogEntriesDataSource struct {
	client *client.ClientWithResponses
}

type IncidentCatalogEntriesDataSourceModel struct {
	CatalogTypeID  types.String                          `tfsdk:"catalog_type_id"`
	CatalogEntries []IncidentCatalogEntryDataSourceModel `tfsdk:"catalog_entries"`
}

func (d *IncidentCatalogEntriesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *IncidentProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client.Client
}

func (d *IncidentCatalogEntriesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_entries"
}

func (d *IncidentCatalogEntriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentCatalogEntriesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var allEntries []client.CatalogEntryV3

	// Fetch entries for the specified catalog type
	catalogTypeID := data.CatalogTypeID.ValueString()
	params := &client.CatalogV3ListEntriesParams{
		CatalogTypeId: catalogTypeID,
	}

	// Paginate through all entries
	params.PageSize = 100

	for {
		result, err := d.client.CatalogV3ListEntriesWithResponse(ctx, params)
		if err == nil && result.StatusCode() >= 400 {
			err = fmt.Errorf("%s", string(result.Body))
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list catalog entries, got error: %s", err))
			return
		}

		allEntries = append(allEntries, result.JSON200.CatalogEntries...)

		// Check if there are more pages
		if result.JSON200.PaginationMeta.After == nil {
			break
		}
		params.After = result.JSON200.PaginationMeta.After
	}

	// Convert catalog entries to the model
	var catalogEntries []IncidentCatalogEntryDataSourceModel
	for _, entry := range allEntries {
		catalogEntries = append(catalogEntries, *d.buildItemModel(entry))
	}

	modelResp := IncidentCatalogEntriesDataSourceModel{
		CatalogTypeID:  data.CatalogTypeID,
		CatalogEntries: catalogEntries,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

func (d *IncidentCatalogEntriesDataSource) buildItemModel(entry client.CatalogEntryV3) *IncidentCatalogEntryDataSourceModel {
	attributeValues := buildCatalogEntryAttributeValuesFromV3(entry.AttributeValues)

	aliases := []attr.Value{}
	for _, alias := range entry.Aliases {
		aliases = append(aliases, types.StringValue(alias))
	}

	return &IncidentCatalogEntryDataSourceModel{
		ID:              types.StringValue(entry.Id),
		Name:            types.StringValue(entry.Name),
		CatalogTypeID:   types.StringValue(entry.CatalogTypeId),
		ExternalID:      types.StringPointerValue(entry.ExternalId),
		Aliases:         types.ListValueMust(types.StringType, aliases),
		Rank:            types.Int64Value(int64(entry.Rank)),
		AttributeValues: attributeValues,
	}
}

func (d *IncidentCatalogEntriesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides a list of catalog entries for a specific catalog type.",
		Attributes: map[string]schema.Attribute{
			"catalog_type_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The catalog type ID to list entries for.",
			},
			"catalog_entries": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of catalog entries for the specified catalog type.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("CatalogEntryV2", "id"),
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("CatalogEntryV2", "name"),
						},
						"catalog_type_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("CatalogEntryV2", "catalog_type_id"),
						},
						"external_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("CatalogEntryV2", "external_id"),
						},
						"aliases": schema.ListAttribute{
							ElementType:         types.StringType,
							Computed:            true,
							MarkdownDescription: apischema.Docstring("CatalogEntryV2", "aliases"),
						},
						"rank": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("CatalogEntryV2", "rank"),
						},
						"attribute_values": schema.SetNestedAttribute{
							Computed: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"attribute": schema.StringAttribute{
										Description: `The ID of this attribute.`,
										Computed:    true,
									},
									"value": schema.StringAttribute{
										Description: `The value of this attribute, in a format suitable for this attribute type.`,
										Computed:    true,
									},
									"array_value": schema.ListAttribute{
										ElementType: types.StringType,
										Description: `The value of this element of the array, in a format suitable for this attribute type.`,
										Computed:    true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
