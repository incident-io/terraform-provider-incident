package models

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
)

// MaxAIGroupingInstructionsLength is the API's limit, counted in characters rather than
// bytes.
const MaxAIGroupingInstructionsLength = 2000

// AIInstructionsBetaNote goes on every ai_instructions attribute, in resources and data
// sources.
const AIInstructionsBetaNote = "AI grouping instructions are in beta. Contact us if you'd " +
	"like them turned on for your organisation."

// AIInstructionsAttribute is the ai_instructions attribute of a set of grouping settings,
// shared by the alert route and the grouping preferences.
//
// It is plain Optional: the API clears the instructions when a write leaves them out, so
// the config is the whole truth, and removing the attribute plans a change that clears
// them. State holds null when there are none, which is how the API's empty string reads
// back, so an empty string in config would never match and is rejected.
func AIInstructionsAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional: true,
		MarkdownDescription: description + "\n\n" + AIInstructionsBetaNote + "\n\n" +
			"Leave the attribute out to have no instructions. Removing it clears any " +
			"instructions the route or team has, including ones set in the dashboard.",
		Validators: []validator.String{
			stringvalidator.UTF8LengthBetween(1, MaxAIGroupingInstructionsLength),
		},
	}
}

// AIInstructionsPayload is the ai_instructions to send for a set of grouping settings. It is
// nil when the config leaves them out, which the API takes as no instructions and clears
// any it has.
func AIInstructionsPayload(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	return value.ValueStringPointer()
}

// AIInstructionsFromAPI reads a set of grouping settings' ai_instructions. The API returns
// an empty string when there are none, which reads as null, so settings without
// instructions match a config that leaves the attribute out.
func AIInstructionsFromAPI(value *string) types.String {
	if lo.FromPtr(value) == "" {
		return types.StringNull()
	}

	return types.StringValue(*value)
}
