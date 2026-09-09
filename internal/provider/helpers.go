package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

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

// useStateForUnknownIncludingNull is like stringplanmodifier.UseStateForUnknown()
// but also preserves null state values. The built-in UseStateForUnknown skips when
// state is null, which causes Computed+Optional attributes to show as "known after
// apply" on every plan when the API doesn't return the field (e.g. email_address
// for non-email alert sources).
type useStateForUnknownIncludingNull struct{}

func (m useStateForUnknownIncludingNull) Description(ctx context.Context) string {
	return "Use the state value for unknown, including null."
}

func (m useStateForUnknownIncludingNull) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m useStateForUnknownIncludingNull) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Do nothing if there is a known planned value.
	if !req.PlanValue.IsUnknown() {
		return
	}

	// Do nothing if there is an unknown configuration value.
	if req.ConfigValue.IsUnknown() {
		return
	}

	// Do nothing if there is no prior state (first creation).
	if req.State.Raw.IsNull() {
		return
	}

	// Preserve the prior state value, even if it's null.
	resp.PlanValue = req.StateValue
}

func (m useStateForUnknownIncludingNull) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsUnknown() {
		return
	}
	if req.State.Raw.IsNull() {
		return
	}
	resp.PlanValue = req.StateValue
}

// lastAttributeName is the name of the attribute a path ends at, for the plan
// walks that classify a leaf by what it is called.
func lastAttributeName(steps *tftypes.AttributePath) (string, bool) {
	if steps == nil || len(steps.Steps()) == 0 {
		return "", false
	}

	name, ok := steps.Steps()[len(steps.Steps())-1].(tftypes.AttributeName)

	return string(name), ok
}
