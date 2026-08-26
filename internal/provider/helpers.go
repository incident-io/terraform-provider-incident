package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
)

// EnumValuesDescription documents an attribute whose values are a fixed enum in the API
// schema. See apischema.EnumValuesDescription.
func EnumValuesDescription(definitionName string, propertyName string) string {
	return apischema.EnumValuesDescription(definitionName, propertyName)
}

// DynamicValuesDescription documents an attribute whose values aren't a fixed enum. See
// apischema.DynamicValuesDescription.
func DynamicValuesDescription(definitionName string, propertyName string, whereToLook string) string {
	return apischema.DynamicValuesDescription(definitionName, propertyName, whereToLook)
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
