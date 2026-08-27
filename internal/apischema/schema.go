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
// them. Reading the enum from here rather than repeating it in the provider means
// validation can't fall behind the API and reject a value it has since started accepting.
//
// For an array property the enum sits on the items rather than the property itself -
// runs_on_incident_modes is ArrayOf(WorkflowIncidentMode) in the API design - so fall back
// to the item enum, which is the set an element may take.
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
// appends the values the enum allows, so the registry docs never just say "String" for an
// attribute that in fact takes one of a fixed set.
//
// It panics when the property has no enum: documenting a free-form attribute as though it
// had a fixed set of values is a mistake we want to hear about when the provider starts,
// rather than one that ships a description promising values it can't list.
func EnumValuesDescription(definitionName, propertyName string) string {
	return DescribeEnumValues(Docstring(definitionName, propertyName), definitionName, propertyName)
}

// DescribeEnumValues is EnumValuesDescription for an attribute the provider describes in
// its own words, either because the schema has no docstring for it or because the
// provider's wording is more useful in a Terraform context.
func DescribeEnumValues(description, definitionName, propertyName string) string {
	quoted := []string{}
	for _, value := range EnumValues(definitionName, propertyName) {
		// Some enums include the empty string, so that the field can be cleared. There's
		// nothing to render for it, and `` would come out as an empty code span.
		if value == "" {
			continue
		}

		quoted = append(quoted, "`"+value+"`")
	}

	if len(quoted) == 0 {
		panic(fmt.Sprintf("%s.%s has no enum values: use Docstring instead", definitionName, propertyName))
	}

	return appendSentence(description, fmt.Sprintf("Possible values are: %s.", strings.Join(quoted, ", ")))
}

// appendSentence adds a sentence to a description, punctuating the join. Docstrings vary:
// some end in a full stop, some in a question mark and some in nothing at all, so joining
// them naively renders "incidents.. Possible values" or "applied to?. Possible values".
func appendSentence(description, next string) string {
	description = strings.TrimSpace(description)

	// Not every property in the schema carries a description, and prefixing the sentence
	// with an empty one leaves a stray ". " at the front.
	if description == "" {
		return next
	}

	// A description of several lines is usually a bulleted list explaining each value, and
	// appending to it inline would tack the sentence onto the end of the last bullet.
	if strings.Contains(description, "\n") {
		return description + "\n\n" + next
	}

	if strings.ContainsAny(description[len(description)-1:], ".?!:") {
		return description + " " + next
	}

	return description + ". " + next
}
