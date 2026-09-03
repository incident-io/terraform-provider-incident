package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ resource.Resource                = &IncidentScheduleReplicaResource{}
	_ resource.ResourceWithConfigure   = &IncidentScheduleReplicaResource{}
	_ resource.ResourceWithImportState = &IncidentScheduleReplicaResource{}
)

type IncidentScheduleReplicaResource struct {
	resourceConfigurer
}

func NewIncidentScheduleReplicaResource() resource.Resource {
	return &IncidentScheduleReplicaResource{}
}

func (r *IncidentScheduleReplicaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_replica"
}

func (r *IncidentScheduleReplicaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manage a schedule replica that mirrors an incident.io schedule into an external provider such as PagerDuty, Opsgenie, or Jira Service Management.

The API does not support updating a replica in place: changing any configuration forces a new resource.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"schedule_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "schedule_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"replica_provider": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: EnumValuesDescription("ScheduleReplicaV2", "replica_provider"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"replica_provider_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "replica_provider_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"replica_fallback_user_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "replica_fallback_user_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mirror_window_days": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(models.DefaultScheduleReplicaMirrorWindowDays),
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "mirror_window_days"),
				Validators: []validator.Int64{
					int64validator.Between(1, 90),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"sources": schema.SetNestedAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "sources"),
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"rotation_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("ScheduleReplicaSourceV2", "rotation_id"),
						},
						"layer_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: apischema.Docstring("ScheduleReplicaSourceV2", "layer_id"),
						},
					},
				},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				CustomType:          timetypes.RFC3339Type{},
				MarkdownDescription: apischema.Docstring("ScheduleReplicaV2", "created_at"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

func (r *IncidentScheduleReplicaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.ScheduleReplicaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SchedulesV2CreateScheduleReplicaWithResponse(ctx, data.ScheduleID.ValueString(), client.SchedulesCreateScheduleReplicaPayloadV2{
		ScheduleReplica: data.ToCreatePayload(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create schedule replica, got error: %s", err))
		return
	}

	if result.JSON201 == nil {
		resp.Diagnostics.AddError(
			"Client Error",
			fmt.Sprintf("Unable to create schedule replica: unexpected response from API (status %s)", result.Status()),
		)
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created schedule replica with id=%s", result.JSON201.ScheduleReplica.Id))

	data = models.ScheduleReplicaModel{}.FromAPI(result.JSON201.ScheduleReplica)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleReplicaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.ScheduleReplicaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SchedulesV2ShowScheduleReplicaWithResponse(ctx, data.ScheduleID.ValueString(), data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Schedule replica with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
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

func (r *IncidentScheduleReplicaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Schedule replicas cannot be updated",
		"The incident.io API does not support updating a schedule replica in place. Changing replica configuration requires replacing the resource.",
	)
}

func (r *IncidentScheduleReplicaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.ScheduleReplicaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.SchedulesV2DestroyScheduleReplicaWithResponse(ctx, data.ScheduleID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete schedule replica, got error: %s", err))
		return
	}
}

// parseScheduleReplicaImportID splits the composite import ID a replica is
// identified by. Replicas are nested under a schedule in the API, so both IDs
// are needed to look one up.
func parseScheduleReplicaImportID(importID string) (scheduleID string, replicaID string, ok bool) {
	idParts := strings.Split(strings.TrimSpace(importID), ":")
	if len(idParts) == 2 {
		scheduleID, replicaID = strings.TrimSpace(idParts[0]), strings.TrimSpace(idParts[1])
	}

	return scheduleID, replicaID, scheduleID != "" && replicaID != ""
}

func (r *IncidentScheduleReplicaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scheduleID, replicaID, ok := parseScheduleReplicaImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("The import ID must be in the format: schedule_id:replica_id (got %q)", req.ID),
		)
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Importing schedule replica with schedule_id=%s and replica_id=%s", scheduleID, replicaID))

	result, err := r.client.SchedulesV2ShowScheduleReplicaWithResponse(ctx, scheduleID, replicaID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError(
				"Schedule Replica Not Found",
				fmt.Sprintf("No replica with ID %q exists on schedule %q. Import IDs must be in the format schedule_id:replica_id.", replicaID, scheduleID),
			)
			return
		}
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

	data := models.ScheduleReplicaModel{}.FromAPI(result.JSON200.ScheduleReplica)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
