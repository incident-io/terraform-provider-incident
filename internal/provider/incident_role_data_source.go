package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

var (
	_ datasource.DataSource                   = &IncidentRoleDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentRoleDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentRoleDataSource{}
)

func NewIncidentRoleDataSource() datasource.DataSource {
	return &IncidentRoleDataSource{}
}

func (i *IncidentRoleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_incident_role"
}

type IncidentRoleDataSource struct {
	dataSourceConfigurer
}

type IncidentRoleDataSourceModel struct {
	ID           types.String `tfsdk:"id" json:"id"`
	Name         types.String `tfsdk:"name" json:"name"`
	Description  types.String `tfsdk:"description" json:"description"`
	Instructions types.String `tfsdk:"instructions" json:"instructions"`
	Shortform    types.String `tfsdk:"shortform" json:"shortform"`
}

func (i *IncidentRoleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This data source provides information about an incident role, looked up either by `id` or by `name`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("IncidentRoleV2", "name"),
				Optional:            true,
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

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are
// Optional and Computed so either can be used, which means setting both would
// otherwise silently ignore one of them.
func (i *IncidentRoleDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *IncidentRoleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	// A value that isn't known yet — an id taken from a resource created in the
	// same apply, or either attribute behind an unresolved conditional — is
	// non-null, so judging it here would reject a config that's actually fine.
	if data.ID.IsUnknown() || data.Name.IsUnknown() {
		return
	}

	switch {
	case !data.ID.IsNull() && !data.Name.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or name, not both.")
	case data.ID.IsNull() && data.Name.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
	}
}

func (i *IncidentRoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentRoleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var role *client.IncidentRoleV2
	switch {
	case !data.ID.IsNull():
		result, err := i.client.IncidentRolesV2ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident role, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident role, unexpected response: %s", result.Status()))
			return
		}

		role = &result.JSON200.IncidentRole

	case !data.Name.IsNull():
		got, err := i.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read incident role by name, got error: %s", err))
			return
		}

		role = got

	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
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

// findByName looks for an exact name match in the role list, and requires
// exactly one. The list endpoint has no name filter and isn't paginated, so
// every role comes back in a single request.
func (i *IncidentRoleDataSource) findByName(ctx context.Context, name string) (*client.IncidentRoleV2, error) {
	result, err := i.client.IncidentRolesV2ListWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response listing incident roles: %s", result.Status())
	}

	matches := lo.Filter(result.JSON200.IncidentRoles, func(role client.IncidentRoleV2, _ int) bool {
		return role.Name == name
	})

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no incident role found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d incident roles named %q; look it up by id instead", len(matches), name)
	}
}
