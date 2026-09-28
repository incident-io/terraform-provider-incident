package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"

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

// TestEscalationPathLoopThenReassignmentRoundTrip covers a loop followed by a
// reassignment: the reassignment runs once the loop has run out of repeats, so it must
// reach the API after the repeat and read back into the same sequence.
func TestEscalationPathLoopThenReassignmentRoundTrip(t *testing.T) {
	ctx := context.Background()

	sequences := map[string][]escalationPathNode{
		"main": {levelNode(t, "page-eng"), loopNode("page-eng", 3), reassignmentNode("01FALLBACK")},
	}

	var diags diag.Diagnostics
	payload := unflattenSequences(ctx, "main", sequences, &diags)
	if diags.HasError() {
		t.Fatalf("unflattenSequences produced errors: %+v", diags)
	}

	wantTypes := []client.EscalationPathNodePayloadV2Type{
		client.EscalationPathNodePayloadV2TypeLevel,
		client.EscalationPathNodePayloadV2TypeRepeat,
		client.EscalationPathNodePayloadV2TypeEscalationPath,
	}
	if len(payload) != len(wantTypes) {
		t.Fatalf("got %d payload nodes, want %d", len(payload), len(wantTypes))
	}
	for i, want := range wantTypes {
		if got := payload[i].Type; got != want {
			t.Errorf("payload node %d: got type %q, want %q", i, got, want)
		}
	}
	if payload[2].EscalationPath == nil || payload[2].EscalationPath.EscalationPathId != "01FALLBACK" {
		t.Fatalf("reassignment lost its escalation_path_id: %+v", payload[2].EscalationPath)
	}

	var readDiags diag.Diagnostics
	_, got := flattenSequences(ctx, apiNodes(payload), priorNames("main", sequences), &readDiags)
	if readDiags.HasError() {
		t.Fatalf("flattenSequences produced errors: %+v", readDiags)
	}
	nodes := got["main"]
	if len(nodes) != 3 {
		t.Fatalf("got %d nodes in main, want 3", len(nodes))
	}
	if nodes[1].Loop == nil {
		t.Error("main's second node lost its loop block")
	} else if backTo := nodes[1].Loop.BackTo.ValueString(); backTo != "page-eng" {
		t.Errorf("got loop back_to %q, want page-eng", backTo)
	}
	if nodes[2].EscalationPath == nil {
		t.Fatal("main's last node lost its escalation_path block")
	}
	if id := nodes[2].EscalationPath.EscalationPathID.ValueString(); id != "01FALLBACK" {
		t.Errorf("got escalation_path_id %q, want 01FALLBACK", id)
	}
}
