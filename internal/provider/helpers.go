package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
)

// EnumValuesDescription reuses the documentation string from the API schema, and then appends the possible values of the enum.
func EnumValuesDescription(definitionName string, propertyName string) string {
	enumValues := []string{}
	for _, enum := range apischema.Property(definitionName, propertyName).Value.Enum {
		enumAsString, _ := enum.(string)
		enumValues = append(enumValues, "`"+enumAsString+"`")
	}

	return fmt.Sprintf("%s. Possible values are: %s.", apischema.Docstring(definitionName, propertyName), strings.Join(enumValues, ", "))
}

// enumValues returns the values the API schema allows for a property, in the order it
// lists them. Validation that reads the enum from here can't fall behind the API and
// start rejecting a value it has since started accepting.
func enumValues(definitionName string, propertyName string) []string {
	values := []string{}
	for _, enum := range apischema.Property(definitionName, propertyName).Value.Enum {
		if enumAsString, ok := enum.(string); ok {
			values = append(values, enumAsString)
		}
	}

	return values
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
