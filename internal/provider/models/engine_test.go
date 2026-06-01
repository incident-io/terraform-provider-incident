package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/jsontypes"
)

func TestIncidentEngineParamBindingValue_JSONOrdering(t *testing.T) {
	tests := []struct {
		name               string
		apiJSON            string
		expectedNormalized string
		notJSON            bool
		description        string
	}{
		{
			name:               "keys_should_be_sorted_lexicographically",
			apiJSON:            `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"name":"description","label":"Payload → Description","missing":false}}]}]}`,
			expectedNormalized: `{"content":[{"content":[{"attrs":{"label":"Payload → Description","missing":false,"name":"description"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`,
			description:        "JSON keys should be sorted lexicographically for consistency",
		},
		{
			name:               "already_sorted_should_remain_unchanged",
			apiJSON:            `{"content":[{"content":[{"attrs":{"label":"Payload → Description","missing":false,"name":"description"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`,
			expectedNormalized: `{"content":[{"content":[{"attrs":{"label":"Payload → Description","missing":false,"name":"description"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`,
			description:        "JSON with lexicographically sorted keys should remain unchanged",
		},
		{
			name:               "nested_objects_keys_sorted_lexicographically",
			apiJSON:            `{"type":"doc","content":[{"type":"paragraph","content":[{"attrs":{"name":"description","missing":false,"label":"Payload → Description"},"type":"varSpec"}]}]}`,
			expectedNormalized: `{"content":[{"content":[{"attrs":{"label":"Payload → Description","missing":false,"name":"description"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`,
			description:        "All nested object keys should be sorted lexicographically",
		},
		{
			name:               "plain_string",
			apiJSON:            `"plain string"`,
			expectedNormalized: `"plain string"`,
			description:        "Plain string should remain unchanged",
		},
		{
			name:    "html_chars_escaped_in_state",
			apiJSON: `{"label":"Alert > Title & <foo>"}`,
			// State normalisation keeps HTML escaping ON (matching jsonencode),
			// so the value written to state escapes '>', '&' and '<'. Semantic
			// equality (jsontypes.NormalizedString) means this still does not
			// produce a diff against a raw-'>' configured value.
			expectedNormalized: `{"label":"Alert \u003e Title \u0026 \u003cfoo\u003e"}`,
			description:        "State normalisation HTML-escapes to stay byte-stable for jsonencode users",
		},
		{
			name:               "non_json_reference_unchanged",
			apiJSON:            `alert.title`,
			expectedNormalized: `alert.title`,
			notJSON:            true,
			description:        "Non-JSON literals (references) should be left untouched",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiResponse := client.EngineParamBindingValueV2{
				Literal: &tt.apiJSON,
			}

			result := IncidentEngineParamBindingValue{}.FromAPI(apiResponse)

			currentResult := result.Literal.ValueString()

			// Verify that JSON normalization is working
			assert.Equal(t, tt.expectedNormalized, currentResult,
				"JSON should be normalized with lexicographically sorted keys")

			// Verify the JSON content is semantically equivalent
			if !tt.notJSON {
				assert.JSONEq(t, tt.apiJSON, tt.expectedNormalized,
					"API JSON and expected normalized JSON should be semantically equivalent")
			}
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
	// provider would store after re-encoding (escaping disabled, keys sorted).
	planned := jsontypes.NewNormalizedStringValue(`{"label":"Alert -> Title","name":"alert.title"}`)
	applied := jsontypes.NewNormalizedStringValue(`{"name":"alert.title","label":"Alert -> Title"}`)

	equal, diags := planned.StringSemanticEquals(ctx, applied)
	require.False(t, diags.HasError())
	assert.True(t, equal, "key-reordered literal should be semantically equal")

	// HTML-escaped vs raw should also compare equal.
	escaped := jsontypes.NewNormalizedStringValue(`{"label":"Alert \u003e Title"}`)
	raw := jsontypes.NewNormalizedStringValue(`{"label":"Alert > Title"}`)
	equal, diags = escaped.StringSemanticEquals(ctx, raw)
	require.False(t, diags.HasError())
	assert.True(t, equal, "HTML-escaped and raw literals should be semantically equal")
}
