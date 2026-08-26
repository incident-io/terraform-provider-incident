package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

// The level, notify_channel and delay blocks are identical in incident_escalation_path
// and incident_escalation_path_beta: only the way nodes are arranged differs between the
// two. Their schema, attribute types and conversions live here so the two resources share
// one definition rather than drifting apart.

// levelAttrTypes returns the attribute types for a level node's block.
func levelAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"targets": targetListType(),
		"round_robin_config": types.ObjectType{AttrTypes: map[string]attr.Type{
			"enabled":              types.BoolType,
			"rotate_after_seconds": types.Int64Type,
		}},
		"retry_config": types.ObjectType{AttrTypes: map[string]attr.Type{
			"attempts":         types.Int64Type,
			"interval_seconds": types.Int64Type,
		}},
		"time_to_ack_seconds":                    types.Int64Type,
		"time_to_ack_interval_condition":         types.StringType,
		"time_to_ack_weekday_interval_config_id": types.StringType,
		"ack_mode":                               types.StringType,
	}
}

// notifyChannelAttrTypes returns the attribute types for a notify_channel node's block.
func notifyChannelAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"targets":                                targetListType(),
		"time_to_ack_seconds":                    types.Int64Type,
		"time_to_ack_interval_condition":         types.StringType,
		"time_to_ack_weekday_interval_config_id": types.StringType,
	}
}

// delayAttrTypes returns the attribute types for a delay node's block.
func delayAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"delay_seconds":                    types.Int64Type,
		"delay_interval_condition":         types.StringType,
		"delay_weekday_interval_config_id": types.StringType,
	}
}

// escalationPathAttrTypes returns the attribute types for an escalation_path node's
// block.
func escalationPathAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"escalation_path_id": types.StringType,
	}
}

// escalationPathTargetsAttribute returns the targets list attribute. docType names the
// API type the docstring is pulled from, which differs between level and notify_channel.
func escalationPathTargetsAttribute(docType string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: apischema.Docstring(docType, "targets"),
		Required:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "id"),
					Required:            true,
				},
				"type": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "type"),
					Required:            true,
				},
				"urgency": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetV2", "urgency"),
					Required:            true,
				},
				"schedule_mode": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "schedule_mode"),
					Optional:            true,
					Computed:            true,
				},
				"selected_rota_id": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathTargetV2", "selected_rota_id"),
					Optional:            true,
				},
			},
		},
	}
}

// escalationPathLevelAttribute returns the level block's schema. The GA resource keeps
// ack_mode defaulting to "all": flipping it would rewrite ack_mode in existing state.
func escalationPathLevelAttribute(ackModeDefault string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "level"),
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"targets": escalationPathTargetsAttribute("EscalationPathNodeLevelV2"),
			"round_robin_config": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRoundRobinConfigV2", "enabled"),
						Required:            true,
					},
					"rotate_after_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRoundRobinConfigV2", "rotate_after_seconds"),
						Optional:            true,
					},
				},
			},
			"retry_config": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"attempts": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRetryConfigV2", "attempts"),
						Required:            true,
					},
					"interval_seconds": schema.Int64Attribute{
						MarkdownDescription: apischema.Docstring("EscalationPathRetryConfigV2", "interval_seconds"),
						Required:            true,
					},
				},
			},
			"time_to_ack_seconds": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeLevelV2", "time_to_ack_seconds"),
				Optional:            true,
			},
			"time_to_ack_interval_condition": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring(
					"EscalationPathNodeLevelV2", "time_to_ack_interval_condition"),
				Optional: true,
			},
			"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring(
					"EscalationPathNodeLevelV2", "time_to_ack_weekday_interval_config_id"),
				Optional: true,
			},
			"ack_mode": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring(
					"EscalationPathNodeLevelV2", "ack_mode"),
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(ackModeDefault),
			},
		},
	}
}

// escalationPathNotifyChannelAttribute returns the notify_channel block's schema.
func escalationPathNotifyChannelAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "notify_channel"),
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"targets": escalationPathTargetsAttribute("EscalationPathNodeNotifyChannelV2"),
			"time_to_ack_seconds": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeNotifyChannelV2", "time_to_ack_seconds"),
				Optional:            true,
			},
			"time_to_ack_interval_condition": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring(
					"EscalationPathNodeNotifyChannelV2", "time_to_ack_interval_condition"),
				Optional: true,
			},
			"time_to_ack_weekday_interval_config_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring(
					"EscalationPathNodeNotifyChannelV2", "time_to_ack_weekday_interval_config_id"),
				Optional: true,
			},
		},
	}
}

