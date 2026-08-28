package models

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/jsontypes"
)

// TestIncidentEngineParamBindingValue_FromAPIVerbatim asserts that FromAPI
// stores the API's literal byte-for-byte. We deliberately do NOT re-encode or
// re-order the literal: jsontypes.NormalizedJSONOrString's semantic equality
// absorbs any key-ordering or HTML-escaping differences against the user's
// configured value, so there's no reason to mangle the bytes here.
func TestIncidentEngineParamBindingValue_FromAPIVerbatim(t *testing.T) {
	tests := []struct {
		name    string
		apiJSON string
	}{
		{
			name:    "unsorted_keys_left_as_is",
			apiJSON: `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"name":"description","label":"Payload → Description","missing":false}}]}]}`,
		},
		{
			name:    "html_chars_left_as_is",
			apiJSON: `{"label":"Alert > Title & <foo>"}`,
		},
		{
			name:    "plain_string",
			apiJSON: `"plain string"`,
		},
		{
			name:    "non_json_reference",
			apiJSON: `alert.title`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiResponse := client.EngineParamBindingValueV2{
				Literal: &tt.apiJSON,
			}

			result := IncidentEngineParamBindingValue{}.FromAPI(apiResponse)

			assert.Equal(t, tt.apiJSON, result.Literal.ValueString(),
				"FromAPI should store the literal verbatim")
		})
	}
}

// TestIncidentEngineParamBindingValue_SemanticEquality reproduces the Fanvue
// scenario at the model layer: a literal supplied with raw HTML characters
// (e.g. from CDKTF JSON.stringify) must be considered semantically equal to the
// HTML-escaped form Terraform would otherwise compare against, preventing the
// "Provider produced inconsistent result after apply" error.
func TestIncidentEngineParamBindingValue_SemanticEquality(t *testing.T) {
	ctx := context.Background()

	// What the user configured (raw '>', not HTML-escaped) and what the
	// provider might receive back with keys in a different order.
	planned := jsontypes.NewNormalizedJSONOrStringValue(`{"label":"Alert -> Title","name":"alert.title"}`)
	applied := jsontypes.NewNormalizedJSONOrStringValue(`{"name":"alert.title","label":"Alert -> Title"}`)

	equal, diags := planned.StringSemanticEquals(ctx, applied)
	require.False(t, diags.HasError())
	assert.True(t, equal, "key-reordered literal should be semantically equal")

	// HTML-escaped vs raw should also compare equal.
	escaped := jsontypes.NewNormalizedJSONOrStringValue(`{"label":"Alert \u003e Title"}`)
	raw := jsontypes.NewNormalizedJSONOrStringValue(`{"label":"Alert > Title"}`)
	equal, diags = escaped.StringSemanticEquals(ctx, raw)
	require.False(t, diags.HasError())
	assert.True(t, equal, "HTML-escaped and raw literals should be semantically equal")
}

func TestIncidentEngineParamBindings_TrimAppendedEmpty(t *testing.T) {
	literal := func(s string) IncidentEngineParamBinding {
		return IncidentEngineParamBinding{
			ArrayValue: []IncidentEngineParamBindingValue{
				{Literal: jsontypes.NewNormalizedJSONOrStringValue(s)},
			},
		}
	}
	ref := func(s string) IncidentEngineParamBinding {
		return IncidentEngineParamBinding{
			Value: &IncidentEngineParamBindingValue{Reference: types.StringValue(s)},
		}
	}
	empty := IncidentEngineParamBinding{}

	tests := []struct {
		name     string
		bindings IncidentEngineParamBindings
		priorLen int
		want     IncidentEngineParamBindings
	}{
		{
			name:     "drops_bindings_appended_by_a_step_gaining_params",
			bindings: IncidentEngineParamBindings{ref("incident"), literal("Write postmortem"), empty, empty, empty, empty},
			priorLen: 3,
			want:     IncidentEngineParamBindings{ref("incident"), literal("Write postmortem"), empty},
		},
		{
			name:     "keeps_a_configured_trailing_empty",
			bindings: IncidentEngineParamBindings{ref("incident"), literal("Write postmortem"), empty},
			priorLen: 3,
			want:     IncidentEngineParamBindings{ref("incident"), literal("Write postmortem"), empty},
		},
		{
			name:     "no_op_when_lengths_already_match",
			bindings: IncidentEngineParamBindings{ref("incident"), literal("Write postmortem")},
			priorLen: 2,
			want:     IncidentEngineParamBindings{ref("incident"), literal("Write postmortem")},
		},
		{
			name:     "keeps_extra_bindings_that_hold_data",
			bindings: IncidentEngineParamBindings{ref("incident"), literal("Write postmortem"), empty, literal("set-elsewhere")},
			priorLen: 3,
			want:     IncidentEngineParamBindings{ref("incident"), literal("Write postmortem"), empty, literal("set-elsewhere")},
		},
		{
			name:     "stops_trimming_at_populated_binding",
			bindings: IncidentEngineParamBindings{ref("incident"), literal("t"), empty, literal("data"), empty},
			priorLen: 2,
			want:     IncidentEngineParamBindings{ref("incident"), literal("t"), empty, literal("data")},
		},
		{
			name:     "leaves_shorter_responses_alone",
			bindings: IncidentEngineParamBindings{ref("incident")},
			priorLen: 3,
			want:     IncidentEngineParamBindings{ref("incident")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.bindings.TrimAppendedEmpty(tc.priorLen))
		})
	}
}

