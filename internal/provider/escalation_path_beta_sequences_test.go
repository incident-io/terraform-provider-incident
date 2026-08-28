package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// levelNode builds a level node paging a single schedule, which is the least interesting
// node these tests can hang a sequence off.
func levelNode(t *testing.T, id string) escalationPathBetaNode {
	t.Helper()

	var diags diag.Diagnostics
	targets := targetsFromAPI(context.Background(), []client.EscalationPathTargetV2{{
		Id:      "01ABC",
		Type:    client.EscalationPathTargetV2TypeSchedule,
		Urgency: client.EscalationPathTargetV2UrgencyHigh,
	}}, &diags)
	if diags.HasError() {
		t.Fatalf("building targets: %+v", diags)
	}

	nodeID := types.StringNull()
	if id != "" {
		nodeID = types.StringValue(id)
	}
	return escalationPathBetaNode{
		ID:    nodeID,
		Level: &IncidentEscalationPathNodeLevel{Targets: targets},
	}
}

func branchNode(then, els string) escalationPathBetaNode {
	elseKey := types.StringNull()
	if els != "" {
		elseKey = types.StringValue(els)
	}
	return escalationPathBetaNode{
		ID: types.StringNull(),
		Branch: &escalationPathBetaBranch{
			If:   workingHoursIf(),
			Then: types.StringValue(then),
			Else: elseKey,
		},
	}
}

func TestUnflattenSequences(t *testing.T) {
	ctx := context.Background()

	t.Run("derives an id from the node's position", func(t *testing.T) {
		var diags diag.Diagnostics
		path := unflattenSequences(ctx, "main", map[string][]escalationPathBetaNode{
			"main": {levelNode(t, ""), levelNode(t, "")},
		}, &diags)

		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if got := lo.Map(path, func(n client.EscalationPathNodePayloadV2, _ int) string { return n.Id }); got[0] != "main/0" || got[1] != "main/1" {
			t.Errorf("got ids %v, want [main/0 main/1]", got)
		}
	})

	t.Run("keeps an id the author wrote", func(t *testing.T) {
		var diags diag.Diagnostics
		path := unflattenSequences(ctx, "main", map[string][]escalationPathBetaNode{
			"main": {levelNode(t, "page-eng")},
		}, &diags)

		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if path[0].Id != "page-eng" {
			t.Errorf("got id %q, want page-eng", path[0].Id)
		}
	})

	t.Run("inlines the sequences a branch names", func(t *testing.T) {
		var diags diag.Diagnostics
		path := unflattenSequences(ctx, "main", map[string][]escalationPathBetaNode{
			"main":   {branchNode("urgent", "quiet")},
			"urgent": {levelNode(t, "")},
			"quiet":  {levelNode(t, ""), levelNode(t, "")},
		}, &diags)

		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if len(path) != 1 || path[0].IfElse == nil {
			t.Fatalf("got %d nodes, want one branch", len(path))
		}
		if got := len(path[0].IfElse.ThenPath); got != 1 {
			t.Errorf("got %d then nodes, want 1", got)
		}
		if got := len(path[0].IfElse.ElsePath); got != 2 {
			t.Errorf("got %d else nodes, want 2", got)
		}
		if got := path[0].IfElse.ThenPath[0].Id; got != "urgent/0" {
			t.Errorf("got then node id %q, want urgent/0", got)
		}
	})

	t.Run("leaves the else path empty when the branch has no else", func(t *testing.T) {
		var diags diag.Diagnostics
		path := unflattenSequences(ctx, "main", map[string][]escalationPathBetaNode{
			"main":   {branchNode("urgent", "")},
			"urgent": {levelNode(t, "")},
		}, &diags)

		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if got := len(path[0].IfElse.ElsePath); got != 0 {
			t.Errorf("got %d else nodes, want 0", got)
		}
	})

	t.Run("reports a sequence that doesn't exist", func(t *testing.T) {
		var diags diag.Diagnostics
		unflattenSequences(ctx, "main", map[string][]escalationPathBetaNode{
			"main": {branchNode("nowhere", "")},
		}, &diags)

		if !diags.HasError() {
			t.Fatal("expected an error")
		}
	})

	t.Run("reports a cycle rather than recursing forever", func(t *testing.T) {
		var diags diag.Diagnostics
		unflattenSequences(ctx, "main", map[string][]escalationPathBetaNode{
			"main": {branchNode("loop", "")},
			"loop": {branchNode("main", "")},
		}, &diags)

		if !diags.HasError() {
			t.Fatal("expected an error")
		}
	})

	t.Run("reports a sequence two branches name rather than copying it", func(t *testing.T) {
		var diags diag.Diagnostics
		unflattenSequences(ctx, "main", map[string][]escalationPathBetaNode{
			"main":   {branchNode("urgent", "quiet")},
			"urgent": {branchNode("shared", "")},
			"quiet":  {branchNode("shared", "")},
			"shared": {levelNode(t, "")},
		}, &diags)

		if !diags.HasError() {
			t.Fatal("expected an error")
		}
	})
}