// escalationPathDelayAttribute returns the delay block's schema.
func escalationPathDelayAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: apischema.Docstring("EscalationPathNodeV2", "delay"),
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"delay_seconds": schema.Int64Attribute{
				MarkdownDescription: apischema.Docstring("EscalationPathNodeDelayV2", "delay_seconds"),
				Optional:            true,
			},
			"delay_interval_condition": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring(
					"EscalationPathNodeDelayV2", "delay_interval_condition"),
				Optional: true,
			},
			"delay_weekday_interval_config_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring(
					"EscalationPathNodeDelayV2", "delay_weekday_interval_config_id"),
				Optional: true,
			},
		},
	}
}

// escalationPathEscalationPathAttribute returns the escalation_path block's schema.
//
// The sub-object carries no description of its own in the API schema, so unlike its
// sibling blocks there's no apischema.Docstring worth reading: what the node does is
// documented on the node's type enum instead, and this repeats it.
func escalationPathEscalationPathAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Reassign the escalation to another escalation path, " +
			"continuing from that path's first node.",
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"escalation_path_id": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring(
					"EscalationPathNodeEscalationPathV2", "escalation_path_id"),
				Required: true,
			},
		},
	}
}

// levelFromAPI builds the level block's model from the API node, or nil when the node
// isn't a level.
func levelFromAPI(ctx context.Context, level *client.EscalationPathNodeLevelV2, diags *diag.Diagnostics) *IncidentEscalationPathNodeLevel {
	if level == nil {
		return nil
	}

	out := &IncidentEscalationPathNodeLevel{
		Targets: targetsFromAPI(ctx, level.Targets, diags),
	}
	if value := level.RoundRobinConfig; value != nil {
		var rotateAfterSeconds basetypes.Int64Value
		if value.RotateAfterSeconds != nil {
			rotateAfterSeconds = types.Int64Value(*value.RotateAfterSeconds)
		}
		out.RoundRobinConfig = &IncidentEscalationRoundRobinConfig{
			Enabled:            types.BoolValue(value.Enabled),
			RotateAfterSeconds: rotateAfterSeconds,
		}
	}
	if value := level.RetryConfig; value != nil {
		out.RetryConfig = &IncidentEscalationRetryConfig{
			Attempts:        types.Int64Value(value.Attempts),
			IntervalSeconds: types.Int64Value(value.IntervalSeconds),
		}
	}
	if value := level.TimeToAckSeconds; value != nil {
		out.TimeToAckSeconds = types.Int64Value(*value)
	}
	if value := level.TimeToAckIntervalCondition; value != nil {
		out.TimeToAckIntervalCondition = types.StringValue(string(*value))
	}
	if value := level.TimeToAckWeekdayIntervalConfigId; value != nil && *value != "" {
		out.TimeToAckWeekdayIntervalConfigID = types.StringValue(*value)
	}
	if value := level.AckMode; value != nil {
		out.AckMode = types.StringValue(string(*value))
	}
	return out
}

// notifyChannelFromAPI builds the notify_channel block's model from the API node, or nil
// when the node isn't a notify_channel.
func notifyChannelFromAPI(ctx context.Context, notifyChannel *client.EscalationPathNodeNotifyChannelV2, diags *diag.Diagnostics) *IncidentEscalationPathNodeNotifyChannel {
	if notifyChannel == nil {
		return nil
	}

	out := &IncidentEscalationPathNodeNotifyChannel{
		Targets: targetsFromAPI(ctx, notifyChannel.Targets, diags),
	}
	if value := notifyChannel.TimeToAckSeconds; value != nil {
		out.TimeToAckSeconds = types.Int64Value(*value)
	}
	if value := notifyChannel.TimeToAckIntervalCondition; value != nil {
		out.TimeToAckIntervalCondition = types.StringValue(string(*value))
	}
	if value := notifyChannel.TimeToAckWeekdayIntervalConfigId; value != nil && *value != "" {
		out.TimeToAckWeekdayIntervalConfigID = types.StringValue(*value)
	}
	return out
}

// delayFromAPI builds the delay block's model from the API node, or nil when the node
// isn't a delay.
func delayFromAPI(delay *client.EscalationPathNodeDelayV2) *IncidentEscalationPathNodeDelay {
	if delay == nil {
		return nil
	}

	out := &IncidentEscalationPathNodeDelay{}
	if value := delay.DelaySeconds; value != nil {
		out.DelaySeconds = types.Int64Value(*value)
	}
	if value := delay.DelayIntervalCondition; value != nil {
		out.DelayIntervalCondition = types.StringValue(string(*value))
	}
	if value := delay.DelayWeekdayIntervalConfigId; value != nil && *value != "" {
		out.DelayWeekdayIntervalConfigID = types.StringValue(*value)
	}
	return out
}

