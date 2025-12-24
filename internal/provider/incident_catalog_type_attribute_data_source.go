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
	_ datasource.DataSource              = &IncidentCatalogTypeAttributeDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentCatalogTypeAttributeDataSource{}
)

func NewIncidentCatalogTypeAttributeDataSource() datasource.DataSource {
	return &IncidentCatalogTypeAttributeDataSource{}
}

type IncidentCatalogTypeAttributeDataSource struct {
	client *client.ClientWithResponses
}

type IncidentCatalogTypeAttributeDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	CatalogTypeID     types.String `tfsdk:"catalog_type_id"`
	Name              types.String `tfsdk:"name"`
	Type              types.String `tfsdk:"type"`
	Array             types.Bool   `tfsdk:"array"`
	BacklinkAttribute types.String `tfsdk:"backlink_attribute"`
	Path              types.List   `tfsdk:"path"`
	SchemaOnly        types.Bool   `tfsdk:"schema_only"`
}

func (i *IncidentCatalogTypeAttributeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides information about a catalog type attribute.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the catalog type attribute",
				Computed:            true,
			},
			"catalog_type_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CatalogTypeV3", "id"),
				Required:            true,
			},
			"name": schema.StringAttribute{
				Description: "The name of this attribute.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description: "The type of this attribute.",
				Computed:    true,
			},
			"array": schema.BoolAttribute{
				Description: "Whether this attribute is an array or scalar.",
				Computed:    true,
			},
			"backlink_attribute": schema.StringAttribute{
				Description: "If this is a backlink, the id of the attribute that it's linked from",
				Computed:    true,
			},
			"path": schema.ListAttribute{
				Description: "If this is a path attribute, the path that we should use to pull the data",
				ElementType: types.StringType,
				Computed:    true,
			},
			"schema_only": schema.BoolAttribute{
				Description: "If true, Terraform will only manage the schema of the attribute. Values for this attribute can be managed from the incident.io web dashboard.",
				Computed:    true,
			},
		},
	}
}

func (i *IncidentCatalogTypeAttributeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Catalog Type Attribute",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client.Client
}

func (i *IncidentCatalogTypeAttributeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_type_attribute"
}

func (i *IncidentCatalogTypeAttributeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentCatalogTypeAttributeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("Reading catalog type attribute with name=%s for catalog type id=%s", data.Name.ValueString(), data.CatalogTypeID.ValueString()))

	result, err := i.client.CatalogV3ShowTypeWithResponse(ctx, data.CatalogTypeID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read catalog type, got error: %s", err))
		return
	}

	var found bool
	for _, attribute := range result.JSON200.CatalogType.Schema.Attributes {
		if attribute.Name == data.Name.ValueString() {
			data.ID = types.StringValue(attribute.Id)
			data.Type = types.StringValue(attribute.Type)
			data.Array = types.BoolValue(attribute.Array)
			data.SchemaOnly = types.BoolValue(isSchemaOnlyMode(attribute.Mode))

			if attribute.BacklinkAttribute != nil {
				data.BacklinkAttribute = types.StringValue(*attribute.BacklinkAttribute)
			} else {
				data.BacklinkAttribute = types.StringNull()
			}

			data.Path = types.ListNull(types.StringType)
			if attribute.Path != nil {
				path := []attr.Value{}
				for _, item := range *attribute.Path {
					path = append(path, types.StringValue(item.AttributeId))
				}
				data.Path = types.ListValueMust(types.StringType, path)
			}

			found = true
			break
		}
	}

	if !found {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find catalog type attribute with name=%s", data.Name.ValueString()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
