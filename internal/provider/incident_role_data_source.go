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
	_ datasource.DataSource              = &IncidentRoleDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentRoleDataSource{}
)

func NewIncidentRoleDataSource() datasource.DataSource {
	return &IncidentRoleDataSource{}
}

func (i *IncidentRoleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_role"
}

type IncidentRoleDataSource struct {
	client *client.ClientWithResponses
}

type IncidentRoleDataSourceModel struct {
	ID           types.String `tfsdk:"id" json:"id"`
	Name         types.String `tfsdk:"name" json:"name"`
	Description  types.String `tfsdk:"description" json:"description"`
	Instructions types.String `tfsdk:"instructions" json:"instructions"`
	Shortform    types.String `tfsdk:"shortform" json:"shortform"`
}

func (i *IncidentRoleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Role Configuration",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	i.client = client.Client
}

func (i *IncidentRoleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides information about an incident role.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "id"),
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "name"),
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "description"),
				Computed:            true,
			},
			"instructions": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "instructions"),
				Computed:            true,
			},
			"shortform": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "shortform"),
				Computed:            true,
			},
		},
	}
}

func (i *IncidentRoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentRoleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	var role *client.IncidentRoleV2
	if !data.ID.IsNull() {
		if resp.Diagnostics.HasError() {
			return
		}
		result, err := i.client.IncidentRolesV2ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read role, got error: %s", err))
			return
		}
		role = &result.JSON200.IncidentRole
	} else {
		resp.Diagnostics.AddError("Client Error", "Unable to read incident role, got error: No ID provided")
		return
	}

	modelResp := &IncidentRoleDataSourceModel{
		ID:           types.StringValue(role.Id),
		Name:         types.StringValue(role.Name),
		Description:  types.StringValue(role.Description),
		Instructions: types.StringValue(role.Instructions),
		Shortform:    types.StringValue(role.Shortform),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &modelResp)...)
}
