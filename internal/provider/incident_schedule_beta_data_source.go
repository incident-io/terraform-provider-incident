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
	_ datasource.DataSource                   = &IncidentScheduleBetaDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentScheduleBetaDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentScheduleBetaDataSource{}
)

// scheduleV3LookupPageSize is the page size used when searching for a schedule by
// name. It's the maximum the endpoint allows, so most organisations resolve in a
// single request.
const scheduleV3LookupPageSize = 250

func NewIncidentScheduleBetaDataSource() datasource.DataSource {
	return &IncidentScheduleBetaDataSource{}
}

type IncidentScheduleBetaDataSource struct {
	dataSourceConfigurer
}

type IncidentScheduleBetaDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Timezone types.String `tfsdk:"timezone"`
	TeamIDs  types.Set    `tfsdk:"team_ids"`
}

func (d *IncidentScheduleBetaDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_beta"
}

func (d *IncidentScheduleBetaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a schedule by `id` or `name`. Exactly one lookup field should be set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Look up the schedule by ID.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Look up the schedule by name. Names aren't unique, so this fails if more than one schedule matches.",
			},
			"timezone": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleV3", "timezone"),
			},
			"team_ids": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("ScheduleV3", "team_ids"),
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are
// Optional and Computed so either can be used, which means setting both would
// otherwise silently ignore one of them.
func (d *IncidentScheduleBetaDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *IncidentScheduleBetaDataSourceModel
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

func (d *IncidentScheduleBetaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentScheduleBetaDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var schedule *client.ScheduleV3
	switch {
	case !data.ID.IsNull():
		result, err := d.client.SchedulesV3ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read schedule", err.Error())
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Unable to read schedule", fmt.Sprintf("unexpected response: %s", result.Status()))
			return
		}
		schedule = &result.JSON200.Schedule
	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read schedule by name", err.Error())
			return
		}
		schedule = got
	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	teamIDs := make([]string, len(schedule.TeamIds))
	copy(teamIDs, schedule.TeamIds)
	teamIDsSet, diags := types.SetValueFrom(ctx, types.StringType, teamIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &IncidentScheduleBetaDataSourceModel{
		ID:       types.StringValue(schedule.Id),
		Name:     types.StringValue(schedule.Name),
		Timezone: types.StringValue(schedule.Timezone),
		TeamIDs:  teamIDsSet,
	})...)
}

// findByName pages the schedule list looking for an exact name match, and
// requires exactly one. The list endpoint has no name filter, but it only returns
// a cursor while pages are full, so this terminates on the last page.
func (d *IncidentScheduleBetaDataSource) findByName(ctx context.Context, name string) (*client.ScheduleV3, error) {
	var (
		after   *string
		matches []client.ScheduleV3
	)

	for {
		result, err := d.client.SchedulesV3ListWithResponse(ctx, &client.SchedulesV3ListParams{
			PageSize: lo.ToPtr(int64(scheduleV3LookupPageSize)),
			After:    after,
		})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing schedules: %s", result.Status())
		}

		for _, schedule := range result.JSON200.Schedules {
			if schedule.Name == name {
				matches = append(matches, schedule)
			}
		}

		after = result.JSON200.PaginationMeta.After
		if after == nil {
			break
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no schedule found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d schedules named %q; look it up by id instead", len(matches), name)
	}
}
