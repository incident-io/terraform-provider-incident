package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ datasource.DataSource              = &IncidentScheduleSyncTargetDataSource{}
	_ datasource.DataSourceWithConfigure = &IncidentScheduleSyncTargetDataSource{}
)

// Page size used when scanning List to find a sync target by Slack user group
// ID. The API caps this at 50.
const scheduleSyncTargetListPageSize = 50

type IncidentScheduleSyncTargetDataSource struct {
	client *client.ClientWithResponses
}

type IncidentScheduleSyncTargetDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	SlackUserGroupID types.String `tfsdk:"slack_user_group_id"`
	SlackTeamID      types.String `tfsdk:"slack_team_id"`
	AddBotToGroup    types.Bool   `tfsdk:"add_bot_to_group"`
}

func NewIncidentScheduleSyncTargetDataSource() datasource.DataSource {
	return &IncidentScheduleSyncTargetDataSource{}
}

func (d *IncidentScheduleSyncTargetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_sync_target"
}

func (d *IncidentScheduleSyncTargetDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *IncidentProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = providerData.Client
}

func (d *IncidentScheduleSyncTargetDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up an existing schedule sync target by its ID or Slack user group ID.",
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
			"slack_team_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "slack_team_id"),
			},
			"add_bot_to_group": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "add_bot_to_group"),
			},
		},
	}
}

func (d *IncidentScheduleSyncTargetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data IncidentScheduleSyncTargetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idSet := !data.ID.IsNull()
	slackSet := !data.SlackUserGroupID.IsNull()
	if idSet && slackSet {
		resp.Diagnostics.AddError(
			"Invalid configuration",
			"Only one of 'id' or 'slack_user_group_id' may be provided, not both.",
		)
		return
	}
	if !idSet && !slackSet {
		resp.Diagnostics.AddError(
			"Invalid configuration",
			"Either 'id' or 'slack_user_group_id' must be provided.",
		)
		return
	}

	var target *client.ScheduleSyncTargetResourceV2
	switch {
	case idSet:
		result, err := d.client.ScheduleSyncTargetsV2ShowWithResponse(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read schedule sync target, got error: %s", err))
			return
		}
		target = &result.JSON200.ScheduleSyncTarget

	case slackSet:
		found, err := findScheduleSyncTargetBySlackUserGroupID(ctx, d.client, data.SlackUserGroupID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", err.Error())
			return
		}
		if found == nil {
			resp.Diagnostics.AddError(
				"Schedule sync target not found",
				fmt.Sprintf("No schedule sync target exists for slack_user_group_id %q.", data.SlackUserGroupID.ValueString()),
			)
			return
		}
		target = found
	}

	model := &IncidentScheduleSyncTargetDataSourceModel{
		ID:               types.StringValue(target.Id),
		SlackUserGroupID: types.StringValue(target.SlackUserGroupId),
		SlackTeamID:      types.StringValue(target.SlackTeamId),
		AddBotToGroup:    types.BoolValue(target.AddBotToGroup),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// findScheduleSyncTargetBySlackUserGroupID pages through List because the API
// has no filter parameter. Returns nil if no match is found.
func findScheduleSyncTargetBySlackUserGroupID(ctx context.Context, c *client.ClientWithResponses, slackUserGroupID string) (*client.ScheduleSyncTargetResourceV2, error) {
	pageSize := int64(scheduleSyncTargetListPageSize)
	var after *string
	for {
		result, err := c.ScheduleSyncTargetsV2ListWithResponse(ctx, &client.ScheduleSyncTargetsV2ListParams{
			PageSize: &pageSize,
			After:    after,
		})
		if err != nil {
			return nil, fmt.Errorf("listing schedule sync targets: %w", err)
		}

		for i := range result.JSON200.ScheduleSyncTargets {
			t := &result.JSON200.ScheduleSyncTargets[i]
			if t.SlackUserGroupId == slackUserGroupID {
				return t, nil
			}
		}

		if result.JSON200.PaginationMeta == nil ||
			result.JSON200.PaginationMeta.After == nil ||
			*result.JSON200.PaginationMeta.After == "" {
			return nil, nil
		}
		after = result.JSON200.PaginationMeta.After
	}
}