// levelToPayload converts the level block's model to the API payload, or nil when the
// node isn't a level.
func levelToPayload(ctx context.Context, level *IncidentEscalationPathNodeLevel, diags *diag.Diagnostics) *client.EscalationPathNodeLevelV2 {
	if level == nil {
		return nil
	}

	var intervalCondition *client.EscalationPathNodeLevelV2TimeToAckIntervalCondition
	if value := level.TimeToAckIntervalCondition.ValueStringPointer(); value != nil {
		intervalCondition = lo.ToPtr(client.EscalationPathNodeLevelV2TimeToAckIntervalCondition(*value))
	}

	out := &client.EscalationPathNodeLevelV2{
		Targets:                          targetsToPayload(ctx, level.Targets, diags),
		TimeToAckIntervalCondition:       intervalCondition,
		TimeToAckSeconds:                 level.TimeToAckSeconds.ValueInt64Pointer(),
		TimeToAckWeekdayIntervalConfigId: level.TimeToAckWeekdayIntervalConfigID.ValueStringPointer(),
	}

	if level.RoundRobinConfig != nil {
		out.RoundRobinConfig = &client.EscalationPathRoundRobinConfigV2{
			Enabled:            level.RoundRobinConfig.Enabled.ValueBool(),
			RotateAfterSeconds: level.RoundRobinConfig.RotateAfterSeconds.ValueInt64Pointer(),
		}
	}

	if level.RetryConfig != nil {
		out.RetryConfig = &client.EscalationPathRetryConfigV2{
			Attempts:        level.RetryConfig.Attempts.ValueInt64(),
			IntervalSeconds: level.RetryConfig.IntervalSeconds.ValueInt64(),
		}
	}

	if !level.AckMode.IsNull() {
		out.AckMode = lo.ToPtr(client.EscalationPathNodeLevelV2AckMode(level.AckMode.ValueString()))
	}

	return out
}

// notifyChannelToPayload converts the notify_channel block's model to the API payload, or
// nil when the node isn't a notify_channel.
func notifyChannelToPayload(ctx context.Context, notifyChannel *IncidentEscalationPathNodeNotifyChannel, diags *diag.Diagnostics) *client.EscalationPathNodeNotifyChannelV2 {
	if notifyChannel == nil {
		return nil
	}

	var intervalCondition *client.EscalationPathNodeNotifyChannelV2TimeToAckIntervalCondition
	if value := notifyChannel.TimeToAckIntervalCondition.ValueStringPointer(); value != nil {
		intervalCondition = lo.ToPtr(client.EscalationPathNodeNotifyChannelV2TimeToAckIntervalCondition(*value))
	}

	return &client.EscalationPathNodeNotifyChannelV2{
		Targets:                          targetsToPayload(ctx, notifyChannel.Targets, diags),
		TimeToAckIntervalCondition:       intervalCondition,
		TimeToAckSeconds:                 notifyChannel.TimeToAckSeconds.ValueInt64Pointer(),
		TimeToAckWeekdayIntervalConfigId: notifyChannel.TimeToAckWeekdayIntervalConfigID.ValueStringPointer(),
	}
}

// delayToPayload converts the delay block's model to the API payload, or nil when the
// node isn't a delay.
func delayToPayload(delay *IncidentEscalationPathNodeDelay) *client.EscalationPathNodeDelayV2 {
	if delay == nil {
		return nil
	}

	var intervalCondition *client.EscalationPathNodeDelayV2DelayIntervalCondition
	if value := delay.DelayIntervalCondition.ValueStringPointer(); value != nil {
		intervalCondition = lo.ToPtr(client.EscalationPathNodeDelayV2DelayIntervalCondition(*value))
	}

	return &client.EscalationPathNodeDelayV2{
		DelayIntervalCondition:       intervalCondition,
		DelaySeconds:                 delay.DelaySeconds.ValueInt64Pointer(),
		DelayWeekdayIntervalConfigId: delay.DelayWeekdayIntervalConfigID.ValueStringPointer(),
	}
}

// escalationPathFromAPI builds the escalation_path block's model from the API node, or
// nil when the node isn't a reassignment.
func escalationPathFromAPI(escalationPath *client.EscalationPathNodeEscalationPathV2) *IncidentEscalationPathNodeEscalationPath {
	if escalationPath == nil {
		return nil
	}

	return &IncidentEscalationPathNodeEscalationPath{
		EscalationPathID: types.StringValue(escalationPath.EscalationPathId),
	}
}

