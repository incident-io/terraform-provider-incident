package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// TestNamespaceRewritesEveryReference is the check the round trip can't make: a reference the
// rewrite misses is missed in both directions, so it round trips and still fails at apply, against
// a name nothing on the source is called. So this asserts on the payload — no reference may leave
// under the name the config wrote.
func TestNamespaceRewritesEveryReference(t *testing.T) {
	payloads, _, err := ExpressionsToPayload(testNamespace, referencesEverywhereBound, referencesEverywhereNamed)
	if err != nil {
		t.Fatalf("ExpressionsToPayload: %v", err)
	}

	written := marshalPayloads(t, payloads)

	// A label is deliberately the name as written, so this looks only at references.
	local := jsonReference(t, "severity")
	if count := strings.Count(written, local); count != 0 {
		t.Errorf("%d references still name the expression %s\n%s", count, local, written)
	}

	stored := jsonReference(t, testNamespace.stored("severity"))
	if count := strings.Count(written, stored); count == 0 {
		t.Errorf("nothing references %s, so the expression is unreachable\n%s", stored, written)
	}
}

func marshalPayloads(t *testing.T, payloads []client.ExpressionPayloadV3) string {
	t.Helper()

	written, err := json.Marshal(payloads)
	if err != nil {
		t.Fatalf("marshalling the payload: %v", err)
	}

	return string(written)
}

// jsonReference escapes the quotes around name, as the marshalled payload does. Searching for the
// unescaped form matches nothing and passes.
func jsonReference(t *testing.T, name string) string {
	t.Helper()

	escaped, err := json.Marshal(ExpressionReference(name))
	if err != nil {
		t.Fatalf("marshalling a reference to %q: %v", name, err)
	}

	return strings.Trim(string(escaped), `"`)
}

// TestNamespaceRoundTrip: what comes back has to be spelled the way the config wrote it, or every
// plan shows a change.
func TestNamespaceRoundTrip(t *testing.T) {
	payloads, _, err := ExpressionsToPayload(testNamespace, referencesEverywhereBound, referencesEverywhereNamed)
	if err != nil {
		t.Fatalf("ExpressionsToPayload: %v", err)
	}

	// Names only, so the mapping runs rather than the prior being echoed back.
	priorOrder := make([]NamedExpression, 0, len(referencesEverywhereNamed))
	for _, expression := range referencesEverywhereNamed {
		priorOrder = append(priorOrder, NamedExpression{Name: expression.Name})
	}

	bound, named := ExpressionsFromPayload(payloads, testNamespace, nil, priorOrder)

	if !reflect.DeepEqual(referencesEverywhereBound, bound) {
		t.Errorf("expression did not round trip\n got: %#v\nwant: %#v", bound, referencesEverywhereBound)
	}
	if !reflect.DeepEqual(referencesEverywhereNamed, named) {
		t.Errorf("named expressions did not round trip\n got: %#v\nwant: %#v", named, referencesEverywhereNamed)
	}
}

func TestNamespaceLeavesOtherNamesAlone(t *testing.T) {
	payloads, _, err := ExpressionsToPayload(testNamespace, nil, []NamedExpression{{
		Name:      types.StringValue("severity"),
		StartFrom: types.StringValue("payload.alert"),
		Operations: []Operation{{Filter: &Conditions{Conditions: []Condition{{
			Subject:   types.StringValue("payload.level"),
			Operation: types.StringValue("one_of"),
			Params: []Binding{{
				ValueReference: types.StringValue(ExpressionReference("written_by_the_dashboard")),
			}},
		}}}}},
	}})
	if err != nil {
		t.Fatalf("ExpressionsToPayload: %v", err)
	}

	written := marshalPayloads(t, payloads)

	if elsewhere := jsonReference(t, "written_by_the_dashboard"); !strings.Contains(written, elsewhere) {
		t.Errorf("a reference to another resource's expression was rewritten\n%s", written)
	}
	for _, path := range []string{"payload.alert", "payload.level"} {
		if !strings.Contains(written, path) {
			t.Errorf("the scope path %q did not survive\n%s", path, written)
		}
	}
}

