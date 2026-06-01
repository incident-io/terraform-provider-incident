// Package jsontypes provides custom terraform-plugin-framework string types that
// implement semantic equality for fields which may contain JSON.
//
// The engine param-binding "literal" field (and similar fields throughout the
// provider) stores arbitrary strings that are frequently, but not always, JSON.
// The same logical JSON value can be encoded in many byte-different ways: keys
// in a different order, different whitespace, and crucially different HTML
// escaping ('>' vs '>'). Go's encoding/json HTML-escapes by default while
// HCL's jsonencode also HTML-escapes, but CDKTF's JSON.stringify, file(),
// heredocs and hand-written JSON do not.
//
// Because the inbound (plan -> API) path sends the literal verbatim while the
// outbound (API -> state) path re-encodes it, the planned and applied byte
// strings can differ even though they represent the same JSON. Terraform then
// raises "Provider produced inconsistent result after apply" or shows a
// perpetual diff. Toggling SetEscapeHTML cannot fix this: it just moves the
// breakage between the HCL-jsonencode and raw-string user populations (see the
// ONC-7057 / ONC-7504 round-trip). The correct fix is semantic equality, so
// Terraform treats two byte-different-but-equivalent JSON strings as equal.
package jsontypes

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// NormalizedJSONOrString is a custom string type that compares two values using
// semantic JSON equality when both sides are valid JSON, and falls back to
// exact string equality otherwise. This makes it safe to apply to fields that
// usually hold JSON but may also hold a plain reference or string (e.g. the
// engine param-binding "literal").
type NormalizedJSONOrString struct {
	basetypes.StringValue
}

// Compile-time interface assertions.
var (
	_ basetypes.StringValuable                   = NormalizedJSONOrString{}
	_ basetypes.StringValuableWithSemanticEquals = NormalizedJSONOrString{}
)

// Type returns the NormalizedJSONOrStringType.
func (v NormalizedJSONOrString) Type(_ context.Context) attr.Type {
	return NormalizedJSONOrStringType{}
}

// Equal returns true if the given value is a NormalizedJSONOrString with an equal
// underlying StringValue.
func (v NormalizedJSONOrString) Equal(o attr.Value) bool {
	other, ok := o.(NormalizedJSONOrString)
	if !ok {
		return false
	}

	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals returns true when the new value is semantically equal to
// the current value. If both sides parse as JSON they are compared by their
// canonical form (key-sorted and escaping-insensitive); otherwise it falls back
// to exact string equality.
func (v NormalizedJSONOrString) StringSemanticEquals(ctx context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(NormalizedJSONOrString)
	if !ok {
		diags.AddError(
			"Semantic Equality Check Error",
			"An unexpected value type was received while performing semantic equality checks. "+
				"Please report this to the provider developers.\n\n"+
				fmt.Sprintf("Expected Value Type: %T\n", v)+
				fmt.Sprintf("Got Value Type: %T", newValuable),
		)

		return false, diags
	}

	return JSONStringsEqual(v.ValueString(), newValue.ValueString()), diags
}

// NewNormalizedJSONOrStringNull creates a NormalizedJSONOrString with a null value.
func NewNormalizedJSONOrStringNull() NormalizedJSONOrString {
	return NormalizedJSONOrString{StringValue: basetypes.NewStringNull()}
}

// NewNormalizedJSONOrStringUnknown creates a NormalizedJSONOrString with an unknown value.
func NewNormalizedJSONOrStringUnknown() NormalizedJSONOrString {
	return NormalizedJSONOrString{StringValue: basetypes.NewStringUnknown()}
}

// NewNormalizedJSONOrStringValue creates a NormalizedJSONOrString with a known value.
func NewNormalizedJSONOrStringValue(value string) NormalizedJSONOrString {
	return NormalizedJSONOrString{StringValue: basetypes.NewStringValue(value)}
}

// NewNormalizedJSONOrStringPointerValue creates a NormalizedJSONOrString with a null value
// if nil, or a known value.
func NewNormalizedJSONOrStringPointerValue(value *string) NormalizedJSONOrString {
	return NormalizedJSONOrString{StringValue: basetypes.NewStringPointerValue(value)}
}

// NormalizedJSONOrStringType is the attr.Type for NormalizedJSONOrString.
type NormalizedJSONOrStringType struct {
	basetypes.StringType
}

// Compile-time interface assertions.
var (
	_ basetypes.StringTypable = NormalizedJSONOrStringType{}
)

// String returns a human-readable name for the type.
func (t NormalizedJSONOrStringType) String() string {
	return "jsontypes.NormalizedJSONOrStringType"
}

// ValueType returns an example NormalizedJSONOrString value.
func (t NormalizedJSONOrStringType) ValueType(_ context.Context) attr.Value {
	return NormalizedJSONOrString{}
}

// Equal returns true if the given type is a NormalizedJSONOrStringType.
func (t NormalizedJSONOrStringType) Equal(o attr.Type) bool {
	other, ok := o.(NormalizedJSONOrStringType)
	if !ok {
		return false
	}

	return t.StringType.Equal(other.StringType)
}

// ValueFromString converts a StringValue into a NormalizedJSONOrString.
func (t NormalizedJSONOrStringType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return NormalizedJSONOrString{StringValue: in}, nil
}

// ValueFromTerraform converts a tftypes.Value into a NormalizedJSONOrString.
func (t NormalizedJSONOrStringType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}

	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", attrValue)
	}

	stringValuable, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to StringValuable: %v", diags)
	}

	return stringValuable, nil
}
