package models

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/jsontypes"
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
