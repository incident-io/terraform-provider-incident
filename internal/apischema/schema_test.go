package apischema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnumValues_ArrayItems covers the case this helper exists for: runs_on_incident_modes
// is ArrayOf(WorkflowIncidentMode) in the API design, so the enum lands on the items
// rather than the property, and reading only the property's own enum finds nothing.
func TestEnumValues_ArrayItems(t *testing.T) {
	assert.Equal(t,
		[]string{"standard", "test", "retrospective"},
		EnumValues("WorkflowV2", "runs_on_incident_modes"))
}

func TestEnumValues_Scalar(t *testing.T) {
	assert.Equal(t,
		[]string{"active", "disabled", "draft", "error"},
		EnumValues("WorkflowV2", "state"))
}

// A property that genuinely has no enum must come back empty rather than panicking, so
// callers can tell the difference between "fixed set" and "dynamic".
func TestEnumValues_NotAnEnum(t *testing.T) {
	assert.Empty(t, EnumValues("WorkflowV2", "once_for"))
}

func TestEnumValuesDescription(t *testing.T) {
	description := EnumValuesDescription("ExpressionOperationV2", "operation_type")

	assert.Equal(t,
		"The type of the operation. Possible values are: `navigate`, `filter`, `concatenate`, "+
			"`count`, `min`, `max`, `sum`, `random`, `first`, `parse`, `branches`, `cast`.",
		description)
}

// The runs_on_incident_modes docstring ends in a full stop and the state one doesn't, so
// the helper has to normalise or one of them renders "..".
func TestEnumValuesDescription_NoDoubleFullStop(t *testing.T) {
	for _, property := range []string{"runs_on_incident_modes", "state", "runs_on_incidents"} {
		description := EnumValuesDescription("WorkflowV2", property)

		assert.NotContains(t, description, "..", "property %s", property)
		assert.True(t, strings.Contains(description, "Possible values are:"), "property %s", property)
	}
}

// Documenting a property that isn't an enum is a mistake we want to hear about at
// startup, not one that silently ships a description promising values it can't list.
func TestEnumValuesDescription_PanicsWithoutEnum(t *testing.T) {
	require.Panics(t, func() {
		EnumValuesDescription("WorkflowV2", "once_for")
	})
}

func TestDynamicValuesDescription(t *testing.T) {
	description := DynamicValuesDescription("TriggerSlimV2", "name", "Look it up in the dashboard.")

	assert.Equal(t,
		"Unique name of the trigger. This isn't a fixed list - it depends on your configuration and "+
			"grows over time, so don't treat a value that isn't listed here as unsupported. "+
			"Look it up in the dashboard.",
		description)
}

// ConditionV2.operation has no description in the schema. Prefixing the caveat with an
// empty string used to render a stray ". " at the front of the docs.
func TestDescribeDynamicValues_EmptyDescription(t *testing.T) {
	assert.Empty(t, Docstring("ConditionV2", "operation"),
		"this test is meaningless if the schema has since gained a description here")

	description := DescribeDynamicValues(Docstring("ConditionV2", "operation"), "Look it up.")

	assert.False(t, strings.HasPrefix(description, "."), "got: %s", description)
	assert.True(t, strings.HasPrefix(description, "This isn't a fixed list"), "got: %s", description)
}