// apiNodes converts a payload tree into the response tree, which is the same shape with a
// different Go type. It lets a test build its input by unflattening, so the round-trip
// cases don't hand-write both representations.
func apiNodes(payload []client.EscalationPathNodePayloadV2) []client.EscalationPathNodeV2 {
	return lo.Map(payload, func(node client.EscalationPathNodePayloadV2, _ int) client.EscalationPathNodeV2 {
		out := client.EscalationPathNodeV2{
			Id:             node.Id,
			Type:           client.EscalationPathNodeV2Type(node.Type),
			Level:          node.Level,
			NotifyChannel:  node.NotifyChannel,
			Delay:          node.Delay,
			EscalationPath: node.EscalationPath,
			Repeat:         node.Repeat,
		}
		if node.IfElse != nil {
			out.IfElse = &client.EscalationPathNodeIfElseV2{
				Conditions: apiConditions(lo.FromPtr(node.IfElse.Conditions)),
				ThenPath:   apiNodes(node.IfElse.ThenPath),
				ElsePath:   apiNodes(node.IfElse.ElsePath),
			}
		}
		return out
	})
}

// priorNames is the naming a read starts from, as the plan or state carries it.
func priorNames(start string, sequences map[string][]escalationPathBetaNode) escalationPathBetaPriorNames {
	return escalationPathBetaPriorNames{start: start, sequences: sequences}
}

