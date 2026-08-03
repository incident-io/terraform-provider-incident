package provider

import (
	"fmt"
	"strings"

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
