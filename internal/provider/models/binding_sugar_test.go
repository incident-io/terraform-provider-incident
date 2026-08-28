package models

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/jsontypes"
)

func literal(s string) jsontypes.NormalizedJSONOrString {
	return jsontypes.NewNormalizedJSONOrStringValue(s)
}

// Each shorthand has to fold onto exactly the payload its long form produces, or the two
// spellings would mean different things to the API.
func TestParamBindingShorthandsResolveToTheLongForm(t *testing.T) {
	for _, tc := range []struct {
		name      string
		shorthand IncidentEngineParamBinding
		longForm  IncidentEngineParamBinding
	}{
		{
			name:      "value_literal",
			shorthand: IncidentEngineParamBinding{ValueLiteral: literal("high")},
			longForm: IncidentEngineParamBinding{Value: &IncidentEngineParamBindingValue{
				Literal: literal("high"), Reference: types.StringNull(),
			}},
		},
		{
			name:      "value_reference",
			shorthand: IncidentEngineParamBinding{ValueReference: types.StringValue("incident.url")},
			longForm: IncidentEngineParamBinding{Value: &IncidentEngineParamBindingValue{
				Literal: jsontypes.NewNormalizedJSONOrStringNull(), Reference: types.StringValue("incident.url"),
			}},
		},
		{
			name:      "expression_ref",
			shorthand: IncidentEngineParamBinding{ExpressionRef: types.StringValue("team")},
			longForm: IncidentEngineParamBinding{Value: &IncidentEngineParamBindingValue{
				Literal:   jsontypes.NewNormalizedJSONOrStringNull(),
				Reference: types.StringValue(`expressions["team"]`),
			}},
		},
		{
			name:      "values",
			shorthand: IncidentEngineParamBinding{Values: []jsontypes.NormalizedJSONOrString{literal("a"), literal("b")}},
			longForm: IncidentEngineParamBinding{ArrayValue: []IncidentEngineParamBindingValue{
				{Literal: literal("a"), Reference: types.StringNull()},
				{Literal: literal("b"), Reference: types.StringNull()},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.longForm.resolved(), tc.shorthand.resolved(),
				"the shorthand and its long form must resolve identically")
			assert.Equal(t, tc.longForm.ToPayload(), tc.shorthand.ToPayload(),
				"and so must reach the API as the same payload")
		})
	}
}

// FromAPI never sets a shorthand, and leaving them at the Go zero value has to mean null: every
// null check in resolved, IsEmpty and the reconciliation reads a binding the API just built.
// The framework picks ValueStateNull as 0 to make that so, and this pins it.
func TestFromAPILeavesTheShorthandsNull(t *testing.T) {
	binding := IncidentEngineParamBinding{}.FromAPI(client.EngineParamBindingV2{
		Value: &client.EngineParamBindingValueV2{Literal: lo.ToPtr("high")},
	})

	assert.True(t, binding.ValueLiteral.IsNull(), "value_literal")
	assert.True(t, binding.ValueReference.IsNull(), "value_reference")
	assert.True(t, binding.ExpressionRef.IsNull(), "expression_ref")
	assert.Empty(t, binding.Values, "values")

	for _, value := range []attr.Value{binding.ValueLiteral, binding.ValueReference, binding.ExpressionRef} {
		assert.False(t, value.IsUnknown(), "a zero-value shorthand must be null, not unknown")
	}
}

// An unknown shorthand is not a value. ValueString reads the empty string off an unknown, so
// treating one as set would turn expression_ref into `expressions[""]`.
func TestResolvedTreatsAnUnknownShorthandAsUnset(t *testing.T) {
	for name, binding := range map[string]IncidentEngineParamBinding{
		"value_literal":   {ValueLiteral: jsontypes.NewNormalizedJSONOrStringUnknown()},
		"value_reference": {ValueReference: types.StringUnknown()},
		"expression_ref":  {ExpressionRef: types.StringUnknown()},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, IncidentEngineParamBinding{}, binding.resolved())
		})
	}
}

// A read only ever sees the long forms, so without this a config using a shorthand fails the
// apply as an inconsistent result.
func TestReconcileSpellingKeepsTheConfigsShorthand(t *testing.T) {
	fromAPI := IncidentEngineParamBinding{Value: &IncidentEngineParamBindingValue{
		Literal: literal("high"), Reference: types.StringNull(),
	}}

	t.Run("prior used the shorthand", func(t *testing.T) {
		prior := IncidentEngineParamBinding{ValueLiteral: literal("high")}

		got := fromAPI.ReconcileSpelling(prior)
		assert.Equal(t, prior, got, "should hand back the shorthand the config wrote")
	})

	t.Run("prior used the long form", func(t *testing.T) {
		got := fromAPI.ReconcileSpelling(fromAPI)
		assert.Equal(t, fromAPI, got, "should leave a long-form config alone")
	})

	// The prior is only preferred when it still means what came back. A value that genuinely
	// changed elsewhere has to win, or the read would hide real drift.
	t.Run("value changed", func(t *testing.T) {
		prior := IncidentEngineParamBinding{ValueLiteral: literal("low")}

		got := fromAPI.ReconcileSpelling(prior)
		assert.Equal(t, fromAPI, got, "should report the API's value, not the stale shorthand")
	})
}

