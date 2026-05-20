package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	_ resource.Resource                = &IncidentScheduleSyncRuleResource{}
	_ resource.ResourceWithConfigure   = &IncidentScheduleSyncRuleResource{}
	_ resource.ResourceWithImportState = &IncidentScheduleSyncRuleResource{}
)

type IncidentScheduleSyncRuleResource struct {
	client           *client.ClientWithResponses
	terraformVersion string
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
				MarkdownDescription: apischema.Docstring("ScheduleSyncRuleV2", "sync_type") + "\n\nValid values: `all_users`, `on_call`.",
			},
		},
	}
}

func (r *IncidentScheduleSyncRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IncidentScheduleSyncRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.ScheduleSyncRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SchedulesV2CreateScheduleSyncRuleWithResponse(ctx, data.ScheduleID.ValueString(), client.SchedulesCreateScheduleSyncRulePayloadV2{
		ScheduleSyncRule: client.ScheduleSyncRuleCreatePayloadV2{
			ScheduleSyncTargetId: data.ScheduleSyncTargetID.ValueString(),
			SyncType:             client.ScheduleSyncRuleCreatePayloadV2SyncType(data.SyncType.ValueString()),
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.ScheduleSyncRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

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

	data = models.ScheduleSyncRuleResourceModel{}.FromAPI(result.JSON200.ScheduleSyncRule)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IncidentScheduleSyncRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.ScheduleSyncRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.SchedulesV2UpdateScheduleSyncRuleWithResponse(ctx, data.ScheduleID.ValueString(), data.ID.ValueString(), client.SchedulesUpdateScheduleSyncRulePayloadV2{
		SyncType: client.SchedulesUpdateScheduleSyncRulePayloadV2SyncType(data.SyncType.ValueString()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update schedule sync rule, got error: %s", err))
		return
	}

	data = models.ScheduleSyncRuleResourceModel{}.FromAPI(result.JSON200.ScheduleSyncRule)

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

func (r *IncidentScheduleSyncRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: schedule_id:rule_id
	idParts := strings.Split(req.ID, ":")
	if len(idParts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"The import ID must be in the format: schedule_id:rule_id",
		)
		return
	}

	scheduleID := idParts[0]
	ruleID := idParts[1]

	tflog.Info(ctx, fmt.Sprintf("Importing schedule sync rule with schedule_id=%s and rule_id=%s", scheduleID, ruleID))

	claimResource(ctx, r.client, ruleID, resp.Diagnostics, client.ScheduleSyncRule, r.terraformVersion)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ruleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schedule_id"), scheduleID)...)
}
