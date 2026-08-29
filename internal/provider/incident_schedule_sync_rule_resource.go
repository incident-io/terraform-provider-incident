package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

var (
	_ resource.Resource                = &IncidentScheduleSyncRuleResource{}
	_ resource.ResourceWithConfigure   = &IncidentScheduleSyncRuleResource{}
	_ resource.ResourceWithImportState = &IncidentScheduleSyncRuleResource{}
)

type IncidentScheduleSyncRuleResource struct {
	resourceConfigurer
}

func NewIncidentScheduleSyncRuleResource() resource.Resource {
	return &IncidentScheduleSyncRuleResource{}
}

func (r *IncidentScheduleSyncRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_schedule_sync_rule"
}

func (r *IncidentScheduleSyncRuleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage schedule sync rules that link schedules to sync targets (Slack user groups).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"schedule_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "schedule_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schedule_sync_target_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "schedule_sync_target_id"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"sync_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: EnumValuesDescription("ScheduleSyncRuleV2", "sync_type"),
			},
			"rotation_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "rotation_id"),
				// The rotation a rule is scoped to is part of its identity and
				// can't be changed via update, so a change forces a replace.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"permanent_member_user_ids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "permanent_member_user_ids") + "\n\nOmitting the attribute leaves existing permanent members unchanged on update. Set to `[]` to clear them.",
			},
		},
	}
}

func (r *IncidentScheduleSyncRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.ScheduleSyncRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plannedPermanentMembers := data.PermanentMemberUserIDs

	result, err := r.client.SchedulesV2CreateScheduleSyncRuleWithResponse(ctx, data.ScheduleID.ValueString(), client.SchedulesCreateScheduleSyncRulePayloadV2{
		ScheduleSyncRule: client.ScheduleSyncRuleCreatePayloadV2{
			ScheduleSyncTargetId:   data.ScheduleSyncTargetID.ValueString(),
			SyncType:               client.ScheduleSyncRuleCreatePayloadV2SyncType(data.SyncType.ValueString()),
			RotationId:             data.RotationID.ValueStringPointer(),
			PermanentMemberUserIds: data.PermanentMemberUserIDsPayload(),
			Annotations: &map[string]string{
				"incident.io/terraform/version": r.terraformVersion,
			},
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create schedule sync rule, got error: %s", err))
		return
	}

	tflog.Trace(ctx, fmt.Sprintf("created schedule sync rule with id=%s", result.JSON201.ScheduleSyncRule.Id))

	data = models.ScheduleSyncRuleResourceModel{}.FromAPI(result.JSON201.ScheduleSyncRule)
	data.PreserveEmptyPermanentMemberUserIDs(plannedPermanentMembers)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.ScheduleSyncRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	priorPermanentMembers := data.PermanentMemberUserIDs

	result, err := r.client.SchedulesV2ShowScheduleSyncRuleWithResponse(ctx, data.ScheduleID.ValueString(), data.ID.ValueString())
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			tflog.Warn(ctx, fmt.Sprintf("Schedule sync rule with ID %s not found: removing from state.", data.ID.ValueString()))
			resp.State.RemoveResource(ctx)
			return
		}
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

	data = models.ScheduleSyncRuleResourceModel{}.FromAPI(result.JSON200.ScheduleSyncRule)
	// When the attribute is unset in state, leave it null even if the API
	// returns members set outside Terraform — omitting the field on update
	// means "leave unchanged", so we must not invent a diff that would clear
	// them. An explicit empty set in state is preserved so `= []` sticks.
	if priorPermanentMembers.IsNull() {
		data.PermanentMemberUserIDs = types.SetNull(types.StringType)
	} else {
		data.PreserveEmptyPermanentMemberUserIDs(priorPermanentMembers)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.ScheduleSyncRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plannedPermanentMembers := data.PermanentMemberUserIDs

	result, err := r.client.SchedulesV2UpdateScheduleSyncRuleWithResponse(ctx, data.ScheduleID.ValueString(), data.ID.ValueString(), client.SchedulesUpdateScheduleSyncRulePayloadV2{
		SyncType:               client.SchedulesUpdateScheduleSyncRulePayloadV2SyncType(data.SyncType.ValueString()),
		PermanentMemberUserIds: data.PermanentMemberUserIDsPayload(),
		Annotations: &map[string]string{
			"incident.io/terraform/version": r.terraformVersion,
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update schedule sync rule, got error: %s", err))
		return
	}

	data = models.ScheduleSyncRuleResourceModel{}.FromAPI(result.JSON200.ScheduleSyncRule)
	if plannedPermanentMembers.IsNull() {
		// Omitted on the wire — members are unchanged and still unmanaged.
		data.PermanentMemberUserIDs = types.SetNull(types.StringType)
	} else {
		data.PreserveEmptyPermanentMemberUserIDs(plannedPermanentMembers)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.ScheduleSyncRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.SchedulesV2DestroyScheduleSyncRuleWithResponse(ctx, data.ScheduleID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete schedule sync rule, got error: %s", err))
		return
	}
}

// parseScheduleSyncRuleImportID splits the composite import ID a sync rule is
// identified by. Rules are nested under a schedule in the API, so both IDs are
// needed to look one up.
func parseScheduleSyncRuleImportID(importID string) (scheduleID string, ruleID string, ok bool) {
	idParts := strings.Split(strings.TrimSpace(importID), ":")
	if len(idParts) == 2 {
		scheduleID, ruleID = strings.TrimSpace(idParts[0]), strings.TrimSpace(idParts[1])
	}

	return scheduleID, ruleID, scheduleID != "" && ruleID != ""
}

func (r *IncidentScheduleSyncRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scheduleID, ruleID, ok := parseScheduleSyncRuleImportID(req.ID)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("The import ID must be in the format: schedule_id:rule_id (got %q)", req.ID),
		)
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Importing schedule sync rule with schedule_id=%s and rule_id=%s", scheduleID, ruleID))

	// Populate the full model rather than just the IDs, so the imported state
	// matches what Read would produce and a missing rule fails with a clear
	// message instead of an empty import.
	result, err := r.client.SchedulesV2ShowScheduleSyncRuleWithResponse(ctx, scheduleID, ruleID)
	if err != nil {
		httpErr := client.HTTPError{}
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			resp.Diagnostics.AddError(
				"Schedule Sync Rule Not Found",
				fmt.Sprintf("No sync rule with ID %q exists on schedule %q. Import IDs must be in the format schedule_id:rule_id.", ruleID, scheduleID),
			)
			return
		}
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

	claimResourceOnImport(ctx, r.client, ruleID, &resp.Diagnostics, client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeScheduleSyncRule, r.terraformVersion, r.markImportedAsManaged)
	if resp.Diagnostics.HasError() {
		return
	}

	data := models.ScheduleSyncRuleResourceModel{}.FromAPI(result.JSON200.ScheduleSyncRule)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
