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

// The builders below all start from NullParamBinding rather than the zero value, because a
// zero-value types.List carries no element type and so never compares equal to one the
// framework built. Writing bindings any other way in a test passes or fails for the wrong
// reason.

// litValue is one binding value holding a literal.
func litValue(s string) IncidentEngineParamBindingValue {
	return IncidentEngineParamBindingValue{Literal: literal(s), Reference: types.StringNull()}
}

// refValue is one binding value holding a reference.
func refValue(s string) IncidentEngineParamBindingValue {
	return IncidentEngineParamBindingValue{
		Literal:   jsontypes.NewNormalizedJSONOrStringNull(),
		Reference: types.StringValue(s),
	}
}

func bindValue(v IncidentEngineParamBindingValue) IncidentEngineParamBinding {
	binding := NullParamBinding()
	binding.Value = v.ToObject()

	return binding
}

func bindArray(values ...IncidentEngineParamBindingValue) IncidentEngineParamBinding {
	binding := NullParamBinding()
	binding.ArrayValue = ParamBindingArrayValue(values...)

	return binding
}

func bindValueLiteral(s string) IncidentEngineParamBinding {
	binding := NullParamBinding()
	binding.ValueLiteral = literal(s)

	return binding
}

func bindValueReference(s string) IncidentEngineParamBinding {
	binding := NullParamBinding()
	binding.ValueReference = types.StringValue(s)

	return binding
}

func bindExpressionRef(s string) IncidentEngineParamBinding {
	binding := NullParamBinding()
	binding.ExpressionRef = types.StringValue(s)

	return binding
}