// TestNamespaceAdoptsUnprefixedNames is importing what the dashboard wrote: no prefix to strip, so
// the names hold until the next write renames them.
func TestNamespaceAdoptsUnprefixedNames(t *testing.T) {
	bound, named := ExpressionsFromPayload([]client.ExpressionPayloadV3{{
		Reference:     "a1b2c3d4",
		Label:         "Severity",
		RootReference: "payload",
		Operations: []client.ExpressionOperationPayloadV3{{
			OperationType: "cast",
			Cast:          &client.ExpressionCastOptsPayloadV3{Returns: client.ReturnsMetaV3{Type: "Text"}},
		}},
	}}, testNamespace, nil, nil)

	if bound != nil {
		t.Error("got a bound expression from a payload set that has none")
	}
	if len(named) != 1 {
		t.Fatalf("read %d named expressions, want 1", len(named))
	}
	if got := named[0].Name.ValueString(); got != "a1b2c3d4" {
		t.Errorf("adopted as %q, want the name it already had", got)
	}
}

// referencesEverywhereNamed puts a reference to a sibling in every position one can appear in.
// Spellings are the canonical ones, the form a read returns, so the round trip means something.
var referencesEverywhereNamed = []NamedExpression{
	{
		Name:       types.StringValue("severity"),
		StartFrom:  types.StringValue("payload"),
		Operations: []Operation{{Cast: &Cast{As: types.StringValue("Text")}}},
	},
	{
		Name:      types.StringValue("team"),
		StartFrom: types.StringValue(ExpressionReference("severity")),
		Operations: []Operation{
			{Concatenate: &Concatenate{With: types.StringValue(ExpressionReference("severity"))}},
			{Navigate: &Navigate{To: types.StringValue(ExpressionReference("severity"))}},
			{Filter: &Conditions{Conditions: []Condition{
				{
					Subject:   types.StringValue(ExpressionReference("severity")),
					Operation: types.StringValue("one_of"),
					Params:    []Binding{{ExpressionRef: types.StringValue("severity")}},
				},
				{
					Subject:   types.StringValue("payload.tags"),
					Operation: types.StringValue("one_of"),
					// A mixed array, which is the long form a read returns.
					Params: []Binding{{ArrayValue: []BindingValue{
						{Reference: types.StringValue(ExpressionReference("severity"))},
						{Literal: types.StringValue("urgent")},
					}}},
				},
			}}},
			{Branches: &Branches{
				As: types.StringValue("Text"),
				If: &Branch{
					Conditions: []Condition{{
						// A reference into a result, not the expression itself, so it stays a
						// value_reference rather than folding to expression_ref.
						Subject:   types.StringValue(ExpressionReference("severity") + ".name"),
						Operation: types.StringValue("is_set"),
					}},
					Result: &Binding{ValueReference: types.StringValue(ExpressionReference("severity") + ".name")},
				},
				ElseIf: []Branch{{
					// Two groups, because a read returns a single one as the `conditions` sugar.
					ConditionGroups: []ConditionGroup{
						{Conditions: []Condition{{
							Subject:   types.StringValue(ExpressionReference("severity")),
							Operation: types.StringValue("is_set"),
						}}},
						{Conditions: []Condition{{
							Subject:   types.StringValue(ExpressionReference("team")),
							Operation: types.StringValue("is_set"),
						}}},
					},
					Result: &Binding{ExpressionRef: types.StringValue("severity")},
				}},
			}},
		},
		Fallback: &Fallback{ExpressionRef: types.StringValue("severity")},
	},
}

// referencesEverywhereBound reaches a sibling through the shorthand, which also has to rename the
// expression it generates.
var referencesEverywhereBound = &Expression{
	StartFrom:  types.StringValue("payload"),
	Operations: []Operation{{Parse: &Parse{Function: types.StringValue("$.severity"), As: types.StringValue("Text")}}},
	Fallback: &Fallback{
		If: &Branch{
			Conditions: []Condition{{
				Subject:   types.StringValue(ExpressionReference("severity")),
				Operation: types.StringValue("is_set"),
			}},
			Result: &Binding{ExpressionRef: types.StringValue("severity")},
		},
		Else: &Else{Result: &Binding{ExpressionRef: types.StringValue("team")}},
	},
}
