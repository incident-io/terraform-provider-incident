package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// TestEscalationPathRoundTrip exercises the types.List-based model conversions
// end to end: API -> model (toPathModel via buildModel) -> payload
// (toPathPayload). It deliberately includes a nested if_else node so we cover
// the recursive nodeAttrTypes path, including the deepest level where the
// if_else attribute is absent from the schema.
func TestEscalationPathRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := &IncidentEscalationPathResource{}

	level := func(id string) client.EscalationPathNodeV2 {
		return client.EscalationPathNodeV2{
			Id:   id,
			Type: client.EscalationPathNodeV2TypeLevel,
			Level: &client.EscalationPathNodeLevelV2{
				Targets: []client.EscalationPathTargetV2{
					{
						Id:      "schedule-1",
						Type:    client.EscalationPathTargetV2TypeSchedule,
						Urgency: client.EscalationPathTargetV2UrgencyHigh,
					},
				},
				TimeToAckSeconds: lo.ToPtr(int64(300)),
			},
		}
	}

	// ifElse wraps the given children in an if_else node, letting us build a
	// deeply nested path to exercise every recursion depth, including the
	// deepest level (depth 0) where the if_else attribute is absent.
	ifElse := func(id string, then, els client.EscalationPathNodeV2) client.EscalationPathNodeV2 {
		return client.EscalationPathNodeV2{
			Id:   id,
			Type: client.EscalationPathNodeV2TypeIfElse,
			IfElse: &client.EscalationPathNodeIfElseV2{
				Conditions: []client.ConditionV2{
					{
						Subject:   client.ConditionSubjectV2{Reference: "incident.severity"},
						Operation: client.ConditionOperationV2{Value: "is"},
					},
				},
				ThenPath: []client.EscalationPathNodeV2{then},
				ElsePath: []client.EscalationPathNodeV2{els},
			},
		}
	}

	// Build 4 levels of nesting (the maximum the schema supports), so the
	// innermost level node sits at recursion depth 1, and its parent if_else
	// produces then/else lists whose element type is nodeAttrTypes(0) (no
	// if_else attribute).
	deeplyNested := ifElse("d1",
		ifElse("d2",
			ifElse("d3",
				ifElse("d4", level("deepest-then"), level("deepest-else")),
				level("d3-else")),
			level("d2-else")),
		level("d1-else"))

	ep := client.EscalationPathV2{
		Id:   "ep-1",
		Name: "Test path",
		Path: []client.EscalationPathNodeV2{
			level("node-1"),
			ifElse("node-2", level("node-2-then"), level("node-2-else")),
			deeplyNested,
		},
		WorkingHours: &[]client.WeekdayIntervalConfigV2{
			{
				Id:       "wh-1",
				Name:     "Office hours",
				Timezone: "Europe/London",
				WeekdayIntervals: []client.WeekdayIntervalV2{
					{StartTime: "09:00", EndTime: "17:00", Weekday: client.Monday},
				},
			},
		},
	}

	var diags diag.Diagnostics
	model := r.buildModel(ctx, ep, &diags)
	if diags.HasError() {
		t.Fatalf("buildModel produced errors: %#v", diags)
	}

	if model.Path.IsNull() || model.Path.IsUnknown() {
		t.Fatalf("expected non-null path list")
	}
	if l := len(model.Path.Elements()); l != 3 {
		t.Fatalf("expected 3 path nodes, got %d", l)
	}
	if model.WorkingHours.IsNull() {
		t.Fatalf("expected non-null working hours list")
	}

	// Now round-trip back to a payload.
	var payloadDiags diag.Diagnostics
	payload := r.toPathPayload(ctx, model.Path, &payloadDiags)
	if payloadDiags.HasError() {
		t.Fatalf("toPathPayload produced errors: %#v", payloadDiags)
	}
	if len(payload) != 3 {
		t.Fatalf("expected 3 payload nodes, got %d", len(payload))
	}

	// Walk to the deepest then-path level to confirm full-depth round-trip.
	deep := payload[2].IfElse
	for i := 0; i < 3 && deep != nil; i++ {
		if len(deep.ThenPath) != 1 {
			t.Fatalf("expected single then-path node at depth %d", i)
		}
		deep = deep.ThenPath[0].IfElse
	}
	if deep == nil || deep.ThenPath[0].Level == nil {
		t.Fatalf("expected deepest then-path to terminate in a level node")
	}

	node2 := payload[1].IfElse
	if node2 == nil {
		t.Fatalf("expected second node to have if_else payload")
	}
	if len(node2.ThenPath) != 1 || len(node2.ElsePath) != 1 {
		t.Fatalf("expected then/else paths to round-trip, got then=%d else=%d", len(node2.ThenPath), len(node2.ElsePath))
	}
	if node2.ThenPath[0].Level == nil || len(node2.ThenPath[0].Level.Targets) != 1 {
		t.Fatalf("expected nested then-path level target to round-trip")
	}
}
