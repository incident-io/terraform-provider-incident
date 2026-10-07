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
	_ datasource.DataSource                   = &IncidentMaintenanceWindowDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentMaintenanceWindowDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentMaintenanceWindowDataSource{}
)

func NewIncidentMaintenanceWindowDataSource() datasource.DataSource {
	return &IncidentMaintenanceWindowDataSource{}
}

type IncidentMaintenanceWindowDataSource struct {
	dataSourceConfigurer
}

// maintenanceWindowDataSourceModel is MaintenanceWindowResourceModel without
// force_destroy, which only says how the resource may be destroyed and is nothing the
// API reports.
type maintenanceWindowDataSourceModel struct {
	ID                       types.String                             `tfsdk:"id"`
	Name                     types.String                             `tfsdk:"name"`
	StartAt                  types.String                             `tfsdk:"start_at"`
	EndAt                    types.String                             `tfsdk:"end_at"`
	LeadID                   types.String                             `tfsdk:"lead_id"`
	AlertConditionGroups     models.IncidentEngineConditionGroups     `tfsdk:"alert_condition_groups"`
	ShowInSidebar            types.Bool                               `tfsdk:"show_in_sidebar"`
	ResolveOnEnd             types.Bool                               `tfsdk:"resolve_on_end"`
	RerouteOnEnd             types.Bool                               `tfsdk:"reroute_on_end"`
	EscalationTargets        []MaintenanceWindowEscalationTargetModel `tfsdk:"escalation_targets"`
	NotifyChannels           []MaintenanceWindowNotifyChannelModel    `tfsdk:"notify_channels"`
	NotifyStartMinutesBefore types.Int64                              `tfsdk:"notify_start_minutes_before"`
	NotifyEndMinutesBefore   types.Int64                              `tfsdk:"notify_end_minutes_before"`
	NotificationMessage      types.String                             `tfsdk:"notification_message"`
	IncidentID               types.String                             `tfsdk:"incident_id"`
}

// maintenanceWindowDataSourceModelFromAPI converts through the resource's buildModel so
// the two agree on every field, then leaves out the one this model doesn't carry.
func maintenanceWindowDataSourceModelFromAPI(window client.MaintenanceWindowV1) maintenanceWindowDataSourceModel {
	full := (&IncidentMaintenanceWindowResource{}).buildModel(window, nil)

	return maintenanceWindowDataSourceModel{
		ID:                       full.ID,
		Name:                     full.Name,
		StartAt:                  full.StartAt,
		EndAt:                    full.EndAt,
		LeadID:                   full.LeadID,
		AlertConditionGroups:     full.AlertConditionGroups,
		ShowInSidebar:            full.ShowInSidebar,
		ResolveOnEnd:             full.ResolveOnEnd,
		RerouteOnEnd:             full.RerouteOnEnd,
		EscalationTargets:        full.EscalationTargets,
		NotifyChannels:           full.NotifyChannels,
		NotifyStartMinutesBefore: full.NotifyStartMinutesBefore,
		NotifyEndMinutesBefore:   full.NotifyEndMinutesBefore,
		NotificationMessage:      full.NotificationMessage,
		IncidentID:               full.IncidentID,
	}
}

func (d *IncidentMaintenanceWindowDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (d *IncidentMaintenanceWindowDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("MaintenanceWindows V1"),
			"Use this data source to look up an existing maintenance window, either by `id` or by `name`, "+
				"without managing it as an `incident_maintenance_window` resource. Set exactly one of the two "+
				"lookup attributes; setting both, or neither, is rejected at plan time. An archived window "+
				"cannot be found by name."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "id"),
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "name"),
				Optional:            true,
				Computed:            true,
			},
			"start_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "start_at"),
				Computed:            true,
			},
			"end_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "end_at"),
				Computed:            true,
			},
			"lead_id": schema.StringAttribute{
				MarkdownDescription: "The incident.io user ID of the lead for this maintenance window",
				Computed:            true,
			},
			"alert_condition_groups": models.ConditionGroupsDataSourceAttribute(),
			"show_in_sidebar": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "show_in_sidebar"),
				Computed:            true,
			},
			"resolve_on_end": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "resolve_on_end"),
				Computed:            true,
			},
			"reroute_on_end": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "reroute_on_end"),
				Computed:            true,
			},
			"escalation_targets": schema.ListNestedAttribute{
				MarkdownDescription: "If set, alerts matching this window will be escalated to these targets",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"escalation_paths": schema.SingleNestedAttribute{
							MarkdownDescription: "Escalation paths to route alerts to",
							Computed:            true,
							Attributes:          models.ParamBindingDataSourceAttributes(),
						},
						"users": schema.SingleNestedAttribute{
							MarkdownDescription: "Users to notify directly",
							Computed:            true,
							Attributes:          models.ParamBindingDataSourceAttributes(),
						},
					},
				},
			},
			"notify_channels": schema.ListNestedAttribute{
				MarkdownDescription: "Channels to notify about the maintenance window starting and ending",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"channel_id": schema.StringAttribute{
							MarkdownDescription: "The external provider channel ID (e.g. Slack channel ID)",
							Computed:            true,
						},
						"channel_name": schema.StringAttribute{
							MarkdownDescription: "Human readable name of the channel",
							Computed:            true,
						},
						"channel_type": schema.StringAttribute{
							MarkdownDescription: "The type of channel (e.g. public, private)",
							Computed:            true,
						},
					},
				},
			},
			"notify_start_minutes_before": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "notify_start_minutes_before"),
				Computed:            true,
			},
			"notify_end_minutes_before": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "notify_end_minutes_before"),
				Computed:            true,
			},
			"notification_message": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "notification_message"),
				Computed:            true,
			},
			"incident_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "incident_id"),
				Computed:            true,
			},
		},
	}
}

func (d *IncidentMaintenanceWindowDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data *maintenanceWindowDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data == nil {
		return
	}

	validateLookupByIDOrName(data.ID, data.Name, &resp.Diagnostics)
}

func (d *IncidentMaintenanceWindowDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data maintenanceWindowDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var window *client.MaintenanceWindowV1
	if !data.ID.IsNull() {
		result, err := d.client.MaintenanceWindowsV1ShowWithResponse(ctx, data.ID.ValueString())
		if err == nil && result.JSON200 == nil {
			err = fmt.Errorf("unexpected response from API (status %s)", result.Status())
		}
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read maintenance window, got error: %s", err))
			return
		}
		window = &result.JSON200.MaintenanceWindow
	} else {
		found, err := d.findByName(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read maintenance window by name, got error: %s", err))
			return
		}
		window = found
	}

	model := maintenanceWindowDataSourceModelFromAPI(*window)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// maintenanceWindowListPageSize is the largest page the list endpoint accepts.
const maintenanceWindowListPageSize = 50

// findByName looks for an exact name match in the maintenance window list, and requires
// exactly one. The endpoint has no name filter, so every page has to be fetched to know a
// name doesn't appear on a later one. Nothing stops two windows sharing a name, so more
// than one match is reported rather than resolved arbitrarily.
func (d *IncidentMaintenanceWindowDataSource) findByName(ctx context.Context, name string) (*client.MaintenanceWindowV1, error) {
	var (
		after   *string
		matches []client.MaintenanceWindowV1
	)

	for {
		result, err := d.client.MaintenanceWindowsV1ListWithResponse(ctx, &client.MaintenanceWindowsV1ListParams{
			PageSize: lo.ToPtr(int64(maintenanceWindowListPageSize)),
			After:    after,
		})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing maintenance windows: %s", result.Status())
		}

		matches = append(matches, lo.Filter(result.JSON200.MaintenanceWindows, func(window client.MaintenanceWindowV1, _ int) bool {
			return window.Name == name
		})...)

		if result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no maintenance window found with name %q", name)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("found %d maintenance windows named %q; look it up by id instead", len(matches), name)
	}
}
