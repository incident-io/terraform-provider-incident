package provider

import (
	"context"
	"fmt"
	"regexp"
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

// CatalogTypeAttributeTypeValidator validates that a catalog type attribute type is valid.
type CatalogTypeAttributeTypeValidator struct{}

var catalogCustomTypePattern = regexp.MustCompile(`^Custom\["[^"]+"\]$`)

func (v CatalogTypeAttributeTypeValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()

	// Check if it's a valid primitive type
	validPrimitiveTypes := []string{"String", "Text", "Number", "Bool"}
	for _, validType := range validPrimitiveTypes {
		if value == validType {
			return
		}
	}

	// Check if it matches the Custom["TypeName"] pattern
	if catalogCustomTypePattern.MatchString(value) {
		return
	}

	resp.Diagnostics.AddError(
		"Invalid Catalog Type Attribute Type",
		fmt.Sprintf("The type %q is not valid. Must be one of: String, Text, Number, Bool, or Custom[\"TypeName\"] format.", value),
	)
}

func (v CatalogTypeAttributeTypeValidator) Description(ctx context.Context) string {
	return "Value must be a valid catalog attribute type (String, Text, Number, Bool) or Custom[\"TypeName\"] format"
}

func (v CatalogTypeAttributeTypeValidator) MarkdownDescription(ctx context.Context) string {
	return "Value must be a valid catalog attribute type (String, Text, Number, Bool) or Custom[\"TypeName\"] format"
}
