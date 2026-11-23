package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// The API clears the instructions when a write leaves them out, so the config is the whole
// truth. The attribute must be plain Optional with no plan modifier: Computed, or a
// modifier that carried the stored value over, would hide the change that removing the
// attribute makes.
func TestAIInstructionsAttributeIsPlainOptional(t *testing.T) {
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
			if !instructions.Optional || instructions.Computed || instructions.Required {
				t.Errorf("%s must be plain Optional, got optional=%v computed=%v required=%v",
					tc.path, instructions.Optional, instructions.Computed, instructions.Required)
			}
			if len(instructions.PlanModifiers) != 0 {
				t.Errorf("%s must have no plan modifiers, got %d", tc.path, len(instructions.PlanModifiers))
			}

			// An empty string reads back as null, so it could never match the state, and
			// the API allows 2000 characters.
			cases := []struct {
				value     string
				wantError bool
			}{
				{value: "", wantError: true},
				{value: "a", wantError: false},
				{value: strings.Repeat("é", models.MaxAIGroupingInstructionsLength), wantError: false},
				{value: strings.Repeat("é", models.MaxAIGroupingInstructionsLength+1), wantError: true},
			}
			for _, vc := range cases {
				validateResp := &validator.StringResponse{}
				for _, v := range instructions.Validators {
					v.ValidateString(context.Background(), validator.StringRequest{
						Path:        tc.path,
						ConfigValue: types.StringValue(vc.value),
					}, validateResp)
				}
				if validateResp.Diagnostics.HasError() != vc.wantError {
					t.Errorf("%d characters: error = %v, want %v",
						len([]rune(vc.value)), validateResp.Diagnostics.HasError(), vc.wantError)
				}
			}
		})
	}
}