func TestFlattenSequences(t *testing.T) {
	ctx := context.Background()

	t.Run("recovers the keys a round trip started with", func(t *testing.T) {
		want := map[string][]escalationPathBetaNode{
			"main":   {branchNode("urgent", "quiet")},
			"urgent": {levelNode(t, "")},
			"quiet":  {levelNode(t, "")},
		}

		var diags diag.Diagnostics
		start, got := flattenSequences(ctx, apiNodes(unflattenSequences(ctx, "main", want, &diags)), priorNames("main", want), &diags)
		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}

		if start != "main" {
			t.Errorf("got start %q, want main", start)
		}
		for _, key := range []string{"main", "urgent", "quiet"} {
			if _, ok := got[key]; !ok {
				t.Errorf("missing sequence %q, got %v", key, lo.Keys(got))
			}
		}
		if branch := got["main"][0].Branch; branch == nil {
			t.Fatal("main lost its branch")
		} else if branch.Then.ValueString() != "urgent" || branch.Else.ValueString() != "quiet" {
			t.Errorf("got then=%q else=%q, want urgent/quiet", branch.Then.ValueString(), branch.Else.ValueString())
		}
	})

	t.Run("drops the ids it derived and keeps the ones the author wrote", func(t *testing.T) {
		var diags diag.Diagnostics
		sequences := map[string][]escalationPathBetaNode{
			"main": {levelNode(t, "page-eng"), levelNode(t, "")},
		}
		_, got := flattenSequences(ctx, apiNodes(unflattenSequences(ctx, "main", sequences, &diags)), priorNames("main", sequences), &diags)
		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}

		if id := got["main"][0].ID; id.ValueString() != "page-eng" {
			t.Errorf("got id %q, want page-eng", id.ValueString())
		}
		if id := got["main"][1].ID; !id.IsNull() {
			t.Errorf("got id %q, want null", id.ValueString())
		}
	})

	t.Run("names sequences after the branch that reached them", func(t *testing.T) {
		// An import: no prior config to take names from, and a path built anywhere but
		// this resource carries no keys in its node ids either.
		nodes := []client.EscalationPathNodeV2{{
			Id:   "01AAA",
			Type: client.EscalationPathNodeV2TypeIfElse,
			IfElse: &client.EscalationPathNodeIfElseV2{
				Conditions: workingHoursConditions("UK"),
				ThenPath:   []client.EscalationPathNodeV2{{Id: "01BBB", Type: client.EscalationPathNodeV2TypeDelay, Delay: &client.EscalationPathNodeDelayV2{}}},
				ElsePath:   []client.EscalationPathNodeV2{{Id: "01CCC", Type: client.EscalationPathNodeV2TypeDelay, Delay: &client.EscalationPathNodeDelayV2{}}},
			},
		}}

		var diags diag.Diagnostics
		start, got := flattenSequences(ctx, nodes, escalationPathBetaPriorNames{}, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}

		if start != "main" {
			t.Errorf("got start %q, want main", start)
		}
		for _, key := range []string{"main", "main_then", "main_else"} {
			if _, ok := got[key]; !ok {
				t.Errorf("missing sequence %q, got %v", key, lo.Keys(got))
			}
		}
	})

	// The API rejects a branch that isn't last in its node list, so it shouldn't hand us one.
	// Reporting it beats splitting the trailing nodes into a sequence nothing reaches.
	t.Run("reports a path with nodes after a branch, which the API shouldn't return", func(t *testing.T) {
		nodes := []client.EscalationPathNodeV2{
			{
				Id:   "01AAA",
				Type: client.EscalationPathNodeV2TypeIfElse,
				IfElse: &client.EscalationPathNodeIfElseV2{
					Conditions: workingHoursConditions("UK"),
					ThenPath:   []client.EscalationPathNodeV2{{Id: "01BBB", Type: client.EscalationPathNodeV2TypeDelay, Delay: &client.EscalationPathNodeDelayV2{}}},
				},
			},
			{Id: "01CCC", Type: client.EscalationPathNodeV2TypeDelay, Delay: &client.EscalationPathNodeDelayV2{}},
		}

		var diags diag.Diagnostics
		flattenSequences(ctx, nodes, escalationPathBetaPriorNames{}, &diags)

		if !diags.HasError() {
			t.Fatal("expected an error")
		}
	})

	// These are the cases where the config we last wrote is the only record of the naming.
	t.Run("keeps the author's names when every node has an id", func(t *testing.T) {
		// The documented shape: a sequence whose only node is a branch, named so a loop can
		// point back at it. Nothing is left to derive a key from.
		want := map[string][]escalationPathBetaNode{
			"entry":  {branchWithID("start", "urgent", "quiet")},
			"urgent": {levelNode(t, "page-eng"), loopNode("start", 3)},
			"quiet":  {levelNode(t, "page-ops")},
		}

		var diags diag.Diagnostics
		start, got := flattenSequences(ctx, apiNodes(unflattenSequences(ctx, "entry", want, &diags)), priorNames("entry", want), &diags)
		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}

		if start != "entry" {
			t.Errorf("got start %q, want entry", start)
		}
		for _, key := range []string{"entry", "urgent", "quiet"} {
			if _, ok := got[key]; !ok {
				t.Errorf("missing sequence %q, got %v", key, lo.Keys(got))
			}
		}
		if branch := got["entry"][0].Branch; branch == nil {
			t.Fatal("entry lost its branch")
		} else if branch.Then.ValueString() != "urgent" || branch.Else.ValueString() != "quiet" {
			t.Errorf("got then=%q else=%q, want urgent/quiet", branch.Then.ValueString(), branch.Else.ValueString())
		}
	})

	t.Run("falls back to a derived key when the prior naming has moved on", func(t *testing.T) {
		// A branch added in the dashboard has no counterpart in the config, so its children
		// can only take the names we derive.
		sequences := map[string][]escalationPathBetaNode{
			"entry": {branchNode("urgent", "")},
			"urgent": {
				levelNode(t, ""),
			},
		}

		var diags diag.Diagnostics
		nodes := apiNodes(unflattenSequences(ctx, "entry", sequences, &diags))

		// The prior config knows "entry", but nothing about a branch inside it.
		prior := priorNames("entry", map[string][]escalationPathBetaNode{
			"entry": {levelNode(t, "")},
		})

		start, got := flattenSequences(ctx, nodes, prior, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}

		if start != "entry" {
			t.Errorf("got start %q, want entry", start)
		}
		// "urgent" survives here because its own node ids carry it; the point is that a
		// prior naming which doesn't describe this tree doesn't derail the read.
		if _, ok := got["entry"]; !ok {
			t.Errorf("missing sequence entry, got %v", lo.Keys(got))
		}
	})

	t.Run("takes the naming from a model, and manages without one", func(t *testing.T) {
		sequences := map[string][]escalationPathBetaNode{
			"entry":  {branchWithID("start", "urgent", "")},
			"urgent": {levelNode(t, "page-eng")},
		}

		var diags diag.Diagnostics
		nodes := apiNodes(unflattenSequences(ctx, "entry", sequences, &diags))

		start, _ := flattenSequences(ctx, nodes, escalationPathBetaPriorNamesFrom(ctx, betaModel(t, "entry", sequences)), &diags)
		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if start != "entry" {
			t.Errorf("got start %q from a model, want entry", start)
		}

		// An import has no prior model at all.
		start, _ = flattenSequences(ctx, nodes, escalationPathBetaPriorNamesFrom(ctx, nil), &diags)
		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if start != rootSequenceKey {
			t.Errorf("got start %q with no prior model, want %s", start, rootSequenceKey)
		}
	})
}
