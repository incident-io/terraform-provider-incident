package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type NonEmptyListValidator struct {
	AllowUnknownValues bool
}

func (n NonEmptyListValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || (n.AllowUnknownValues && req.ConfigValue.IsUnknown()) {
		return
	} else if len(req.ConfigValue.Elements()) == 0 {
		resp.Diagnostics.AddError("List cannot be empty", fmt.Sprintf("%s cannot be empty", req.Path.String()))
		return
	}
}

func (n NonEmptyListValidator) Description(ctx context.Context) string {
	return "List cannot be empty"
}

func (n NonEmptyListValidator) MarkdownDescription(ctx context.Context) string {
	return "List cannot be empty"
}

// RFC3339TimestampValidator validates that a string value is a valid RFC3339 timestamp.
type RFC3339TimestampValidator struct{}

func (v RFC3339TimestampValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Timestamp Format",
			fmt.Sprintf("The timestamp value %q is not a valid RFC3339 format (YYYY-MM-DDThh:mm:ssZ): %s", value, err),
		)
	}
}

func (v RFC3339TimestampValidator) Description(ctx context.Context) string {
	return "Value must be a valid RFC3339 timestamp (YYYY-MM-DDThh:mm:ssZ)"
}

func (v RFC3339TimestampValidator) MarkdownDescription(ctx context.Context) string {
	return "Value must be a valid RFC3339 timestamp (YYYY-MM-DDThh:mm:ssZ)"
}

// StringOneOfValidator validates that a string value is one of the allowed values.
type StringOneOfValidator struct {
	AllowedValues []string
}

func (v StringOneOfValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	for _, allowed := range v.AllowedValues {
		if value == allowed {
			return
		}
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid value",
		fmt.Sprintf("%q is not a valid value. Must be one of: %s", value, strings.Join(v.AllowedValues, ", ")),
	)
}

func (v StringOneOfValidator) Description(ctx context.Context) string {
	return fmt.Sprintf("Value must be one of: %s", strings.Join(v.AllowedValues, ", "))
}

func (v StringOneOfValidator) MarkdownDescription(ctx context.Context) string {
	return fmt.Sprintf("Value must be one of: `%s`", strings.Join(v.AllowedValues, "`, `"))
}
