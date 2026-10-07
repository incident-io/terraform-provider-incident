package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateLookupByIDOrName covers the id-XOR-name lookup, including the case a value
// isn't known yet: an id read off a resource created in the same apply is non-null but
// unknown, and rejecting that would fail a plan that would have applied.
func TestValidateLookupByIDOrName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		id      types.String
		lookup  types.String
		wantErr string
	}{
		{name: "id only", id: types.StringValue("01PAYMENTS"), lookup: types.StringNull()},
		{name: "name only", id: types.StringNull(), lookup: types.StringValue("Urgent support")},
		{name: "both", id: types.StringValue("01PAYMENTS"), lookup: types.StringValue("Urgent support"), wantErr: "Ambiguous lookup"},
		{name: "neither", id: types.StringNull(), lookup: types.StringNull(), wantErr: "Missing lookup"},
		{name: "an id another resource computes", id: types.StringUnknown(), lookup: types.StringNull()},
		{name: "a name another resource computes", id: types.StringNull(), lookup: types.StringUnknown()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateLookupByIDOrName(tc.id, tc.lookup, &diags)

			if tc.wantErr == "" {
				require.False(t, diags.HasError(), "unexpected diagnostics: %s", diags)
				return
			}

			require.True(t, diags.HasError(), "expected an error diagnostic")
			assert.Equal(t, tc.wantErr, diags.Errors()[0].Summary())
		})
	}
}
