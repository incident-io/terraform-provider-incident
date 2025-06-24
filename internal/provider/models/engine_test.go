package models

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func TestIncidentEngineParamBindingValue_JSONOrdering(t *testing.T) {
	tests := []struct {
		name               string
		apiJSON            string
		expectedNormalized string
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
			assert.JSONEq(t, tt.apiJSON, tt.expectedNormalized,
				"API JSON and expected normalized JSON should be semantically equivalent")
		})
	}
}
