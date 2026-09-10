package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// An escalation path template's nodes are an escalation path's nodes with one addition: a
// level or notify_channel target may carry a binding to one of the template's params or
// expressions instead of naming a concrete id. The API models that as parallel
// "WithBinding" types, so this file holds the template-side twins of the target, level and
// notify_channel helpers in escalation_path_nodes.go and the codec that plugs them into the
// shared sequence conversions.

// escalationPathTemplateTarget is a target on a template node: either a concrete id, as on
// an escalation path, or a binding the template resolves per templated path.
type escalationPathTemplateTarget struct {
	ID             types.String                       `tfsdk:"id"`
	Type           types.String                       `tfsdk:"type"`
	Urgency        types.String                       `tfsdk:"urgency"`
	ScheduleMode   types.String                       `tfsdk:"schedule_mode"`
	SelectedRotaID types.String                       `tfsdk:"selected_rota_id"`
	Binding        *models.IncidentEngineParamBinding `tfsdk:"binding"`
}

func escalationPathTemplateTargetAttrTypes() map[string]attr.Type {
	attrTypes := targetAttrTypes()
	attrTypes["binding"] = types.ObjectType{AttrTypes: models.ParamBindingAttrTypes()}
	return attrTypes
}

func escalationPathTemplateTargetListType() types.ListType {
	return types.ListType{ElemType: types.ObjectType{AttrTypes: escalationPathTemplateTargetAttrTypes()}}
}

func escalationPathTemplateLevelAttrTypes() map[string]attr.Type {
	attrTypes := levelAttrTypes()
	attrTypes["targets"] = escalationPathTemplateTargetListType()
	return attrTypes
}

func escalationPathTemplateNotifyChannelAttrTypes() map[string]attr.Type {
	attrTypes := notifyChannelAttrTypes()
	attrTypes["targets"] = escalationPathTemplateTargetListType()
	return attrTypes
}

// escalationPathTemplateNodeAttrTypes is escalationPathBetaNodeAttrTypes with the template's
// targets in its level and notify_channel blocks.
func escalationPathTemplateNodeAttrTypes() map[string]attr.Type {
	attrTypes := escalationPathBetaNodeAttrTypes()
	attrTypes["level"] = types.ObjectType{AttrTypes: escalationPathTemplateLevelAttrTypes()}
	attrTypes["notify_channel"] = types.ObjectType{AttrTypes: escalationPathTemplateNotifyChannelAttrTypes()}
	return attrTypes
}

// escalationPathTemplateTargetsAttribute is the targets list on a template's level or
// notify_channel. id is optional here, because a target may bind instead.
func escalationPathTemplateTargetsAttribute(docType string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: apischema.Docstring(docType, "targets"),
		Required:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathTargetWithBindingV2", "id") +
						" Set exactly one of `id` and `binding`.",
					Optional: true,
				},
				"type": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetWithBindingV2", "type"),
					Required:            true,
				},
				"urgency": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetWithBindingV2", "urgency"),
					Required:            true,
				},
				"schedule_mode": schema.StringAttribute{
					MarkdownDescription: EnumValuesDescription("EscalationPathTargetWithBindingV2", "schedule_mode"),
					Optional:            true,
					Computed:            true,
				},
				"selected_rota_id": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("EscalationPathTargetWithBindingV2", "selected_rota_id"),
					Optional:            true,
				},
				"binding": schema.SingleNestedAttribute{
					MarkdownDescription: "Who this target resolves to, decided per templated path. `value_reference` names one of the template's `params`, and `expression_ref` one of its `expressions`. Set exactly one of `id` and `binding`.",
					Optional:            true,
					Attributes:          models.ParamBindingAttributes(),
				},
			},
		},
	}
}

// escalationPathTemplateNodeSchema is escalationPathBetaNodeSchema with the template's
// targets in its level and notify_channel blocks.
func escalationPathTemplateNodeSchema() schema.NestedAttributeObject {
	node := escalationPathBetaNodeSchema()

	level := escalationPathLevelAttribute("first")
	level.Attributes["targets"] = escalationPathTemplateTargetsAttribute("EscalationPathNodeLevelWithBindingV2")
	node.Attributes["level"] = level

	notifyChannel := escalationPathNotifyChannelAttribute()
	notifyChannel.Attributes["targets"] = escalationPathTemplateTargetsAttribute("EscalationPathNodeNotifyChannelWithBindingV2")
	node.Attributes["notify_channel"] = notifyChannel

	return node
}

