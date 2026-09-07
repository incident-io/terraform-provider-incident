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
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentScheduleReplicasDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentScheduleReplicasDataSource{}
)

func NewIncidentScheduleReplicasDataSource() datasource.DataSource {
	return &IncidentScheduleReplicasDataSource{}
}

type IncidentScheduleReplicasDataSource struct {
	dataSourceConfigurer
}

type IncidentScheduleReplicasDataSourceModel struct {
	ScheduleID       types.String                  `tfsdk:"schedule_id"`
	ScheduleReplicas []models.ScheduleReplicaModel `tfsdk:"schedule_replicas"`
}

func (d *IncidentScheduleReplicasDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_replicas"
}

func (d *IncidentScheduleReplicasDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List the schedule replicas for an incident.io schedule. Replicas mirror a schedule into an external provider such as PagerDuty, Opsgenie, or Jira Service Management.",
		Attributes: map[string]schema.Attribute{
			"schedule_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "schedule_id"),
			},
			"schedule_replicas": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The replicas configured for this schedule.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: scheduleReplicaDataSourceItemAttributes(),
				},
			},
		},
	}
}

func (d *IncidentScheduleReplicasDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentScheduleReplicasDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.SchedulesV2ListScheduleReplicasWithResponse(ctx, data.ScheduleID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to list schedule replicas, got error: %s", err))
		return
	}

	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to list schedule replicas: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	replicas := lo.Map(result.JSON200.ScheduleReplicas, func(replica client.ScheduleReplicaV2, _ int) models.ScheduleReplicaModel {
		return models.ScheduleReplicaModel{}.FromAPI(replica)
	})
	if replicas == nil {
		replicas = []models.ScheduleReplicaModel{}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &IncidentScheduleReplicasDataSourceModel{
		ScheduleID:       data.ScheduleID,
		ScheduleReplicas: replicas,
	})...)
}
