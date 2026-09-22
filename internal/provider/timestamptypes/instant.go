// Package timestamptypes provides a custom terraform-plugin-framework string type for an
// attribute that names an instant in time.
//
// An RFC 3339 timestamp can write one instant many ways: 2027-01-01T00:00:00+01:00 and
// 2026-12-31T23:00:00Z are the same moment. The API stores the instant and reports it back
// in UTC, so a configuration written with an offset reads back as a different string. The
// framework's own timetypes.RFC3339 only normalises formatting, not offsets, so it still
// sees a change there and Terraform reports "Provider produced inconsistent result after
// apply" or a perpetual diff. Instant compares the moments, so any spelling of the same
// one is equal and the configured spelling is what stays in state.
package timestamptypes

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Instant is a string attribute holding an RFC 3339 timestamp, compared by the moment it
// names rather than by its spelling.
type Instant struct {
	basetypes.StringValue
}

var (
	_ basetypes.StringValuable                   = Instant{}
	_ basetypes.StringValuableWithSemanticEquals = Instant{}
	_ xattr.ValidateableAttribute                = Instant{}
)

// Type returns the InstantType.
func (v Instant) Type(_ context.Context) attr.Type {
	return InstantType{}
}

// Equal returns true if the given value is an Instant with an equal underlying string.
// This is byte equality, as the framework requires: semantic equality is separate.
func (v Instant) Equal(o attr.Value) bool {
	other, ok := o.(Instant)
	if !ok {
		return false
	}

	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals returns true when the new value names the same moment as this
// one, whatever offset either is written in.
func (v Instant) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(Instant)
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

	current, err := time.Parse(time.RFC3339, v.ValueString())
	if err != nil {
		return false, diags
	}
	proposed, err := time.Parse(time.RFC3339, newValue.ValueString())
	if err != nil {
		return false, diags
	}

	return current.Equal(proposed), diags
}

// ValidateAttribute rejects a known value that isn't an RFC 3339 timestamp at plan time.
func (v Instant) ValidateAttribute(_ context.Context, req xattr.ValidateAttributeRequest, resp *xattr.ValidateAttributeResponse) {
	if v.IsNull() || v.IsUnknown() {
		return
	}

	if _, err := time.Parse(time.RFC3339, v.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid RFC 3339 timestamp",
			fmt.Sprintf("%q isn't an RFC 3339 timestamp. Use a form like 2026-12-25T00:00:00Z, or with an offset, "+
				"2026-12-25T00:00:00+01:00. Error: %s", v.ValueString(), err),
		)
	}
}

// ValueTime returns the moment this value names. A null or unknown value, or one that
// doesn't parse, is the zero time with ok false.
func (v Instant) ValueTime() (time.Time, bool) {
	if v.IsNull() || v.IsUnknown() {
		return time.Time{}, false
	}

	parsed, err := time.Parse(time.RFC3339, v.ValueString())
	if err != nil {
		return time.Time{}, false
	}

	return parsed, true
}

// NewInstantNull creates an Instant with a null value.
func NewInstantNull() Instant {
	return Instant{StringValue: basetypes.NewStringNull()}
}

// NewInstantUnknown creates an Instant with an unknown value.
func NewInstantUnknown() Instant {
	return Instant{StringValue: basetypes.NewStringUnknown()}
}

// NewInstantStringValue creates an Instant from a string as written, which is how a
// configured value arrives.
func NewInstantStringValue(value string) Instant {
	return Instant{StringValue: basetypes.NewStringValue(value)}
}

// NewInstantTimeValue creates an Instant from a time, written as RFC 3339 in the time's
// own location.
func NewInstantTimeValue(value time.Time) Instant {
	return Instant{StringValue: basetypes.NewStringValue(value.Format(time.RFC3339))}
}

// InstantType is the attr.Type for Instant.
type InstantType struct {
	basetypes.StringType
}

var _ basetypes.StringTypable = InstantType{}

// String returns a human-readable name for the type.
func (t InstantType) String() string {
	return "timestamptypes.InstantType"
}

// ValueType returns an example Instant value.
func (t InstantType) ValueType(_ context.Context) attr.Value {
	return Instant{}
}

// Equal returns true if the given type is an InstantType.
func (t InstantType) Equal(o attr.Type) bool {
	other, ok := o.(InstantType)
	if !ok {
		return false
	}

	return t.StringType.Equal(other.StringType)
}

// ValueFromString converts a StringValue into an Instant.
func (t InstantType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return Instant{StringValue: in}, nil
}

// ValueFromTerraform converts a tftypes.Value into an Instant.
func (t InstantType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
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
