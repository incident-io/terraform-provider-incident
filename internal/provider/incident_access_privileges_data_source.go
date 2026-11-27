package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentAccessPrivilegesDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentAccessPrivilegesDataSource{}
)

func NewIncidentAccessPrivilegesDataSource() datasource.DataSource {
	return &IncidentAccessPrivilegesDataSource{}
}

type IncidentAccessPrivilegesDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentAccessPrivilegesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_privileges"
}

func (d *IncidentAccessPrivilegesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Access Privileges V2"),
		Attributes: map[string]schema.Attribute{
			"include_deprecated": schema.BoolAttribute{
				MarkdownDescription: "Whether to include deprecated privileges. Defaults to `false`.",
				Optional:            true,
			},
			"privileges": schema.ListNestedAttribute{
				MarkdownDescription: "The privileges an access grant can give.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "name"),
							Computed:            true,
						},
						"label": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "label"),
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "description"),
							Computed:            true,
						},
						"parents": schema.ListAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "parents"),
							Computed:            true,
							ElementType:         types.StringType,
						},
						"children": schema.ListAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "children"),
							Computed:            true,
							ElementType:         types.StringType,
						},
						"aliases": schema.ListAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "aliases"),
							Computed:            true,
							ElementType:         types.StringType,
						},
						"deprecated": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "deprecated"),
							Computed:            true,
						},
						"replaced_by": schema.ListAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "replaced_by"),
							Computed:            true,
							ElementType:         types.StringType,
						},
						"grants_nothing": schema.BoolAttribute{
							MarkdownDescription: apischema.Docstring("AccessPrivilegeV2", "grants_nothing"),
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *IncidentAccessPrivilegesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.AccessPrivilegesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := &client.AccessPrivilegesV2ListParams{}
	if !data.IncludeDeprecated.IsNull() && !data.IncludeDeprecated.IsUnknown() {
		params.IncludeDeprecated = lo.ToPtr(data.IncludeDeprecated.ValueBool())
	}

	result, err := d.client.AccessPrivilegesV2ListWithResponse(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list access privileges, got error: %s", err))
		return
	}
	if result.JSON200 == nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list access privileges, unexpected response: %s", result.Status()))
		return
	}

	data.Privileges = lo.Map(result.JSON200.AccessPrivileges, func(privilege client.AccessPrivilegeV2, _ int) models.AccessPrivilegeModel {
		return models.AccessPrivilegeModel{}.FromAPI(privilege)
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
