package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var (
	_ resource.Resource                = &IncidentScheduleSyncTargetResource{}
	_ resource.ResourceWithImportState = &IncidentScheduleSyncTargetResource{}
)

type IncidentScheduleSyncTargetResource struct {
	client *client.ClientWithResponses
}

type IncidentScheduleSyncTargetResourceModel struct {
	ID               types.String `tfsdk:"id"`
	SlackUserGroupID types.String `tfsdk:"slack_user_group_id"`
	SlackTeamID      types.String `tfsdk:"slack_team_id"`
	AddBotToGroup    types.Bool   `tfsdk:"add_bot_to_group"`
}

func NewIncidentScheduleSyncTargetResource() resource.Resource {
	return &IncidentScheduleSyncTargetResource{}
}

func (r *IncidentScheduleSyncTargetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_sync_target"
}

func (r *IncidentScheduleSyncTargetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("%s\n\n%s", apischema.TagDocstring("Schedule Sync Targets V2"), strings.TrimSpace(`
The schedule sync target API has no update endpoint, so every writable attribute on this resource (`+"`slack_user_group_id`, `add_bot_to_group`"+`) is marked as requiring replacement. Changing any of them will destroy and recreate the target.

Destroying a sync target — including via the destroy-then-create that a replacement performs — will fail with HTTP 422 if any `+"`incident_schedule_sync_rule`"+` references it. Remove those rules first, or migrate them to a different target, before changing this resource.
`)),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"slack_user_group_id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "slack_user_group_id") +
					". Changing this attribute forces the sync target to be destroyed and recreated; the destroy step will fail if any sync rules reference the target.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"slack_team_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "slack_team_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"add_bot_to_group": schema.BoolAttribute{
				Required: true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "add_bot_to_group") +
					". Changing this attribute forces the sync target to be destroyed and recreated; the destroy step will fail if any sync rules reference the target.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *IncidentScheduleSyncTargetResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *IncidentProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = providerData.Client
}

func (r *IncidentScheduleSyncTargetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *IncidentScheduleSyncTargetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.ScheduleSyncTargetsV2CreateWithResponse(ctx, client.ScheduleSyncTargetsV2CreateJSONRequestBody{
		ScheduleSyncTarget: client.ScheduleSyncTargetCreatePayloadV2{
			SlackUserGroupId: data.SlackUserGroupID.ValueString(),
			AddBotToGroup:    data.AddBotToGroup.ValueBool(),
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create schedule sync target, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a schedule sync target with id=%s", result.JSON201.ScheduleSyncTarget.Id))
	data = buildScheduleSyncTargetResourceModel(result.JSON201.ScheduleSyncTarget)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncTargetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *IncidentScheduleSyncTargetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.ScheduleSyncTargetsV2ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Schedule sync target with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read schedule sync target, got error: %s", err))
		return
	}

	data = buildScheduleSyncTargetResourceModel(result.JSON200.ScheduleSyncTarget)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update should never be invoked: every writable attribute uses RequiresReplace
// and the API has no PUT endpoint. Fail loudly so a future contributor who adds
// an updatable attribute without RequiresReplace catches the gap in CI rather
// than silently drifting state.
func (r *IncidentScheduleSyncTargetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"incident_schedule_sync_target has no updatable attributes — every writable attribute must use RequiresReplace. This is a provider bug; please report it.",
	)
}

func (r *IncidentScheduleSyncTargetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *IncidentScheduleSyncTargetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.ScheduleSyncTargetsV2DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Schedule sync target with ID %s already gone: treating delete as success.", data.ID.ValueString()))
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete schedule sync target, got error: %s", err))
		return
	}
}

func (r *IncidentScheduleSyncTargetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func buildScheduleSyncTargetResourceModel(target client.ScheduleSyncTargetResourceV2) *IncidentScheduleSyncTargetResourceModel {
	return &IncidentScheduleSyncTargetResourceModel{
		ID:               types.StringValue(target.Id),
		SlackUserGroupID: types.StringValue(target.SlackUserGroupId),
		SlackTeamID:      types.StringValue(target.SlackTeamId),
		AddBotToGroup:    types.BoolValue(target.AddBotToGroup),
	}
}
