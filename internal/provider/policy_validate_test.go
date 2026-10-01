package provider

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timetypes/timetypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/jsontypes"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

func policySchema(t *testing.T) resource.SchemaResponse {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentPolicyResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	return schemaResp
}

// TestPolicyModelMatchesSchema writes a fully populated model into state through the
// schema. The model is hand-written against a hand-written schema, so a field either side
// forgets is only caught here: at apply it surfaces as "struct defines fields not found in
// object", long after the plan looked fine.
func TestPolicyModelMatchesSchema(t *testing.T) {
	schemaResp := policySchema(t)

	// Built from NullParamBinding, not the zero value: a binding's array_value, value and
	// values are framework types, whose zero values carry no element or attribute type and
	// so can't be written to state.
	binding := models.NullParamBinding()
	binding.ValueLiteral = jsontypes.NewNormalizedJSONOrStringValue("5")

	model := &incidentPolicyResourceModel{
		ID:          types.StringValue("01POLICY"),
		Name:        types.StringValue("Post-mortems within 5 working days"),
		Description: types.StringValue("Major+ incidents need a post-mortem"),
		Status:      types.StringValue("enabled"),
		PolicyType:  types.StringValue("post_mortem"),
		ConditionGroups: models.IncidentEngineConditionGroups{
			{Conditions: models.IncidentEngineConditions{
				{
					Subject:       types.StringValue("incident.severity"),
					Operation:     types.StringValue("gte"),
					ParamBindings: models.IncidentEngineParamBindings{binding},
				},
			}},
		},
		AssignmentRules: &incidentPolicyAssignmentRules{
			Bindings:                   models.IncidentEngineParamBindings{binding},
			ReminderDueDateOffsetHours: []types.Int64{types.Int64Value(-24)},
			ReminderCadenceBefore:      &incidentPolicyReminderCadence{Interval: types.StringValue("weekly")},
			ReminderCadenceAfter:       &incidentPolicyReminderCadence{Interval: types.StringValue("daily")},
		},
		PostMortem: &incidentPolicyIncidentConfig{
			Requirements:          models.IncidentEngineConditionGroups{},
			RunOnPrivateIncidents: types.BoolValue(false),
			DueDateConfig: &incidentPolicyDueDateConfig{
				IncidentTimestampID: types.StringValue("01TIMESTAMP"),
				Days:                binding,
				CalculationType:     types.StringValue("weekdays"),
				CalculationTimezone: types.StringNull(),
				AppliesFrom:         timetypes.NewRFC3339Null(),
			},
		},
	}

	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("model does not match schema: %+v", diags)
	}
}

// TestPolicyOnCallReadinessModelMatchesSchema covers the one block the post-mortem model
// above leaves out, so neither branch of the union goes unexercised.
func TestPolicyOnCallReadinessModelMatchesSchema(t *testing.T) {
	schemaResp := policySchema(t)

	model := &incidentPolicyResourceModel{
		ID:              types.StringValue("01POLICY"),
		Name:            types.StringValue("Responders carry a phone"),
		Description:     types.StringValue("On-call users need a phone method"),
		Status:          types.StringValue("enabled"),
		PolicyType:      types.StringValue("on_call_readiness"),
		ConditionGroups: models.IncidentEngineConditionGroups{},
		OnCallReadiness: &incidentPolicyOnCallReadiness{
			Enforcement: types.StringValue("advisory"),
			HighUrgency: []incidentPolicyReadinessRule{
				{
					MethodTypes:     []types.String{types.StringValue("phone")},
					MaxDelaySeconds: types.Int64Value(300),
				},
			},
		},
	}

	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("model does not match schema: %+v", diags)
	}
}

