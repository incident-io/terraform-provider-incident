package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// validateLookupByIDOrName rejects an ambiguous lookup at plan time, for a data source
// whose id and name are both Optional and Computed so either can be used. Setting both
// would otherwise silently ignore one of them.
func validateLookupByIDOrName(id, name types.String, diags *diag.Diagnostics) {
	// A value that isn't known yet — an id taken from a resource created in the same apply,
	// or either attribute behind an unresolved conditional — is non-null, so judging it here
	// would reject a config that's actually fine.
	if id.IsUnknown() || name.IsUnknown() {
		return
	}

	switch {
	case !id.IsNull() && !name.IsNull():
		diags.AddError("Ambiguous lookup", "Set either id or name, not both.")
	case id.IsNull() && name.IsNull():
		diags.AddError("Missing lookup", "Set one of id or name.")
	}
}
