package provider

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// maxAIGroupingInstructionsLength is the API's limit, counted in characters rather than
// bytes.
const maxAIGroupingInstructionsLength = 2000

// aiInstructionsBetaNote goes on every ai_instructions attribute, in resources and data
// sources.
const aiInstructionsBetaNote = "AI grouping instructions are in beta. Contact us if you'd " +
	"like them turned on for your organisation."

// aiInstructionsAttribute is the ai_instructions attribute of a set of grouping settings,
// shared by the alert route and the team grouping preference.
//
// It is plain Optional: the API clears the instructions when a write leaves them out, so
// the config is the whole truth, and removing the attribute plans a change that clears
// them. State holds null when there are none, which is how the API's empty string reads
// back, so an empty string in config would never match and is rejected.
func aiInstructionsAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional: true,
		MarkdownDescription: description + "\n\n" + aiInstructionsBetaNote + "\n\n" +
			"Leave the attribute out to have no instructions. Removing it clears any " +
			"instructions the route or team has, including ones set in the dashboard.",
		Validators: []validator.String{
			stringvalidator.UTF8LengthBetween(1, maxAIGroupingInstructionsLength),
		},
	}
}
