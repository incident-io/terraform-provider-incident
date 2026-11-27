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
				MarkdownDescription: EnumValuesDescription("AccessGroupV2", "type"),
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
		got, err := d.findBySlug(ctx, data.Slug.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read access group by slug, got error: %s", err))
			return
		}
		group = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or slug.")
		return
	}

	model := models.AccessGroupDataSourceModel{}.FromAPI(*group)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// findBySlug looks for an exact slug match, and requires exactly one. The API filters the
// list by slug, but the provider checks the match itself and walks every page rather than
// trusting the filter to be exact.
func (d *IncidentAccessGroupDataSource) findBySlug(ctx context.Context, slug string) (*client.AccessGroupV2, error) {
	var (
		after   *string
		matches []client.AccessGroupV2
	)

	for {
		result, err := d.client.AccessGroupsV2ListWithResponse(ctx, &client.AccessGroupsV2ListParams{
			Slug:     lo.ToPtr(slug),
			PageSize: lo.ToPtr(int64(250)),
			After:    after,
		})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing access groups: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.AccessGroups, func(group client.AccessGroupV2, _ int) bool {
			return group.Slug == slug
		})...)

		if result.JSON200.PaginationMeta == nil || result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no access group found with slug %q", slug)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d access groups with slug %q; look it up by id instead", len(matches), slug)
	}
}
