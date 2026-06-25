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

// TestIncidentEngineExpressions_ReorderToMatch covers the realignment that
// keeps the list-typed `expressions` attribute stable even though the API does
// not preserve the order of expressions in its responses. Without it, switching
// `expressions` from a set to a list (needed so a mix of expressions with and
// without an optional else_branch passes `terraform validate` — RESP-17992)
// would resurface the ordering drift that caused the v5.21.1 revert.
func TestIncidentEngineExpressions_ReorderToMatch(t *testing.T) {
	expr := func(reference string) IncidentEngineExpression {
		return IncidentEngineExpression{
			Reference: types.StringValue(reference),
			Label:     types.StringValue(reference),
		}
	}

	references := func(exprs IncidentEngineExpressions) []string {
		out := make([]string, 0, len(exprs))
		for _, e := range exprs {
			out = append(out, e.Reference.ValueString())
		}
		return out
	}

	t.Run("reorders API response to match desired order", func(t *testing.T) {
		// Desired (planned/prior) order.
		desired := IncidentEngineExpressions{expr("a"), expr("b"), expr("c")}
		// API returns the same expressions in a different order.
		api := IncidentEngineExpressions{expr("c"), expr("a"), expr("b")}

		got := api.ReorderToMatch(desired)
		assert.Equal(t, []string{"a", "b", "c"}, references(got))
	})

	t.Run("appends expressions missing from desired in API order", func(t *testing.T) {
		desired := IncidentEngineExpressions{expr("a")}
		api := IncidentEngineExpressions{expr("b"), expr("a"), expr("c")}

		got := api.ReorderToMatch(desired)
		// "a" first (matched), then the unmatched ones in their API order.
		assert.Equal(t, []string{"a", "b", "c"}, references(got))
	})

	t.Run("empty desired returns API order unchanged", func(t *testing.T) {
		api := IncidentEngineExpressions{expr("b"), expr("a")}

		got := api.ReorderToMatch(IncidentEngineExpressions{})
		assert.Equal(t, []string{"b", "a"}, references(got))
	})

	t.Run("empty API returns empty", func(t *testing.T) {
		desired := IncidentEngineExpressions{expr("a"), expr("b")}

		got := IncidentEngineExpressions{}.ReorderToMatch(desired)
		assert.Empty(t, got)
	})
}
