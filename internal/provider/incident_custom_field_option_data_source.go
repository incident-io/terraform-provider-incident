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
	_ datasource.DataSource              = &IncidentCustomFieldOptionDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentCustomFieldOptionDataSource{}
)

func NewIncidentCustomFieldOptionDataSource() datasource.DataSource {
	return &IncidentCustomFieldOptionDataSource{}
}

type IncidentCustomFieldOptionDataSource struct {
	client *client.ClientWithResponses
}

func (i *IncidentCustomFieldOptionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides information about a custom field option.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldOptionV1", "id"),
				Computed:            true,
			},
			"custom_field_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldOptionV1", "custom_field_id"),
				Required:            true,
			},
			"value": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("CustomFieldOptionV1", "value"),
				Required:            true,
			},
			"sort_key": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("CustomFieldOptionV1", "sort_key"),
				Optional:            true,
			},
		},
	}
}

func (i *IncidentCustomFieldOptionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Custom Field Option",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client.Client
}

func (i *IncidentCustomFieldOptionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_field_option"
}

func (i *IncidentCustomFieldOptionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentCustomFieldOptionResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.CustomFieldID.IsNull() {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find custom field options, got error: %s", "custom_field_id must be set"))
		return
	}
	if data.Value.IsNull() {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find custom field options, got error: %s", "value must be set"))
		return
	}

	result, err := i.client.CustomFieldOptionsV1ListWithResponse(ctx, &client.CustomFieldOptionsV1ListParams{
		CustomFieldId: data.CustomFieldID.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read custom field options, got error: %s", err))
		return
	}

	customFieldOptions := result.JSON200.CustomFieldOptions
	customFieldOptions = lo.Filter(customFieldOptions, func(ct client.CustomFieldOptionV1, _ int) bool {
		return ct.Value == data.Value.ValueString()
	})

	var customFieldOption *client.CustomFieldOptionV1
	if len(customFieldOptions) > 0 {
		customFieldOption = lo.ToPtr(customFieldOptions[0])
	}

	if customFieldOption == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to find custom field option, got error: %s", "Custom field option not found"))
		return
	}

	modelResp := new(IncidentCustomFieldOptionResource).buildModel(*customFieldOption)

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}
