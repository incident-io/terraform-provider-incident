package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

func TestRFC3339TimestampValidator(t *testing.T) {
	testCases := []struct {
		name          string
		value         string
		expectedError bool
	}{
		{
			name:          "valid RFC3339 timestamp",
			value:         "2023-04-20T15:30:45Z",
			expectedError: false,
		},
		{
			name:          "another valid RFC3339 timestamp with timezone offset",
			value:         "2023-04-20T15:30:45+01:00",
			expectedError: false,
		},
		{
			name:          "invalid month",
			value:         "2023-13-20T15:30:45Z",
			expectedError: true,
		},
		{
			name:          "invalid day",
			value:         "2023-04-32T15:30:45Z",
			expectedError: true,
		},
		{
			name:          "invalid hours",
			value:         "2023-04-20T25:30:45Z",
			expectedError: true,
		},
		{
			name:          "invalid minutes",
			value:         "2023-04-20T15:70:45Z",
			expectedError: true,
		},
		{
			name:          "invalid seconds",
			value:         "2023-04-20T15:30:65Z",
			expectedError: true,
		},
		{
			name:          "incorrect format",
			value:         "2023-04-20 15:30:45",
			expectedError: true,
		},
		{
			name:          "missing time part",
			value:         "2023-04-20",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := validator.StringRequest{
				ConfigValue: types.StringValue(tc.value),
			}
			response := validator.StringResponse{}

			v := RFC3339TimestampValidator{}
			v.ValidateString(context.Background(), request, &response)

			if tc.expectedError {
				assert.True(t, response.Diagnostics.HasError(), "expected validation to fail")
			} else {
				assert.False(t, response.Diagnostics.HasError(), "expected validation to succeed")
			}
		})
	}
}

func TestTimestampValidatorWithNullAndUnknown(t *testing.T) {
	v := RFC3339TimestampValidator{}

	// Test with null value
	nullRequest := validator.StringRequest{
		ConfigValue: types.StringNull(),
	}
	nullResponse := validator.StringResponse{}
	v.ValidateString(context.Background(), nullRequest, &nullResponse)
	assert.False(t, nullResponse.Diagnostics.HasError(), "null values should not cause validation errors")

	// Test with unknown value
	unknownRequest := validator.StringRequest{
		ConfigValue: types.StringUnknown(),
	}
	unknownResponse := validator.StringResponse{}
	v.ValidateString(context.Background(), unknownRequest, &unknownResponse)
	assert.False(t, unknownResponse.Diagnostics.HasError(), "unknown values should not cause validation errors")
}

func TestNonEmptyListValidator(t *testing.T) {
	testCases := []struct {
		name          string
		elements      []string
		expectedError bool
	}{
		{
			name:          "valid non-empty list with one element",
			elements:      []string{"item1"},
			expectedError: false,
		},
		{
			name:          "valid non-empty list with multiple elements",
			elements:      []string{"item1", "item2", "item3"},
			expectedError: false,
		},
		{
			name:          "empty list should fail validation",
			elements:      []string{},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert string slice to types.List
			elements := make([]types.String, len(tc.elements))
			for i, elem := range tc.elements {
				elements[i] = types.StringValue(elem)
			}

			listValue, diags := types.ListValue(types.StringType, []attr.Value{})
			if len(elements) > 0 {
				attrValues := make([]attr.Value, len(elements))
				for i, elem := range elements {
					attrValues[i] = elem
				}
				listValue, diags = types.ListValue(types.StringType, attrValues)
			}
			assert.False(t, diags.HasError(), "should not have diagnostics creating test list")

			request := validator.ListRequest{
				ConfigValue: listValue,
			}
			response := validator.ListResponse{}

			v := NonEmptyListValidator{
				AllowUnknownValues: true,
			}
			v.ValidateList(context.Background(), request, &response)

			if tc.expectedError {
				assert.True(t, response.Diagnostics.HasError(), "expected validation to fail")
				assert.Contains(t, response.Diagnostics.Errors()[0].Summary(), "List cannot be empty")
			} else {
				assert.False(t, response.Diagnostics.HasError(), "expected validation to succeed")
			}
		})
	}
}