func bindValues(literals ...string) IncidentEngineParamBinding {
	binding := NullParamBinding()
	binding.Values = ParamBindingValuesList(lo.Map(literals,
		func(s string, _ int) jsontypes.NormalizedJSONOrString { return literal(s) })...)

	return binding
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
			shorthand: bindValueLiteral("high"),
			longForm:  bindValue(litValue("high")),
		},
		{
			name:      "value_reference",
			shorthand: bindValueReference("incident.url"),
			longForm:  bindValue(refValue("incident.url")),
		},
		{
			name:      "expression_ref",
			shorthand: bindExpressionRef("team"),
			longForm:  bindValue(refValue(`expressions["team"]`)),
		},
		{
			name:      "values",
			shorthand: bindValues("a", "b"),
			longForm:  bindArray(litValue("a"), litValue("b")),
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

// FromAPI never sets a shorthand, and they have to come back null: every null check in
// resolved, IsEmpty and the reconciliation reads a binding the API just built.
func TestFromAPILeavesTheShorthandsNull(t *testing.T) {
	binding := IncidentEngineParamBinding{}.FromAPI(client.EngineParamBindingV2{
		Value: &client.EngineParamBindingValueV2{Literal: lo.ToPtr("high")},
	})

	assert.True(t, binding.ValueLiteral.IsNull(), "value_literal")
	assert.True(t, binding.ValueReference.IsNull(), "value_reference")
	assert.True(t, binding.ExpressionRef.IsNull(), "expression_ref")
	assert.True(t, binding.Values.IsNull(), "values")

	for _, value := range []attr.Value{
		binding.ValueLiteral, binding.ValueReference, binding.ExpressionRef, binding.Values,
	} {
		assert.False(t, value.IsUnknown(), "an unset shorthand must be null, not unknown")
	}
}

// FromAPI has to give every list and object an element type, or the framework can't write
// the binding to state — a failure that surfaces a long way from the binding that caused it.
func TestFromAPITypesTheEmptyForms(t *testing.T) {
	ctx := context.Background()

	binding := IncidentEngineParamBinding{}.FromAPI(client.EngineParamBindingV2{})

	assert.Equal(t, ParamBindingValueType(), binding.ArrayValue.ElementType(ctx), "array_value")
	assert.Equal(t, ParamBindingValueAttrTypes(), binding.Value.AttributeTypes(ctx), "value")
	assert.Equal(t, jsontypes.NormalizedJSONOrStringType{}, binding.Values.ElementType(ctx), "values")
}

// An array_value the API returns but leaves empty has to read back as unset. A config that
// didn't write the attribute holds null, so keeping the empty list would show as a diff on
// every plan.
func TestFromAPIReadsAnEmptyArrayAsUnset(t *testing.T) {
	binding := IncidentEngineParamBinding{}.FromAPI(client.EngineParamBindingV2{
		ArrayValue: lo.ToPtr([]client.EngineParamBindingValueV2{}),
	})

	assert.True(t, binding.ArrayValue.IsNull(), "an empty array_value should read as unset")
}

// An unknown shorthand is not a value. ValueString reads the empty string off an unknown, so
// treating one as set would turn expression_ref into `expressions[""]`.
func TestResolvedTreatsAnUnknownShorthandAsUnset(t *testing.T) {
	for name, set := range map[string]func(b *IncidentEngineParamBinding){
		"value_literal":   func(b *IncidentEngineParamBinding) { b.ValueLiteral = jsontypes.NewNormalizedJSONOrStringUnknown() },
		"value_reference": func(b *IncidentEngineParamBinding) { b.ValueReference = types.StringUnknown() },
		"expression_ref":  func(b *IncidentEngineParamBinding) { b.ExpressionRef = types.StringUnknown() },
	} {
		t.Run(name, func(t *testing.T) {
			binding := NullParamBinding()
			set(&binding)

			assert.Equal(t, resolvedBinding{}, binding.resolved())
		})
	}
}

// A read only ever sees the long forms, so without this a config using a shorthand fails the
// apply as an inconsistent result.
func TestReconcileSpellingKeepsTheConfigsShorthand(t *testing.T) {
	fromAPI := bindValue(litValue("high"))

	t.Run("prior used the shorthand", func(t *testing.T) {
		prior := bindValueLiteral("high")

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
		prior := bindValueLiteral("low")

		got := fromAPI.ReconcileSpelling(prior)
		assert.Equal(t, fromAPI, got, "should report the API's value, not the stale shorthand")
	})
}

// The API re-encodes a JSON literal, so the comparison has to be the semantic one
// NormalizedJSONOrString provides. Byte comparison would read a difference in key order as a
// changed value, drop the shorthand, and fail the apply — the failure this type exists to prevent.
func TestReconcileSpellingComparesJSONSemantically(t *testing.T) {
	fromAPI := bindValue(litValue(`{"a":1,"b":2}`))

	t.Run("same JSON, different key order", func(t *testing.T) {
		prior := bindValueLiteral(`{"b":2,"a":1}`)

		assert.Equal(t, prior, fromAPI.ReconcileSpelling(prior), "should keep the config's shorthand")
	})

	t.Run("different JSON", func(t *testing.T) {
		prior := bindValueLiteral(`{"a":1,"b":3}`)

		assert.Equal(t, fromAPI, fromAPI.ReconcileSpelling(prior), "should report the API's value")
	})

	// A null literal and an empty string are different values, and ValueString() flattens both to
	// "" — so the null check has to come before the string comparison.
	t.Run("null literal is not an empty one", func(t *testing.T) {
		prior := bindValues("")
		empty := bindArray(IncidentEngineParamBindingValue{
			Literal:   jsontypes.NewNormalizedJSONOrStringNull(),
			Reference: types.StringNull(),
		})

		assert.Equal(t, empty, empty.ReconcileSpelling(prior))
	})
}

// Bindings are a list, correlated positionally, and a prior shorter than the read must not panic
// or misalign the entries it does cover.
func TestReconcileSpellingOverAList(t *testing.T) {
	fromAPI := IncidentEngineParamBindings{
		bindValue(litValue("one")),
		bindValue(litValue("two")),
	}
	prior := IncidentEngineParamBindings{bindValueLiteral("one")}

	got := fromAPI.ReconcileSpelling(prior)

	assert.Equal(t, prior[0], got[0], "the covered entry keeps its shorthand")
	assert.False(t, got[1].Value.IsNull(), "the uncovered entry keeps the API's long form")
	assert.True(t, got[1].ValueLiteral.IsNull())
}

// A binding held as a types.Object has to survive the trip through it, shorthands included:
// alert route severity reads and writes its binding that way, and a lost attribute there fails
// the apply rather than showing up as a diff.
func TestBindingSurvivesTheObjectRoundTrip(t *testing.T) {
	for name, binding := range map[string]IncidentEngineParamBinding{
		"empty":           NullParamBinding(),
		"value":           bindValue(litValue("high")),
		"array_value":     bindArray(litValue("a"), refValue("incident.url")),
		"value_literal":   bindValueLiteral("high"),
		"value_reference": bindValueReference("incident.url"),
		"expression_ref":  bindExpressionRef("team"),
		"values":          bindValues("a", "b"),
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
	applied := lo.ToPtr(bindValue(litValue("high")))
	prior := lo.ToPtr(bindValueLiteral("high"))

	assert.Equal(t, prior, ReconcileBindingSpelling(applied, prior))
	assert.Equal(t, applied, ReconcileBindingSpelling(applied, nil))
	assert.Nil(t, ReconcileBindingSpelling(nil, prior))
}

// IsEmpty decides whether a trailing binding is padding the API added, so it has to count the
// shorthands too — otherwise `values = ["a"]` in the last position reads as empty and is trimmed.
func TestIsEmptyCountsTheShorthands(t *testing.T) {
	assert.True(t, NullParamBinding().IsEmpty())

	for name, binding := range map[string]IncidentEngineParamBinding{
		"value_literal":   bindValueLiteral("x"),
		"value_reference": bindValueReference("incident.url"),
		"expression_ref":  bindExpressionRef("team"),
		"values":          bindValues("x"),
		"value":           bindValue(litValue("x")),
		"array_value":     bindArray(litValue("x")),
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, binding.IsEmpty())
		})
	}
}

