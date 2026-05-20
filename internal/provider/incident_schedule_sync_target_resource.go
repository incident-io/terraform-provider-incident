package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

var (
	_ resource.Resource                   = &IncidentScheduleSyncTargetResource{}
	_ resource.ResourceWithConfigure      = &IncidentScheduleSyncTargetResource{}
	_ resource.ResourceWithImportState    = &IncidentScheduleSyncTargetResource{}
	_ resource.ResourceWithValidateConfig = &IncidentScheduleSyncTargetResource{}
)

type IncidentScheduleSyncTargetResource struct {
	client *client.ClientWithResponses
}

func NewIncidentScheduleSyncTargetResource() resource.Resource {
	return &IncidentScheduleSyncTargetResource{}
}

func (r *IncidentScheduleSyncTargetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_sync_target"
}

func (r *IncidentScheduleSyncTargetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("Schedule Sync Targets V2"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"add_bot_to_group": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "add_bot_to_group"),
			},
			"slack_user_group_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "slack_user_group_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"slack_team_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncTargetResourceV2", "slack_team_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"new_slack_user_group": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Configuration for creating a new Slack user group. Mutually exclusive with `slack_user_group_id`.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("NewSlackUserGroupPayloadV2", "name"),
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"handle": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("NewSlackUserGroupPayloadV2", "handle"),
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"description": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: apischema.Docstring("NewSlackUserGroupPayloadV2", "description"),
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"slack_team_id": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: apischema.Docstring("NewSlackUserGroupPayloadV2", "slack_team_id"),
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
				},
			},
		},
	}
}

func (r *IncidentScheduleSyncTargetResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.ScheduleSyncTargetResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only check IsNull, not IsUnknown. Unknown values (e.g., references to other resources)
	// are valid - the actual value will be resolved during apply.
	hasExisting := !data.SlackUserGroupID.IsNull()
	hasNew := data.NewSlackUserGroup != nil

	if hasExisting && hasNew {
		resp.Diagnostics.Append(diag.NewErrorDiagnostic(
			"Cannot specify both slack_user_group_id and new_slack_user_group",
			"Exactly one of slack_user_group_id or new_slack_user_group must be set. "+
				"Use slack_user_group_id to sync to an existing Slack user group, or "+
				"new_slack_user_group to create a new one."))
		return
	}

	if !hasExisting && !hasNew {
		resp.Diagnostics.Append(diag.NewErrorDiagnostic(
			"Must specify either slack_user_group_id or new_slack_user_group",
			"Exactly one of slack_user_group_id or new_slack_user_group must be set. "+
				"Use slack_user_group_id to sync to an existing Slack user group, or "+
				"new_slack_user_group to create a new one."))
		return
	}
}

func (r *IncidentScheduleSyncTargetResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*IncidentProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client.Client
}

func (r *IncidentScheduleSyncTargetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.ScheduleSyncTargetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.ScheduleSyncTargetsV2CreateWithResponse(ctx, client.ScheduleSyncTargetsCreatePayloadV2{
		ScheduleSyncTarget: data.ToPayload(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create schedule sync target, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created schedule sync target with id=%s", result.JSON201.ScheduleSyncTarget.Id))

	// Preserve new_slack_user_group from plan since it's not returned by API
	newSlackUserGroup := data.NewSlackUserGroup

	data = models.ScheduleSyncTargetResourceModel{}.FromAPI(result.JSON201.ScheduleSyncTarget)
	data.NewSlackUserGroup = newSlackUserGroup

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncTargetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.ScheduleSyncTargetResourceModel
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

	// Preserve new_slack_user_group from state since it's not returned by API
	newSlackUserGroup := data.NewSlackUserGroup

	data = models.ScheduleSyncTargetResourceModel{}.FromAPI(result.JSON200.ScheduleSyncTarget)
	data.NewSlackUserGroup = newSlackUserGroup

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncTargetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.ScheduleSyncTargetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.ScheduleSyncTargetsV2UpdateWithResponse(ctx, data.ID.ValueString(), client.ScheduleSyncTargetsUpdatePayloadV2{
		AddBotToGroup: data.AddBotToGroup.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update schedule sync target, got error: %s", err))
		return
	}

	// Preserve new_slack_user_group from plan since it's not returned by API
	newSlackUserGroup := data.NewSlackUserGroup

	data = models.ScheduleSyncTargetResourceModel{}.FromAPI(result.JSON200.ScheduleSyncTarget)
	data.NewSlackUserGroup = newSlackUserGroup

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncTargetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.ScheduleSyncTargetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.ScheduleSyncTargetsV2DestroyWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete schedule sync target, got error: %s", err))
		return
	}
}

func (r *IncidentScheduleSyncTargetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
