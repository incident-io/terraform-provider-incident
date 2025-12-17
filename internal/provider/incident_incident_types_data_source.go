package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ datasource.DataSource              = &IncidentIncidentTypesDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentIncidentTypesDataSource{}
)

func NewIncidentIncidentTypesDataSource() datasource.DataSource {
	return &IncidentIncidentTypesDataSource{}
}

type IncidentIncidentTypesDataSource struct {
	client *client.ClientWithResponses
}

type IncidentIncidentTypesDataSourceModel struct {
	IncidentTypes []IncidentIncidentTypesDataSourceItemModel `tfsdk:"incident_types"`
}

type IncidentIncidentTypesDataSourceItemModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	CreateInTriage       types.String `tfsdk:"create_in_triage"`
	IsDefault            types.Bool   `tfsdk:"is_default"`
	PrivateIncidentsOnly types.Bool   `tfsdk:"private_incidents_only"`
}

func (d *IncidentIncidentTypesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *IncidentIncidentTypesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_types"
}

func (d *IncidentIncidentTypesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentIncidentTypesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get all incident types
	result, err := d.client.IncidentTypesV1ListWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list incident types, got error: %s", err))
		return
	}

	// Convert incident types to the model
	var incidentTypes []IncidentIncidentTypesDataSourceItemModel
	for _, incidentType := range result.JSON200.IncidentTypes {
		incidentTypes = append(incidentTypes, *d.buildItemModel(incidentType))
	}

	modelResp := IncidentIncidentTypesDataSourceModel{
		IncidentTypes: incidentTypes,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}

func (d *IncidentIncidentTypesDataSource) buildItemModel(incidentType client.IncidentTypeV1) *IncidentIncidentTypesDataSourceItemModel {
	return &IncidentIncidentTypesDataSourceItemModel{
		ID:                   types.StringValue(incidentType.Id),
		Name:                 types.StringValue(incidentType.Name),
		Description:          types.StringValue(incidentType.Description),
		CreateInTriage:       types.StringValue(string(incidentType.CreateInTriage)),
		IsDefault:            types.BoolValue(incidentType.IsDefault),
		PrivateIncidentsOnly: types.BoolValue(incidentType.PrivateIncidentsOnly),
	}
}

func (d *IncidentIncidentTypesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides a list of incident types.",
		Attributes: map[string]schema.Attribute{
			"incident_types": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of incident types.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentTypeV1", "id"),
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentTypeV1", "name"),
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentTypeV1", "description"),
						},
						"create_in_triage": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentTypeV1", "create_in_triage"),
						},
						"is_default": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentTypeV1", "is_default"),
						},
						"private_incidents_only": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("IncidentTypeV1", "private_incidents_only"),
						},
					},
				},
			},
		},
	}
}