func decodeTemplateTargets(ctx context.Context, list types.List, diags *diag.Diagnostics) []escalationPathTemplateTarget {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var targets []escalationPathTemplateTarget
	diags.Append(list.ElementsAs(ctx, &targets, false)...)
	return targets
}

// validateEscalationPathTemplateTarget applies the escalation path's target checks, plus the
// one the template adds: a target names an id or carries a binding, never both or neither.
func validateEscalationPathTemplateTarget(target escalationPathTemplateTarget, diags *diag.Diagnostics) {
	validateEscalationPathTarget(IncidentEscalationPathTarget{
		ID:             target.ID,
		Type:           target.Type,
		Urgency:        target.Urgency,
		ScheduleMode:   target.ScheduleMode,
		SelectedRotaID: target.SelectedRotaID,
	}, diags)

	if target.ID.IsUnknown() {
		return
	}

	hasID := target.ID.ValueString() != ""
	hasBinding := target.Binding != nil && !target.Binding.IsEmpty()
	switch {
	case hasID && hasBinding:
		diags.AddError(
			"Target sets both id and binding",
			"An escalation path template target is either a concrete `id` or a `binding` to one of the template's params or expressions, not both.",
		)
	case !hasID && !hasBinding:
		diags.AddError(
			"Target sets neither id nor binding",
			"An escalation path template target needs a concrete `id`, or a `binding` to one of the template's params or expressions.",
		)
	}
}

func escalationPathTemplateTargetsToPayload(ctx context.Context, list types.List, diags *diag.Diagnostics) []client.EscalationPathTargetWithBindingPayloadV2 {
	targets := decodeTemplateTargets(ctx, list, diags)
	return lo.Map(targets, func(target escalationPathTemplateTarget, _ int) client.EscalationPathTargetWithBindingPayloadV2 {
		payload := client.EscalationPathTargetWithBindingPayloadV2{
			Type:    client.EscalationPathTargetWithBindingPayloadV2Type(target.Type.ValueString()),
			Urgency: client.EscalationPathTargetWithBindingPayloadV2Urgency(target.Urgency.ValueString()),
		}
		if id := target.ID.ValueString(); id != "" {
			payload.Id = lo.ToPtr(id)
		}
		if target.Binding != nil && !target.Binding.IsEmpty() {
			payload.Binding = lo.ToPtr(target.Binding.ToPayload())
		}
		if mode := target.ScheduleMode.ValueString(); mode != "" {
			payload.ScheduleMode = lo.ToPtr(client.EscalationPathTargetWithBindingPayloadV2ScheduleMode(mode))
		}
		if rota := target.SelectedRotaID.ValueString(); rota != "" {
			payload.SelectedRotaId = lo.ToPtr(rota)
		}
		return payload
	})
}

func escalationPathTemplateTargetsFromAPI(ctx context.Context, targets []client.EscalationPathTargetWithBindingV2, diags *diag.Diagnostics) types.List {
	targetModels := lo.Map(targets, func(target client.EscalationPathTargetWithBindingV2, _ int) escalationPathTemplateTarget {
		out := escalationPathTemplateTarget{
			ID:             types.StringNull(),
			Type:           types.StringValue(string(target.Type)),
			Urgency:        types.StringValue(string(target.Urgency)),
			ScheduleMode:   types.StringNull(),
			SelectedRotaID: types.StringNull(),
		}
		if target.Id != nil && *target.Id != "" {
			out.ID = types.StringValue(*target.Id)
		}
		if target.ScheduleMode != nil {
			out.ScheduleMode = types.StringValue(string(*target.ScheduleMode))
		}
		if target.SelectedRotaId != nil && *target.SelectedRotaId != "" {
			out.SelectedRotaID = types.StringValue(*target.SelectedRotaId)
		}
		if target.Binding != nil {
			out.Binding = lo.ToPtr(models.IncidentEngineParamBinding{}.FromAPI(*target.Binding))
		}
		return out
	})

	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: escalationPathTemplateTargetAttrTypes()}, targetModels)
	diags.Append(d...)
	return list
}

