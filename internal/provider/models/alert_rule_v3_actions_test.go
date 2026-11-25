package models

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// escalateAction is an escalate action as a plan holds it: the schema has filled in the
// defaults of every setting the config left out.
func escalateAction(escalate AlertRuleEscalateActionV3Model) AlertRuleActionV3Model {
	if escalate.Mode.IsNull() {
		escalate.Mode = types.StringValue("on_each_new_alert")
	}
	if escalate.GracePeriodSeconds.IsNull() {
		escalate.GracePeriodSeconds = types.Int64Value(0)
	}
	if escalate.AutoCancel.IsNull() {
		escalate.AutoCancel = types.BoolValue(false)
	}

	return AlertRuleActionV3Model{
		ID:       types.StringNull(),
		Type:     types.StringValue("escalate"),
		Escalate: &escalate,
	}
}

func decodeAction(t *testing.T, response string) client.AlertRuleActionV3 {
	t.Helper()

	var action client.AlertRuleActionV3
	require.NoError(t, json.Unmarshal([]byte(response), &action))

	return action
}

// Each action type has to reach the API as the payload it expects, and read back from the
// response the server gives for it as exactly the plan that sent it. Anything else fails the
// apply with "Provider produced inconsistent result after apply", after the write has landed.
//
// The responses follow the server's serialiser: it fills in every setting's default, and
// assigns an id the config left out.
func TestAlertRuleActionRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name     string
		plan     AlertRuleActionV3Model
		payload  string
		response string
	}{
		{
			name: "escalate with every field set",
			plan: func() AlertRuleActionV3Model {
				action := escalateAction(AlertRuleEscalateActionV3Model{
					EscalationPaths:    v3ExpressionRef("team_path"),
					Users:              v3Values("U1", "U2"),
					Urgency:            v3Value(v3Literal("URG")),
					Mode:               types.StringValue("on_each_new_alert"),
					GracePeriodSeconds: types.Int64Value(120),
					AutoCancel:         types.BoolValue(true),
				})
				action.ID = types.StringValue("page-team")
				return action
			}(),
			payload: `{
				"id": "page-team",
				"type": "escalate",
				"escalate": {
					"escalation_paths": {"value": {"reference": "expressions[\"team_path\"]"}},
					"users": {"array_value": [{"literal": "U1"}, {"literal": "U2"}]},
					"urgency": {"value": {"literal": "URG"}},
					"mode": "on_each_new_alert",
					"grace_period_seconds": 120,
					"auto_cancel": true
				}
			}`,
			response: `{
				"id": "page-team",
				"type": "escalate",
				"escalate": {
					"escalation_paths": {"value": {"reference": "expressions[\"team_path\"]"}},
					"users": {"array_value": [{"literal": "U1"}, {"literal": "U2"}]},
					"urgency": {"value": {"literal": "URG"}},
					"mode": "on_each_new_alert",
					"grace_period_seconds": 120,
					"auto_cancel": true
				}
			}`,
		},
		{
			name: "escalate to users only, at the priority's urgency",
			plan: escalateAction(AlertRuleEscalateActionV3Model{
				Users: v3Values("U1"),
				Mode:  types.StringValue("on_priority_increase"),
			}),
			payload: `{
				"type": "escalate",
				"escalate": {
					"users": {"array_value": [{"literal": "U1"}]},
					"mode": "on_priority_increase",
					"grace_period_seconds": 0,
					"auto_cancel": false
				}
			}`,
			response: `{
				"id": "01SERVERASSIGNED",
				"type": "escalate",
				"escalate": {
					"users": {"array_value": [{"literal": "U1"}]},
					"mode": "on_priority_increase",
					"grace_period_seconds": 0,
					"auto_cancel": false
				}
			}`,
		},
		{
			name: "post to Slack",
			plan: AlertRuleActionV3Model{
				ID:   types.StringNull(),
				Type: types.StringValue("post_slack"),
				PostSlack: &AlertRulePostActionV3Model{
					Channels:          v3Values("C1"),
					Mode:              types.StringValue("per_alert"),
					ChannelVisibility: types.StringValue("public"),
				},
			},
			payload: `{
				"type": "post_slack",
				"post_slack": {"channels": {"array_value": [{"literal": "C1"}]}, "mode": "per_alert", "channel_visibility": "public"}
			}`,
			response: `{
				"id": "01SERVERASSIGNED",
				"type": "post_slack",
				"post_slack": {"channels": {"array_value": [{"literal": "C1"}]}, "mode": "per_alert", "channel_visibility": "public"}
			}`,
		},
		{
			name: "post to a private Slack channel",
			plan: AlertRuleActionV3Model{
				ID:   types.StringValue("private"),
				Type: types.StringValue("post_slack"),
				PostSlack: &AlertRulePostActionV3Model{
					Channels:          v3Values("C1"),
					Mode:              types.StringValue("per_alert"),
					ChannelVisibility: types.StringValue("private"),
				},
			},
			payload: `{
				"id": "private",
				"type": "post_slack",
				"post_slack": {"channels": {"array_value": [{"literal": "C1"}]}, "mode": "per_alert", "channel_visibility": "private"}
			}`,
			response: `{
				"id": "private",
				"type": "post_slack",
				"post_slack": {"channels": {"array_value": [{"literal": "C1"}]}, "mode": "per_alert", "channel_visibility": "private"}
			}`,
		},
		{
			name: "post a group summary to Microsoft Teams",
			plan: AlertRuleActionV3Model{
				ID:   types.StringValue("teams"),
				Type: types.StringValue("post_microsoft_teams"),
				PostMicrosoftTeams: &AlertRulePostActionV3Model{
					Channels:          v3Array(v3Literal("C1"), v3Reference("alert.attributes.channel")),
					Mode:              types.StringValue("group_summary"),
					ChannelVisibility: types.StringValue("public"),
				},
			},
			payload: `{
				"id": "teams",
				"type": "post_microsoft_teams",
				"post_microsoft_teams": {
					"channels": {"array_value": [{"literal": "C1"}, {"reference": "alert.attributes.channel"}]},
					"mode": "group_summary",
					"channel_visibility": "public"
				}
			}`,
			response: `{
				"id": "teams",
				"type": "post_microsoft_teams",
				"post_microsoft_teams": {
					"channels": {"array_value": [{"literal": "C1"}, {"reference": "alert.attributes.channel"}]},
					"mode": "group_summary",
					"channel_visibility": "public"
				}
			}`,
		},
		{
			name: "create an incident from the default template",
			plan: AlertRuleActionV3Model{
				ID:   types.StringNull(),
				Type: types.StringValue("create_incident"),
				CreateIncident: &AlertRuleCreateIncidentActionV3Model{
					MembershipTeams: v3Values("T1"),
					AutoDecline:     types.BoolValue(true),
					IsPrivate:       types.BoolValue(false),
				},
			},
			payload: `{
				"type": "create_incident",
				"create_incident": {
					"membership_teams": {"array_value": [{"literal": "T1"}]},
					"auto_decline": true,
					"is_private": false
				}
			}`,
			response: `{
				"id": "01SERVERASSIGNED",
				"type": "create_incident",
				"create_incident": {
					"membership_teams": {"array_value": [{"literal": "T1"}]},
					"auto_decline": true,
					"is_private": false
				}
			}`,
		},
		{
			name: "investigate",
			plan: AlertRuleActionV3Model{
				ID:   types.StringNull(),
				Type: types.StringValue("investigate"),
			},
			payload:  `{"type": "investigate"}`,
			response: `{"id": "01SERVERASSIGNED", "type": "investigate"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := tc.plan.ToPayload()
			require.NoError(t, err)

			sent, err := json.Marshal(payload)
			require.NoError(t, err)
			assert.JSONEq(t, tc.payload, string(sent))

			got := alertRuleActionV3FromAPI(decodeAction(t, tc.response), &tc.plan)
			assert.Equal(t, tc.plan, got, "the response must read back as the plan that sent it")
		})
	}
}

// Leaving urgency out is how a rule pages at the alert priority's default urgency, so it
// must reach the API as no urgency at all, not as an empty one.
func TestAlertRuleEscalateUrgency(t *testing.T) {
	for _, tc := range []struct {
		name    string
		urgency *Binding
		want    *client.EngineParamBindingPayloadV3
	}{
		{"omitted sends nothing", nil, nil},
		{
			name:    "set sends the binding",
			urgency: v3ValueLiteral("URG"),
			want: &client.EngineParamBindingPayloadV3{
				Value: &client.EngineParamBindingValuePayloadV3{Literal: lo.ToPtr("URG")},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := escalateAction(AlertRuleEscalateActionV3Model{
				Users:   v3Values("U1"),
				Urgency: tc.urgency,
			}).ToPayload()
			require.NoError(t, err)
			assert.Equal(t, tc.want, payload.Escalate.Urgency)

			// And reads back the same way.
			got := escalateActionFromAPI(payload.Escalate, nil)
			assert.Equal(t, tc.urgency == nil, got.Urgency == nil)
		})
	}
}

// A setting the config left out has to plan the value the server fills in, or every plan
// after the first apply shows a change. The response is the server's for an escalate or
// create incident action that sets nothing.
func TestAlertRuleActionDefaultsMatchTheServer(t *testing.T) {
	ctx := context.Background()
	attributes := AlertRuleActionAttributes()
	escalateAttribute, ok := attributes["escalate"].(schema.SingleNestedAttribute)
	require.True(t, ok)
	escalate := escalateAttribute.Attributes
	createAttribute, ok := attributes["create_incident"].(schema.SingleNestedAttribute)
	require.True(t, ok)
	create := createAttribute.Attributes
	postAttribute, ok := attributes["post_slack"].(schema.SingleNestedAttribute)
	require.True(t, ok)
	post := postAttribute.Attributes

	server := alertRuleActionV3FromAPI(decodeAction(t, `{
		"type": "escalate",
		"escalate": {"mode": "on_each_new_alert", "grace_period_seconds": 0, "auto_cancel": false}
	}`), nil).Escalate
	serverCreate := alertRuleActionV3FromAPI(decodeAction(t, `{
		"type": "create_incident",
		"create_incident": {"auto_decline": false, "is_private": false}
	}`), nil).CreateIncident
	serverPost := alertRuleActionV3FromAPI(decodeAction(t, `{
		"type": "post_slack",
		"post_slack": {"channels": {"array_value": [{"literal": "C1"}]}, "mode": "per_alert", "channel_visibility": "public"}
	}`), nil).PostSlack

	modeAttribute, ok := escalate["mode"].(schema.StringAttribute)
	require.True(t, ok)
	var mode defaults.StringResponse
	modeAttribute.Default.DefaultString(ctx, defaults.StringRequest{}, &mode)
	assert.Equal(t, server.Mode, mode.PlanValue, "mode")

	graceAttribute, ok := escalate["grace_period_seconds"].(schema.Int64Attribute)
	require.True(t, ok)
	var grace defaults.Int64Response
	graceAttribute.Default.DefaultInt64(ctx, defaults.Int64Request{}, &grace)
	assert.Equal(t, server.GracePeriodSeconds, grace.PlanValue, "grace_period_seconds")

	visibilityAttribute, ok := post["channel_visibility"].(schema.StringAttribute)
	require.True(t, ok)
	var visibility defaults.StringResponse
	visibilityAttribute.Default.DefaultString(ctx, defaults.StringRequest{}, &visibility)
	assert.Equal(t, serverPost.ChannelVisibility, visibility.PlanValue, "channel_visibility")

	for _, tc := range []struct {
		name      string
		attribute schema.Attribute
		want      types.Bool
	}{
		{"auto_cancel", escalate["auto_cancel"], server.AutoCancel},
		{"auto_decline", create["auto_decline"], serverCreate.AutoDecline},
		{"is_private", create["is_private"], serverCreate.IsPrivate},
	} {
		attribute, ok := tc.attribute.(schema.BoolAttribute)
		require.True(t, ok, tc.name)
		var got defaults.BoolResponse
		attribute.Default.DefaultBool(ctx, defaults.BoolRequest{}, &got)
		assert.Equal(t, tc.want, got.PlanValue, tc.name)
	}
}

// The server assigns an id the config left out, and a new one on every write, so it can only
// be kept when nothing says the config left it out: an import, or an action new to the list.
func TestAlertRuleActionServerAssignedID(t *testing.T) {
	response := decodeAction(t, `{"id": "01SERVERASSIGNED", "type": "investigate"}`)

	for _, tc := range []struct {
		name  string
		prior *AlertRuleActionV3Model
		want  types.String
	}{
		{"kept on import", nil, types.StringValue("01SERVERASSIGNED")},
		{"dropped when the config left it out", &AlertRuleActionV3Model{ID: types.StringNull()}, types.StringNull()},
		{"kept when the config set one", &AlertRuleActionV3Model{ID: types.StringValue("old")}, types.StringValue("01SERVERASSIGNED")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, alertRuleActionV3FromAPI(response, tc.prior).ID)
		})
	}

	// Matched by position, so an action appended to the list has no prior.
	actions := AlertRuleActionsV3FromAPI(
		[]client.AlertRuleActionV3{response, response},
		[]AlertRuleActionV3Model{{ID: types.StringNull()}},
	)
	assert.Equal(t, []types.String{types.StringNull(), types.StringValue("01SERVERASSIGNED")},
		[]types.String{actions[0].ID, actions[1].ID})
}

// With no prior, as on import, each binding reads back in the narrowest spelling that holds
// it.
func TestAlertRuleActionImport(t *testing.T) {
	got := alertRuleActionV3FromAPI(decodeAction(t, `{
		"id": "01SERVERASSIGNED",
		"type": "escalate",
		"escalate": {
			"escalation_paths": {"value": {"reference": "expressions[\"team_path\"]"}},
			"urgency": {"value": {"literal": "URG"}},
			"mode": "on_each_new_alert",
			"grace_period_seconds": 0,
			"auto_cancel": false
		}
	}`), nil)

	want := escalateAction(AlertRuleEscalateActionV3Model{
		EscalationPaths: v3ExpressionRef("team_path"),
		Urgency:         v3ValueLiteral("URG"),
	})
	want.ID = types.StringValue("01SERVERASSIGNED")

	assert.Equal(t, want, got)
}

// `actions = []` and leaving actions out both send an empty list, and each has to read back
// the way the config wrote it.
func TestAlertRuleActionsEmptyList(t *testing.T) {
	payload, err := AlertRuleActionsV3ToPayload(nil)
	require.NoError(t, err)
	sent, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, string(sent), "the API takes an empty list, not null")

	assert.Nil(t, AlertRuleActionsV3FromAPI([]client.AlertRuleActionV3{}, nil), "omitted reads as null")
	assert.Equal(t, []AlertRuleActionV3Model{},
		AlertRuleActionsV3FromAPI([]client.AlertRuleActionV3{}, []AlertRuleActionV3Model{}), "[] reads as []")
}

func TestValidateAlertRuleActions(t *testing.T) {
	users := v3Values("U1")
	at := path.Root("actions").AtListIndex(0)

	for _, tc := range []struct {
		name   string
		action AlertRuleActionV3Model
		want   []path.Path
	}{
		{
			name:   "a valid escalate",
			action: AlertRuleActionV3Model{Type: types.StringValue("escalate"), Escalate: &AlertRuleEscalateActionV3Model{Users: users}},
		},
		{
			name:   "investigate needs nothing",
			action: AlertRuleActionV3Model{Type: types.StringValue("investigate")},
		},
		{
			name:   "the type's attribute is missing",
			action: AlertRuleActionV3Model{Type: types.StringValue("escalate")},
			want:   []path.Path{at.AtName("escalate")},
		},
		{
			name: "another type's attribute is set",
			action: AlertRuleActionV3Model{
				Type:      types.StringValue("investigate"),
				PostSlack: &AlertRulePostActionV3Model{Channels: v3Values("C1"), Mode: types.StringValue("per_alert")},
			},
			want: []path.Path{at.AtName("post_slack")},
		},
		{
			name: "a type Terraform hasn't settled matches nothing yet",
			action: AlertRuleActionV3Model{
				Type:     types.StringUnknown(),
				Escalate: &AlertRuleEscalateActionV3Model{Users: users},
			},
		},
		{
			name:   "escalate pages nobody",
			action: AlertRuleActionV3Model{Type: types.StringValue("escalate"), Escalate: &AlertRuleEscalateActionV3Model{Urgency: v3ValueLiteral("URG")}},
			want:   []path.Path{at.AtName("escalate")},
		},
		{
			name: "a grace period with on_priority_increase, which the API keeps",
			action: AlertRuleActionV3Model{Type: types.StringValue("escalate"), Escalate: &AlertRuleEscalateActionV3Model{
				Users:              users,
				Mode:               types.StringValue("on_priority_increase"),
				GracePeriodSeconds: types.Int64Value(60),
			}},
		},
		{
			name: "a binding to an expression that doesn't exist",
			action: AlertRuleActionV3Model{Type: types.StringValue("escalate"), Escalate: &AlertRuleEscalateActionV3Model{
				EscalationPaths: v3ExpressionRef("missing"),
			}},
			want: []path.Path{at.AtName("escalate").AtName("escalation_paths").AtName("expression_ref")},
		},
		{
			name: "a binding with nothing in it",
			action: AlertRuleActionV3Model{
				Type: types.StringValue("post_microsoft_teams"),
				PostMicrosoftTeams: &AlertRulePostActionV3Model{
					Channels: lo.ToPtr(NullBinding()),
					Mode:     types.StringValue("per_alert"),
				},
			},
			want: []path.Path{at.AtName("post_microsoft_teams").AtName("channels")},
		},
		{
			name: "a create incident binding in two forms",
			action: AlertRuleActionV3Model{
				Type: types.StringValue("create_incident"),
				CreateIncident: &AlertRuleCreateIncidentActionV3Model{
					IncidentTemplate: func() *Binding {
						binding := v3ValueLiteral("TPL")
						binding.ValueReference = types.StringValue("alert.template")
						return binding
					}(),
				},
			},
			want: []path.Path{at.AtName("create_incident").AtName("incident_template")},
		},
		{
			name: "a binding to a known expression",
			action: AlertRuleActionV3Model{Type: types.StringValue("escalate"), Escalate: &AlertRuleEscalateActionV3Model{
				EscalationPaths: v3ExpressionRef("team_path"),
			}},
		},
		{
			name: "a list for a single-value param",
			action: AlertRuleActionV3Model{Type: types.StringValue("escalate"), Escalate: &AlertRuleEscalateActionV3Model{
				Users:   users,
				Urgency: v3Values("URG"),
			}},
			want: []path.Path{at.AtName("escalate").AtName("urgency")},
		},
		{
			name: "an array_value for a single-value param",
			action: AlertRuleActionV3Model{Type: types.StringValue("create_incident"), CreateIncident: &AlertRuleCreateIncidentActionV3Model{
				IncidentTemplate: v3Array(v3Literal("TPL")),
			}},
			want: []path.Path{at.AtName("create_incident").AtName("incident_template")},
		},
		{
			name: "value_literal for a list param",
			action: AlertRuleActionV3Model{Type: types.StringValue("create_incident"), CreateIncident: &AlertRuleCreateIncidentActionV3Model{
				MembershipTeams: v3ValueLiteral("TEAM"),
			}},
			want: []path.Path{at.AtName("create_incident").AtName("membership_teams")},
		},
		{
			name: "a value_literal Terraform hasn't settled for a list param",
			action: AlertRuleActionV3Model{Type: types.StringValue("create_incident"), CreateIncident: &AlertRuleCreateIncidentActionV3Model{
				MembershipTeams: func() *Binding {
					binding := NullBinding()
					binding.ValueLiteral = types.StringUnknown()
					return &binding
				}(),
			}},
		},
		{
			name: "value.literal for a list param",
			action: AlertRuleActionV3Model{Type: types.StringValue("create_incident"), CreateIncident: &AlertRuleCreateIncidentActionV3Model{
				MembershipTeams: v3Value(v3Literal("TEAM")),
			}},
			want: []path.Path{at.AtName("create_incident").AtName("membership_teams")},
		},
		{
			name: "a private Slack channel by its ID",
			action: AlertRuleActionV3Model{Type: types.StringValue("post_slack"), PostSlack: &AlertRulePostActionV3Model{
				Channels: v3Values("C1"), Mode: types.StringValue("per_alert"), ChannelVisibility: types.StringValue("private"),
			}},
		},
		{
			name: "a private Slack channel in array_value",
			action: AlertRuleActionV3Model{Type: types.StringValue("post_slack"), PostSlack: &AlertRulePostActionV3Model{
				Channels: v3Array(v3Literal("C1")), Mode: types.StringValue("per_alert"), ChannelVisibility: types.StringValue("private"),
			}},
		},
		{
			name: "a private channel on Microsoft Teams",
			action: AlertRuleActionV3Model{Type: types.StringValue("post_microsoft_teams"), PostMicrosoftTeams: &AlertRulePostActionV3Model{
				Channels: v3Values("C1"), Mode: types.StringValue("per_alert"), ChannelVisibility: types.StringValue("private"),
			}},
			want: []path.Path{at.AtName("post_microsoft_teams").AtName("channel_visibility")},
		},
		{
			name: "a private post to two channels",
			action: AlertRuleActionV3Model{Type: types.StringValue("post_slack"), PostSlack: &AlertRulePostActionV3Model{
				Channels: v3Values("C1", "C2"), Mode: types.StringValue("per_alert"), ChannelVisibility: types.StringValue("private"),
			}},
			want: []path.Path{at.AtName("post_slack").AtName("channels")},
		},
		{
			name: "a private post to a referenced channel",
			action: AlertRuleActionV3Model{Type: types.StringValue("post_slack"), PostSlack: &AlertRulePostActionV3Model{
				Channels: v3ValueReference("alert.attributes.channel"), Mode: types.StringValue("per_alert"), ChannelVisibility: types.StringValue("private"),
			}},
			want: []path.Path{at.AtName("post_slack").AtName("channels")},
		},
		{
			name: "a private post to a channel ID and a referenced channel",
			action: AlertRuleActionV3Model{Type: types.StringValue("post_slack"), PostSlack: &AlertRulePostActionV3Model{
				Channels: v3Array(v3Literal("C1"), v3Reference("alert.attributes.channel")), Mode: types.StringValue("per_alert"), ChannelVisibility: types.StringValue("private"),
			}},
			want: []path.Path{at.AtName("post_slack").AtName("channels")},
		},
		{
			name: "a private Slack channel whose ID is not known yet",
			action: AlertRuleActionV3Model{Type: types.StringValue("post_slack"), PostSlack: &AlertRulePostActionV3Model{
				Channels: v3Array(BindingValue{Literal: types.StringUnknown(), Reference: types.StringNull()}), Mode: types.StringValue("per_alert"), ChannelVisibility: types.StringValue("private"),
			}},
		},
		{
			name: "a reference for a list param, which can resolve to a list",
			action: AlertRuleActionV3Model{Type: types.StringValue("create_incident"), CreateIncident: &AlertRuleCreateIncidentActionV3Model{
				MembershipTeams: v3ValueReference("alert.attributes.teams"),
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			ValidateAlertRuleActions([]AlertRuleActionV3Model{tc.action}, path.Root("actions"),
				map[string]bool{"team_path": true}, &diags)

			got := []path.Path{}
			for _, d := range diags.Errors() {
				withPath, ok := d.(diag.DiagnosticWithPath)
				require.True(t, ok, "every error points at the attribute at fault: %s", d.Summary())
				got = append(got, withPath.Path())
			}
			if tc.want == nil {
				tc.want = []path.Path{}
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// The schema has to load, and every attribute has to match a field of the model: a mismatch
// only shows when the framework first reads a plan into the model, at apply.
func TestAlertRuleActionsSchemaMatchesTheModel(t *testing.T) {
	ctx := context.Background()

	resourceSchema := schema.Schema{
		Attributes: map[string]schema.Attribute{"actions": AlertRuleActionsAttribute()},
	}
	if diags := resourceSchema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("the schema is not a valid implementation: %s", diags)
	}

	type resourceModel struct {
		Actions []AlertRuleActionV3Model `tfsdk:"actions"`
	}
	want := resourceModel{Actions: []AlertRuleActionV3Model{
		escalateAction(AlertRuleEscalateActionV3Model{Users: v3Values("U1"), Urgency: v3ValueLiteral("URG")}),
		{
			ID:        types.StringValue("slack"),
			Type:      types.StringValue("post_slack"),
			PostSlack: &AlertRulePostActionV3Model{Channels: v3Values("C1"), Mode: types.StringValue("per_alert"), ChannelVisibility: types.StringValue("private")},
		},
		{
			ID:   types.StringNull(),
			Type: types.StringValue("create_incident"),
			CreateIncident: &AlertRuleCreateIncidentActionV3Model{
				IncidentTemplate: v3ValueLiteral("TPL"),
				AutoDecline:      types.BoolValue(false),
				IsPrivate:        types.BoolValue(true),
			},
		},
	}}

	state := tfsdk.State{
		Schema: resourceSchema,
		Raw:    tftypes.NewValue(resourceSchema.Type().TerraformType(ctx), nil),
	}
	require.False(t, state.Set(ctx, &want).HasError(), "the model must write to state")

	var got resourceModel
	require.False(t, state.Get(ctx, &got).HasError(), "and read back from it")
	assert.Equal(t, want, got)
}
