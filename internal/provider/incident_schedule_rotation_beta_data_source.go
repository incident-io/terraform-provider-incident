package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ datasource.DataSource                   = &IncidentScheduleRotationBetaDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentScheduleRotationBetaDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentScheduleRotationBetaDataSource{}
)

// scheduleRotationLookupPageSize is the page size used when searching a schedule's
// rotations by name. It's the maximum the endpoint allows, so a schedule resolves in a
// single request unless it has an implausible number of rotations.
const scheduleRotationLookupPageSize = 250

func NewIncidentScheduleRotationBetaDataSource() datasource.DataSource {
	return &IncidentScheduleRotationBetaDataSource{}
}

type IncidentScheduleRotationBetaDataSource struct {
	client *client.ClientWithResponses
}

type IncidentScheduleRotationBetaDataSourceModel struct {
	ID                    types.String                    `tfsdk:"id"`
	ScheduleID            types.String                    `tfsdk:"schedule_id"`
	Name                  types.String                    `tfsdk:"name"`
	Users                 []types.String                  `tfsdk:"users"`
	Handovers             []IncidentScheduleRotationBetaHandover      `tfsdk:"handovers"`
	FirstIntervalStartsAt timetypes.RFC3339               `tfsdk:"first_interval_starts_at"`
	ConcurrentShifts      types.Int64                     `tfsdk:"concurrent_shifts"`
	WorkingIntervals      []IncidentScheduleRotationBetaWorkingWindow `tfsdk:"working_intervals"`
	Rank                  types.Int64                     `tfsdk:"rank"`
	SchedulingMode        types.String                    `tfsdk:"scheduling_mode"`
	EffectiveFrom         timetypes.RFC3339               `tfsdk:"effective_from"`
}

func (d *IncidentScheduleRotationBetaDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_rotation_beta"
}

func (d *IncidentScheduleRotationBetaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a rotation on a schedule by `id` or `name`. Exactly one lookup field should be set.",
		Attributes: map[string]schema.Attribute{
			"schedule_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "schedule_id"),
			},
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Look up the rotation by ID.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Look up the rotation by name, which is unique within the schedule.",
			},
			"users": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "IDs of the people in the rotation, in the order they take shifts.",
			},
			"handovers": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "handovers"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"interval": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "How many of the interval type to wait between handovers.",
						},
						"interval_type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "One of hourly, daily or weekly.",
						},
					},
				},
			},
			"first_interval_starts_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "first_interval_starts_at"),
			},
			"concurrent_shifts": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "concurrent_shifts"),
			},
			"working_intervals": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Weekday intervals the rotation is on call for, if it's restricted to any.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"weekday": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Day of the week, lowercase, e.g. monday.",
						},
						"start_time": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Time of day this window opens, as HH:MM.",
						},
						"end_time": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Time of day this window closes, as HH:MM.",
						},
					},
				},
			},
			"rank": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "rank"),
			},
			"scheduling_mode": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "scheduling_mode"),
			},
			"effective_from": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("ScheduleRotationV3", "effective_from"),
			},
		},
	}
}

func (d *IncidentScheduleRotationBetaDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *IncidentProviderData, got %T. This is a provider bug.", req.ProviderData),
		)
		return
	}
	d.client = data.Client
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are
// Optional and Computed so either can be used, which means setting both would
// otherwise silently ignore one of them.
func (d *IncidentScheduleRotationBetaDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var id, name types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A value that isn't known yet — an id taken from a resource created in the same
	// apply, or either attribute behind an unresolved conditional — is non-null, so
	// judging it here would reject a config that's actually fine.
	if id.IsUnknown() || name.IsUnknown() {
		return
	}

	switch {
	case !id.IsNull() && !name.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or name, not both.")
	case id.IsNull() && name.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
	}
}

func (d *IncidentScheduleRotationBetaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentScheduleRotationBetaDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var rotation *client.ScheduleRotationV3
	switch {
	case !data.ID.IsNull():
		result, err := d.client.SchedulesV3ShowRotationWithResponse(ctx,
			data.ScheduleID.ValueString(), data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read schedule rotation", err.Error())
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError("Unable to read schedule rotation",
				fmt.Sprintf("unexpected response: %s", result.Status()))
			return
		}
		rotation = &result.JSON200.Rotation
	case !data.Name.IsNull():
		got, err := d.findByName(ctx, data.ScheduleID.ValueString(), data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read schedule rotation by name", err.Error())
			return
		}
		rotation = got
	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or name.")
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, incidentScheduleRotationBetaDataSourceFromAPI(*rotation))...)
}

// findByName looks through the schedule's rotations for an exact name match. Names
// are unique within a schedule, so at most one can match.
func (d *IncidentScheduleRotationBetaDataSource) findByName(ctx context.Context, scheduleID, name string) (*client.ScheduleRotationV3, error) {
	var after *string

	for {
		result, err := d.client.SchedulesV3ListRotationsWithResponse(ctx, scheduleID,
			&client.SchedulesV3ListRotationsParams{
				PageSize: lo.ToPtr(int64(scheduleRotationLookupPageSize)),
				After:    after,
			})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing rotations: %s", result.Status())
		}

		for _, rotation := range result.JSON200.Rotations {
			if rotation.Name == name {
				return &rotation, nil
			}
		}

		after = result.JSON200.PaginationMeta.After
		if after == nil {
			break
		}
	}

	return nil, fmt.Errorf("no rotation found named %q on schedule %s", name, scheduleID)
}

func incidentScheduleRotationBetaDataSourceFromAPI(rotation client.ScheduleRotationV3) *IncidentScheduleRotationBetaDataSourceModel {
	users := make([]types.String, len(rotation.Users))
	for i, user := range rotation.Users {
		users[i] = types.StringValue(user.Id)
	}

	handovers := make([]IncidentScheduleRotationBetaHandover, len(rotation.Handovers))
	for i, handover := range rotation.Handovers {
		handovers[i] = IncidentScheduleRotationBetaHandover{
			Interval:     types.Int64Value(handover.Interval),
			IntervalType: types.StringValue(string(handover.IntervalType)),
		}
	}

	windows := []IncidentScheduleRotationBetaWorkingWindow{}
	if rotation.WorkingIntervals != nil {
		for _, window := range *rotation.WorkingIntervals {
			windows = append(windows, IncidentScheduleRotationBetaWorkingWindow{
				Weekday:   types.StringValue(string(window.Weekday)),
				StartTime: types.StringValue(window.StartTime),
				EndTime:   types.StringValue(window.EndTime),
			})
		}
	}

	rank := types.Int64Null()
	if rotation.Rank != nil {
		rank = types.Int64Value(*rotation.Rank)
	}

	schedulingMode := types.StringNull()
	if rotation.SchedulingMode != nil {
		schedulingMode = types.StringValue(string(*rotation.SchedulingMode))
	}

	return &IncidentScheduleRotationBetaDataSourceModel{
		ID:                    types.StringValue(rotation.Id),
		ScheduleID:            types.StringValue(rotation.ScheduleId),
		Name:                  types.StringValue(rotation.Name),
		Users:                 users,
		Handovers:             handovers,
		FirstIntervalStartsAt: timetypes.NewRFC3339TimeValue(rotation.FirstIntervalStartsAt),
		ConcurrentShifts:      types.Int64Value(rotation.ConcurrentShifts),
		WorkingIntervals:      windows,
		Rank:                  rank,
		SchedulingMode:        schedulingMode,
		EffectiveFrom:         timetypes.NewRFC3339TimePointerValue(rotation.EffectiveFrom),
	}
}