// TestPolicyVacationConflictModelMatchesSchema covers the marker block, whose whole job is
// to make "exactly one block" hold for a type with nothing to configure.
func TestPolicyVacationConflictModelMatchesSchema(t *testing.T) {
	schemaResp := policySchema(t)

	model := &incidentPolicyResourceModel{
		ID:               types.StringValue("01POLICY"),
		Name:             types.StringValue("No on-call during vacation"),
		Description:      types.StringValue("Flag responders rota'd on while away"),
		Status:           types.StringValue("enabled"),
		PolicyType:       types.StringValue("vacation_conflict"),
		ConditionGroups:  models.IncidentEngineConditionGroups{},
		VacationConflict: &incidentPolicyVacationConflict{},
	}

	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("model does not match schema: %+v", diags)
	}
}

func TestPolicyTypeDerivedFromBlock(t *testing.T) {
	for _, tc := range []struct {
		want  string
		model *incidentPolicyResourceModel
	}{
		{"follow_up", &incidentPolicyResourceModel{FollowUp: &incidentPolicyIncidentConfig{}}},
		{"debrief", &incidentPolicyResourceModel{Debrief: &incidentPolicyIncidentConfig{}}},
		{"post_mortem", &incidentPolicyResourceModel{PostMortem: &incidentPolicyIncidentConfig{}}},
		{"schedule", &incidentPolicyResourceModel{Schedule: &incidentPolicySchedule{}}},
		{"on_call_readiness", &incidentPolicyResourceModel{OnCallReadiness: &incidentPolicyOnCallReadiness{}}},
		{"vacation_conflict", &incidentPolicyResourceModel{VacationConflict: &incidentPolicyVacationConflict{}}},
		{"", &incidentPolicyResourceModel{}},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.model.policyType(); got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestPolicyBlocksCoverEveryPolicyType is the guard for a policy type added to the API
// later. Without a block of its own it would be unreachable from Terraform, and because
// ExactlyOneOf spans the blocks rather than the enum, nothing else would notice.
func TestPolicyBlocksCoverEveryPolicyType(t *testing.T) {
	fromAPI := apischema.EnumValues("PolicyV2", "policy_type")
	if len(fromAPI) == 0 {
		t.Fatal("no policy_type enum values in the schema")
	}

	// Policy types the API has that the resource deliberately doesn't support yet. Each
	// one is a block still to be written; remove it from here when it lands.
	notYetSupported := map[string]bool{
		"shift_conflict": true,
	}

	blocks := map[string]bool{}
	for _, block := range policyBlocks {
		blocks[block] = true
	}

	covered := 0
	for _, policyType := range fromAPI {
		if notYetSupported[policyType] {
			if blocks[policyType] {
				t.Errorf("policy type %q has a config block now: drop it from notYetSupported", policyType)
			}
			continue
		}
		covered++

		if !blocks[policyType] {
			t.Errorf("policy type %q has no config block: add one, and add it to policyBlocks", policyType)
		}
	}

	if len(policyBlocks) != covered {
		t.Errorf("want %d blocks to match the enum, got %d", covered, len(policyBlocks))
	}
}

// TestPolicyConfigValidators pins the rules the resource leans on, so removing one doesn't
// quietly take the enforcement with it.
func TestPolicyConfigValidators(t *testing.T) {
	validators := (&incidentPolicyResource{}).ConfigValidators(context.Background())
	if want := 2 + len(policyTypesWithForcedAssignee); len(validators) != want {
		t.Fatalf("want %d config validators, got %d", want, len(validators))
	}

	descriptions := []string{}
	for _, validator := range validators {
		descriptions = append(descriptions, validator.Description(context.Background()))
	}

	for _, block := range policyBlocks {
		if !strings.Contains(descriptions[0], block) {
			t.Errorf("ExactlyOneOf does not cover %q: %s", block, descriptions[0])
		}
	}

	// Every type that picks its own assignee needs a Conflicting rule of its own, against
	// the bindings only: its reminders are still configurable.
	for idx, block := range policyTypesWithForcedAssignee {
		description := descriptions[idx+1]
		if !strings.Contains(description, "assignment_rules.bindings") || !strings.Contains(description, block) {
			t.Errorf("Conflicting does not cover %q with assignment_rules.bindings: %s", block, description)
		}
	}

	if _, ok := validators[len(validators)-1].(policyBindingsRequiredValidator); !ok {
		t.Error("the last validator does not require bindings for the other types")
	}
}

// importPrior is the state a read sees during an import: just the ID, so prior is a model
// with every other field nil rather than a nil pointer.
func importPrior() *incidentPolicyResourceModel {
	return &incidentPolicyResourceModel{ID: types.StringValue("01POLICY")}
}

// TestPolicyImportKeepsAssignmentRules covers the read an import performs, which would
// otherwise drop the assignees because the prior state carried none.
func TestPolicyImportKeepsAssignmentRules(t *testing.T) {
	policy := client.PolicyV2{
		Id:         "01POLICY",
		Name:       "Post-mortems",
		Status:     client.PolicyV2StatusEnabled,
		PolicyType: client.PolicyV2PolicyTypePostMortem,
		AssignmentRules: &client.PolicyAssignmentRulesV2{
			Bindings: []client.EngineParamBindingV2{
				{ArrayValue: &[]client.EngineParamBindingValueV2{{Literal: lo.ToPtr("01USER")}}},
			},
			ReminderDueDateOffsetHours: []int64{-24},
		},
		PostMortem: &client.PolicyPostMortemV2{},
	}

	model := policyFromAPI(policy, importPrior())
	if model.AssignmentRules == nil {
		t.Fatal("assignment_rules were dropped on import")
	}
	if len(model.AssignmentRules.Bindings) != 1 {
		t.Errorf("want 1 assignee binding, got %d", len(model.AssignmentRules.Bindings))
	}
}

// TestPolicyReadDropsForcedAssignee is the other half: the types that pick their own
// assignee must never carry one into state, whatever the read is serving. An import has no
// prior to notice it with, so this has to hold on the type alone.
func TestPolicyReadDropsForcedAssignee(t *testing.T) {
	forced := &client.PolicyAssignmentRulesV2{
		Bindings: []client.EngineParamBindingV2{
			{ArrayValue: &[]client.EngineParamBindingValueV2{{Reference: lo.ToPtr("on_call_user")}}},
		},
		ReminderDueDateOffsetHours: []int64{},
	}

	for _, policyType := range policyTypesWithForcedAssignee {
		policy := client.PolicyV2{
			Id:              "01POLICY",
			Name:            "Picks its own assignee",
			Status:          client.PolicyV2StatusEnabled,
			PolicyType:      client.PolicyV2PolicyType(policyType),
			AssignmentRules: forced,
		}

		// policy_type is unknown rather than null in a create plan, which is what keeps
		// that case apart from an import.
		for name, prior := range map[string]*incidentPolicyResourceModel{
			"import": importPrior(),
			"create": {PolicyType: types.StringUnknown()},
		} {
			t.Run(policyType+"/"+name, func(t *testing.T) {
				if model := policyFromAPI(policy, prior); model.AssignmentRules != nil {
					t.Error("kept the assignee the API picked for itself")
				}
			})
		}
	}
}

// TestPolicyTypesWithForcedAssigneeAreRealTypes guards the list against a typo, which would
// otherwise silently stop dropping the assignee for that type.
func TestPolicyTypesWithForcedAssigneeAreRealTypes(t *testing.T) {
	fromAPI := apischema.EnumValues("PolicyV2", "policy_type")

	for _, policyType := range policyTypesWithForcedAssignee {
		if !slices.Contains(fromAPI, policyType) {
			t.Errorf("%q is not a policy type: %v", policyType, fromAPI)
		}
	}
}

// TestPolicyDueDateConfigRequired pins the rule for the three types that carry a due date.
// The API rejects an update to one without a due_date_config, so a config that omits it
// creates a policy and then fails on its next apply.
func TestPolicyDueDateConfigRequired(t *testing.T) {
	attributes := policySchema(t).Schema.Attributes

	for _, block := range []string{"follow_up", "debrief", "post_mortem"} {
		t.Run(block, func(t *testing.T) {
			nested, ok := attributes[block].(schema.SingleNestedAttribute)
			if !ok {
				t.Fatalf("%s is not a single nested attribute", block)
			}

			if !nested.Attributes["due_date_config"].IsRequired() {
				t.Error("due_date_config is not required")
			}
		})
	}

	// The other three don't carry a due date at all, and the API rejects one. Their
	// blocks have no due_date_config attribute to require.
	for _, block := range []string{"schedule", "on_call_readiness", "vacation_conflict"} {
		t.Run(block, func(t *testing.T) {
			nested, ok := attributes[block].(schema.SingleNestedAttribute)
			if !ok {
				t.Fatalf("%s is not a single nested attribute", block)
			}

			if _, found := nested.Attributes["due_date_config"]; found {
				t.Error("due_date_config should not exist on a type without a due date")
			}
		})
	}
}

// TestPolicyReadKeepsForcedAssigneeReminders covers the reminders on a type that picks its
// own assignee: they are the caller's to set, so a read keeps them and drops only the
// assignee, on an import as much as on a refresh.
func TestPolicyReadKeepsForcedAssigneeReminders(t *testing.T) {
	rules := &client.PolicyAssignmentRulesV2{
		Bindings: []client.EngineParamBindingV2{
			{ArrayValue: &[]client.EngineParamBindingValueV2{{Reference: lo.ToPtr("on_call_user")}}},
		},
		ReminderDueDateOffsetHours: []int64{24},
		ReminderCadenceAfter:       &client.PolicyReminderCadenceV2{Interval: "daily"},
	}

	configured := &incidentPolicyAssignmentRules{
		ReminderDueDateOffsetHours: []types.Int64{types.Int64Value(24)},
		ReminderCadenceAfter:       &incidentPolicyReminderCadence{Interval: types.StringValue("daily")},
	}

	for _, policyType := range policyTypesWithForcedAssignee {
		policy := client.PolicyV2{
			Id:              "01POLICY",
			Name:            "Picks its own assignee",
			Status:          client.PolicyV2StatusEnabled,
			PolicyType:      client.PolicyV2PolicyType(policyType),
			AssignmentRules: rules,
		}

		for name, prior := range map[string]*incidentPolicyResourceModel{
			"import":  importPrior(),
			"refresh": {PolicyType: types.StringValue(policyType), AssignmentRules: configured},
		} {
			t.Run(policyType+"/"+name, func(t *testing.T) {
				model := policyFromAPI(policy, prior)
				if model.AssignmentRules == nil {
					t.Fatal("dropped the reminders along with the assignee")
				}
				if model.AssignmentRules.Bindings != nil {
					t.Error("kept the assignee the API picked for itself")
				}
				if len(model.AssignmentRules.ReminderDueDateOffsetHours) != 1 ||
					model.AssignmentRules.ReminderCadenceAfter == nil {
					t.Errorf("lost reminders: %+v", model.AssignmentRules)
				}
			})
		}
	}

	// A config that writes the block with no reminders in it reads back the same block,
	// rather than the null an import with nothing to keep would get.
	empty := &incidentPolicyAssignmentRules{ReminderDueDateOffsetHours: []types.Int64{}}
	policy := client.PolicyV2{
		Id:         "01POLICY",
		Name:       "Picks its own assignee",
		Status:     client.PolicyV2StatusEnabled,
		PolicyType: client.PolicyV2PolicyTypeOnCallReadiness,
		AssignmentRules: &client.PolicyAssignmentRulesV2{
			Bindings:                   rules.Bindings,
			ReminderDueDateOffsetHours: []int64{},
		},
	}
	prior := &incidentPolicyResourceModel{PolicyType: types.StringUnknown(), AssignmentRules: empty}
	if model := policyFromAPI(policy, prior); model.AssignmentRules == nil {
		t.Error("dropped an empty assignment_rules block the config wrote")
	}
}
