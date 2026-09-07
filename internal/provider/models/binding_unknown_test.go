package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"

	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/jsontypes"
)

// unknownBindings is every form a config can point at an expression Terraform hasn't settled:
// `array_value = local.x`, `value = local.x`, `values = local.x`, and the same through a for,
// a ternary or each.value. All three used to be Go slices and pointers, which can hold no
// such thing, so reading the config failed before any of this code ran.
func unknownBindings() map[string]IncidentEngineParamBinding {
	arrayValue := NullParamBinding()
	arrayValue.ArrayValue = types.ListUnknown(ParamBindingValueType())

	value := NullParamBinding()
	value.Value = types.ObjectUnknown(ParamBindingValueAttrTypes())

	values := NullParamBinding()
	values.Values = types.ListUnknown(jsontypes.NormalizedJSONOrStringType{})

	// The list is KNOWN here and only the element inside it isn't, which is what a ternary
	// whose branches are the same length produces - and is how the reported failure actually
	// arrived, as array_value[0] rather than array_value.
	arrayElement := NullParamBinding()
	arrayElement.ArrayValue = types.ListValueMust(ParamBindingValueType(), []attr.Value{
		types.ObjectUnknown(ParamBindingValueAttrTypes()),
	})

	return map[string]IncidentEngineParamBinding{
		"array_value":         arrayValue,
		"array_value element": arrayElement,
		"value":               value,
		"values":              values,
	}
}

// An unsettled binding has no form yet, so nothing downstream may treat it as one it
// recognises: reading it as empty would silently drop what the apply is about to bind.
func TestResolvedMarksAnUnknownFormUnsettled(t *testing.T) {
	for name, binding := range unknownBindings() {
		t.Run(name, func(t *testing.T) {
			resolved := binding.resolved()

			assert.True(t, resolved.unsettled, "an unknown form must resolve as unsettled")
			assert.Nil(t, resolved.Value)
			assert.Empty(t, resolved.ArrayValue)
		})
	}
}

// Reconciliation hands back the prior spelling only when the two mean the same thing. An
// unsettled binding means nothing yet, so it can't be shown to match anything — including
// another unsettled one, which may land somewhere else entirely.
func TestUnsettledBindingsNeverMeanTheSame(t *testing.T) {
	settled := bindValueLiteral("high")

	for name, binding := range unknownBindings() {
		t.Run(name, func(t *testing.T) {
			assert.False(t, binding.meansTheSameAs(settled))
			assert.False(t, settled.meansTheSameAs(binding))
			assert.False(t, binding.meansTheSameAs(binding), "not even against itself")

			assert.Equal(t, settled, settled.ReconcileSpelling(binding),
				"an unsettled prior must not replace what came back")
		})
	}
}

// A form the config wrote wins over one it left for Terraform to settle, the same way a set
// shorthand wins over a set long form. Every shorthand has to behave the same here: reading
// one as unsettled because a form nobody wrote is unknown would drop a value the config gave.
func TestASetFormBeatsAnUnknownOne(t *testing.T) {
	t.Run("value_literal", func(t *testing.T) {
		binding := bindValueLiteral("high")
		binding.ArrayValue = types.ListUnknown(ParamBindingValueType())

		resolved := binding.resolved()

		assert.False(t, resolved.unsettled)
		if assert.NotNil(t, resolved.Value) {
			assert.Equal(t, "high", resolved.Value.Literal.ValueString())
		}
	})

	t.Run("values", func(t *testing.T) {
		binding := bindValues("a", "b")
		binding.ArrayValue = types.ListUnknown(ParamBindingValueType())

		resolved := binding.resolved()

		assert.False(t, resolved.unsettled)
		assert.Len(t, resolved.ArrayValue, 2)
	})
}

// An unsettled binding reaching a payload would send the API an empty binding, which is not
// what the config asked for. It can't happen — Terraform settles every optional attribute
// before an apply — but the payload must be harmless rather than wrong if it ever does.
func TestUnsettledBindingSendsNothing(t *testing.T) {
	for name, binding := range unknownBindings() {
		t.Run(name, func(t *testing.T) {
			payload := binding.ToPayload()

			assert.Nil(t, payload.Value)
			if assert.NotNil(t, payload.ArrayValue) {
				assert.Empty(t, *payload.ArrayValue)
			}
		})
	}
}

// TrimAppendedEmpty drops the padding the API adds to a step that gained params. An unknown
// binding is not padding: it's one the apply is about to fill in, and trimming it would
// change what the config asked for.
func TestTrimAppendedEmptyKeepsAnUnknownBinding(t *testing.T) {
	unknown := NullParamBinding()
	unknown.ArrayValue = types.ListUnknown(ParamBindingValueType())

	bindings := IncidentEngineParamBindings{bindValueLiteral("high"), unknown}

	assert.Equal(t, bindings, bindings.TrimAppendedEmpty(1),
		"an unknown trailing binding must survive the trim")
}
