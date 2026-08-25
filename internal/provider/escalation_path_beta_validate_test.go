package provider

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// betaModel builds the model validateSequences reads, going through the same map
// construction the resource uses to write state.
func betaModel(t *testing.T, start string, sequences map[string][]escalationPathBetaNode) *escalationPathBetaModel {
	t.Helper()

	var diags diag.Diagnostics
	value := escalationPathBetaSequencesToMap(context.Background(), sequences, &diags)
	if diags.HasError() {
		t.Fatalf("building sequences map: %+v", diags)
	}

	return &escalationPathBetaModel{
		Start:     types.StringValue(start),
		Sequences: value,
	}
}

func branchWithID(id, then, els string) escalationPathBetaNode {
	node := branchNode(then, els)
	node.ID = types.StringValue(id)
	return node
}

func loopNode(backTo string, times int64) escalationPathBetaNode {
	return escalationPathBetaNode{
		ID: types.StringNull(),
		Loop: &escalationPathBetaLoop{
			BackTo: types.StringValue(backTo),
			Times:  types.Int64Value(times),
		},
	}
}

// twoBlockNode is a node claiming to be both a level and a loop.
func twoBlockNode(t *testing.T) escalationPathBetaNode {
	node := levelNode(t, "page-eng")
	node.Loop = &escalationPathBetaLoop{
		BackTo: types.StringValue("page-eng"),
		Times:  types.Int64Value(3),
	}
	return node
}

// unknownBranchNode is a branch naming a sequence another resource hasn't computed yet.
func unknownBranchNode() escalationPathBetaNode {
	node := branchNode("urgent", "")
	node.Branch.Then = types.StringUnknown()
	return node
}

// unknownLoopNode is a loop whose target another resource hasn't computed yet.
func unknownLoopNode() escalationPathBetaNode {
	node := loopNode("page-eng", 3)
	node.Loop.BackTo = types.StringUnknown()
	return node
}

