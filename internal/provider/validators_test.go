package provider

import (
	"context"
	"testing"

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
