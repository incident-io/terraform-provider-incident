package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A customer who upgrades the provider must keep the instructions they wrote in the
// dashboard. That needs the attribute to be Computed, so leaving it out isn't a change to
// "", and to carry the stored value into the plan, so the payload sends it back unchanged.
func TestAIInstructionsAttributeKeepsStoredValue(t *testing.T) {
	cases := []struct {
		name     string
		resource resource.Resource
		path     path.Path
	}{
		{
			name:     "alert route",
			resource: NewIncidentAlertRouteResource(),
			path:     path.Root("grouping_config").AtName("default").AtName("ai_instructions"),
		},
		{
			name:     "team grouping preference",
			resource: NewIncidentTeamGroupingPreferenceResource(),
			path:     path.Root("default").AtName("settings").AtName("ai_instructions"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp resource.SchemaResponse
			tc.resource.Schema(context.Background(), resource.SchemaRequest{}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("schema build failed: %+v", resp.Diagnostics)
			}

			attribute, diags := resp.Schema.AttributeAtPath(context.Background(), tc.path)
			if diags.HasError() {
				t.Fatalf("finding %s: %+v", tc.path, diags)
			}
			instructions, ok := attribute.(schema.StringAttribute)
			if !ok {
				t.Fatalf("%s is a %T, want a string attribute", tc.path, attribute)
			}
			if !instructions.Optional || !instructions.Computed {
				t.Errorf("%s must be Optional and Computed, got optional=%v computed=%v",
					tc.path, instructions.Optional, instructions.Computed)
			}

			// The plan modifier carries the stored value over when the config leaves the
			// attribute out of a resource that already exists. Only whether the state is
			// null matters to it, so any value stands for the existing resource.
			existing := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(tftypes.String, "existing")}
			planResp := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
			for _, modifier := range instructions.PlanModifiers {
				modifier.PlanModifyString(context.Background(), planmodifier.StringRequest{
					Path:        tc.path,
					State:       existing,
					ConfigValue: types.StringNull(),
					PlanValue:   types.StringUnknown(),
					StateValue:  types.StringValue("Group alerts from both clusters."),
				}, planResp)
			}
			if planResp.PlanValue.ValueString() != "Group alerts from both clusters." {
				t.Errorf("planned %v, want the stored instructions", planResp.PlanValue)
			}

			// A null in state is grouping settings added to an existing resource, or a
			// state from before the attribute existed. The API returns a value either way,
			// so planning null would fail the apply.
			nullResp := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
			for _, modifier := range instructions.PlanModifiers {
				modifier.PlanModifyString(context.Background(), planmodifier.StringRequest{
					Path:        tc.path,
					State:       existing,
					ConfigValue: types.StringNull(),
					PlanValue:   types.StringUnknown(),
					StateValue:  types.StringNull(),
				}, nullResp)
			}
			if !nullResp.PlanValue.IsUnknown() {
				t.Errorf("planned %v for a null state value, want unknown", nullResp.PlanValue)
			}

			// The API allows 2000 characters, so reject more at plan time.
			for _, tooLong := range []bool{false, true} {
				value := strings.Repeat("é", maxAIGroupingInstructionsLength)
				if tooLong {
					value += "é"
				}

				validateResp := &validator.StringResponse{}
				for _, v := range instructions.Validators {
					v.ValidateString(context.Background(), validator.StringRequest{
						Path:        tc.path,
						ConfigValue: types.StringValue(value),
					}, validateResp)
				}
				if validateResp.Diagnostics.HasError() != tooLong {
					t.Errorf("%d characters: error = %v, want %v",
						len([]rune(value)), validateResp.Diagnostics.HasError(), tooLong)
				}
			}
		})
	}
}
