package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// levelNodeV3 is levelNode with its target's high/low setting under its V3 name.
func levelNodeV3(t *testing.T) escalationPathNode {
	t.Helper()

	var diags diag.Diagnostics
	targets := targetsFromAPIV3(context.Background(), []client.EscalationPathTargetV3{{
		Id:                "01ABC",
		Type:              client.EscalationPathTargetV3TypeSchedule,
		InterruptionLevel: client.EscalationPathTargetV3InterruptionLevelHigh,
	}}, &diags)
	if diags.HasError() {
		t.Fatalf("building targets: %+v", diags)
	}
	return escalationPathNode{ID: types.StringNull(), Level: &IncidentEscalationPathNodeLevel{Targets: targets}}
}

// levelNodeNoSetting is a level whose target sets neither urgency nor interruption_level.
func levelNodeNoSetting(t *testing.T) escalationPathNode {
	t.Helper()

	node := levelNodeV3(t)
	var diags diag.Diagnostics
	targets := decodeTargets(context.Background(), node.Level.Targets, &diags)
	targets[0].InterruptionLevel = types.StringNull()
	list, d := types.ListValueFrom(context.Background(), types.ObjectType{AttrTypes: targetAttrTypes()}, targets)
	if d.HasError() {
		t.Fatalf("building targets: %+v", d)
	}
	node.Level.Targets = list
	return node
}

func TestEscalationPathVersionOf(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		sequences map[string][]escalationPathNode
		want      escalationPathAPIVersion
	}{
		{
			name:      "targets setting urgency",
			sequences: map[string][]escalationPathNode{"main": {levelNode(t, "")}},
			want:      escalationPathAPIV2,
		},
		{
			name:      "targets setting interruption_level",
			sequences: map[string][]escalationPathNode{"main": {levelNodeV3(t)}},
			want:      escalationPathAPIV3,
		},
		{
			name: "a branch on priority",
			sequences: map[string][]escalationPathNode{
				"main":   {levelNodeV3(t), branchWith(priorityIf(t, "01P1"))},
				"urgent": {levelNodeV3(t)},
			},
			want: escalationPathAPIV2,
		},
		{
			name: "a branch on urgency",
			sequences: map[string][]escalationPathNode{
				"main":   {levelNodeV3(t), branchWith(urgencyIf(t, "01HIGH"))},
				"urgent": {levelNodeV3(t)},
			},
			want: escalationPathAPIV3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			if got := escalationPathVersionOf(ctx, betaModel(t, "main", tc.sequences), &diags); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
			if diags.HasError() {
				t.Fatalf("unexpected errors: %+v", diags)
			}
		})
	}

	t.Run("a templated path, or an import, which has no sequences", func(t *testing.T) {
		var diags diag.Diagnostics
		data := &escalationPathModel{Sequences: types.MapNull(sequenceMapType(escalationPathNodeAttrTypes()).ElemType)}
		if got := escalationPathVersionOf(ctx, data, &diags); got != escalationPathAPIV3 {
			t.Errorf("got %s, want V3", got)
		}
	})
}

func TestValidateEscalationPathVersion(t *testing.T) {
	ctx := context.Background()

	t.Run("lists every field from each version", func(t *testing.T) {
		data := betaModel(t, "main", map[string][]escalationPathNode{
			"main":     {levelNode(t, ""), levelNodeV3(t), branchWith(priorityIf(t, "01P1"))},
			"urgent":   {levelNodeV3(t)},
			"fallback": {},
		})

		var diags diag.Diagnostics
		validateEscalationPathVersion(ctx, data, &diags)
		if diags.ErrorsCount() != 1 {
			t.Fatalf("got %d errors, want 1: %+v", diags.ErrorsCount(), diags)
		}

		got := diags.Errors()[0]
		if got.Summary() != "Escalation path mixes deprecated and current attributes" {
			t.Errorf("got summary %q", got.Summary())
		}
		want := `This escalation path uses the deprecated urgency and priority_one_of attributes together with their replacements, interruption_level and urgency_one_of. The two sets of attributes correspond to versions 2 and 3 of the incident.io public API, so an escalation path must use one set throughout. Replace the deprecated attributes with interruption_level and urgency_one_of.

Deprecated (urgency, priority_one_of):
  - sequences["main"].nodes[0].level.targets[0].urgency
  - sequences["main"].nodes[2].branch.if.priority_one_of

Current (interruption_level, urgency_one_of):
  - sequences["main"].nodes[1].level.targets[0].interruption_level
  - sequences["urgent"].nodes[0].level.targets[0].interruption_level`
		if got.Detail() != want {
			t.Errorf("got detail:\n%s\n\nwant:\n%s", got.Detail(), want)
		}
	})

	t.Run("accepts a path using one version throughout", func(t *testing.T) {
		for _, sequences := range []map[string][]escalationPathNode{
			{"main": {levelNode(t, ""), branchWith(priorityIf(t, "01P1"))}, "urgent": {levelNode(t, "")}},
			{"main": {levelNodeV3(t), branchWith(urgencyIf(t, "01HIGH"))}, "urgent": {levelNodeV3(t)}},
		} {
			var diags diag.Diagnostics
			validateEscalationPathVersion(ctx, betaModel(t, "main", sequences), &diags)
			if diags.HasError() {
				t.Errorf("unexpected errors: %+v", diags)
			}
		}
	})
}