func TestValidateSequences(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		start     string
		sequences map[string][]escalationPathBetaNode
		wantError string
	}{
		{
			name:  "a tree with a branch and a loop back to the first node",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {levelNode(t, "page-eng"), branchNode("urgent", "quiet")},
				"urgent": {levelNode(t, ""), loopNode("page-eng", 3)},
				"quiet":  {levelNode(t, "")},
			},
		},
		{
			name:  "a sequence continues after a loop",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {levelNode(t, "page-eng"), loopNode("page-eng", 3), levelNode(t, "")},
			},
			wantError: "continues after a loop node",
		},
		{
			// then is required, so an empty one is a name the author wrote, not a side
			// they left off.
			name:  "a branch names the empty sequence",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {branchNode("", "urgent")},
				"urgent": {levelNode(t, "")},
			},
			wantError: "There is no sequence named \"\"",
		},
		{
			name:  "a loop back to a node we derived an id for",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {levelNode(t, ""), loopNode("main/0", 2)},
			},
		},
		{
			name:  "a loop back to the branch it sits under",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {branchWithID("split", "urgent", "")},
				"urgent": {levelNode(t, ""), loopNode("split", 2)},
			},
		},
		{
			name:  "a loop back to a node it doesn't sit under",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {branchWithID("split", "urgent", "quiet")},
				"urgent": {levelNode(t, "page-eng")},
				"quiet":  {levelNode(t, ""), loopNode("page-eng", 2)},
			},
			wantError: "may only go back to the escalation path's first node",
		},
		{
			name:  "a loop back to a level in its own sequence that isn't the first node",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {branchWithID("split", "urgent", "")},
				"urgent": {levelNode(t, "page-eng"), loopNode("page-eng", 2)},
			},
			wantError: "may only go back to the escalation path's first node",
		},
		{
			name:      "start names a sequence that doesn't exist",
			start:     "nowhere",
			sequences: map[string][]escalationPathBetaNode{"main": {levelNode(t, "")}},
			wantError: "start names \"nowhere\"",
		},
		{
			name:  "a branch names a sequence that doesn't exist",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {branchNode("nowhere", "")},
			},
			wantError: "There is no sequence named \"nowhere\"",
		},
		{
			name:  "a loop names a node that doesn't exist",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {levelNode(t, ""), loopNode("nowhere", 2)},
			},
			wantError: "There is no node with id \"nowhere\"",
		},
		{
			name:  "a sequence continues after a branch",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {branchNode("urgent", ""), levelNode(t, "")},
				"urgent": {levelNode(t, "")},
			},
			wantError: "continues after a branch node",
		},
		{
			name:  "nothing branches to a sequence",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":     {levelNode(t, "")},
				"stranded": {levelNode(t, "")},
			},
			wantError: "Nothing branches to \"stranded\"",
		},
		{
			name:  "a sequence has no nodes",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {branchNode("urgent", "")},
				"urgent": {},
			},
			wantError: "Sequence \"urgent\" has no nodes",
		},
		{
			name:  "two nodes share an id",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {levelNode(t, "page-eng"), levelNode(t, "page-eng")},
			},
			wantError: "More than one node is called \"page-eng\"",
		},
		{
			name:  "a node id would collide with one we derive",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {levelNode(t, "other/0")},
			},
			wantError: "must not contain \"/\"",
		},
		{
			name:  "a sequence name isn't usable",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":     {branchNode("2 urgent", "")},
				"2 urgent": {levelNode(t, "")},
			},
			wantError: "must start with a letter",
		},
		{
			name:  "two branches name the same sequence",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {branchNode("urgent", "quiet")},
				"urgent": {branchNode("shared", "")},
				"quiet":  {branchNode("shared", "")},
				"shared": {levelNode(t, "")},
			},
			wantError: "branches name \"shared\"",
		},
		{
			name:  "a branch goes back to the start",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {branchNode("urgent", "")},
				"urgent": {branchNode("main", "")},
			},
			wantError: "nothing may branch to it",
		},
		{
			name:  "a node sets two type blocks",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {twoBlockNode(t)},
			},
			wantError: "sets level and loop",
		},
		{
			// What a node says about itself doesn't depend on any name resolving, so an
			// unknown reference elsewhere mustn't stop us checking it. unflattenSequences
			// matches one block and ignores the rest, so missing this drops one on apply.
			name:  "a node sets two type blocks while another sequence isn't computed yet",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {unknownBranchNode()},
				"urgent": {twoBlockNode(t)},
			},
			wantError: "sets level and loop",
		},
		{
			name:  "a node sets none of them",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {{ID: types.StringValue("does-nothing")}},
			},
			wantError: "sets none of level, notify_channel, delay, branch or loop",
		},
		{
			// The naming can't be checked until the resource it comes from exists, and
			// checking around the gap reports a stranded sequence that an apply won't hit.
			name:  "a branch names a sequence that isn't computed yet",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":   {unknownBranchNode()},
				"urgent": {levelNode(t, "")},
			},
		},
		{
			name:  "a loop names a node that isn't computed yet",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main": {levelNode(t, "page-eng"), unknownLoopNode()},
			},
		},
		{
			// The shape the documented example uses: the root is a named branch, and the
			// sequence below it loops back to that branch, which is the only node in the
			// path a loop is allowed to name.
			name:  "a loop back to the branch it sits under",
			start: "main",
			sequences: map[string][]escalationPathBetaNode{
				"main":         {branchWithID("start", "in_hours", "out_of_hours")},
				"in_hours":     {levelNode(t, ""), loopNode("start", 3)},
				"out_of_hours": {levelNode(t, "")},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateSequences(ctx, betaModel(t, tc.start, tc.sequences), &diags)

			if tc.wantError == "" {
				if diags.HasError() {
					t.Fatalf("expected no errors, got %+v", diags)
				}
				return
			}

			if !diags.HasError() {
				t.Fatal("expected an error")
			}
			for _, d := range diags.Errors() {
				if strings.Contains(d.Detail(), tc.wantError) {
					return
				}
			}
			t.Errorf("no error mentioned %q, got %+v", tc.wantError, diags.Errors())
		})
	}
}

// TestValidateSequencesSkipsUnknownNodes covers a sequence whose whole nodes list is a value
// another resource computes — `nodes = local.from_a_resource`. decodeSequences reads an
// unknown list as no nodes at all, so without a guard every check below it sees an empty
// sequence and an escalation path that reaches nothing.
func TestValidateSequencesSkipsUnknownNodes(t *testing.T) {
	ctx := context.Background()

	nodeType := types.ObjectType{AttrTypes: escalationPathBetaNodeAttrTypes()}
	sequenceType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"nodes": types.ListType{ElemType: nodeType},
	}}

	sequences, d := types.MapValue(sequenceType, map[string]attr.Value{
		"main": types.ObjectValueMust(sequenceType.AttrTypes, map[string]attr.Value{
			"nodes": types.ListUnknown(nodeType),
		}),
	})
	if d.HasError() {
		t.Fatalf("building sequences map: %+v", d)
	}

	var diags diag.Diagnostics
	validateSequences(ctx, &escalationPathBetaModel{
		Start:     types.StringValue("main"),
		Sequences: sequences,
	}, &diags)

	if diags.HasError() {
		t.Fatalf("expected no errors for a sequence whose nodes aren't computed yet, got %+v", diags.Errors())
	}
}

