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

// useStateForUnknownIfSet copies the prior state value onto an unknown plan value, like the
// built-in UseStateForUnknown, but only when state actually holds one.
//
// It exists for a computed attribute nested in a list, such as the ID of a rule on a pay
// config. The built-in modifier checks only that the resource has state at all, so for a
// list element the plan adds beyond the end of the state list it copies the null the
// framework hands it for a missing element, and the apply then fails the consistency check
// when the API mints an ID. Leaving such a value unknown is what the plan should say.
type useStateForUnknownIfSet struct{}

func (m useStateForUnknownIfSet) Description(ctx context.Context) string {
	return "Once set, the value of this attribute in state will not change."
}

func (m useStateForUnknownIfSet) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m useStateForUnknownIfSet) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsUnknown() {
		return
	}
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	resp.PlanValue = req.StateValue
}
