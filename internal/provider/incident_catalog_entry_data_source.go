package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ datasource.DataSource              = &IncidentCatalogEntryDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentCatalogEntryDataSource{}
)

func NewIncidentCatalogEntryDataSource() datasource.DataSource {
	return &IncidentCatalogEntryDataSource{}
}

type IncidentCatalogEntryDataSource struct {
	client *client.ClientWithResponses
}

type IncidentCatalogEntryDataSourceModel struct {
	ID              types.String                 `tfsdk:"id"`
	CatalogTypeID   types.String                 `tfsdk:"catalog_type_id"`
	Identifier      types.String                 `tfsdk:"identifier"`
	Name            types.String                 `tfsdk:"name"`
	ExternalID      types.String                 `tfsdk:"external_id"`
	Aliases         types.List                   `tfsdk:"aliases"`
	Rank            types.Int64                  `tfsdk:"rank"`
	AttributeValues []CatalogEntryAttributeValue `tfsdk:"attribute_values"`
}

func (i *IncidentCatalogEntryDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `This data source provides information about a catalog entry.
It can be used to look up a catalog entry by providing the catalog_type_id and an identifier.

The API will automatically match the identifier against names, external IDs, and aliases.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the catalog entry",
				Computed:            true,
			},
			"catalog_type_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "catalog_type_id"),
				Required:            true,
			},
			"identifier": schema.StringAttribute{
				MarkdownDescription: "The identifier to use for finding the catalog entry. This can be a name, external ID, or alias.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "name"),
				Computed:            true,
			},
			"external_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "external_id"),
				Computed:            true,
			},
			"aliases": schema.ListAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "aliases"),
				Computed:            true,
			},
			"rank": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("CatalogEntryV2", "rank"),
				Computed:            true,
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
	}
}

func (i *IncidentCatalogEntryDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Catalog Entry",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client.Client
}

func (i *IncidentCatalogEntryDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_entry"
}

func (i *IncidentCatalogEntryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentCatalogEntryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Searching for catalog entry with identifier=%s in catalog_type_id=%s",
		data.Identifier.ValueString(), data.CatalogTypeID.ValueString()))

	// Use the identifier parameter to let the API handle the search
	identifier := data.Identifier.ValueString()
	result, err := i.client.CatalogV3ListEntriesWithResponse(ctx, &client.CatalogV3ListEntriesParams{
		CatalogTypeId: data.CatalogTypeID.ValueString(),
		Identifier:    &identifier,
		PageSize:      1, // We only need one result since we're searching by identifier
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find catalog entry, got error: %s", err))
		return
	}

	// Check if we got any matching entries
	var matchedEntry *client.CatalogEntryV3
	if len(result.JSON200.CatalogEntries) > 0 {
		entry := result.JSON200.CatalogEntries[0]
		matchedEntry = &entry
	}

	if matchedEntry == nil {
		resp.Diagnostics.AddError(
			"Catalog Entry Not Found",
			fmt.Sprintf("No catalog entry found with identifier=%s in catalog_type_id=%s",
				data.Identifier.ValueString(), data.CatalogTypeID.ValueString()),
		)
		return
	}

	// Build the data model from the matched entry
	values := buildCatalogEntryAttributeValuesFromV3(matchedEntry.AttributeValues)

	aliases := []attr.Value{}
	for _, alias := range matchedEntry.Aliases {
		aliases = append(aliases, types.StringValue(alias))
	}

	data.ID = types.StringValue(matchedEntry.Id)
	data.Name = types.StringValue(matchedEntry.Name)
	data.ExternalID = types.StringPointerValue(matchedEntry.ExternalId)
	data.Aliases = types.ListValueMust(types.StringType, aliases)
	data.Rank = types.Int64Value(int64(matchedEntry.Rank))
	data.AttributeValues = values

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
