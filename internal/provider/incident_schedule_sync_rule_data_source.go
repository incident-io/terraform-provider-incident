package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ datasource.DataSource              = &IncidentScheduleSyncRuleDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentScheduleSyncRuleDataSource{}
)

func NewIncidentScheduleSyncRuleDataSource() datasource.DataSource {
	return &IncidentScheduleSyncRuleDataSource{}
}

type IncidentScheduleSyncRuleDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentScheduleSyncRuleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_sync_rule"
}

func (d *IncidentScheduleSyncRuleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing schedule sync rule by schedule ID and rule ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "id"),
			},
			"schedule_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "schedule_id"),
			},
			"schedule_sync_target_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "schedule_sync_target_id"),
			},
			"sync_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: EnumValuesDescription("ScheduleSyncRuleV2", "sync_type"),
			},
			"rotation_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "rotation_id"),
			},
			"permanent_member_user_ids": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "permanent_member_user_ids"),
			},
		},
	}
}

func (d *IncidentScheduleSyncRuleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.ScheduleSyncRuleResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := d.client.SchedulesV2ShowScheduleSyncRuleWithResponse(ctx, data.ScheduleID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read schedule sync rule, got error: %s", err))
		return
	}

	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read schedule sync rule: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	data = models.ScheduleSyncRuleResourceModel{}.FromAPIDataSource(result.JSON200.ScheduleSyncRule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
