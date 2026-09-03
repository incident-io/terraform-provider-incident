package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentScheduleReplicaDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentScheduleReplicaDataSource{}
)

func NewIncidentScheduleReplicaDataSource() datasource.DataSource {
	return &IncidentScheduleReplicaDataSource{}
}

type IncidentScheduleReplicaDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentScheduleReplicaDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_replica"
}

func (d *IncidentScheduleReplicaDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing schedule replica by schedule ID and replica ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "id"),
			},
			"schedule_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "schedule_id"),
			},
			"replica_provider": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: EnumValuesDescription("ScheduleReplicaV2", "replica_provider"),
			},
			"replica_provider_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "replica_provider_id"),
			},
			"replica_fallback_user_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "replica_fallback_user_id"),
			},
			"mirror_window_days": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "mirror_window_days"),
			},
			"sources": schema.SetNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "sources"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"rotation_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("ScheduleReplicaSourceV2", "rotation_id"),
						},
						"layer_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("ScheduleReplicaSourceV2", "layer_id"),
						},
					},
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "created_at"),
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "updated_at"),
			},
			"last_synced_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "last_synced_at"),
			},
			"last_sync_error": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "last_sync_error"),
			},
			"user_statuses": schema.SetNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "user_statuses"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"user_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("ScheduleReplicaUserStatusV2", "user_id"),
						},
						"external_user_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("ScheduleReplicaUserStatusV2", "external_user_id"),
						},
					},
				},
			},
		},
	}
}

func (d *IncidentScheduleReplicaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.ScheduleReplicaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.SchedulesV2ShowScheduleReplicaWithResponse(ctx, data.ScheduleID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read schedule replica, got error: %s", err))
		return
	}

	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read schedule replica: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	data = models.ScheduleReplicaModel{}.FromAPI(result.JSON200.ScheduleReplica)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