// escalationPathToPayload builds the escalation_path node's payload, or nil when the node
// isn't a reassignment.
func escalationPathToPayload(escalationPath *IncidentEscalationPathNodeEscalationPath) *client.EscalationPathNodeEscalationPathV2 {
	if escalationPath == nil {
		return nil
	}

	return &client.EscalationPathNodeEscalationPathV2{
		EscalationPathId: escalationPath.EscalationPathID.ValueString(),
	}
}

// The working_hours, repeat_config and team_ids attributes sit on the escalation path
// itself rather than on a node, and are identical in both resources.

// escalationPathWorkingHoursToPayload decodes the working_hours list into the API
// payload, or returns nil when unset.
func escalationPathWorkingHoursToPayload(ctx context.Context, list types.List, diags *diag.Diagnostics) *[]client.WeekdayIntervalConfigV2 {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}

	var whModels []models.IncidentWeekdayIntervalConfig
	diags.Append(list.ElementsAs(ctx, &whModels, false)...)
	if diags.HasError() || len(whModels) == 0 {
		return nil
	}

	workingHours := make([]client.WeekdayIntervalConfigV2, 0, len(whModels))
	for _, wh := range whModels {
		workingHours = append(workingHours, wh.ToClientV2(ctx, diags))
	}
	return &workingHours
}

// escalationPathWorkingHoursFromAPI builds the working_hours list for state.
func escalationPathWorkingHoursFromAPI(ctx context.Context, workingHours *[]client.WeekdayIntervalConfigV2, diags *diag.Diagnostics) types.List {
	workingHoursType := types.ObjectType{AttrTypes: models.WeekdayIntervalConfigAttrTypes()}
	if workingHours == nil {
		return types.ListNull(workingHoursType)
	}

	whModels := lo.Map(*workingHours, func(wh client.WeekdayIntervalConfigV2, _ int) models.IncidentWeekdayIntervalConfig {
		return models.IncidentWeekdayIntervalConfig{}.FromClientV2(ctx, wh, diags)
	})
	list, d := types.ListValueFrom(ctx, workingHoursType, whModels)
	diags.Append(d...)
	return list
}

// escalationPathTeamIDsToPayload decodes the team_ids set into the API payload, or
// returns nil when unset.
func escalationPathTeamIDsToPayload(ctx context.Context, set types.Set, diags *diag.Diagnostics) *[]string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	ids := []string{}
	diags.Append(set.ElementsAs(ctx, &ids, false)...)
	if diags.HasError() {
		return nil
	}
	return &ids
}

// escalationPathTeamIDsFromAPI builds the team_ids set for state.
func escalationPathTeamIDsFromAPI(teamIDs []string) types.Set {
	if teamIDs == nil {
		return types.SetNull(types.StringType)
	}

	elements := lo.Map(teamIDs, func(id string, _ int) attr.Value {
		return types.StringValue(id)
	})
	return types.SetValueMust(types.StringType, elements)
}

// escalationPathRepeatConfigAttrTypes returns the attribute types for the path-level
// repeat_config block.
func escalationPathRepeatConfigAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"repeat_after_seconds":     types.Int64Type,
		"delay_repeat_on_activity": types.BoolType,
	}
}

// escalationPathRepeatConfigToPayload decodes the repeat_config object into the API
// payload, or returns nil when unset.
func escalationPathRepeatConfigToPayload(ctx context.Context, obj types.Object, diags *diag.Diagnostics) *client.EscalationPathRepeatConfigV2 {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}

	var repeatConfig IncidentEscalationPathRepeatConfig
	diags.Append(obj.As(ctx, &repeatConfig, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil
	}
	return &client.EscalationPathRepeatConfigV2{
		RepeatAfterSeconds:    int32(repeatConfig.RepeatAfterSeconds.ValueInt64()),
		DelayRepeatOnActivity: repeatConfig.DelayRepeatOnActivity.ValueBool(),
	}
}

// escalationPathRepeatConfigFromAPI builds the repeat_config object for state.
func escalationPathRepeatConfigFromAPI(repeatConfig *client.EscalationPathRepeatConfigV2) types.Object {
	attrTypes := escalationPathRepeatConfigAttrTypes()
	if repeatConfig == nil {
		return types.ObjectNull(attrTypes)
	}

	return types.ObjectValueMust(attrTypes, map[string]attr.Value{
		"repeat_after_seconds":     types.Int64Value(int64(repeatConfig.RepeatAfterSeconds)),
		"delay_repeat_on_activity": types.BoolValue(repeatConfig.DelayRepeatOnActivity),
	})
}
