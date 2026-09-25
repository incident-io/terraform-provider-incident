package provider

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// TestEscalationPathReassignmentRoundTrip is the test the escalation_path node type exists
// for: before the provider modelled it, a read returned a node with every sub-object nil
// and the next apply sent "type: escalation_path" with no config, which the API rejects.
// A branch's nodes live in a sequence the branch names, so this covers a nested
// reassignment as well as a top-level one.
func TestEscalationPathReassignmentRoundTrip(t *testing.T) {
	ctx := context.Background()

	// A branch puts the reassignment in a sequence the branch names, which is how a
	// nested reassignment reaches the API.
	sequences := map[string][]escalationPathNode{
		"main":   {branchNode("urgent", "quiet")},
		"urgent": {reassignmentNode("01URGENT")},
		"quiet":  {levelNode(t, ""), reassignmentNode("01QUIET")},
	}

	var diags diag.Diagnostics
	payload := unflattenSequences(ctx, "main", sequences, &diags)
	if diags.HasError() {
		t.Fatalf("unflattenSequences produced errors: %+v", diags)
	}

	branch := payload[0].IfElse
	if branch == nil {
		t.Fatal("expected main's branch to convert to an if_else payload")
	}
	if len(branch.ThenPath) != 1 {
		t.Fatalf("expected 1 then node, got %d", len(branch.ThenPath))
	}
	then := branch.ThenPath[0]
	if then.Type != client.EscalationPathNodePayloadV2TypeEscalationPath {
		t.Errorf("got then type %q, want escalation_path", then.Type)
	}
	if then.EscalationPath == nil {
		t.Fatal("then node lost its escalation_path block")
	}
	if got := then.EscalationPath.EscalationPathId; got != "01URGENT" {
		t.Errorf("got then escalation_path_id %q, want 01URGENT", got)
	}

	// Read it back: a reassignment node must survive the flatten, or the next apply
	// fails on a node the resource can't represent.
	var readDiags diag.Diagnostics
	start, got := flattenSequences(ctx, apiNodes(payload), priorNames("main", sequences), &readDiags)
	if readDiags.HasError() {
		t.Fatalf("flattenSequences produced errors: %+v", readDiags)
	}
	if start != "main" {
		t.Errorf("got start %q, want main", start)
	}
	for key, want := range map[string]string{"urgent": "01URGENT", "quiet": "01QUIET"} {
		nodes, ok := got[key]
		if !ok {
			t.Errorf("missing sequence %q", key)
			continue
		}
		node := nodes[len(nodes)-1]
		if node.EscalationPath == nil {
			t.Errorf("sequence %q lost its escalation_path block", key)
			continue
		}
		if id := node.EscalationPath.EscalationPathID.ValueString(); id != want {
			t.Errorf("sequence %q: got escalation_path_id %q, want %s", key, id, want)
		}
	}
}

// TestEscalationPathReassignAfterLoopRoundTrip covers handing over to another path once a
// loop runs out, which the API accepts: the loop mustn't be taken for the end of its
// sequence on the way to the API or on the way back. It sits under a branch so the order
// is checked inside an inlined then path, not just at the top level.
func TestEscalationPathReassignAfterLoopRoundTrip(t *testing.T) {
	ctx := context.Background()

	sequences := map[string][]escalationPathNode{
		"main":   {branchWithID("split", "urgent", "")},
		"urgent": {levelNode(t, ""), loopNode("split", 2), reassignmentNode("01OTHER")},
	}

	var diags diag.Diagnostics
	validateSequences(ctx, betaModel(t, "main", sequences), &diags)
	if diags.HasError() {
		t.Fatalf("validateSequences rejected it: %+v", diags)
	}

	payload := unflattenSequences(ctx, "main", sequences, &diags)
	if diags.HasError() {
		t.Fatalf("unflattenSequences produced errors: %+v", diags)
	}

	then := payload[0].IfElse.ThenPath
	gotTypes := lo.Map(then, func(node client.EscalationPathNodePayloadV2, _ int) client.EscalationPathNodePayloadV2Type {
		return node.Type
	})
	wantTypes := []client.EscalationPathNodePayloadV2Type{
		client.EscalationPathNodePayloadV2TypeLevel,
		client.EscalationPathNodePayloadV2TypeRepeat,
		client.EscalationPathNodePayloadV2TypeEscalationPath,
	}
	if !slices.Equal(gotTypes, wantTypes) {
		t.Fatalf("got then path %v, want %v", gotTypes, wantTypes)
	}
	if got := then[1].Repeat.ToNode; got != "split" {
		t.Errorf("got repeat to_node %q, want split", got)
	}
	if got := then[2].EscalationPath.EscalationPathId; got != "01OTHER" {
		t.Errorf("got escalation_path_id %q, want 01OTHER", got)
	}

	var readDiags diag.Diagnostics
	_, got := flattenSequences(ctx, apiNodes(payload), priorNames("main", sequences), &readDiags)
	if readDiags.HasError() {
		t.Fatalf("flattenSequences produced errors: %+v", readDiags)
	}

	urgent := got["urgent"]
	gotBlocks := lo.FlatMap(urgent, func(node escalationPathNode, _ int) []string { return node.blockNames() })
	if want := []string{"level", "loop", "escalation_path"}; !slices.Equal(gotBlocks, want) {
		t.Fatalf("got urgent %v, want %v", gotBlocks, want)
	}
	if got := urgent[1].Loop.BackTo.ValueString(); got != "split" {
		t.Errorf("got loop back_to %q, want split", got)
	}
	if got := urgent[2].EscalationPath.EscalationPathID.ValueString(); got != "01OTHER" {
		t.Errorf("got escalation_path_id %q, want 01OTHER", got)
	}
	// The reassignment's id was derived from its position, so it must read back unset to
	// match a config that never wrote one.
	if !urgent[2].ID.IsNull() {
		t.Errorf("got reassignment id %q, want it left unset", urgent[2].ID.ValueString())
	}
}