// The API re-encodes a JSON literal, so the comparison has to be the semantic one
// NormalizedJSONOrString provides. Byte comparison would read a difference in key order as a
// changed value, drop the shorthand, and fail the apply — the failure this type exists to prevent.
func TestReconcileSpellingComparesJSONSemantically(t *testing.T) {
	fromAPI := IncidentEngineParamBinding{Value: &IncidentEngineParamBindingValue{
		Literal:   literal(`{"a":1,"b":2}`),
		Reference: types.StringNull(),
	}}

	t.Run("same JSON, different key order", func(t *testing.T) {
		prior := IncidentEngineParamBinding{ValueLiteral: literal(`{"b":2,"a":1}`)}

		assert.Equal(t, prior, fromAPI.ReconcileSpelling(prior), "should keep the config's shorthand")
	})

	t.Run("different JSON", func(t *testing.T) {
		prior := IncidentEngineParamBinding{ValueLiteral: literal(`{"a":1,"b":3}`)}

		assert.Equal(t, fromAPI, fromAPI.ReconcileSpelling(prior), "should report the API's value")
	})

	// A null literal and an empty string are different values, and ValueString() flattens both to
	// "" — so the null check has to come before the string comparison.
	t.Run("null literal is not an empty one", func(t *testing.T) {
		prior := IncidentEngineParamBinding{Values: []jsontypes.NormalizedJSONOrString{literal("")}}
		empty := IncidentEngineParamBinding{ArrayValue: []IncidentEngineParamBindingValue{
			{Literal: jsontypes.NewNormalizedJSONOrStringNull(), Reference: types.StringNull()},
		}}

		assert.Equal(t, empty, empty.ReconcileSpelling(prior))
	})
}

// Bindings are a list, correlated positionally, and a prior shorter than the read must not panic
// or misalign the entries it does cover.
func TestReconcileSpellingOverAList(t *testing.T) {
	fromAPI := IncidentEngineParamBindings{
		{Value: &IncidentEngineParamBindingValue{Literal: literal("one"), Reference: types.StringNull()}},
		{Value: &IncidentEngineParamBindingValue{Literal: literal("two"), Reference: types.StringNull()}},
	}
	prior := IncidentEngineParamBindings{
		{ValueLiteral: literal("one")},
	}

	got := fromAPI.ReconcileSpelling(prior)

	assert.Equal(t, prior[0], got[0], "the covered entry keeps its shorthand")
	assert.NotNil(t, got[1].Value, "the uncovered entry keeps the API's long form")
	assert.True(t, got[1].ValueLiteral.IsNull())
}

// A binding held as a types.Object has to survive the trip through it, shorthands included:
// alert route severity reads and writes its binding that way, and a lost attribute there fails
// the apply rather than showing up as a diff.
func TestBindingSurvivesTheObjectRoundTrip(t *testing.T) {
	for name, binding := range map[string]IncidentEngineParamBinding{
		"empty": {},
		"value": {Value: &IncidentEngineParamBindingValue{
			Literal: literal("high"), Reference: types.StringNull(),
		}},
		"array_value": {ArrayValue: []IncidentEngineParamBindingValue{
			{Literal: literal("a"), Reference: types.StringNull()},
			{Literal: jsontypes.NewNormalizedJSONOrStringNull(), Reference: types.StringValue("incident.url")},
		}},
		"value_literal":   {ValueLiteral: literal("high")},
		"value_reference": {ValueReference: types.StringValue("incident.url")},
		"expression_ref":  {ExpressionRef: types.StringValue("team")},
		"values":          {Values: []jsontypes.NormalizedJSONOrString{literal("a"), literal("b")}},
	} {
		t.Run(name, func(t *testing.T) {
			obj := binding.ToObject()

			assert.Equal(t, ParamBindingAttrTypes(), obj.AttributeTypes(context.Background()),
				"the object must carry every attribute the schema declares")
			assert.Equal(t, binding, ParamBindingFromObject(obj))
		})
	}
}

// ReconcileBindingSpelling covers the standalone bindings, where nil on either side means there
// is nothing to reconcile against.
func TestReconcileBindingSpellingHandlesNil(t *testing.T) {
	applied := &IncidentEngineParamBinding{Value: &IncidentEngineParamBindingValue{
		Literal: literal("high"), Reference: types.StringNull(),
	}}
	prior := &IncidentEngineParamBinding{ValueLiteral: literal("high")}

	assert.Equal(t, prior, ReconcileBindingSpelling(applied, prior))
	assert.Equal(t, applied, ReconcileBindingSpelling(applied, nil))
	assert.Nil(t, ReconcileBindingSpelling(nil, prior))
}

// IsEmpty decides whether a trailing binding is padding the API added, so it has to count the
// shorthands too — otherwise `values = ["a"]` in the last position reads as empty and is trimmed.
func TestIsEmptyCountsTheShorthands(t *testing.T) {
	assert.True(t, IncidentEngineParamBinding{}.IsEmpty())

	for name, binding := range map[string]IncidentEngineParamBinding{
		"value_literal":   {ValueLiteral: literal("x")},
		"value_reference": {ValueReference: types.StringValue("incident.url")},
		"expression_ref":  {ExpressionRef: types.StringValue("team")},
		"values":          {Values: []jsontypes.NormalizedJSONOrString{literal("x")}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, binding.IsEmpty())
		})
	}
}
