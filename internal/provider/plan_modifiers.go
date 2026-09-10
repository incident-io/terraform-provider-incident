package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// useStateForUnknownIncludingNull copies the prior state value onto an unknown plan value,
// like the built-in UseStateForUnknown, but also preserves null state values. The built-in
// skips when state is null, which causes Computed+Optional attributes to show as "known
// after apply" on every plan when the API doesn't return the field (e.g. email_address for
// non-email alert sources).
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