// escalationPathBetaPlanType is the resource's schema as a tftypes.Object, so the plans
// these tests build are the ones Terraform would build.
func escalationPathBetaPlanType(t *testing.T) tftypes.Object {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewEscalationPathBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	return objType
}

func TestEscalationPathBetaPlanSettled(t *testing.T) {
	objType := escalationPathBetaPlanType(t)

	// A plan holding nothing but nulls, which each case then makes unknown in one place.
	plan := func(overrides map[string]tftypes.Value) tftypes.Value {
		return nullObject(t, objType, overrides)
	}

	t.Run("an id the API will mint doesn't stop the check", func(t *testing.T) {
		settled := escalationPathBetaPlanSettled(plan(map[string]tftypes.Value{
			"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}))
		if !settled {
			t.Error("got settled=false, want true")
		}
	})

	t.Run("a name another resource computes does", func(t *testing.T) {
		settled := escalationPathBetaPlanSettled(plan(map[string]tftypes.Value{
			"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}))
		if settled {
			t.Error("got settled=true, want false")
		}
	})

	t.Run("so does a sequence waiting on one", func(t *testing.T) {
		settled := escalationPathBetaPlanSettled(plan(map[string]tftypes.Value{
			"sequences": tftypes.NewValue(objType.AttributeTypes["sequences"], tftypes.UnknownValue),
		}))
		if settled {
			t.Error("got settled=true, want false")
		}
	})

	// The nested half of the rule: schedule_mode is the API filling a value in, a target
	// id is another resource we're waiting on.
	targetPlan := func(t *testing.T, override map[string]tftypes.Value) tftypes.Value {
		t.Helper()

		sequencesType := tftypeAs[tftypes.Map](t, objType.AttributeTypes["sequences"])
		sequenceType := tftypeAs[tftypes.Object](t, sequencesType.ElementType)
		nodesType := tftypeAs[tftypes.List](t, sequenceType.AttributeTypes["nodes"])
		nodeType := tftypeAs[tftypes.Object](t, nodesType.ElementType)
		levelType := tftypeAs[tftypes.Object](t, nodeType.AttributeTypes["level"])
		targetsType := tftypeAs[tftypes.List](t, levelType.AttributeTypes["targets"])
		targetType := tftypeAs[tftypes.Object](t, targetsType.ElementType)

		level := nullObject(t, levelType, map[string]tftypes.Value{
			"targets": tftypes.NewValue(targetsType, []tftypes.Value{nullObject(t, targetType, override)}),
		})
		sequence := nullObject(t, sequenceType, map[string]tftypes.Value{
			"nodes": tftypes.NewValue(nodesType, []tftypes.Value{
				nullObject(t, nodeType, map[string]tftypes.Value{"level": level}),
			}),
		})
		return plan(map[string]tftypes.Value{
			"sequences": tftypes.NewValue(sequencesType, map[string]tftypes.Value{"main": sequence}),
		})
	}

	t.Run("a schedule_mode the API will derive doesn't stop the check", func(t *testing.T) {
		settled := escalationPathBetaPlanSettled(targetPlan(t, map[string]tftypes.Value{
			"schedule_mode": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}))
		if !settled {
			t.Error("got settled=false, want true")
		}
	})

	t.Run("a target id another resource computes does", func(t *testing.T) {
		settled := escalationPathBetaPlanSettled(targetPlan(t, map[string]tftypes.Value{
			"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}))
		if settled {
			t.Error("got settled=true, want false")
		}
	})
}

func tftypeAs[T tftypes.Type](t *testing.T, in tftypes.Type) T {
	t.Helper()

	out, ok := in.(T)
	if !ok {
		t.Fatalf("expected %T, got %T", out, in)
	}
	return out
}

// nullObject builds an object whose attributes are all null bar the overrides, so a case
// only spells out the value it cares about.
func nullObject(t *testing.T, objType tftypes.Object, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	values := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	maps.Copy(values, overrides)
	return tftypes.NewValue(objType, values)
}
