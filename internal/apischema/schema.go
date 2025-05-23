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
