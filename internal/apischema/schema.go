package apischema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/getkin/kin-openapi/openapi3"

	_ "embed"
)

//go:embed public-schema-v3-including-secret-endpoints.json
var openAPIData []byte
var openAPI openapi3.T

func init() {
	if err := json.Unmarshal(openAPIData, &openAPI); err != nil {
		panic(err)
	}
}

func Def(name string) *openapi3.SchemaRef {
	def := openAPI.Components.Schemas[name]
	if def == nil {
		panic(fmt.Sprintf("unrecognised component: %s", name))
	}

	return def
}

func TagDocstring(name string) string {
	for _, tag := range openAPI.Tags {
		if tag.Name == name {
			return tag.Description
		}
	}

	panic(fmt.Sprintf("schema has no tag for %s", name))
}

func Property(definitionName, propertyName string) *openapi3.SchemaRef {
	property := Def(definitionName).Value.Properties[propertyName]
	if property == nil {
		panic(fmt.Sprintf("definition %s has no property %s", definitionName, propertyName))
	}

	if strings.HasPrefix(property.Ref, "#/components/schemas/") {
		return Def(strings.TrimPrefix(property.Ref, "#/components/schemas/"))
	}

	return property
}

func Docstring(definitionName, propertyName string) string {
	p := Property(definitionName, propertyName)
	if p.Value == nil {
		panic(fmt.Sprintf("property %s has no value: %s", propertyName, spew.Sdump(p)))
	}

	return p.Value.Description
}

// EnumValues returns the values the schema allows for a property, in the order it lists
// them. Validation that reads the enum from here can't fall behind the API and start
// rejecting a value it has since started accepting.
//
// For an array property the enum sits on the items rather than the property itself -
// runs_on_incident_modes is ArrayOf(WorkflowIncidentMode) in the API design - so fall
// back to the item enum, which is the set an element may take.
func EnumValues(definitionName, propertyName string) []string {
	property := Property(definitionName, propertyName).Value

	enum := property.Enum
	if len(enum) == 0 && property.Items != nil && property.Items.Value != nil {
		enum = property.Items.Value.Enum
	}

	values := []string{}
	for _, value := range enum {
		if valueAsString, ok := value.(string); ok {
			values = append(values, valueAsString)
		}
	}

	return values
}

// EnumValuesDescription reuses the documentation string from the API schema, and then
// appends the possible values of the enum.
func EnumValuesDescription(definitionName, propertyName string) string {
	values := EnumValues(definitionName, propertyName)
	if len(values) == 0 {
		panic(fmt.Sprintf("%s.%s has no enum values: use Docstring or DynamicValuesDescription", definitionName, propertyName))
	}

	quoted := []string{}
	for _, value := range values {
		quoted = append(quoted, "`"+value+"`")
	}

	return fmt.Sprintf("%s. Possible values are: %s.", sentence(Docstring(definitionName, propertyName)), strings.Join(quoted, ", "))
}

// DynamicValuesDescription documents an attribute whose valid values aren't a fixed
// enum: they come from a registry we add to regularly, or from the organisation's own
// configuration. Listing them in the schema would either go stale or force a provider
// upgrade every time we add one, so we say where to find the current set instead.
//
// The wording deliberately tells the reader the set can grow, so that neither a person
// nor an agent treats a value it doesn't mention as unsupported.
func DynamicValuesDescription(definitionName, propertyName, whereToLook string) string {
	return DescribeDynamicValues(Docstring(definitionName, propertyName), whereToLook)
}

// DescribeDynamicValues is DynamicValuesDescription for an attribute the schema has no
// docstring for, such as ConditionV2.operation, where the provider supplies its own.
func DescribeDynamicValues(description, whereToLook string) string {
	caveat := fmt.Sprintf("This isn't a fixed list - it depends on your configuration and grows over "+
		"time, so don't treat a value that isn't listed here as unsupported. %s", whereToLook)

	// Not every property in the schema carries a description, and prefixing the caveat
	// with an empty one leaves a stray ". " at the front.
	if sentence(description) == "" {
		return caveat
	}

	return fmt.Sprintf("%s. %s", sentence(description), caveat)
}

// sentence trims a trailing full stop so appending another sentence doesn't leave "..",
// since some schema docstrings end in one and some don't.
func sentence(description string) string {
	return strings.TrimRight(strings.TrimSpace(description), ".")
}
