package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	client           *client.ClientWithResponses
	terraformVersion string
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
	r.terraformVersion = client.TerraformVersion
}

func (r *IncidentScheduleSyncTargetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.ScheduleSyncTargetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := data.ToPayload()
	payload.Annotations = &map[string]string{
		"incident.io/terraform/version": r.terraformVersion,
	}
	result, err := r.client.ScheduleSyncTargetsV2CreateWithResponse(ctx, client.ScheduleSyncTargetsCreatePayloadV2{
		ScheduleSyncTarget: payload,
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

	if result.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to read schedule sync target: unexpected response from API (status %s)", result.Status()),
		)
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
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
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
	targetID := strings.TrimSpace(req.ID)
	if targetID == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"The import ID must be the ID of the sync target to import.",
		)
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Importing schedule sync target with id=%s", targetID))

	// Populate the full model rather than just the ID, so the imported state
	// matches what Read would produce and a missing target fails with a clear
	// message instead of an empty import.
	result, err := r.client.ScheduleSyncTargetsV2ShowWithResponse(ctx, targetID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError(
				"Schedule Sync Target Not Found",
				fmt.Sprintf("No sync target exists with ID %q.", targetID),
			)
			return
		}
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

	claimResource(ctx, r.client, targetID, &resp.Diagnostics, client.ScheduleSyncTarget, r.terraformVersion)
	if resp.Diagnostics.HasError() {
		return
	}

	// new_slack_user_group describes how to create a Slack user group, so it is
	// never part of an imported target: the group already exists, and the
	// imported configuration should reference it with slack_user_group_id.
	data := models.ScheduleSyncTargetResourceModel{}.FromAPI(result.JSON200.ScheduleSyncTarget)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