func TestNonEmptyListValidatorWithNullAndUnknown(t *testing.T) {
	v := NonEmptyListValidator{
		AllowUnknownValues: true,
	}

	// Test with null value
	nullRequest := validator.ListRequest{
		ConfigValue: types.ListNull(types.StringType),
	}
	nullResponse := validator.ListResponse{}
	v.ValidateList(context.Background(), nullRequest, &nullResponse)
	assert.False(t, nullResponse.Diagnostics.HasError(), "null values should not cause validation errors")

	// Test with unknown value
	unknownRequest := validator.ListRequest{
		ConfigValue: types.ListUnknown(types.StringType),
	}
	unknownResponse := validator.ListResponse{}
	v.ValidateList(context.Background(), unknownRequest, &unknownResponse)
	assert.False(t, unknownResponse.Diagnostics.HasError(), "unknown values should not cause validation errors")
}

func TestCatalogTypeAttributeTypeValidator(t *testing.T) {
	testCases := []struct {
		name          string
		value         string
		expectedError bool
	}{
		// Valid primitive types
		{
			name:          "valid String type",
			value:         "String",
			expectedError: false,
		},
		{
			name:          "valid Text type",
			value:         "Text",
			expectedError: false,
		},
		{
			name:          "valid Number type",
			value:         "Number",
			expectedError: false,
		},
		{
			name:          "valid Bool type",
			value:         "Bool",
			expectedError: false,
		},
		// Valid Custom types
		{
			name:          "valid Custom type with simple name",
			value:         `Custom["Service"]`,
			expectedError: false,
		},
		{
			name:          "valid Custom type with spaces",
			value:         `Custom["Service Tier"]`,
			expectedError: false,
		},
		{
			name:          "valid Custom type with special characters",
			value:         `Custom["Service-Tier_123"]`,
			expectedError: false,
		},
		// Invalid types
		{
			name:          "invalid type - lowercase string",
			value:         "string",
			expectedError: true,
		},
		{
			name:          "invalid type - DateTime",
			value:         "DateTime",
			expectedError: true,
		},
		{
			name:          "invalid type - Integer",
			value:         "Integer",
			expectedError: true,
		},
		{
			name:          "invalid Custom type - single quotes",
			value:         `Custom['Service']`,
			expectedError: true,
		},
		{
			name:          "invalid Custom type - no quotes",
			value:         "Custom[Service]",
			expectedError: true,
		},
		{
			name:          "invalid Custom type - missing closing bracket",
			value:         `Custom["Service"`,
			expectedError: true,
		},
		{
			name:          "invalid Custom type - empty name",
			value:         `Custom[""]`,
			expectedError: true,
		},
		{
			name:          "completely invalid value",
			value:         "random-invalid-type",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := validator.StringRequest{
				ConfigValue: types.StringValue(tc.value),
			}
			response := validator.StringResponse{}

			v := CatalogTypeAttributeTypeValidator{}
			v.ValidateString(context.Background(), request, &response)

			if tc.expectedError {
				assert.True(t, response.Diagnostics.HasError(), "expected validation to fail")
				if response.Diagnostics.HasError() {
					assert.Contains(t, response.Diagnostics.Errors()[0].Summary(), "Invalid Catalog Type Attribute Type")
				}
			} else {
				assert.False(t, response.Diagnostics.HasError(), "expected validation to succeed")
			}
		})
	}
}

func TestCatalogTypeAttributeTypeValidatorWithNullAndUnknown(t *testing.T) {
	v := CatalogTypeAttributeTypeValidator{}

	// Test with null value
	nullRequest := validator.StringRequest{
		ConfigValue: types.StringNull(),
	}
	nullResponse := validator.StringResponse{}
	v.ValidateString(context.Background(), nullRequest, &nullResponse)
	assert.False(t, nullResponse.Diagnostics.HasError(), "null values should not cause validation errors")

	// Test with unknown value
	unknownRequest := validator.StringRequest{
		ConfigValue: types.StringUnknown(),
	}
	unknownResponse := validator.StringResponse{}
	v.ValidateString(context.Background(), unknownRequest, &unknownResponse)
	assert.False(t, unknownResponse.Diagnostics.HasError(), "unknown values should not cause validation errors")
}
