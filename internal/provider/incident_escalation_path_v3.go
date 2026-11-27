package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// The V3 escalation path API takes the same nodes as V2, with the same JSON fields, except
// for a target's high/low setting: V2's urgency is V3's interruption_level. Branch
// conditions differ only in what they may test, which the conditions in
// incident_escalation_path_conditions.go handle for both versions.
//
// So rather than a second copy of every node conversion, this codec converts each node as
// V2 does, bridges it to its V3 type with models.ConvertAPIType, and sets the targets
// itself, since that's the one field the bridge can't carry across.

// pathSequenceCodecV3 is the codec for an escalation path's nodes written and read through
// V3.
type pathSequenceCodecV3 struct{}

func (pathSequenceCodecV3) LeafPayload(ctx context.Context, id string, node escalationPathNode, diags *diag.Diagnostics) (client.EscalationPathNodePayloadV3, bool) {
	v2, ok := pathSequenceCodec{}.LeafPayload(ctx, id, node, diags)
	if !ok {
		return client.EscalationPathNodePayloadV3{Id: id}, false
	}

	payload := models.ConvertAPIType[client.EscalationPathNodePayloadV3](v2)
	if node.Level != nil && payload.Level != nil {
		payload.Level.Targets = targetsToPayloadV3(ctx, node.Level.Targets, diags)
	}
	if node.NotifyChannel != nil && payload.NotifyChannel != nil {
		payload.NotifyChannel.Targets = targetsToPayloadV3(ctx, node.NotifyChannel.Targets, diags)
	}
	return payload, true
}

func (pathSequenceCodecV3) BranchPayload(id string, conditions []client.ConditionPayloadV2, thenPath, elsePath []client.EscalationPathNodePayloadV3) client.EscalationPathNodePayloadV3 {
	return client.EscalationPathNodePayloadV3{
		Id:   id,
		Type: client.EscalationPathNodePayloadV3TypeIfElse,
		IfElse: &client.EscalationPathNodeIfElsePayloadV3{
			Conditions: lo.ToPtr(models.ConvertAPIType[[]client.ConditionPayloadV3](conditions)),
			ThenPath:   thenPath,
			ElsePath:   elsePath,
		},
	}
}

func (pathSequenceCodecV3) NodeID(node client.EscalationPathNodeV3) string { return node.Id }
func (pathSequenceCodecV3) NodeType(node client.EscalationPathNodeV3) string {
	return string(node.Type)
}

func (pathSequenceCodecV3) Branch(node client.EscalationPathNodeV3) ([]client.ConditionV2, []client.EscalationPathNodeV3, []client.EscalationPathNodeV3, bool) {
	if node.IfElse == nil {
		return nil, nil, nil, false
	}
	return models.ConvertAPIType[[]client.ConditionV2](node.IfElse.Conditions), node.IfElse.ThenPath, node.IfElse.ElsePath, true
}

func (pathSequenceCodecV3) Leaf(ctx context.Context, node client.EscalationPathNodeV3, diags *diag.Diagnostics) (escalationPathNode, bool) {
	converted, ok := pathSequenceCodec{}.Leaf(ctx, models.ConvertAPIType[client.EscalationPathNodeV2](node), diags)
	if !ok {
		return converted, false
	}

	if node.Level != nil && converted.Level != nil {
		converted.Level.Targets = targetsFromAPIV3(ctx, node.Level.Targets, diags)
	}
	if node.NotifyChannel != nil && converted.NotifyChannel != nil {
		converted.NotifyChannel.Targets = targetsFromAPIV3(ctx, node.NotifyChannel.Targets, diags)
	}
	return converted, true
}

// showEscalationPath reads a path through the API version given, and builds state from it.
//
// V2 refuses to show a path stored with urgency conditions, with a 409: one saved through
// V3 or the dashboard since we last wrote it. V3 can read it, so a V2 read that gets a 409
// reads through V3 instead. The state then holds V3 fields, so a config still using V2
// fields plans a change.
func showEscalationPath(ctx context.Context, apiClient *client.ClientWithResponses, id string, version escalationPathAPIVersion, prior *escalationPathModel, diags *diag.Diagnostics) (*escalationPathModel, error) {
	if version == escalationPathAPIV2 {
		result, err := apiClient.EscalationPathsV2ShowWithResponse(ctx, id)
		switch {
		case isConflict(err):
			tflog.Info(ctx, fmt.Sprintf("Escalation path %s branches on urgency, so V2 can't read it: reading it through V3.", id))
		case err != nil:
			return nil, err
		case result.JSON200 == nil:
			return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
		default:
			return (&escalationPathResource{}).buildModel(ctx, result.JSON200.EscalationPath, prior, diags), nil
		}
	}

	result, err := apiClient.EscalationPathsV3ShowWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if result.JSON200 == nil {
		return nil, fmt.Errorf("unexpected response from API (status %s)", result.Status())
	}
	return (&escalationPathResource{}).buildModelV3(ctx, result.JSON200.EscalationPath, prior, diags), nil
}

// escalationPathBranchesOnUrgencyDetail explains a V2 update refused because the path now
// branches on urgency, which V2 can't write.
func escalationPathBranchesOnUrgencyDetail(err error) string {
	return "This escalation path now branches on escalation urgency, probably because someone " +
		"saved it in the dashboard. Only the V3 API can update it. To move this resource to V3, " +
		"replace urgency with interruption_level on every target, and priority_one_of with " +
		"urgency_one_of on every branch.\n\n" + err.Error()
}
