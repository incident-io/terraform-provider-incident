package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// reassignmentNode builds an API escalation_path node reassigning to the given path.
func reassignmentNode(id, targetPathID string) client.EscalationPathNodeV2 {
	return client.EscalationPathNodeV2{
		Id:   id,
		Type: client.EscalationPathNodeV2TypeEscalationPath,
		EscalationPath: &client.EscalationPathNodeEscalationPathV2{
			EscalationPathId: targetPathID,
		},
	}
}

// TestEscalationPathReassignmentRoundTrip is the test this node type exists for: before
// the provider modelled escalation_path, a read returned a node with every sub-object nil
// and the next apply sent "type: escalation_path" with no config, which the API rejects.
// It covers the top level and both branches of an if_else, since a branch's nodes convert
// through the same recursive functions at a lower depth.
func TestEscalationPathReassignmentRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := &IncidentEscalationPathResource{}

	ep := client.EscalationPathV2{
		Id:   "ep-1",
		Name: "Reassigning path",
		Path: []client.EscalationPathNodeV2{
			reassignmentNode("node-1", "01TARGET"),
			{
				Id:   "node-2",
				Type: client.EscalationPathNodeV2TypeIfElse,
				IfElse: &client.EscalationPathNodeIfElseV2{
					Conditions: []client.ConditionV2{
						{
							Subject:   client.ConditionSubjectV2{Reference: "incident.severity"},
							Operation: client.ConditionOperationV2{Value: "is"},
						},
					},
					ThenPath: []client.EscalationPathNodeV2{reassignmentNode("then-1", "01THEN")},
					ElsePath: []client.EscalationPathNodeV2{reassignmentNode("else-1", "01ELSE")},
				},
			},
		},
	}

	var diags diag.Diagnostics
	model := r.buildModel(ctx, ep, nil, &diags)
	if diags.HasError() {
		t.Fatalf("buildModel produced errors: %#v", diags)
	}

	var payloadDiags diag.Diagnostics
	payload := r.toPathPayload(ctx, model.Path, &payloadDiags)
	if payloadDiags.HasError() {
		t.Fatalf("toPathPayload produced errors: %#v", payloadDiags)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 payload nodes, got %d", len(payload))
	}

	top := payload[0]
	if top.Type != client.EscalationPathNodePayloadV2TypeEscalationPath {
		t.Errorf("got type %q, want escalation_path", top.Type)
	}
	if top.EscalationPath == nil {
		t.Fatal("top-level reassignment node lost its escalation_path block")
	}
	if got := top.EscalationPath.EscalationPathId; got != "01TARGET" {
		t.Errorf("got escalation_path_id %q, want 01TARGET", got)
	}

	branch := payload[1].IfElse
	if branch == nil {
		t.Fatal("expected the second node to keep its if_else payload")
	}
	for _, tc := range []struct {
		name  string
		nodes []client.EscalationPathNodePayloadV2
		want  string
	}{
		{name: "then_path", nodes: branch.ThenPath, want: "01THEN"},
		{name: "else_path", nodes: branch.ElsePath, want: "01ELSE"},
	} {
		if len(tc.nodes) != 1 {
			t.Fatalf("%s: expected 1 node, got %d", tc.name, len(tc.nodes))
		}
		node := tc.nodes[0]
		if node.Type != client.EscalationPathNodePayloadV2TypeEscalationPath {
			t.Errorf("%s: got type %q, want escalation_path", tc.name, node.Type)
		}
		if node.EscalationPath == nil {
			t.Fatalf("%s: reassignment node lost its escalation_path block", tc.name)
		}
		if got := node.EscalationPath.EscalationPathId; got != tc.want {
			t.Errorf("%s: got escalation_path_id %q, want %s", tc.name, got, tc.want)
		}
	}
}

// TestValidateEscalationPathNodeEscalationPath covers the plan-time check that a node's
// type and its escalation_path block agree. Nothing else catches a mismatch: the type
// attribute has no enum validator, and every block is optional.
func TestValidateEscalationPathNodeEscalationPath(t *testing.T) {
	reassignment := &IncidentEscalationPathNodeEscalationPath{
		EscalationPathID: types.StringValue("01TARGET"),
	}

	for _, tc := range []struct {
		name      string
		node      IncidentEscalationPathNode
		wantError string
	}{
		{
			name: "a reassignment node with its block",
			node: IncidentEscalationPathNode{
				Type:           types.StringValue("escalation_path"),
				EscalationPath: reassignment,
			},
		},
		{
			name: "another node type with no block",
			node: IncidentEscalationPathNode{Type: types.StringValue("delay")},
		},
		{
			name:      "a reassignment node with no block",
			node:      IncidentEscalationPathNode{Type: types.StringValue("escalation_path")},
			wantError: "must set an escalation_path block",
		},
		{
			name: "the block on another node type",
			node: IncidentEscalationPathNode{
				Type:           types.StringValue("delay"),
				EscalationPath: reassignment,
			},
			wantError: "must not set an escalation_path block",
		},
		{
			// A type that isn't settled yet can't be checked, and guessing would fail a
			// plan that the apply would have accepted.
			name: "an unknown type",
			node: IncidentEscalationPathNode{
				Type:           types.StringUnknown(),
				EscalationPath: reassignment,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateEscalationPathNodeEscalationPath(tc.node, &diags)

			if tc.wantError == "" {
				if diags.HasError() {
					t.Fatalf("unexpected errors: %+v", diags)
				}
				return
			}

			if !diags.HasError() {
				t.Fatal("expected an error, got none")
			}
			for _, d := range diags.Errors() {
				if strings.Contains(d.Detail(), tc.wantError) {
					return
				}
			}
			t.Errorf("no error mentioned %q, got %+v", tc.wantError, diags)
		})
	}
}

// TestEscalationPathBetaReassignmentRoundTrip covers the same node in
// incident_escalation_path_beta, whose flat sequences take the place of nested branches.
// Before this the resource refused the node outright on read.
func TestEscalationPathBetaReassignmentRoundTrip(t *testing.T) {
	ctx := context.Background()

	reassignment := func(targetPathID string) escalationPathBetaNode {
		return escalationPathBetaNode{
			ID: types.StringNull(),
			EscalationPath: &IncidentEscalationPathNodeEscalationPath{
				EscalationPathID: types.StringValue(targetPathID),
			},
		}
	}

	// A branch puts the reassignment in a sequence the branch names, which is how a
	// nested reassignment reaches the API.
	sequences := map[string][]escalationPathBetaNode{
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
