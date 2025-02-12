package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ datasource.DataSource              = &IncidentCustomFieldDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentCustomFieldDataSource{}
)

func NewIncidentCustomFieldDataSource() datasource.DataSource {
	return &IncidentCustomFieldDataSource{}
}

type IncidentCustomFieldDataSource struct {
	client *client.ClientWithResponses
}

func (i *IncidentCustomFieldDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides information about a custom field.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The custom field ID",
				Computed:            true,
			},
			"field_type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "field_type"),
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "name"),
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "description"),
				Computed:            true,
			},
			"catalog_type_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "catalog_type_id"),
				Computed:            true,
			},
			"filter_by": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "filter_by"),
				Attributes: map[string]schema.Attribute{
					"custom_field_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("CustomFieldFilterByOptionsV2", "custom_field_id"),
						Computed:            true,
					},
					"catalog_attribute_id": schema.StringAttribute{
						MarkdownDescription: apischema.Docstring("CustomFieldFilterByOptionsV2", "catalog_attribute_id"),
						Computed:            true,
					},
				},
			},
			"group_by_catalog_attribute_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "group_by_catalog_attribute_id"),
				Computed:            true,
			},
			"helptext_catalog_attribute_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldV2", "helptext_catalog_attribute_id"),
				Computed:            true,
			},
		},
	}
}

func (i *IncidentCustomFieldDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Custom Field",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client.Client
}

func (i *IncidentCustomFieldDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_field"
}

func (i *IncidentCustomFieldDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentCustomFieldResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := i.client.CustomFieldsV2ListWithResponse(ctx)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read custom fields, got error: %s", err))
		return
	}

	if data.Name.IsNull() {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find custom field, got error: %s", "name must be set"))
		return
	}

	customFields := result.JSON200.CustomFields
	if !data.Name.IsNull() {
		customFields = lo.Filter(customFields, func(ct client.CustomFieldV2, _ int) bool {
			return ct.Name == data.Name.ValueString()
		})
	}

	var customField *client.CustomFieldV2
	if len(customFields) > 0 {
		customField = lo.ToPtr(customFields[0])
	}

	if customField == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find custom field, got error: %s", "Custom field not found"))
		return
	}

	modelResp := new(IncidentCustomFieldResource).buildModel(*customField)

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}
