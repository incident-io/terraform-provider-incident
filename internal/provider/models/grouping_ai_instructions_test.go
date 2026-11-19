package models

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// aiInstructionsCases are the ways a plan can hold ai_instructions. The attribute is
// Optional and Computed, so a config that leaves it out plans unknown on create and the
// stored value on update. Either way the instructions the account has are kept.
var aiInstructionsCases = []struct {
	name     string
	planned  types.String
	wantSent bool
	want     string
}{
	{
		name:     "keeps the stored instructions when the plan doesn't know them",
		planned:  types.StringUnknown(),
		wantSent: false,
	},
	{
		name:     "keeps the stored instructions when the plan carries them over",
		planned:  types.StringValue("Group alerts from both clusters."),
		wantSent: true,
		want:     "Group alerts from both clusters.",
	},
	{
		name:     "sets new instructions",
		planned:  types.StringValue("Alerts from the old and new clusters describe the same outage."),
		wantSent: true,
		want:     "Alerts from the old and new clusters describe the same outage.",
	},
	{
		name:     "clears the instructions with an empty string",
		planned:  types.StringValue(""),
		wantSent: true,
		want:     "",
	},
}

// sentAIInstructions marshals settings as the client would, and reports what it sends for
// ai_instructions. Leaving the key out is what makes the API keep the stored value, so
// this checks the JSON rather than the pointer.
func sentAIInstructions(t *testing.T, settings any) (string, bool) {
	t.Helper()

	body, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshalling settings: %v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshalling settings: %v", err)
	}

	raw, ok := fields["ai_instructions"]
	if !ok {
		return "", false
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decoding ai_instructions: %v", err)
	}

	return value, true
}

func checkSentAIInstructions(t *testing.T, settings any, wantSent bool, want string) {
	t.Helper()

	got, sent := sentAIInstructions(t, settings)
	if sent != wantSent {
		t.Fatalf("ai_instructions sent = %v, want %v", sent, wantSent)
	}
	if sent && got != want {
		t.Errorf("ai_instructions = %q, want %q", got, want)
	}
}

func TestAlertRouteV3AIInstructionsPayload(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, tc := range aiInstructionsCases {
			name := tc.name
			if !enabled {
				name += " with grouping disabled"
			}

			t.Run(name, func(t *testing.T) {
				model := AlertRouteResourceModel{
					GroupingConfig: &AlertRouteV3GroupingConfigModel{
						Default: &AlertRouteV3GroupingSettingsModel{
							Enabled:        types.BoolValue(enabled),
							AIEnabled:      types.BoolValue(false),
							AIInstructions: tc.planned,
							WindowSeconds:  types.Int64Null(),
							WindowType:     types.StringNull(),
						},
					},
				}

				checkSentAIInstructions(t, model.ToCreatePayloadV3().GroupingConfig.Default, tc.wantSent, tc.want)
				checkSentAIInstructions(t, model.ToUpdatePayloadV3().GroupingConfig.Default, tc.wantSent, tc.want)
			})
		}
	}
}

func TestTeamGroupingPreferenceAIInstructionsPayload(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		for _, tc := range aiInstructionsCases {
			name := tc.name
			if !enabled {
				name += " with grouping disabled"
			}

			t.Run(name, func(t *testing.T) {
				model := TeamGroupingPreferenceResourceModel{
					TeamID: types.StringValue("01TEAM"),
					Default: &TeamGroupingPreferenceBranchModel{
						Settings: &TeamGroupingPreferenceSettingsModel{
							Enabled:        types.BoolValue(enabled),
							AIEnabled:      types.BoolValue(false),
							AIInstructions: tc.planned,
							WindowSeconds:  types.Int64Null(),
							WindowType:     types.StringNull(),
						},
					},
				}

				checkSentAIInstructions(t, model.ToCreatePayload().Default.Settings, tc.wantSent, tc.want)
				checkSentAIInstructions(t, model.ToUpdatePayload(2).Default.Settings, tc.wantSent, tc.want)
			})
		}
	}
}

// The API returns the instructions as an empty string when there are none. Reading that,
// or a response with no field at all, as an empty string matches a config that sets "" to
// clear them, and gives a known value for the plan to carry over.
func TestAIInstructionsFromAPI(t *testing.T) {
	cases := []struct {
		name string
		api  *string
		want string
	}{
		{name: "stored instructions", api: lo.ToPtr("Group alerts from both clusters."), want: "Group alerts from both clusters."},
		{name: "no instructions", api: lo.ToPtr(""), want: ""},
		{name: "field missing", api: nil, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route := AlertRouteResourceModel{}.FromAPIV3(client.AlertRouteV3{
				Id:              "01ROUTE",
				ConditionGroups: []client.ConditionGroupV3{},
				Expressions:     []client.ExpressionV3{},
				GroupingConfig: client.AlertGroupingConfigV3{
					Default: client.GroupingSettingsV3{Enabled: false, AiInstructions: tc.api},
				},
				MessageConfig: client.AlertMessageConfigV3{
					Destinations: []client.AlertMessageDestinationV3{},
				},
				EscalationConfig: client.AlertRouteEscalationConfigV3{
					EscalationTargets: []client.AlertRouteEscalationTargetV3{},
				},
			})
			if got := route.GroupingConfig.Default.AIInstructions; got.IsNull() || got.IsUnknown() || got.ValueString() != tc.want {
				t.Errorf("alert route ai_instructions = %v, want %q", got, tc.want)
			}

			preference := TeamGroupingPreferenceResourceModel{}.FromAPI(client.TeamGroupingPreferenceV3{
				Id:     "01PREFERENCE",
				TeamId: "01TEAM",
				Default: client.TeamGroupingBranchV3{
					Settings: client.TeamGroupingSettingsV3{Enabled: false, AiInstructions: tc.api},
				},
			}, nil)
			if got := preference.Default.Settings.AIInstructions; got.IsNull() || got.IsUnknown() || got.ValueString() != tc.want {
				t.Errorf("team grouping preference ai_instructions = %v, want %q", got, tc.want)
			}
		})
	}
}

// The provider doesn't model a team's conditional branches yet. The API keeps the stored
// branches and expressions when an update leaves them out, so we must never send either,
// not even as an empty list, which would delete them.
func TestTeamGroupingPreferencePayloadLeavesBranchesOut(t *testing.T) {
	model := TeamGroupingPreferenceResourceModel{
		TeamID: types.StringValue("01TEAM"),
		Default: &TeamGroupingPreferenceBranchModel{
			Settings: &TeamGroupingPreferenceSettingsModel{
				Enabled:        types.BoolValue(true),
				AIEnabled:      types.BoolValue(false),
				AIInstructions: types.StringValue("Group alerts from both clusters."),
				WindowSeconds:  types.Int64Value(600),
				WindowType:     types.StringValue("rolling"),
			},
		},
	}

	payloads := map[string]any{
		"create": model.ToCreatePayload(),
		"update": model.ToUpdatePayload(2),
	}
	for name, payload := range payloads {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshalling the %s payload: %v", name, err)
		}

		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatalf("unmarshalling the %s payload: %v", name, err)
		}
		for _, field := range []string{"branches", "expressions"} {
			if raw, ok := fields[field]; ok {
				t.Errorf("the %s payload sends %s = %s, want it left out", name, field, raw)
			}
		}
	}
}
