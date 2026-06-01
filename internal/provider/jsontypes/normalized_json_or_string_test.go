package jsontypes

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizedJSONOrString_StringSemanticEquals(t *testing.T) {
	tests := []struct {
		name     string
		current  NormalizedJSONOrString
		new      NormalizedJSONOrString
		expected bool
	}{
		{
			name:     "identical JSON",
			current:  NewNormalizedJSONOrStringValue(`{"a":1,"b":2}`),
			new:      NewNormalizedJSONOrStringValue(`{"a":1,"b":2}`),
			expected: true,
		},
		{
			name:     "key-reordered JSON is equal",
			current:  NewNormalizedJSONOrStringValue(`{"a":1,"b":2}`),
			new:      NewNormalizedJSONOrStringValue(`{"b":2,"a":1}`),
			expected: true,
		},
		{
			name:     "unicode-escaped vs raw greater-than is equal",
			current:  NewNormalizedJSONOrStringValue(`{"label":"Alert \u003e Title"}`),
			new:      NewNormalizedJSONOrStringValue(`{"label":"Alert > Title"}`),
			expected: true,
		},
		{
			name:     "unicode-escaped vs raw less-than is equal",
			current:  NewNormalizedJSONOrStringValue(`{"label":"a \u003c b"}`),
			new:      NewNormalizedJSONOrStringValue(`{"label":"a < b"}`),
			expected: true,
		},
		{
			name:     "unicode-escaped vs raw ampersand is equal",
			current:  NewNormalizedJSONOrStringValue(`{"label":"a \u0026 b"}`),
			new:      NewNormalizedJSONOrStringValue(`{"label":"a & b"}`),
			expected: true,
		},
		{
			name:     "whitespace-different JSON is equal",
			current:  NewNormalizedJSONOrStringValue(`{"a":1,"b":[1,2,3]}`),
			new:      NewNormalizedJSONOrStringValue("{\n  \"a\": 1,\n  \"b\": [ 1, 2, 3 ]\n}"),
			expected: true,
		},
		{
			name:     "nested key-reordered JSON is equal",
			current:  NewNormalizedJSONOrStringValue(`{"outer":{"x":1,"y":2}}`),
			new:      NewNormalizedJSONOrStringValue(`{"outer":{"y":2,"x":1}}`),
			expected: true,
		},
		{
			name:     "semantically different JSON is not equal",
			current:  NewNormalizedJSONOrStringValue(`{"a":1}`),
			new:      NewNormalizedJSONOrStringValue(`{"a":2}`),
			expected: false,
		},
		{
			name:     "non-JSON identical string is equal",
			current:  NewNormalizedJSONOrStringValue("alert.title"),
			new:      NewNormalizedJSONOrStringValue("alert.title"),
			expected: true,
		},
		{
			name:     "non-JSON different string is not equal",
			current:  NewNormalizedJSONOrStringValue("alert.title"),
			new:      NewNormalizedJSONOrStringValue("alert.description"),
			expected: false,
		},
		{
			name:     "one JSON one not is not equal",
			current:  NewNormalizedJSONOrStringValue(`{"a":1}`),
			new:      NewNormalizedJSONOrStringValue("alert.title"),
			expected: false,
		},
		{
			name:     "JSON string scalar vs reordered object is not equal",
			current:  NewNormalizedJSONOrStringValue(`"hello"`),
			new:      NewNormalizedJSONOrStringValue(`{"a":1}`),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			equal, diags := tt.current.StringSemanticEquals(context.Background(), tt.new)
			require.False(t, diags.HasError(), "unexpected diagnostics: %v", diags)
			assert.Equal(t, tt.expected, equal)
		})
	}
}

func TestNormalizedJSONOrString_StringSemanticEquals_NullAndUnknown(t *testing.T) {
	ctx := context.Background()

	// Two null values are equal.
	equal, diags := NewNormalizedJSONOrStringNull().StringSemanticEquals(ctx, NewNormalizedJSONOrStringNull())
	require.False(t, diags.HasError())
	assert.True(t, equal)

	// Two unknown values are equal.
	equal, diags = NewNormalizedJSONOrStringUnknown().StringSemanticEquals(ctx, NewNormalizedJSONOrStringUnknown())
	require.False(t, diags.HasError())
	assert.True(t, equal)

	// Null vs known is not equal.
	equal, diags = NewNormalizedJSONOrStringNull().StringSemanticEquals(ctx, NewNormalizedJSONOrStringValue(`{"a":1}`))
	require.False(t, diags.HasError())
	assert.False(t, equal)
}

func TestNormalizedJSONOrString_StringSemanticEquals_WrongType(t *testing.T) {
	// Passing a plain basetypes.StringValue (not a NormalizedJSONOrString) should
	// produce an error diagnostic rather than panicking.
	_, diags := NewNormalizedJSONOrStringValue(`{"a":1}`).StringSemanticEquals(
		context.Background(),
		basetypes.NewStringValue(`{"a":1}`),
	)
	assert.True(t, diags.HasError())
}

func TestNormalizedJSONOrStringType_ValueFromTerraform(t *testing.T) {
	ctx := context.Background()
	typ := NormalizedJSONOrStringType{}

	val := NewNormalizedJSONOrStringValue(`{"a":1}`)
	tfVal, err := val.ToTerraformValue(ctx)
	require.NoError(t, err)

	got, err := typ.ValueFromTerraform(ctx, tfVal)
	require.NoError(t, err)

	normalized, ok := got.(NormalizedJSONOrString)
	require.True(t, ok, "expected NormalizedJSONOrString, got %T", got)
	assert.Equal(t, `{"a":1}`, normalized.ValueString())
}

func TestNormaliseJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		expectErr bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "keys sorted and whitespace removed",
			input:    "{\n  \"b\": 2,\n  \"a\": 1\n}",
			expected: `{"a":1,"b":2}`,
		},
		{
			name:  "HTML chars escaped (matches jsonencode / preserves state)",
			input: `{"label":"Alert > Title & <b>"}`,
			// NormaliseJSON keeps HTML escaping ON so the form written to state
			// matches what HCL's jsonencode produces.
			expected: `{"label":"Alert \u003e Title \u0026 \u003cb\u003e"}`,
		},
		{
			name:      "invalid JSON returns error",
			input:     "alert.title",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormaliseJSON(tt.input)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}