// The level and notify_channel conversions below reuse the escalation path's for everything
// but the targets, whose types differ. The path-typed value is built with no targets, its
// fields copied across, and the template's targets set alongside.

func escalationPathTemplateLevelFromAPI(ctx context.Context, level *client.EscalationPathNodeLevelWithBindingV2, diags *diag.Diagnostics) *IncidentEscalationPathNodeLevel {
	if level == nil {
		return nil
	}

	var ackMode *client.EscalationPathNodeLevelV2AckMode
	if level.AckMode != nil {
		ackMode = lo.ToPtr(client.EscalationPathNodeLevelV2AckMode(*level.AckMode))
	}
	var intervalCondition *client.EscalationPathNodeLevelV2TimeToAckIntervalCondition
	if level.TimeToAckIntervalCondition != nil {
		intervalCondition = lo.ToPtr(client.EscalationPathNodeLevelV2TimeToAckIntervalCondition(*level.TimeToAckIntervalCondition))
	}

	out := levelFromAPI(ctx, &client.EscalationPathNodeLevelV2{
		AckMode:                          ackMode,
		RetryConfig:                      level.RetryConfig,
		RoundRobinConfig:                 level.RoundRobinConfig,
		TimeToAckIntervalCondition:       intervalCondition,
		TimeToAckSeconds:                 level.TimeToAckSeconds,
		TimeToAckWeekdayIntervalConfigId: level.TimeToAckWeekdayIntervalConfigId,
	}, diags)
	out.Targets = escalationPathTemplateTargetsFromAPI(ctx, level.Targets, diags)
	return out
}

func escalationPathTemplateNotifyChannelFromAPI(ctx context.Context, notifyChannel *client.EscalationPathNodeNotifyChannelWithBindingV2, diags *diag.Diagnostics) *IncidentEscalationPathNodeNotifyChannel {
	if notifyChannel == nil {
		return nil
	}

	var intervalCondition *client.EscalationPathNodeNotifyChannelV2TimeToAckIntervalCondition
	if notifyChannel.TimeToAckIntervalCondition != nil {
		intervalCondition = lo.ToPtr(client.EscalationPathNodeNotifyChannelV2TimeToAckIntervalCondition(*notifyChannel.TimeToAckIntervalCondition))
	}

	out := notifyChannelFromAPI(ctx, &client.EscalationPathNodeNotifyChannelV2{
		TimeToAckIntervalCondition:       intervalCondition,
		TimeToAckSeconds:                 notifyChannel.TimeToAckSeconds,
		TimeToAckWeekdayIntervalConfigId: notifyChannel.TimeToAckWeekdayIntervalConfigId,
	}, diags)
	out.Targets = escalationPathTemplateTargetsFromAPI(ctx, notifyChannel.Targets, diags)
	return out
}

func escalationPathTemplateLevelToPayload(ctx context.Context, level *IncidentEscalationPathNodeLevel, diags *diag.Diagnostics) *client.EscalationPathNodeLevelWithBindingPayloadV2 {
	if level == nil {
		return nil
	}

	targets := level.Targets
	level.Targets = types.ListNull(targetListType())
	base := levelToPayload(ctx, level, diags)
	level.Targets = targets

	var ackMode *client.EscalationPathNodeLevelWithBindingPayloadV2AckMode
	if base.AckMode != nil {
		ackMode = lo.ToPtr(client.EscalationPathNodeLevelWithBindingPayloadV2AckMode(*base.AckMode))
	}
	var intervalCondition *client.EscalationPathNodeLevelWithBindingPayloadV2TimeToAckIntervalCondition
	if base.TimeToAckIntervalCondition != nil {
		intervalCondition = lo.ToPtr(client.EscalationPathNodeLevelWithBindingPayloadV2TimeToAckIntervalCondition(*base.TimeToAckIntervalCondition))
	}

	return &client.EscalationPathNodeLevelWithBindingPayloadV2{
		AckMode:                          ackMode,
		RetryConfig:                      base.RetryConfig,
		RoundRobinConfig:                 base.RoundRobinConfig,
		Targets:                          escalationPathTemplateTargetsToPayload(ctx, targets, diags),
		TimeToAckIntervalCondition:       intervalCondition,
		TimeToAckSeconds:                 base.TimeToAckSeconds,
		TimeToAckWeekdayIntervalConfigId: base.TimeToAckWeekdayIntervalConfigId,
	}
}

