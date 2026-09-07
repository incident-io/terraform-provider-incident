package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

// An element Terraform hasn't settled reads back with neither literal nor reference, which
// is exactly what a value the config left empty looks like. Validation must not mistake one
// for the other: the list around an unsettled element can be known - a ternary whose
// branches are the same length gives that - so this is the shape the reported failure
// actually arrived in, and reporting "Missing value" against it would fail a plan the change
// is meant to allow.
func TestValidateBindingSkipsAnUnsettledArrayElement(t *testing.T) {
	for name, arrayValue := range map[string]types.List{
		"unknown element": types.ListValueMust(BindingValueType(), []attr.Value{
			types.ObjectUnknown(BindingValueAttrTypes()),
		}),
		"unknown list": types.ListUnknown(BindingValueType()),
	} {
		t.Run(name, func(t *testing.T) {
			binding := NullBinding()
			binding.ArrayValue = arrayValue

			var diags diag.Diagnostics
			ValidateBinding(&binding, path.Root("value"), map[string]bool{}, &diags)

			for _, d := range diags.Errors() {
				t.Errorf("%s: %s", d.Summary(), d.Detail())
			}
		})
	}
}

// The settled elements still get checked, or the change would buy unknown support by
// dropping validation for everyone else.
func TestValidateBindingStillChecksSettledArrayElements(t *testing.T) {
	binding := NullBinding()
	binding.ArrayValue = BindingArrayValue(BindingValue{
		Literal:   types.StringNull(),
		Reference: types.StringNull(),
	})

	var diags diag.Diagnostics
	ValidateBinding(&binding, path.Root("value"), map[string]bool{}, &diags)

	assert.True(t, diags.HasError(), "an element with neither literal nor reference is still an error")
}
