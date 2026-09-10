package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

var (
	_ datasource.DataSource                   = &IncidentScheduleSyncTargetDataSource{}
	_ datasource.DataSourceWithConfigure      = &IncidentScheduleSyncTargetDataSource{}
	_ datasource.DataSourceWithValidateConfig = &IncidentScheduleSyncTargetDataSource{}
)

// scheduleSyncTargetLookupPageSize is the page size used when searching sync
// targets by Slack user group ID. It's the maximum the endpoint allows, so most
// organisations resolve in a single request.
const scheduleSyncTargetLookupPageSize = 250

func NewIncidentScheduleSyncTargetDataSource() datasource.DataSource {
	return &IncidentScheduleSyncTargetDataSource{}
}

type IncidentScheduleSyncTargetDataSource struct {
	dataSourceConfigurer
}

func (d *IncidentScheduleSyncTargetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_sync_target"
}

func (d *IncidentScheduleSyncTargetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Schedule Sync Targets V2"),
			"Use this data source to look up an existing schedule sync target, either by `id` or by `slack_user_group_id`."),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "id"),
			},
			"slack_user_group_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "slack_user_group_id"),
			},
			"add_bot_to_group": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "add_bot_to_group"),
			},
			"slack_team_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "slack_team_id"),
			},
			"linked_schedules": schema.SetNestedAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "linked_schedules"),
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("LinkedScheduleV2", "id"),
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: apischema.Docstring("LinkedScheduleV2", "name"),
						},
						"team_ids": schema.SetAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: apischema.Docstring("LinkedScheduleV2", "team_ids"),
						},
					},
				},
			},
		},
	}
}

// ValidateConfig rejects an ambiguous lookup at plan time. Both attributes are
// Optional and Computed so either can be used, which means setting both would
// otherwise silently ignore one of them.
func (d *IncidentScheduleSyncTargetDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var id, slackUserGroupID types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("slack_user_group_id"), &slackUserGroupID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if id.IsUnknown() || slackUserGroupID.IsUnknown() {
		return
	}

	switch {
	case !id.IsNull() && !slackUserGroupID.IsNull():
		resp.Diagnostics.AddError("Ambiguous lookup", "Set either id or slack_user_group_id, not both.")
	case id.IsNull() && slackUserGroupID.IsNull():
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or slack_user_group_id.")
	}
}

func (d *IncidentScheduleSyncTargetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data models.ScheduleSyncTargetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var target *client.ScheduleSyncTargetResourceV2
	switch {
	case !data.ID.IsNull():
		result, err := d.client.ScheduleSyncTargetsV2ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read schedule sync target, got error: %s", err))
			return
		}
		if result.JSON200 == nil {
			resp.Diagnostics.AddError(
				"Client Error",
				fmt.Sprintf("Unable to read schedule sync target: unexpected response from API (status %s)", result.Status()),
			)
			return
		}
		target = &result.JSON200.ScheduleSyncTarget
	case !data.SlackUserGroupID.IsNull():
		got, err := d.findBySlackUserGroupID(ctx, data.SlackUserGroupID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read schedule sync target by Slack user group ID", err.Error())
			return
		}
		target = got
	default:
		resp.Diagnostics.AddError("Missing lookup", "Set one of id or slack_user_group_id.")
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, models.ScheduleSyncTargetDataSourceModel{}.FromAPIDataSource(*target))...)
}

// findBySlackUserGroupID looks through every sync target for an exact Slack
// user group ID match. A Slack user group can only belong to one target.
func (d *IncidentScheduleSyncTargetDataSource) findBySlackUserGroupID(ctx context.Context, slackUserGroupID string) (*client.ScheduleSyncTargetResourceV2, error) {
	var after *string

	for {
		result, err := d.client.ScheduleSyncTargetsV2ListWithResponse(ctx, &client.ScheduleSyncTargetsV2ListParams{
			PageSize: lo.ToPtr(int64(scheduleSyncTargetLookupPageSize)),
			After:    after,
		})
		if err != nil {
			return nil, err
		}
		if result.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing schedule sync targets: %s", result.Status())
		}

		for _, target := range result.JSON200.ScheduleSyncTargets {
			if target.SlackUserGroupId == slackUserGroupID {
				return &target, nil
			}
		}

		if result.JSON200.PaginationMeta == nil || result.JSON200.PaginationMeta.After == nil {
			break
		}
		after = result.JSON200.PaginationMeta.After
	}

	return nil, fmt.Errorf("no schedule sync target found for Slack user group ID %q", slackUserGroupID)
}
