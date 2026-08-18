package models

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/richtexttypes"
)

// Rich text fields — an alert source's title and description. They live here rather than in
// engine.go, which is a trimmed V2 port carrying the ...V2 client types.

// TemplatedTextValue is one rich text value, either fixed content or a reference producing it.
//
// The API types these as ordinary param bindings, but only two of a binding's six spellings
// mean anything for one: there is no array of titles, and an expression_ref is spelled inside
// the content as `{{expressions["name"]}}`.
type TemplatedTextValue struct {
	Literal   richtexttypes.TemplatedText `tfsdk:"literal"`
	Reference types.String                `tfsdk:"reference"`
}

// TemplatedTextAttribute is a whole field taking a rich text value. featureSet is the field's
// surface, deciding what the server keeps when it renders — and so which feature_set to ask
// data.incident_rich_text for.
func TemplatedTextAttribute(description, featureSet string) schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: description,
		Attributes:  TemplatedTextValueAttributes(featureSet),
	}
}

func TemplatedTextValueAttributes(featureSet string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"literal": schema.StringAttribute{
			CustomType: richtexttypes.TemplatedTextType{},
			Optional:   true,
			Description: fmt.Sprintf(
				"Fixed content, which may interpolate the scope with `{{ variable }}`. Filters "+
					"`truncate: N` and `omit_if_unset` are supported. For content needing formatting "+
					"a template can't express, pass a document from "+
					"`data.incident_rich_text` with `feature_set = %q`.",
				featureSet,
			),
		},
		"reference": schema.StringAttribute{
			Optional: true,
			Description: "A reference into the scope whose value becomes the content, such as " +
				"`payload.summary`. Prefer a literal interpolating `{{ payload.summary }}`, which " +
				"can also carry surrounding text.",
		},
	}
}

// TemplatedTextValueToPayload maps a value onto the binding the API takes. Nil means absent.
func TemplatedTextValueToPayload(value *TemplatedTextValue) (*client.EngineParamBindingPayloadV3, error) {
	if value == nil {
		return nil, nil
	}

	literal, err := value.Literal.Literal()
	if err != nil {
		return nil, err
	}

	reference := value.Reference.ValueStringPointer()
	if value.Reference.IsUnknown() {
		reference = nil
	}

	switch {
	case literal != nil && reference != nil:
		return nil, fmt.Errorf("set either literal or reference, not both")

	// Unknown, not unset: nothing to send yet, and it's the plan that decides the value.
	case literal == nil && reference == nil:
		if value.Literal.IsUnknown() || value.Reference.IsUnknown() {
			return nil, nil
		}
		return nil, fmt.Errorf("set either literal or reference")
	}

	return &client.EngineParamBindingPayloadV3{
		Value: &client.EngineParamBindingValuePayloadV3{
			Literal:   literal,
			Reference: reference,
		},
	}, nil
}

// TemplatedTextValueFromPayload reads a binding back. An array is reported rather than dropped:
// the API assigns these fields unconditionally, so reading one as absent would have the next
// apply delete it.
func TemplatedTextValueFromPayload(binding *client.EngineParamBindingPayloadV3) (*TemplatedTextValue, error) {
	if binding == nil {
		return nil, nil
	}

	if values := lo.FromPtr(binding.ArrayValue); len(values) > 0 {
		return nil, fmt.Errorf("holds several values, which a single piece of rich text cannot represent")
	}

	if binding.Value == nil {
		return nil, nil
	}

	value := &TemplatedTextValue{
		Literal:   richtexttypes.NewTemplatedTextNull(),
		Reference: types.StringPointerValue(binding.Value.Reference),
	}
	if binding.Value.Literal != nil {
		value.Literal = richtexttypes.NewTemplatedTextFromLiteral(*binding.Value.Literal)
	}

	// Neither form set is how the API spells a binding holding nothing, and storing that object
	// would diff against a config that omitted the field.
	if value.Literal.IsNull() && value.Reference.IsNull() {
		return nil, nil
	}

	return value, nil
}

// ValidateTemplatedTextValue rejects an empty or ambiguous value at plan time, rather than
// letting the mapping fail at apply where there is no rollback.
func ValidateTemplatedTextValue(value *TemplatedTextValue, at path.Path, diags *diag.Diagnostics) {
	if value == nil {
		return
	}

	// An unresolved interpolation counts as neither set nor unset.
	literalSet := !value.Literal.IsNull() && !value.Literal.IsUnknown()
	referenceSet := !value.Reference.IsNull() && !value.Reference.IsUnknown()
	pending := value.Literal.IsUnknown() || value.Reference.IsUnknown()

	switch {
	case literalSet && referenceSet:
		diags.AddAttributeError(at, "Ambiguous value", "Set either literal or reference, not both.")
	case !literalSet && !referenceSet && !pending:
		diags.AddAttributeError(at, "Missing value", "Set either literal or reference.")
	}
}