func escalationPathTemplateNotifyChannelToPayload(ctx context.Context, notifyChannel *IncidentEscalationPathNodeNotifyChannel, diags *diag.Diagnostics) *client.EscalationPathNodeNotifyChannelWithBindingPayloadV2 {
	if notifyChannel == nil {
		return nil
	}

	targets := notifyChannel.Targets
	notifyChannel.Targets = types.ListNull(targetListType())
	base := notifyChannelToPayload(ctx, notifyChannel, diags)
	notifyChannel.Targets = targets

	var intervalCondition *client.EscalationPathNodeNotifyChannelWithBindingPayloadV2TimeToAckIntervalCondition
	if base.TimeToAckIntervalCondition != nil {
		intervalCondition = lo.ToPtr(client.EscalationPathNodeNotifyChannelWithBindingPayloadV2TimeToAckIntervalCondition(*base.TimeToAckIntervalCondition))
	}

	return &client.EscalationPathNodeNotifyChannelWithBindingPayloadV2{
		Targets:                          escalationPathTemplateTargetsToPayload(ctx, targets, diags),
		TimeToAckIntervalCondition:       intervalCondition,
		TimeToAckSeconds:                 base.TimeToAckSeconds,
		TimeToAckWeekdayIntervalConfigId: base.TimeToAckWeekdayIntervalConfigId,
	}
}

// templateSequenceCodec plugs the template's node types into the shared sequence
// conversions.
type templateSequenceCodec struct{}

func (templateSequenceCodec) LeafPayload(ctx context.Context, id string, node escalationPathBetaNode, diags *diag.Diagnostics) (client.EscalationPathTemplateNodePayloadV2, bool) {
	payload := client.EscalationPathTemplateNodePayloadV2{Id: id}

	switch {
	case node.Loop != nil:
		payload.Type = client.EscalationPathTemplateNodePayloadV2TypeRepeat
		payload.Repeat = &client.EscalationPathNodeRepeatV2{
			RepeatTimes: node.Loop.Times.ValueInt64(),
			ToNode:      node.Loop.BackTo.ValueString(),
		}

	case node.Level != nil:
		payload.Type = client.EscalationPathTemplateNodePayloadV2TypeLevel
		payload.Level = escalationPathTemplateLevelToPayload(ctx, node.Level, diags)

	case node.NotifyChannel != nil:
		payload.Type = client.EscalationPathTemplateNodePayloadV2TypeNotifyChannel
		payload.NotifyChannel = escalationPathTemplateNotifyChannelToPayload(ctx, node.NotifyChannel, diags)

	case node.Delay != nil:
		payload.Type = client.EscalationPathTemplateNodePayloadV2TypeDelay
		payload.Delay = delayToPayload(node.Delay)

	case node.EscalationPath != nil:
		payload.Type = client.EscalationPathTemplateNodePayloadV2TypeEscalationPath
		payload.EscalationPath = escalationPathToPayload(node.EscalationPath)

	default:
		return payload, false
	}

	return payload, true
}

func (templateSequenceCodec) BranchPayload(id string, conditions []client.ConditionPayloadV2, thenPath, elsePath []client.EscalationPathTemplateNodePayloadV2) client.EscalationPathTemplateNodePayloadV2 {
	return client.EscalationPathTemplateNodePayloadV2{
		Id:   id,
		Type: client.EscalationPathTemplateNodePayloadV2TypeIfElse,
		IfElse: &client.EscalationPathTemplateNodeIfElsePayloadV2{
			Conditions: lo.ToPtr(conditions),
			ThenPath:   thenPath,
			ElsePath:   elsePath,
		},
	}
}

func (templateSequenceCodec) NodeID(node client.EscalationPathTemplateNodeV2) string { return node.Id }
func (templateSequenceCodec) NodeType(node client.EscalationPathTemplateNodeV2) string {
	return string(node.Type)
}

