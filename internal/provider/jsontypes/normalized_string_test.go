package jsontypes

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizedString_StringSemanticEquals(t *testing.T) {
	tests := []struct {
		name     string
		current  NormalizedString
		new      NormalizedString
		expected bool
	}{
		{
			name:     "identical JSON",
			current:  NewNormalizedStringValue(`{"a":1,"b":2}`),
			new:      NewNormalizedStringValue(`{"a":1,"b":2}`),
			expected: true,
		},
		{
			name:     "key-reordered JSON is equal",
			current:  NewNormalizedStringValue(`{"a":1,"b":2}`),
			new:      NewNormalizedStringValue(`{"b":2,"a":1}`),
			expected: true,
		},
		{
			name:     "unicode-escaped vs raw greater-than is equal",
			current:  NewNormalizedStringValue(`{"label":"Alert \u003e Title"}`),
			new:      NewNormalizedStringValue(`{"label":"Alert > Title"}`),
			expected: true,
		},
		{
			name:     "unicode-escaped vs raw less-than is equal",
			current:  NewNormalizedStringValue(`{"label":"a \u003c b"}`),
			new:      NewNormalizedStringValue(`{"label":"a < b"}`),
			expected: true,
		},
		{
			name:     "unicode-escaped vs raw ampersand is equal",
			current:  NewNormalizedStringValue(`{"label":"a \u0026 b"}`),
			new:      NewNormalizedStringValue(`{"label":"a & b"}`),
			expected: true,
		},
		{
			name:     "whitespace-different JSON is equal",
			current:  NewNormalizedStringValue(`{"a":1,"b":[1,2,3]}`),
			new:      NewNormalizedStringValue("{\n  \"a\": 1,\n  \"b\": [ 1, 2, 3 ]\n}"),
			expected: true,
		},
		{
			name:     "nested key-reordered JSON is equal",
			current:  NewNormalizedStringValue(`{"outer":{"x":1,"y":2}}`),
			new:      NewNormalizedStringValue(`{"outer":{"y":2,"x":1}}`),
			expected: true,
		},
		{
			name:     "semantically different JSON is not equal",
			current:  NewNormalizedStringValue(`{"a":1}`),
			new:      NewNormalizedStringValue(`{"a":2}`),
			expected: false,
		},
		{
			name:     "non-JSON identical string is equal",
			current:  NewNormalizedStringValue("alert.title"),
			new:      NewNormalizedStringValue("alert.title"),
			expected: true,
		},
		{
			name:     "non-JSON different string is not equal",
			current:  NewNormalizedStringValue("alert.title"),
			new:      NewNormalizedStringValue("alert.description"),
			expected: false,
		},
		{
			name:     "one JSON one not is not equal",
			current:  NewNormalizedStringValue(`{"a":1}`),
			new:      NewNormalizedStringValue("alert.title"),
			expected: false,
		},
		{
			name:     "JSON string scalar vs reordered object is not equal",
			current:  NewNormalizedStringValue(`"hello"`),
			new:      NewNormalizedStringValue(`{"a":1}`),
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

func TestNormalizedString_StringSemanticEquals_NullAndUnknown(t *testing.T) {
	ctx := context.Background()

	// Two null values are equal.
	equal, diags := NewNormalizedStringNull().StringSemanticEquals(ctx, NewNormalizedStringNull())
	require.False(t, diags.HasError())
	assert.True(t, equal)

	// Two unknown values are equal.
	equal, diags = NewNormalizedStringUnknown().StringSemanticEquals(ctx, NewNormalizedStringUnknown())
	require.False(t, diags.HasError())
	assert.True(t, equal)

	// Null vs known is not equal.
	equal, diags = NewNormalizedStringNull().StringSemanticEquals(ctx, NewNormalizedStringValue(`{"a":1}`))
	require.False(t, diags.HasError())
	assert.False(t, equal)
}

func TestNormalizedString_StringSemanticEquals_WrongType(t *testing.T) {
	// Passing a plain basetypes.StringValue (not a NormalizedString) should
	// produce an error diagnostic rather than panicking.
	_, diags := NewNormalizedStringValue(`{"a":1}`).StringSemanticEquals(
		context.Background(),
		basetypes.NewStringValue(`{"a":1}`),
	)
	assert.True(t, diags.HasError())
}

func TestNormalizedStringType_ValueFromTerraform(t *testing.T) {
	ctx := context.Background()
	typ := NormalizedStringType{}

	val := NewNormalizedStringValue(`{"a":1}`)
	tfVal, err := val.ToTerraformValue(ctx)
	require.NoError(t, err)

	got, err := typ.ValueFromTerraform(ctx, tfVal)
	require.NoError(t, err)

	normalized, ok := got.(NormalizedString)
	require.True(t, ok, "expected NormalizedString, got %T", got)
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
