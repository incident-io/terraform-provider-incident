package provider

import (
	"errors"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
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

// apiErrorWithStatus returns the API error behind err when it carries the given status code.
// Every status the provider reacts to goes through here, so there's one place that knows how
// the client reports one.
func apiErrorWithStatus(err error, statusCode int) (client.HTTPError, bool) {
	httpErr := client.HTTPError{}
	if errors.As(err, &httpErr) && httpErr.StatusCode == statusCode {
		return httpErr, true
	}

	return client.HTTPError{}, false
}

// isNotFound reports whether err is a 404 from the API, which for a Read means the resource
// has been deleted outside Terraform and should be removed from state.
func isNotFound(err error) bool {
	_, ok := apiErrorWithStatus(err, http.StatusNotFound)
	return ok
}

// isConflict reports whether err is a 409 from the API.
func isConflict(err error) bool {
	_, ok := apiErrorWithStatus(err, http.StatusConflict)
	return ok
}
