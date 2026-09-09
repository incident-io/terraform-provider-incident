package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// TestEscalationPathReassignmentRoundTrip covers the same node in
// incident_escalation_path, whose flat sequences take the place of nested branches.
// Before this the resource refused the node outright on read.
func TestEscalationPathReassignmentRoundTrip(t *testing.T) {
	ctx := context.Background()

	reassignment := func(targetPathID string) escalationPathNode {
		return escalationPathNode{
			ID: types.StringNull(),
			EscalationPath: &IncidentEscalationPathNodeEscalationPath{
				EscalationPathID: types.StringValue(targetPathID),
			},
		}
	}

	// A branch puts the reassignment in a sequence the branch names, which is how a
	// nested reassignment reaches the API.
	sequences := map[string][]escalationPathNode{
		"main":   {branchNode("urgent", "quiet")},
		"urgent": {reassignment("01URGENT")},
		"quiet":  {levelNode(t, ""), reassignment("01QUIET")},
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