func TestValidateEscalationPathTargetInterruptionLevels(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		other escalationPathNode
		want  string
	}{
		{"a path using interruption_level", levelNodeV3(t), "Set interruption_level"},
		// Suggesting interruption_level here would lead to the mixed-attributes error.
		{"a path still using urgency", levelNode(t, ""), "Set urgency"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateEscalationPathTargetInterruptionLevels(ctx, betaModel(t, "main", map[string][]escalationPathNode{
				"main": {tc.other, levelNodeNoSetting(t)},
			}), &diags)

			if diags.ErrorsCount() != 1 {
				t.Fatalf("got %d errors, want 1: %+v", diags.ErrorsCount(), diags)
			}
			if got := diags.Errors()[0].Detail(); !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want it to say %q", got, tc.want)
			}
		})
	}
}

func TestPathSequenceCodecV3(t *testing.T) {
	ctx := context.Background()

	sequences := map[string][]escalationPathNode{
		"main":   {levelNodeV3(t), branchWith(urgencyIf(t, "01HIGH"))},
		"urgent": {levelNodeV3(t)},
	}

	var diags diag.Diagnostics
	payload := unflattenSequencesWith(ctx, pathSequenceCodecV3{}, "main", sequences, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected errors: %+v", diags)
	}

	t.Run("sends interruption_level and urgency conditions", func(t *testing.T) {
		if got := payload[0].Level.Targets[0].InterruptionLevel; got != client.EscalationPathTargetV3InterruptionLevelHigh {
			t.Errorf("got interruption_level %q, want high", got)
		}
		conditions := *payload[1].IfElse.Conditions
		if len(conditions) != 1 || conditions[0].Subject != "escalation.urgency" {
			t.Errorf("got conditions %+v, want one on escalation.urgency", conditions)
		}
	})

	t.Run("reads back what it sent", func(t *testing.T) {
		start, got := flattenSequencesWith(ctx, pathSequenceCodecV3{}, apiNodesV3(payload), priorNames("main", sequences), &diags)
		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if start != "main" {
			t.Errorf("got start %q, want main", start)
		}

		targets := decodeTargets(ctx, got["urgent"][0].Level.Targets, &diags)
		if targets[0].InterruptionLevel.ValueString() != "high" || !targets[0].Urgency.IsNull() {
			t.Errorf("got target %+v, want interruption_level high and no urgency", targets[0])
		}

		branch := got["main"][1].Branch
		if branch == nil || !branch.If.UrgencyOneOf.Equal(urgencyIf(t, "01HIGH").UrgencyOneOf) || !branch.If.PriorityOneOf.IsNull() {
			t.Errorf("got branch %+v, want it on urgency 01HIGH", branch)
		}
	})
}

// apiNodesV3 is apiNodes for the V3 types.
func apiNodesV3(payload []client.EscalationPathNodePayloadV3) []client.EscalationPathNodeV3 {
	out := make([]client.EscalationPathNodeV3, 0, len(payload))
	for _, node := range payload {
		converted := client.EscalationPathNodeV3{
			Id:             node.Id,
			Type:           client.EscalationPathNodeV3Type(node.Type),
			Level:          node.Level,
			NotifyChannel:  node.NotifyChannel,
			Delay:          node.Delay,
			EscalationPath: node.EscalationPath,
			Repeat:         node.Repeat,
		}
		if node.IfElse != nil {
			// Conditions have the same JSON shape in both versions, so reuse apiConditions.
			conditions := apiConditions(models.ConvertAPIType[[]client.ConditionPayloadV2](lo.FromPtr(node.IfElse.Conditions)))
			converted.IfElse = &client.EscalationPathNodeIfElseV3{
				Conditions: models.ConvertAPIType[[]client.ConditionV3](conditions),
				ThenPath:   apiNodesV3(node.IfElse.ThenPath),
				ElsePath:   apiNodesV3(node.IfElse.ElsePath),
			}
		}
		out = append(out, converted)
	}
	return out
}

func TestValidateEscalationPathTemplateBranches(t *testing.T) {
	ctx := context.Background()

	data := betaModel(t, "main", map[string][]escalationPathNode{
		"main":   {levelNode(t, ""), branchWith(urgencyIf(t, "01HIGH"))},
		"urgent": {levelNode(t, "")},
	})

	var diags diag.Diagnostics
	validateEscalationPathTemplateBranches(ctx, data.Sequences, &diags)
	if diags.ErrorsCount() != 1 || diags.Errors()[0].Summary() != "Templates can't branch on urgency" {
		t.Errorf("got %+v, want one error refusing urgency_one_of", diags)
	}
}