// TestIncidentEngineConditionGroups_ReconcileOperations covers ONC-12602: restore
// the planned value for a known alias, leave everything else alone.
func TestIncidentEngineConditionGroups_ReconcileOperations(t *testing.T) {
	condition := func(subject, operation string) IncidentEngineCondition {
		return IncidentEngineCondition{
			Subject:   types.StringValue(subject),
			Operation: types.StringValue(operation),
		}
	}
	groups := func(conds ...IncidentEngineCondition) IncidentEngineConditionGroups {
		return IncidentEngineConditionGroups{{Conditions: IncidentEngineConditions(conds)}}
	}

	tests := []struct {
		name    string
		applied IncidentEngineConditionGroups
		plan    IncidentEngineConditionGroups
		want    string
	}{
		{
			name:    "restores planned one_of when API returns contains_one_of",
			applied: groups(condition("alert.attributes.01ABC", "contains_one_of")),
			plan:    groups(condition("alert.attributes.01ABC", "one_of")),
			want:    "one_of",
		},
		{
			name:    "restores planned contains when API returns name_contains",
			applied: groups(condition("incident.custom_field.01ABC", "name_contains")),
			plan:    groups(condition("incident.custom_field.01ABC", "contains")),
			want:    "contains",
		},
		{
			name:    "leaves matching operations untouched",
			applied: groups(condition("alert.attributes.01ABC", "one_of")),
			plan:    groups(condition("alert.attributes.01ABC", "one_of")),
			want:    "one_of",
		},
		{
			// `contains` is canonical on String, so the API returns it unchanged.
			name:    "leaves an alias that is canonical for the subject untouched",
			applied: groups(condition("alert.attributes.01ABC", "contains")),
			plan:    groups(condition("alert.attributes.01ABC", "contains")),
			want:    "contains",
		},
		{
			name:    "does not mask an unrelated divergence",
			applied: groups(condition("alert.attributes.01ABC", "is_set")),
			plan:    groups(condition("alert.attributes.01ABC", "one_of")),
			want:    "is_set",
		},
		{
			name:    "does not reconcile across a different subject",
			applied: groups(condition("alert.attributes.02XYZ", "contains_one_of")),
			plan:    groups(condition("alert.attributes.01ABC", "one_of")),
			want:    "contains_one_of",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.applied.ReconcileOperations(tc.plan)
			got := tc.applied[0].Conditions[0].Operation.ValueString()
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestIncidentEngineConditionGroups_ReconcileOperationsMismatchedLengths asserts
// reconciliation is safe when the plan and read-back have different numbers of
// groups or conditions.
func TestIncidentEngineConditionGroups_ReconcileOperationsMismatchedLengths(t *testing.T) {
	applied := IncidentEngineConditionGroups{
		{Conditions: IncidentEngineConditions{
			{Subject: types.StringValue("alert.attributes.01ABC"), Operation: types.StringValue("contains_one_of")},
			{Subject: types.StringValue("alert.attributes.02XYZ"), Operation: types.StringValue("contains_one_of")},
		}},
	}
	plan := IncidentEngineConditionGroups{
		{Conditions: IncidentEngineConditions{
			{Subject: types.StringValue("alert.attributes.01ABC"), Operation: types.StringValue("one_of")},
		}},
	}

	assert.NotPanics(t, func() { applied.ReconcileOperations(plan) })
	assert.Equal(t, "one_of", applied[0].Conditions[0].Operation.ValueString())
	// The second condition has no planned counterpart, so it is left as-is.
	assert.Equal(t, "contains_one_of", applied[0].Conditions[1].Operation.ValueString())

	// A nil plan must be a no-op rather than a panic.
	assert.NotPanics(t, func() { applied.ReconcileOperations(nil) })
}

// TestIncidentEngineExpressions_ReconcileOperationsCorrelatesByReference asserts
// filter conditions still reconcile when the API returns expressions in a
// different order to the plan.
func TestIncidentEngineExpressions_ReconcileOperationsCorrelatesByReference(t *testing.T) {
	expression := func(reference, operation string) IncidentEngineExpression {
		return IncidentEngineExpression{
			Reference: types.StringValue(reference),
			Operations: IncidentEngineExpressionOperations{
				{
					OperationType: types.StringValue("filter"),
					Filter: &IncidentEngineExpressionFilterOpts{
						ConditionGroups: IncidentEngineConditionGroups{
							{Conditions: IncidentEngineConditions{
								{
									Subject:   types.StringValue("alert.attributes.01ABC"),
									Operation: types.StringValue(operation),
								},
							}},
						},
					},
				},
			},
		}
	}
	operationOf := func(e IncidentEngineExpression) string {
		return e.Operations[0].Filter.ConditionGroups[0].Conditions[0].Operation.ValueString()
	}

	applied := IncidentEngineExpressions{
		expression("first", "contains_one_of"),
		expression("second", "contains_one_of"),
	}
	// The plan holds the same expressions in the opposite order.
	plan := IncidentEngineExpressions{
		expression("second", "one_of"),
		expression("first", "one_of"),
	}

	applied.ReconcileOperations(plan)

	assert.Equal(t, "one_of", operationOf(applied[0]), "first expression reconciled")
	assert.Equal(t, "one_of", operationOf(applied[1]), "second expression reconciled")

	// An unplanned expression is left alone, and a nil plan must not panic.
	unplanned := IncidentEngineExpressions{expression("third", "contains_one_of")}
	assert.NotPanics(t, func() { unplanned.ReconcileOperations(plan) })
	assert.Equal(t, "contains_one_of", operationOf(unplanned[0]))
	assert.NotPanics(t, func() { unplanned.ReconcileOperations(nil) })
}

func TestIncidentEngineParamBinding_IsEmpty(t *testing.T) {
	assert.True(t, IncidentEngineParamBinding{}.IsEmpty(), "zero binding is empty")
	assert.True(t, IncidentEngineParamBinding{ArrayValue: []IncidentEngineParamBindingValue{}}.IsEmpty(),
		"empty (non-nil) array_value is still empty")

	assert.False(t, IncidentEngineParamBinding{
		Value: &IncidentEngineParamBindingValue{},
	}.IsEmpty(), "a present value object is not empty, even with zero fields")
	assert.False(t, IncidentEngineParamBinding{
		ArrayValue: []IncidentEngineParamBindingValue{{Reference: types.StringValue("incident")}},
	}.IsEmpty(), "a populated array_value is not empty")
}

// TestIncidentEngineExpressionOperation_CastRoundTrip covers the cast operation,
// which the dashboard exports but the provider used to drop: without the cast
// options the API rejects the payload with
// "expressions.N.operations.M.cast: Must be provided".
func TestIncidentEngineExpressionOperation_CastRoundTrip(t *testing.T) {
	operations := IncidentEngineExpressionOperations{
		{
			OperationType: types.StringValue("cast"),
			Cast: &IncidentEngineExpressionCastOpts{
				Returns: IncidentEngineReturnsMeta{
					Array: types.BoolValue(false),
					Type:  types.StringValue("CatalogEntry[\"01ABC\"]"),
				},
			},
		},
	}

	payload := operations.toPayload()
	require.Len(t, payload, 1)
	require.NotNil(t, payload[0].Cast, "cast options must reach the API")
	assert.Equal(t, "CatalogEntry[\"01ABC\"]", payload[0].Cast.Returns.Type)
	assert.False(t, payload[0].Cast.Returns.Array)

	// The API doesn't echo cast options back: it reports the cast target as the
	// operation's own returns, so that's what we rebuild the block from.
	applied := IncidentEngineExpressionOperation{}.FromAPI([]client.ExpressionOperationV2{
		{
			OperationType: client.ExpressionOperationV2OperationTypeCast,
			Returns:       client.ReturnsMetaV2{Array: false, Type: "CatalogEntry[\"01ABC\"]"},
		},
	})

	assert.Equal(t, operations, IncidentEngineExpressionOperations(applied))
}

// TestIncidentEngineExpressionOperation_FromAPINonCast asserts we only synthesise
// a cast block for cast operations: every other operation carries returns too, and
// a spurious block would diff against config forever.
func TestIncidentEngineExpressionOperation_FromAPINonCast(t *testing.T) {
	applied := IncidentEngineExpressionOperation{}.FromAPI([]client.ExpressionOperationV2{
		{
			OperationType: client.ExpressionOperationV2OperationTypeNavigate,
			Navigate:      &client.ExpressionNavigateOptsV2{Reference: "catalog_attribute[\"01ABC\"]"},
			Returns:       client.ReturnsMetaV2{Array: true, Type: "CatalogEntry[\"01DEF\"]"},
		},
	})

	require.Len(t, applied, 1)
	assert.Nil(t, applied[0].Cast)
}
