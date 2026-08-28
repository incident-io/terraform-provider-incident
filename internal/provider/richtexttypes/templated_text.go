// Package richtexttypes provides a terraform-plugin-framework string type for rich text
// fields, letting a config write `literal = "{{description}}"` instead of a jsonencode'd
// ProseMirror document. Raw AST keeps working permanently — this is not a deprecation.
//
// Converting in the resource's FromAPI/ToAPI and leaving the field a plain string does
// not work: FromAPI never sees config, so it must pick one form to return, and whichever
// it picks gives the other a permanent diff. Semantic equality is the only mechanism that
// can make both compare equal to the same stored document — the same reasoning behind
// jsontypes.NormalizedJSONOrString.
package richtexttypes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/jsontypes"
)

// TemplatedText holds either a "{{ }}" template or a raw JSON document, and compares the
// two semantically.
//
// It never needs its feature set: expressibility is decided by document content, not by
// the field it lives on.
type TemplatedText struct {
	basetypes.StringValue
}

// Compile-time interface assertions.
var (
	_ basetypes.StringValuable                   = TemplatedText{}
	_ basetypes.StringValuableWithSemanticEquals = TemplatedText{}
	_ xattr.ValidateableAttribute                = TemplatedText{}
)

// Type returns the TemplatedTextType.
func (v TemplatedText) Type(_ context.Context) attr.Type {
	return TemplatedTextType{}
}

// Equal returns true if the given value is a TemplatedText with an equal underlying
// StringValue. Must be overridden: the embedded StringValue compares against
// basetypes.StringValue, so without this a TemplatedText reports itself equal to a plain
// types.String holding the same contents.
func (v TemplatedText) Equal(o attr.Value) bool {
	other, ok := o.(TemplatedText)
	if !ok {
		return false
	}

	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals returns true when the new value means the same as the current one,
// so a template and the document it produces compare equal whichever way round they
// arrive. Falls back to exact string equality when either side can't be normalised, the
// same shape as jsontypes.JSONStringsEqual.
func (v TemplatedText) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(TemplatedText)
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

	current, newer := v.ValueString(), newValue.ValueString()
	if current == newer {
		return true, diags
	}

	currentAST, currentErr := normalise(current)
	newerAST, newerErr := normalise(newer)
	if currentErr != nil || newerErr != nil {
		// One side is neither a template nor a document: fall back to exact string
		// equality, which we already know is false here.
		return false, diags
	}

	return jsontypes.JSONStringsEqual(string(currentAST), string(newerAST)), diags
}

// ValidateAttribute reports template syntax errors at plan time. The framework calls it
// implicitly when converting Terraform values into framework types, so it covers every
// attribute using this type with no per-resource wiring.
func (v TemplatedText) ValidateAttribute(_ context.Context, req xattr.ValidateAttributeRequest, resp *xattr.ValidateAttributeResponse) {
	if v.IsNull() || v.IsUnknown() {
		return
	}

	// Raw AST isn't our grammar, so there's nothing to validate.
	if isDocument(v.ValueString()) {
		return
	}

	_, err := ToDocument(v.ValueString())
	if err == nil {
		return
	}

	summary, detail := "Invalid template syntax", err.Error()
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		summary = parseErr.Summary
	}

	resp.Diagnostics.AddAttributeError(req.Path, summary, detail)
}

// Literal returns the binding literal to send: the document a template compiles to, or raw
// AST verbatim. Nil means send nothing.
//
// It is always a document, never a bare string. The server uplifts a non-JSON literal into a
// document before storing it, so a string would read back as one we never wrote and diff on
// every plan.
func (v TemplatedText) Literal() (*string, error) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}

	if isDocument(v.ValueString()) {
		return v.ValueStringPointer(), nil
	}

	document, err := ToDocument(v.ValueString())
	if err != nil {
		return nil, err
	}

	return lo.ToPtr(string(document)), nil
}

// NewTemplatedTextFromLiteral builds a value from a stored binding literal, collapsing a
// document to template form where that is lossless and keeping the AST where it isn't.
// Semantic equality reconciles either against whichever form the config used.
func NewTemplatedTextFromLiteral(literal string) TemplatedText {
	if template, ok := FromDocument(json.RawMessage(literal)); ok {
		return NewTemplatedTextValue(template)
	}

	return NewTemplatedTextValue(literal)
}

// NewTemplatedTextNull creates a TemplatedText with a null value.
func NewTemplatedTextNull() TemplatedText {
	return TemplatedText{StringValue: basetypes.NewStringNull()}
}

// NewTemplatedTextUnknown creates a TemplatedText with an unknown value.
func NewTemplatedTextUnknown() TemplatedText {
	return TemplatedText{StringValue: basetypes.NewStringUnknown()}
}

// NewTemplatedTextValue creates a TemplatedText with a known value.
func NewTemplatedTextValue(value string) TemplatedText {
	return TemplatedText{StringValue: basetypes.NewStringValue(value)}
}

// NewTemplatedTextPointerValue creates a TemplatedText with a null value if nil, or a
// known value.
func NewTemplatedTextPointerValue(value *string) TemplatedText {
	return TemplatedText{StringValue: basetypes.NewStringPointerValue(value)}
}

// TemplatedTextType is the attr.Type for TemplatedText.
type TemplatedTextType struct {
	basetypes.StringType
}

// Compile-time interface assertions.
var (
	_ basetypes.StringTypable = TemplatedTextType{}
)

// String returns a human-readable name for the type.
func (t TemplatedTextType) String() string {
	return "richtexttypes.TemplatedTextType"
}

// ValueType returns an example TemplatedText value.
func (t TemplatedTextType) ValueType(_ context.Context) attr.Value {
	return TemplatedText{}
}

// Equal returns true if the given type is a TemplatedTextType. Overridden for the same
// reason as TemplatedText.Equal.
func (t TemplatedTextType) Equal(o attr.Type) bool {
	other, ok := o.(TemplatedTextType)
	if !ok {
		return false
	}

	return t.StringType.Equal(other.StringType)
}

// ValueFromString converts a StringValue into a TemplatedText.
func (t TemplatedTextType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return TemplatedText{StringValue: in}, nil
}

// ValueFromTerraform converts a tftypes.Value into a TemplatedText.
func (t TemplatedTextType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
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
