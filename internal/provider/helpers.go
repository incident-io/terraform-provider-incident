package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
)

// EnumValuesDescription documents an attribute whose values are a fixed enum in the API
// schema, by appending those values to the schema's own docstring. See
// apischema.EnumValuesDescription.
func EnumValuesDescription(definitionName string, propertyName string) string {
	return apischema.EnumValuesDescription(definitionName, propertyName)
}

// DescribeEnumValues is EnumValuesDescription for an attribute the provider describes in
// its own words. See apischema.DescribeEnumValues.
func DescribeEnumValues(description string, definitionName string, propertyName string) string {
	return apischema.DescribeEnumValues(description, definitionName, propertyName)
}

// enumValues returns the values the API schema allows for a property. See
// apischema.EnumValues.
func enumValues(definitionName string, propertyName string) []string {
	return apischema.EnumValues(definitionName, propertyName)
}

// knownString returns the value of a string attribute, and false when it's missing, null
// or unknown — none of which can be judged at plan time.
func knownString(value attr.Value) (string, bool) {
	str, ok := value.(types.String)
	if !ok || str.IsNull() || str.IsUnknown() {
		return "", false
	}

	return str.ValueString(), true
}

// knownInt64 is knownString for a number attribute.
func knownInt64(value attr.Value) (int64, bool) {
	number, ok := value.(types.Int64)
	if !ok || number.IsNull() || number.IsUnknown() {
		return 0, false
	}

	return number.ValueInt64(), true
}

// diagnosticsError renders diagnostics as a single string, for the code paths that carry an
// error rather than a diag.Diagnostics and so can't append them.
func diagnosticsError(diags diag.Diagnostics) string {
	messages := make([]string, 0, len(diags.Errors()))
	for _, d := range diags.Errors() {
		messages = append(messages, strings.TrimSpace(d.Summary()+": "+d.Detail()))
	}

	return strings.Join(messages, "; ")
}
