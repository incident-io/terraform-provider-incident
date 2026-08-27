package apischema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnumValues(t *testing.T) {
	assert.Equal(t,
		[]string{"active", "disabled", "draft", "error"},
		EnumValues("WorkflowV2", "state"))
}

// TestEnumValues_ArrayItems covers the case the items fallback exists for:
// runs_on_incident_modes is ArrayOf(WorkflowIncidentMode) in the API design, so the enum
// lands on the items rather than the property, and reading only the property's own enum
// finds nothing.
func TestEnumValues_ArrayItems(t *testing.T) {
	assert.Equal(t,
		[]string{"standard", "test", "retrospective"},
		EnumValues("WorkflowV2", "runs_on_incident_modes"))
}

// A property that genuinely has no enum must come back empty rather than panicking, so
// callers can tell the difference between a fixed set of values and a free-form one.
func TestEnumValues_NotAnEnum(t *testing.T) {
	assert.Empty(t, EnumValues("WorkflowV2", "once_for"))
}

func TestEnumValuesDescription(t *testing.T) {
	assert.Equal(t,
		"The type of the operation. Possible values are: `navigate`, `filter`, `concatenate`, "+
			"`count`, `min`, `max`, `sum`, `random`, `first`, `parse`, `branches`, `cast`.",
		EnumValuesDescription("ExpressionOperationV2", "operation_type"))
}

// Docstrings end in a full stop (runs_on_incident_modes), a question mark
// (runs_on_incidents) or nothing at all (state), so the helper has to punctuate the join
// itself or they render "incidents.. Possible values" and "applied to?. Possible values".
func TestEnumValuesDescription_PunctuatesTheJoin(t *testing.T) {
	for property, expected := range map[string]string{
		"runs_on_incident_modes": "incidents. Possible values are: `standard`, `test`, `retrospective`.",
		"runs_on_incidents":      "applied to? Possible values are: `newly_created`, `newly_created_and_active`.",
		"state":                  "workflow is in. Possible values are: `active`, `disabled`, `draft`, `error`.",
	} {
		description := EnumValuesDescription("WorkflowV2", property)

		assert.True(t, strings.HasSuffix(description, expected),
			"property %s: wanted a description ending %q, got %q", property, expected, description)
	}
}

// A docstring of several lines is a bulleted list explaining each value. Appending inline
// would tack the values onto the end of the last bullet, where they'd read as part of it.
func TestEnumValuesDescription_MultiLineDocstring(t *testing.T) {
	require.Contains(t, Docstring("EscalationPathNodeV2", "type"), "\n",
		"this test is meaningless unless the docstring is still a multi-line list")

	description := EnumValuesDescription("EscalationPathNodeV2", "type")

	assert.Contains(t, description, "\n\nPossible values are: `if_else`,")
}

// Some enums carry the empty string so the field can be cleared. Rendering it as a code
// span would leave an empty pair of backticks in the docs.
func TestEnumValuesDescription_SkipsEmptyValue(t *testing.T) {
	require.Contains(t, EnumValues("EscalationPathTargetV2", "schedule_mode"), "",
		"this test is meaningless if the schema no longer allows an empty schedule_mode")

	description := EnumValuesDescription("EscalationPathTargetV2", "schedule_mode")

	assert.NotContains(t, description, "``")
	assert.True(t, strings.HasSuffix(description, "`next_on_call`."), "got: %s", description)
}

// Documenting a property that isn't an enum is a mistake we want to hear about at startup,
// not one that silently ships a description promising values it can't list.
func TestEnumValuesDescription_PanicsWithoutEnum(t *testing.T) {
	require.Panics(t, func() {
		EnumValuesDescription("WorkflowV2", "once_for")
	})
}

// Some attributes are described in the provider's own words rather than the schema's. An
// empty description must not render a stray ". " at the front.
func TestDescribeEnumValues(t *testing.T) {
	assert.Equal(t,
		"How the alert's severity is decided. Possible values are: `first-wins`, `max`.",
		DescribeEnumValues("How the alert's severity is decided.", "AlertRouteSeverityBindingV3", "merge_strategy"))

	assert.Equal(t,
		"Possible values are: `first-wins`, `max`.",
		DescribeEnumValues("", "AlertRouteSeverityBindingV3", "merge_strategy"))
}
