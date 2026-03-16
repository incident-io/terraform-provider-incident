package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

var (
	_ resource.Resource                = &IncidentMaintenanceWindowResource{}
	_ resource.ResourceWithImportState = &IncidentMaintenanceWindowResource{}
)

type IncidentMaintenanceWindowResource struct {
	client *client.ClientWithResponses
}

type MaintenanceWindowResourceModel struct {
	ID                       types.String                              `tfsdk:"id"`
	Name                     types.String                              `tfsdk:"name"`
	StartAt                  types.String                              `tfsdk:"start_at"`
	EndAt                    types.String                              `tfsdk:"end_at"`
	LeadID                   types.String                              `tfsdk:"lead_id"`
	AlertConditionGroups     models.IncidentEngineConditionGroups      `tfsdk:"alert_condition_groups"`
	ShowInSidebar            types.Bool                                `tfsdk:"show_in_sidebar"`
	ResolveOnEnd             types.Bool                                `tfsdk:"resolve_on_end"`
	RerouteOnEnd             types.Bool                                `tfsdk:"reroute_on_end"`
	EscalationTargets        []MaintenanceWindowEscalationTargetModel  `tfsdk:"escalation_targets"`
	NotifyChannels           []MaintenanceWindowNotifyChannelModel     `tfsdk:"notify_channels"`
	NotifyStartMinutesBefore types.Int64                               `tfsdk:"notify_start_minutes_before"`
	NotifyEndMinutesBefore   types.Int64                               `tfsdk:"notify_end_minutes_before"`
	NotificationMessage      types.String                              `tfsdk:"notification_message"`
	IncidentID               types.String                              `tfsdk:"incident_id"`
}

type MaintenanceWindowEscalationTargetModel struct {
	EscalationPaths *models.IncidentEngineParamBinding `tfsdk:"escalation_paths"`
	Users           *models.IncidentEngineParamBinding `tfsdk:"users"`
}

type MaintenanceWindowNotifyChannelModel struct {
	ChannelID   types.String `tfsdk:"channel_id"`
	ChannelName types.String `tfsdk:"channel_name"`
	ChannelType types.String `tfsdk:"channel_type"`
}

func NewIncidentMaintenanceWindowResource() resource.Resource {
	return &IncidentMaintenanceWindowResource{}
}

func (r *IncidentMaintenanceWindowResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance_window"
}

func (r *IncidentMaintenanceWindowResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: apischema.TagDocstring("MaintenanceWindows V1"),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "name"),
				Required:            true,
			},
			"start_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "start_at"),
				Required:            true,
				Validators: []validator.String{
					RFC3339TimestampValidator{},
				},
			},
			"end_at": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "end_at"),
				Required:            true,
				Validators: []validator.String{
					RFC3339TimestampValidator{},
				},
			},
			"lead_id": schema.StringAttribute{
				MarkdownDescription: "The incident.io user ID of the lead for this maintenance window",
				Required:            true,
			},
			"alert_condition_groups": models.ConditionGroupsAttribute(),
			"show_in_sidebar": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "show_in_sidebar"),
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"resolve_on_end": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "resolve_on_end"),
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"reroute_on_end": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "reroute_on_end"),
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"escalation_targets": schema.ListNestedAttribute{
				MarkdownDescription: "If set, alerts matching this window will be escalated to these targets",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"escalation_paths": schema.SingleNestedAttribute{
							MarkdownDescription: "Escalation paths to route alerts to",
							Optional:            true,
							Attributes:          models.ParamBindingAttributes(),
						},
						"users": schema.SingleNestedAttribute{
							MarkdownDescription: "Users to notify directly",
							Optional:            true,
							Attributes:          models.ParamBindingAttributes(),
						},
					},
				},
			},
			"notify_channels": schema.ListNestedAttribute{
				MarkdownDescription: "Channels to notify about the maintenance window starting and ending",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"channel_id": schema.StringAttribute{
							MarkdownDescription: "The external provider channel ID (e.g. Slack channel ID)",
							Required:            true,
						},
						"channel_name": schema.StringAttribute{
							MarkdownDescription: "Human readable name of the channel",
							Optional:            true,
						},
						"channel_type": schema.StringAttribute{
							MarkdownDescription: "The type of channel (e.g. public, private)",
							Required:            true,
						},
					},
				},
			},
			"notify_start_minutes_before": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "notify_start_minutes_before"),
				Optional:            true,
			},
			"notify_end_minutes_before": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "notify_end_minutes_before"),
				Optional:            true,
			},
			"notification_message": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "notification_message"),
				Optional:            true,
			},
			"incident_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("MaintenanceWindowV1", "incident_id"),
				Optional:            true,
			},
		},
	}
}

func (r *IncidentMaintenanceWindowResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentMaintenanceWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *MaintenanceWindowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, err := r.buildPayload(data)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to build maintenance window payload: %s", err))
		return
	}

	result, err := r.client.MaintenanceWindowsV1CreateWithResponse(ctx, client.MaintenanceWindowsV1CreateJSONRequestBody{
		Name:                     payload.Name,
		StartAt:                  payload.StartAt,
		EndAt:                    payload.EndAt,
		Lead:                     payload.Lead,
		AlertConditionGroups:     payload.AlertConditionGroups,
		ShowInSidebar:            payload.ShowInSidebar,
		ResolveOnEnd:             payload.ResolveOnEnd,
		RerouteOnEnd:             payload.RerouteOnEnd,
		EscalationTargets:        payload.EscalationTargets,
		NotifyChannels:           payload.NotifyChannels,
		NotifyStartMinutesBefore: payload.NotifyStartMinutesBefore,
		NotifyEndMinutesBefore:   payload.NotifyEndMinutesBefore,
		NotificationMessage:      payload.NotificationMessage,
		IncidentId:               payload.IncidentId,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create maintenance window, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created a maintenance window resource with id=%s", result.JSON201.MaintenanceWindow.Id))
	data = r.buildModel(result.JSON201.MaintenanceWindow)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentMaintenanceWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *MaintenanceWindowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.MaintenanceWindowsV1ShowWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Maintenance window with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read maintenance window, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.MaintenanceWindow)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentMaintenanceWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *MaintenanceWindowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, err := r.buildPayload(data)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to build maintenance window payload: %s", err))
		return
	}

	result, err := r.client.MaintenanceWindowsV1UpdateWithResponse(ctx, data.ID.ValueString(), client.MaintenanceWindowsV1UpdateJSONRequestBody{
		Name:                     payload.Name,
		StartAt:                  payload.StartAt,
		EndAt:                    payload.EndAt,
		Lead:                     payload.Lead,
		AlertConditionGroups:     payload.AlertConditionGroups,
		ShowInSidebar:            payload.ShowInSidebar,
		ResolveOnEnd:             payload.ResolveOnEnd,
		RerouteOnEnd:             payload.RerouteOnEnd,
		EscalationTargets:        payload.EscalationTargets,
		NotifyChannels:           payload.NotifyChannels,
		NotifyStartMinutesBefore: payload.NotifyStartMinutesBefore,
		NotifyEndMinutesBefore:   payload.NotifyEndMinutesBefore,
		NotificationMessage:      payload.NotificationMessage,
		IncidentId:               payload.IncidentId,
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update maintenance window, got error: %s", err))
		return
	}

	data = r.buildModel(result.JSON200.MaintenanceWindow)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentMaintenanceWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *MaintenanceWindowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.MaintenanceWindowsV1DeleteWithResponse(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete maintenance window, got error: %s", err))
		return
	}
}

