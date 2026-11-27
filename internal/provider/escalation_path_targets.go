package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// A target is who a level or notify_channel node escalates to. Both node types hold
// targets in the same shape, and both the resource and the node conversions in
// escalation_path_nodes.go need to read and write them, so the model and its
// conversions live here rather than with either caller.

// Urgency and InterruptionLevel are the V2 and V3 names for the same high/low setting. A
// target sets one, and which one picks the API version: see escalationPathAPIVersion.
type IncidentEscalationPathTarget struct {
	ID                types.String `tfsdk:"id"`
	Type              types.String `tfsdk:"type"`
	Urgency           types.String `tfsdk:"urgency"`
	InterruptionLevel types.String `tfsdk:"interruption_level"`
	ScheduleMode      types.String `tfsdk:"schedule_mode"`
	SelectedRotaID    types.String `tfsdk:"selected_rota_id"`
}

// targetAttrTypes returns the attribute types for an escalation path target
// object.
func targetAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"id":                 types.StringType,
		"type":               types.StringType,
		"urgency":            types.StringType,
		"interruption_level": types.StringType,
		"schedule_mode":      types.StringType,
		"selected_rota_id":   types.StringType,
	}
}

// targetListType returns the list type of escalation path targets.
func targetListType() types.ListType {
	return types.ListType{ElemType: types.ObjectType{AttrTypes: targetAttrTypes()}}
}

// decodeTargets decodes a types.List of target objects into the Go model structs.
func decodeTargets(ctx context.Context, list types.List, diags *diag.Diagnostics) []IncidentEscalationPathTarget {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var targets []IncidentEscalationPathTarget
	diags.Append(list.ElementsAs(ctx, &targets, false)...)
	return targets
}

// targetsFromAPI builds a types.List of escalation path target objects from API
// targets.
func targetsFromAPI(ctx context.Context, targets []client.EscalationPathTargetV2, diags *diag.Diagnostics) types.List {
	targetModels := lo.Map(targets, func(target client.EscalationPathTargetV2, _ int) IncidentEscalationPathTarget {
		scheduleMode := types.StringNull()
		if target.ScheduleMode != nil {
			scheduleMode = types.StringValue(string(*target.ScheduleMode))
		}

		selectedRotaID := types.StringNull()
		if target.SelectedRotaId != nil && *target.SelectedRotaId != "" {
			selectedRotaID = types.StringValue(*target.SelectedRotaId)
		}

		return IncidentEscalationPathTarget{
			ID:                types.StringValue(target.Id),
			Type:              types.StringValue(string(target.Type)),
			Urgency:           types.StringValue(string(target.Urgency)),
			InterruptionLevel: types.StringNull(),
			ScheduleMode:      scheduleMode,
			SelectedRotaID:    selectedRotaID,
		}
	})

	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: targetAttrTypes()}, targetModels)
	diags.Append(d...)
	return list
}

// targetsToPayload converts a types.List of target objects to client payloads.
func targetsToPayload(ctx context.Context, list types.List, diags *diag.Diagnostics) []client.EscalationPathTargetV2 {
	targets := decodeTargets(ctx, list, diags)
	return lo.Map(targets, func(target IncidentEscalationPathTarget, _ int) client.EscalationPathTargetV2 {
		targetPayload := client.EscalationPathTargetV2{
			Id:      target.ID.ValueString(),
			Type:    client.EscalationPathTargetV2Type(target.Type.ValueString()),
			Urgency: client.EscalationPathTargetV2Urgency(target.Urgency.ValueString()),
		}

		if target.ScheduleMode.ValueString() != "" {
			targetPayload.ScheduleMode = lo.ToPtr(client.EscalationPathTargetV2ScheduleMode(target.ScheduleMode.ValueString()))
		}

		if target.SelectedRotaID.ValueString() != "" {
			targetPayload.SelectedRotaId = lo.ToPtr(target.SelectedRotaID.ValueString())
		}

		return targetPayload
	})
}

// targetsFromAPIV3 is targetsFromAPI for a path read through V3, which calls the
// high/low setting interruption_level.
func targetsFromAPIV3(ctx context.Context, targets []client.EscalationPathTargetV3, diags *diag.Diagnostics) types.List {
	// Everything but the high/low setting is the same in both versions, so read it as V2
	// and move the setting across.
	list := targetsFromAPI(ctx, models.ConvertAPIType[[]client.EscalationPathTargetV2](targets), diags)

	converted := decodeTargets(ctx, list, diags)
	for index, target := range targets {
		converted[index].Urgency = types.StringNull()
		converted[index].InterruptionLevel = types.StringValue(string(target.InterruptionLevel))
	}

	out, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: targetAttrTypes()}, converted)
	diags.Append(d...)
	return out
}

// targetsToPayloadV3 is targetsToPayload for a path written through V3.
func targetsToPayloadV3(ctx context.Context, list types.List, diags *diag.Diagnostics) []client.EscalationPathTargetV3 {
	targets := decodeTargets(ctx, list, diags)
	out := models.ConvertAPIType[[]client.EscalationPathTargetV3](targetsToPayload(ctx, list, diags))
	for index, target := range targets {
		out[index].InterruptionLevel = client.EscalationPathTargetV3InterruptionLevel(target.InterruptionLevel.ValueString())
	}
	return out
}

// rotaRequiredScheduleModes is the set of schedule_mode values that require a
// selected_rota_id. Other modes must leave selected_rota_id unset.
var rotaRequiredScheduleModes = map[string]bool{
	"all_users_for_rota":         true,
	"currently_on_call_for_rota": true,
	"next_on_call_for_rota":      true,
}

func validateEscalationPathTarget(target IncidentEscalationPathTarget, diags *diag.Diagnostics) {
	if target.ScheduleMode.IsUnknown() || target.SelectedRotaID.IsUnknown() {
		return
	}

	mode := target.ScheduleMode.ValueString()
	rotaID := target.SelectedRotaID.ValueString()

	if rotaRequiredScheduleModes[mode] {
		if rotaID == "" {
			diags.Append(diag.NewErrorDiagnostic(
				"Missing selected_rota_id",
				fmt.Sprintf("Escalation path target with schedule_mode %q requires selected_rota_id to be set.", mode),
			))
		}
		return
	}

	if rotaID != "" {
		diags.Append(diag.NewErrorDiagnostic(
			"Unexpected selected_rota_id",
			fmt.Sprintf("Escalation path target with schedule_mode %q must not set selected_rota_id; it is only valid for all_users_for_rota, currently_on_call_for_rota, and next_on_call_for_rota.", mode),
		))
	}
}
