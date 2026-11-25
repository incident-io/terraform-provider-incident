package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource                   = &IncidentAccessGroupDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentAccessGroupDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentAccessGroupDataSource{}
)

func NewIncidentAccessGroupDataSource() datasource.DataSource {
	return &IncidentAccessGroupDataSource{}
}

type IncidentAccessGroupDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentAccessGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_group"
}

func (d *IncidentAccessGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Access Groups V2"),
			"Use this data source to look up an access group by `id` or by `slug`. Set exactly one of "+
				"the two. This is how you refer to the base groups every organization has, whose slugs "+
				"are `owner`, `admin` and `standard`, or to a group created in the dashboard."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"slug": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "slug"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "name"),
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "description"),
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("AccessGroupV2", "type"),
				Computed:            true,
			},
		},
	}
}

func (d *IncidentAccessGroupDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *models.AccessGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// An unknown value is non-null, so judging it now would reject a config that is fine.
	if data.ID.IsUnknown() || data.Slug.IsUnknown() {
		return
	}

	switch {
	case !data.ID.IsNull() && !data.Slug.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or slug, not both.")
	case data.ID.IsNull() && data.Slug.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or slug.")
	}
}

func (d *IncidentAccessGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.AccessGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var group *client.AccessGroupV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.AccessGroupsV2ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access group, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access group, unexpected response: %s", result.Status()))
			return
		}
		group = &result.JSON200.AccessGroup

	case !data.Slug.IsNull():
		// A slug is unique within an organization, so the filtered list has at most one entry.
		result, err := d.client.AccessGroupsV2ListWithResponse(ctx, &client.AccessGroupsV2ListParams{
			Slug:     lo.ToPtr(data.Slug.ValueString()),
			PageSize: lo.ToPtr(int64(1)),
		})
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list access groups, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list access groups, unexpected response: %s", result.Status()))
			return
		}
		if len(result.JSON200.AccessGroups) == 0 {
			resp.Diagnostics.AddError("Access Group Not Found", fmt.Sprintf("No access group with slug %q exists.", data.Slug.ValueString()))
			return
		}
		group = &result.JSON200.AccessGroups[0]

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or slug.")
		return
	}

	model := models.AccessGroupDataSourceModel{}.FromAPI(*group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