func (r *IncidentMaintenanceWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildPayload is a shared helper used by both Create and Update.
type maintenanceWindowPayload struct {
	Name                     string
	StartAt                  time.Time
	EndAt                    time.Time
	Lead                     client.UserReferencePayloadV2
	AlertConditionGroups     []client.ConditionGroupPayloadV2
	ShowInSidebar            bool
	ResolveOnEnd             *bool
	RerouteOnEnd             *bool
	EscalationTargets        *[]client.MaintenanceWindowEscalationTargetPayloadV1
	NotifyChannels           *[]client.MaintenanceWindowNotifyChannelPayloadV1
	NotifyStartMinutesBefore *int64
	NotifyEndMinutesBefore   *int64
	NotificationMessage      *string
	IncidentId               *string
}

func (r *IncidentMaintenanceWindowResource) buildPayload(data *MaintenanceWindowResourceModel) (*maintenanceWindowPayload, error) {
	startAt, err := time.Parse(time.RFC3339, data.StartAt.ValueString())
	if err != nil {
		return nil, fmt.Errorf("parsing start_at: %w", err)
	}
	endAt, err := time.Parse(time.RFC3339, data.EndAt.ValueString())
	if err != nil {
		return nil, fmt.Errorf("parsing end_at: %w", err)
	}

	payload := &maintenanceWindowPayload{
		Name:                 data.Name.ValueString(),
		StartAt:              startAt,
		EndAt:                endAt,
		Lead:                 client.UserReferencePayloadV2{Id: data.LeadID.ValueStringPointer()},
		AlertConditionGroups: data.AlertConditionGroups.ToPayload(),
		ShowInSidebar:        data.ShowInSidebar.ValueBool(),
	}

	if !data.ResolveOnEnd.IsNull() && !data.ResolveOnEnd.IsUnknown() {
		payload.ResolveOnEnd = lo.ToPtr(data.ResolveOnEnd.ValueBool())
	}
	if !data.RerouteOnEnd.IsNull() && !data.RerouteOnEnd.IsUnknown() {
		payload.RerouteOnEnd = lo.ToPtr(data.RerouteOnEnd.ValueBool())
	}

	if data.EscalationTargets != nil {
		targets := []client.MaintenanceWindowEscalationTargetPayloadV1{}
		for _, t := range data.EscalationTargets {
			target := client.MaintenanceWindowEscalationTargetPayloadV1{}
			if t.EscalationPaths != nil {
				target.EscalationPaths = lo.ToPtr(t.EscalationPaths.ToPayload())
			}
			if t.Users != nil {
				target.Users = lo.ToPtr(t.Users.ToPayload())
			}
			targets = append(targets, target)
		}
		payload.EscalationTargets = &targets
	}

	if data.NotifyChannels != nil {
		channels := []client.MaintenanceWindowNotifyChannelPayloadV1{}
		for _, c := range data.NotifyChannels {
			channel := client.MaintenanceWindowNotifyChannelPayloadV1{
				ChannelId:   c.ChannelID.ValueString(),
				ChannelType: c.ChannelType.ValueString(),
			}
			if !c.ChannelName.IsNull() {
				channel.ChannelName = c.ChannelName.ValueStringPointer()
			}
			channels = append(channels, channel)
		}
		payload.NotifyChannels = &channels
	}

	if !data.NotifyStartMinutesBefore.IsNull() {
		payload.NotifyStartMinutesBefore = lo.ToPtr(data.NotifyStartMinutesBefore.ValueInt64())
	}
	if !data.NotifyEndMinutesBefore.IsNull() {
		payload.NotifyEndMinutesBefore = lo.ToPtr(data.NotifyEndMinutesBefore.ValueInt64())
	}
	if !data.NotificationMessage.IsNull() {
		payload.NotificationMessage = data.NotificationMessage.ValueStringPointer()
	}
	if !data.IncidentID.IsNull() {
		payload.IncidentId = data.IncidentID.ValueStringPointer()
	}

	return payload, nil
}

func (r *IncidentMaintenanceWindowResource) buildModel(mw client.MaintenanceWindowV1) *MaintenanceWindowResourceModel {
	model := &MaintenanceWindowResourceModel{
		ID:                   types.StringValue(mw.Id),
		Name:                 types.StringValue(mw.Name),
		StartAt:              types.StringValue(mw.StartAt.Format(time.RFC3339)),
		EndAt:                types.StringValue(mw.EndAt.Format(time.RFC3339)),
		AlertConditionGroups: models.IncidentEngineConditionGroups{}.FromAPI(mw.AlertConditionGroups),
		ShowInSidebar:        types.BoolValue(mw.ShowInSidebar),
		ResolveOnEnd:         types.BoolValue(mw.ResolveOnEnd),
		RerouteOnEnd:         types.BoolValue(mw.RerouteOnEnd),
	}

	// Lead: extract user ID from ActorV2
	if mw.Lead.User != nil {
		model.LeadID = types.StringValue(mw.Lead.User.Id)
	}

	// Escalation targets
	if mw.EscalationTargets != nil {
		for _, t := range *mw.EscalationTargets {
			target := MaintenanceWindowEscalationTargetModel{}
			if t.EscalationPaths != nil {
				binding := models.IncidentEngineParamBinding{}.FromAPI(*t.EscalationPaths)
				target.EscalationPaths = &binding
			}
			if t.Users != nil {
				binding := models.IncidentEngineParamBinding{}.FromAPI(*t.Users)
				target.Users = &binding
			}
			model.EscalationTargets = append(model.EscalationTargets, target)
		}
	}

	// Notify channels
	if mw.NotifyChannels != nil {
		for _, c := range *mw.NotifyChannels {
			model.NotifyChannels = append(model.NotifyChannels, MaintenanceWindowNotifyChannelModel{
				ChannelID:   types.StringValue(c.ChannelId),
				ChannelName: types.StringPointerValue(c.ChannelName),
				ChannelType: types.StringValue(c.ChannelType),
			})
		}
	}

	// Nullable ints
	if mw.NotifyStartMinutesBefore != nil {
		model.NotifyStartMinutesBefore = types.Int64Value(*mw.NotifyStartMinutesBefore)
	}
	if mw.NotifyEndMinutesBefore != nil {
		model.NotifyEndMinutesBefore = types.Int64Value(*mw.NotifyEndMinutesBefore)
	}

	// Nullable strings
	model.NotificationMessage = types.StringPointerValue(mw.NotificationMessage)
	model.IncidentID = types.StringPointerValue(mw.IncidentId)

	return model
}