func (templateSequenceCodec) Branch(node client.EscalationPathTemplateNodeV2) ([]client.ConditionV2, []client.EscalationPathTemplateNodeV2, []client.EscalationPathTemplateNodeV2, bool) {
	if node.IfElse == nil {
		return nil, nil, nil, false
	}
	return node.IfElse.Conditions, node.IfElse.ThenPath, node.IfElse.ElsePath, true
}

func (templateSequenceCodec) Leaf(ctx context.Context, node client.EscalationPathTemplateNodeV2, diags *diag.Diagnostics) (escalationPathBetaNode, bool) {
	converted := escalationPathBetaNode{}

	switch {
	case node.Repeat != nil:
		converted.Loop = &escalationPathBetaLoop{
			BackTo: types.StringValue(node.Repeat.ToNode),
			Times:  types.Int64Value(node.Repeat.RepeatTimes),
		}

	case node.Level != nil:
		converted.Level = escalationPathTemplateLevelFromAPI(ctx, node.Level, diags)

	case node.NotifyChannel != nil:
		converted.NotifyChannel = escalationPathTemplateNotifyChannelFromAPI(ctx, node.NotifyChannel, diags)

	case node.Delay != nil:
		converted.Delay = delayFromAPI(node.Delay)

	case node.EscalationPath != nil:
		converted.EscalationPath = escalationPathFromAPI(node.EscalationPath)

	default:
		return converted, false
	}

	return converted, true
}

// reconcileTemplateBindingSpelling keeps the author's spelling of each target binding. The
// API returns every binding in its long form, so a config written as `value_reference`
// would otherwise read back as `value = { reference = ... }` and plan a change forever.
// Targets are matched to the prior config by sequence, node and position.
func reconcileTemplateBindingSpelling(ctx context.Context, sequences, prior map[string][]escalationPathBetaNode, diags *diag.Diagnostics) {
	for key, nodes := range sequences {
		priorNodes := prior[key]
		for index := range nodes {
			if index >= len(priorNodes) {
				break
			}
			node, priorNode := &nodes[index], priorNodes[index]

			if node.Level != nil && priorNode.Level != nil {
				node.Level.Targets = reconcileTargetBindings(ctx, node.Level.Targets, priorNode.Level.Targets, diags)
			}
			if node.NotifyChannel != nil && priorNode.NotifyChannel != nil {
				node.NotifyChannel.Targets = reconcileTargetBindings(ctx, node.NotifyChannel.Targets, priorNode.NotifyChannel.Targets, diags)
			}
		}
	}
}

func reconcileTargetBindings(ctx context.Context, applied, prior types.List, diags *diag.Diagnostics) types.List {
	appliedTargets := decodeTemplateTargets(ctx, applied, diags)
	priorTargets := decodeTemplateTargets(ctx, prior, diags)
	if diags.HasError() || len(appliedTargets) == 0 {
		return applied
	}

	for index := range appliedTargets {
		if index >= len(priorTargets) {
			break
		}
		appliedTargets[index].Binding = models.ReconcileBindingSpelling(appliedTargets[index].Binding, priorTargets[index].Binding)
	}

	list, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: escalationPathTemplateTargetAttrTypes()}, appliedTargets)
	diags.Append(d...)
	return list
}

// validateEscalationPathTemplateTargets checks every target in every sequence.
func validateEscalationPathTemplateTargets(ctx context.Context, sequences types.Map, diags *diag.Diagnostics) {
	for key, nodes := range decodeSequences(ctx, sequences, diags) {
		for index, node := range nodes {
			var targets types.List
			switch {
			case node.Level != nil:
				targets = node.Level.Targets
			case node.NotifyChannel != nil:
				targets = node.NotifyChannel.Targets
			default:
				continue
			}
			for _, target := range decodeTemplateTargets(ctx, targets, diags) {
				var targetDiags diag.Diagnostics
				validateEscalationPathTemplateTarget(target, &targetDiags)
				for _, d := range targetDiags {
					diags.AddError(d.Summary(), fmt.Sprintf("%s (sequence %q, node %d)", d.Detail(), key, index))
				}
			}
		}
	}
}
