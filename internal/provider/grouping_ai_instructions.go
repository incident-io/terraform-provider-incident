package provider

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// maxAIGroupingInstructionsLength is the API's limit, counted in characters rather than
// bytes.
const maxAIGroupingInstructionsLength = 2000

// aiInstructionsAttribute is the ai_instructions attribute of a set of grouping settings,
// shared by the alert route and the team grouping preference.
//
// It is Optional and Computed so that a config that doesn't set it keeps the instructions
// the account has, such as ones written in the dashboard before the attribute existed. The
// plan then holds the stored value, and sending that back changes nothing. An empty string
// clears them.
func aiInstructionsAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional: true,
		Computed: true,
		MarkdownDescription: description + "\n\n" +
			"Leave the attribute unset to keep the current instructions, such as ones set in " +
			"the dashboard. Set it to `\"\"` to clear them.",
		Validators: []validator.String{
			stringvalidator.UTF8LengthAtMost(maxAIGroupingInstructionsLength),
		},
		// Not the built-in UseStateForUnknown, which copies a null state value. That null
		// is what state holds when grouping settings are added to an existing resource,
		// or when the state is from before the attribute existed, and the API always
		// returns a value, so the plan must stay unknown.
		PlanModifiers: []planmodifier.String{
			useStateForUnknownIfSet{},
		},
	}
}