// An `= []` and an unset attribute both bind nothing, and the slice these forms used to be
// read both as length zero. An unknown is neither: it may yet turn out to hold something, so
// trimming it as padding would drop a binding the apply is about to fill in.
func TestIsEmptyCountsAnEmptyListButNotAnUnknownOne(t *testing.T) {
	empty := NullParamBinding()
	empty.ArrayValue = types.ListValueMust(ParamBindingValueType(), []attr.Value{})
	assert.True(t, empty.IsEmpty(), "an empty array_value binds nothing")

	emptyValues := NullParamBinding()
	emptyValues.Values = types.ListValueMust(jsontypes.NormalizedJSONOrStringType{}, []attr.Value{})
	assert.True(t, emptyValues.IsEmpty(), "an empty values binds nothing")

	unknown := NullParamBinding()
	unknown.ArrayValue = types.ListUnknown(ParamBindingValueType())
	assert.False(t, unknown.IsEmpty(), "an unknown array_value is not empty")
}

// TestReconcileScalarBindingFoldsAOneElementArray covers what the policies API does to an
// assignee binding: it binds against an array param, so a scalar goes in and a one-element
// array comes back. Without this the apply fails as an inconsistent result.
func TestReconcileScalarBindingFoldsAOneElementArray(t *testing.T) {
	prior := bindValueLiteral("01USER")
	fromAPI := bindArray(litValue("01USER"))

	got := ReconcileScalarBinding(fromAPI, prior)
	assert.Equal(t, prior, got, "the config's scalar spelling should survive the round trip")

	// Plain ReconcileSpelling cannot see these as equal, which is why the scalar variant
	// exists: for every other endpoint a scalar and an array differ for real.
	assert.Equal(t, fromAPI, fromAPI.ReconcileSpelling(prior))
}

// TestReconcileScalarBindingKeepsRealDrift asserts the fold is not a blanket "arrays equal
// scalars": a different value, and an array of more than one, both stay as the API sent them.
func TestReconcileScalarBindingKeepsRealDrift(t *testing.T) {
	prior := bindValueLiteral("01USER")

	changed := bindArray(litValue("01SOMEONE-ELSE"))
	assert.Equal(t, changed, ReconcileScalarBinding(changed, prior))

	twoValues := bindArray(litValue("01USER"), litValue("01SECOND"))
	assert.Equal(t, twoValues, ReconcileScalarBinding(twoValues, prior))
}
