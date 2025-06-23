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
