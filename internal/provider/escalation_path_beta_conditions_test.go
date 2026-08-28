package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// workingHoursIf is the branch condition the tests reach for by default. The id matches the
// working hours the sequence fixtures declare, so a branch built from it resolves.
func workingHoursIf() *escalationPathBetaBranchIf {
	return &escalationPathBetaBranchIf{
		WorkingHoursActive: types.StringValue("UK"),
		PriorityOneOf:      types.SetNull(types.StringType),
	}
}

func priorityIf(t *testing.T, ids ...string) *escalationPathBetaBranchIf {
	t.Helper()

	set, diags := types.SetValueFrom(context.Background(), types.StringType, ids)
	if diags.HasError() {
		t.Fatalf("building priority set: %+v", diags)
	}
	return &escalationPathBetaBranchIf{
		WorkingHoursActive: types.StringNull(),
		PriorityOneOf:      set,
	}
}

// apiConditions converts payload conditions into the response shape, so a round-trip test
// can build its input by converting a config rather than hand-writing both.
func apiConditions(conditions []client.ConditionPayloadV2) []client.ConditionV2 {
	return lo.Map(conditions, func(condition client.ConditionPayloadV2, _ int) client.ConditionV2 {
		return client.ConditionV2{
			Subject:   client.ConditionSubjectV2{Reference: condition.Subject},
			Operation: client.ConditionOperationV2{Value: condition.Operation},
			ParamBindings: lo.Map(condition.ParamBindings, func(binding client.EngineParamBindingPayloadV2, _ int) client.EngineParamBindingV2 {
				values := lo.Map(lo.FromPtr(binding.ArrayValue), func(value client.EngineParamBindingValuePayloadV2, _ int) client.EngineParamBindingValueV2 {
					return client.EngineParamBindingValueV2{Literal: value.Literal, Reference: value.Reference}
				})
				return client.EngineParamBindingV2{ArrayValue: &values}
			}),
		}
	})
}

// workingHoursConditions is the API's rendering of a branch on working hours, for tests
// building a path the config never wrote.
func workingHoursConditions(id string) []client.ConditionV2 {
	return []client.ConditionV2{{
		Subject:       client.ConditionSubjectV2{Reference: fmt.Sprintf(escalationWorkingHoursSubject, id)},
		Operation:     client.ConditionOperationV2{Value: escalationOperationIsActive},
		ParamBindings: []client.EngineParamBindingV2{},
	}}
}

func TestEscalationPathBetaBranchIfToPayload(t *testing.T) {
	ctx := context.Background()

	t.Run("builds the working hours condition from an id", func(t *testing.T) {
		var diags diag.Diagnostics
		got := workingHoursIf().toPayload(ctx, &diags)

		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if len(got) != 1 {
			t.Fatalf("got %d conditions, want 1", len(got))
		}
		if got[0].Subject != `escalation.working_hours["UK"]` {
			t.Errorf("got subject %q", got[0].Subject)
		}
		if got[0].Operation != "is_active" {
			t.Errorf("got operation %q, want is_active", got[0].Operation)
		}
		if len(got[0].ParamBindings) != 0 {
			t.Errorf("got %d param bindings, want none", len(got[0].ParamBindings))
		}
	})

	t.Run("builds the priority condition from a set of ids", func(t *testing.T) {
		var diags diag.Diagnostics
		got := priorityIf(t, "01AAA", "01BBB").toPayload(ctx, &diags)

		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if len(got) != 1 {
			t.Fatalf("got %d conditions, want 1", len(got))
		}
		if got[0].Subject != "escalation.priority" || got[0].Operation != "one_of" {
			t.Errorf("got %q %q, want escalation.priority one_of", got[0].Subject, got[0].Operation)
		}
		if len(got[0].ParamBindings) != 1 {
			t.Fatalf("got %d param bindings, want 1", len(got[0].ParamBindings))
		}
		literals := lo.Map(lo.FromPtr(got[0].ParamBindings[0].ArrayValue), func(v client.EngineParamBindingValuePayloadV2, _ int) string {
			return lo.FromPtr(v.Literal)
		})
		if !lo.Contains(literals, "01AAA") || !lo.Contains(literals, "01BBB") {
			t.Errorf("got literals %v, want both priority ids", literals)
		}
	})

	// The API rejects a branch carrying no condition, so an empty payload would fail the
	// apply with its error rather than one naming the attribute.
	t.Run("refuses to build a branch that tests nothing", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			branchIf *escalationPathBetaBranchIf
		}{
			{"neither attribute set", &escalationPathBetaBranchIf{
				WorkingHoursActive: types.StringNull(),
				PriorityOneOf:      types.SetNull(types.StringType),
			}},
			{"an empty working hours id", &escalationPathBetaBranchIf{
				WorkingHoursActive: types.StringValue(""),
				PriorityOneOf:      types.SetNull(types.StringType),
			}},
			{"an empty priority set", &escalationPathBetaBranchIf{
				WorkingHoursActive: types.StringNull(),
				PriorityOneOf:      types.SetValueMust(types.StringType, []attr.Value{}),
			}},
			{"no if at all", nil},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var diags diag.Diagnostics
				if got := tc.branchIf.toPayload(ctx, &diags); len(got) != 0 {
					t.Errorf("got %d conditions, want none", len(got))
				}
				if !diags.HasError() {
					t.Error("expected an error")
				}
			})
		}
	})

	t.Run("refuses to drop one of two tests", func(t *testing.T) {
		var diags diag.Diagnostics
		both := workingHoursIf()
		both.PriorityOneOf = priorityIf(t, "01AAA").PriorityOneOf

		if got := both.toPayload(ctx, &diags); len(got) != 0 {
			t.Errorf("got %d conditions, want none", len(got))
		}
		if !diags.HasError() {
			t.Error("expected an error")
		}
	})
}

func TestEscalationPathBetaBranchIfFromAPI(t *testing.T) {
	ctx := context.Background()

	t.Run("round-trips working hours", func(t *testing.T) {
		var diags diag.Diagnostics
		want := workingHoursIf()
		got := escalationPathBetaBranchIfFromAPI(ctx, apiConditions(want.toPayload(ctx, &diags)), &diags)

		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if got.WorkingHoursActive.ValueString() != "UK" || !got.PriorityOneOf.IsNull() {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("round-trips priorities", func(t *testing.T) {
		var diags diag.Diagnostics
		want := priorityIf(t, "01AAA", "01BBB")
		got := escalationPathBetaBranchIfFromAPI(ctx, apiConditions(want.toPayload(ctx, &diags)), &diags)

		if diags.HasError() {
			t.Fatalf("unexpected errors: %+v", diags)
		}
		if !got.WorkingHoursActive.IsNull() {
			t.Errorf("got working hours %q, want null", got.WorkingHoursActive.ValueString())
		}
		if !got.PriorityOneOf.Equal(want.PriorityOneOf) {
			t.Errorf("got %v, want %v", got.PriorityOneOf, want.PriorityOneOf)
		}
	})

	for _, tc := range []struct {
		name       string
		conditions []client.ConditionV2
		wantError  string
	}{
		{
			name:       "a branch with no conditions",
			conditions: nil,
			wantError:  "tests 0 conditions",
		},
		{
			name:       "a branch testing two things",
			conditions: append(workingHoursConditions("UK"), workingHoursConditions("US")...),
			wantError:  "tests 2 conditions",
		},
		{
			name: "working hours tested with another operation",
			conditions: []client.ConditionV2{{
				Subject:   client.ConditionSubjectV2{Reference: `escalation.working_hours["UK"]`},
				Operation: client.ConditionOperationV2{Value: "is_set"},
			}},
			wantError: `working hours are "is_set"`,
		},
		{
			name: "priority tested with another operation",
			conditions: []client.ConditionV2{{
				Subject:   client.ConditionSubjectV2{Reference: "escalation.priority"},
				Operation: client.ConditionOperationV2{Value: "not_one_of"},
			}},
			wantError: `priority is "not_one_of"`,
		},
		{
			name: "priority bound to a reference rather than ids",
			conditions: []client.ConditionV2{{
				Subject:   client.ConditionSubjectV2{Reference: "escalation.priority"},
				Operation: client.ConditionOperationV2{Value: "one_of"},
				ParamBindings: []client.EngineParamBindingV2{{
					Value: &client.EngineParamBindingValueV2{Reference: lo.ToPtr("escalation.priority")},
				}},
			}},
			wantError: "other than a list of priority ids",
		},
		{
			// The API parses the id with [^"]+ too, so a subject it couldn't read
			// shouldn't read here as working hours either.
			name: "working hours with no id in the subject",
			conditions: []client.ConditionV2{{
				Subject:   client.ConditionSubjectV2{Reference: `escalation.working_hours[""]`},
				Operation: client.ConditionOperationV2{Value: "is_active"},
			}},
			wantError: "neither the priority nor a set of working hours",
		},
		{
			name: "a subject the resource knows nothing about",
			conditions: []client.ConditionV2{{
				Subject:   client.ConditionSubjectV2{Reference: "escalation.something_else"},
				Operation: client.ConditionOperationV2{Value: "is_set"},
			}},
			wantError: "neither the priority nor a set of working hours",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			escalationPathBetaBranchIfFromAPI(ctx, tc.conditions, &diags)

			if !diags.HasError() {
				t.Fatal("expected an error")
			}
			for _, d := range diags.Errors() {
				if strings.Contains(d.Detail(), tc.wantError) {
					return
				}
			}
			t.Errorf("no error mentioned %q, got %+v", tc.wantError, diags.Errors())
		})
	}
}

func TestValidateSequenceConditions(t *testing.T) {
	ctx := context.Background()

	branchWith := func(branchIf *escalationPathBetaBranchIf) escalationPathBetaNode {
		node := branchNode("urgent", "")
		node.Branch.If = branchIf
		return node
	}

	for _, tc := range []struct {
		name         string
		branchIf     *escalationPathBetaBranchIf
		workingHours []string
		wantError    string
	}{
		{
			name:         "a branch on working hours the path declares",
			branchIf:     workingHoursIf(),
			workingHours: []string{"UK"},
		},
		{
			name:     "a branch on priority",
			branchIf: priorityIf(t, "01AAA"),
		},
		{
			name:      "a branch on working hours the path doesn't declare",
			branchIf:  workingHoursIf(),
			wantError: `no working hours with the id "UK"`,
		},
		{
			name: "a branch testing nothing",
			branchIf: &escalationPathBetaBranchIf{
				WorkingHoursActive: types.StringNull(),
				PriorityOneOf:      types.SetNull(types.StringType),
			},
			wantError: "Set working_hours_active or priority_one_of",
		},
		{
			name: "a branch testing both",
			branchIf: &escalationPathBetaBranchIf{
				WorkingHoursActive: types.StringValue("UK"),
				PriorityOneOf:      priorityIf(t, "01AAA").PriorityOneOf,
			},
			workingHours: []string{"UK"},
			wantError:    "a branch tests one thing",
		},
		{
			// Either could still resolve to null, so calling this two tests reports a
			// problem an apply won't hit.
			name: "a branch whose tests another resource hasn't computed yet",
			branchIf: &escalationPathBetaBranchIf{
				WorkingHoursActive: types.StringUnknown(),
				PriorityOneOf:      types.SetUnknown(types.StringType),
			},
			workingHours: []string{"UK"},
		},
		{
			name: "a branch naming no working hours at all",
			branchIf: &escalationPathBetaBranchIf{
				WorkingHoursActive: types.StringValue(""),
				PriorityOneOf:      types.SetNull(types.StringType),
			},
			wantError: "working_hours_active is empty",
		},
		{
			name: "a branch matching no priority at all",
			branchIf: &escalationPathBetaBranchIf{
				WorkingHoursActive: types.StringNull(),
				PriorityOneOf:      types.SetValueMust(types.StringType, []attr.Value{}),
			},
			wantError: "priority_one_of is empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := betaModel(t, "main", map[string][]escalationPathBetaNode{
				"main":   {branchWith(tc.branchIf)},
				"urgent": {levelNode(t, "")},
			})
			data.WorkingHours = workingHoursList(t, tc.workingHours)

			var diags diag.Diagnostics
			validateSequenceConditions(ctx, data, &diags)

			if tc.wantError == "" {
				if diags.HasError() {
					t.Fatalf("expected no errors, got %+v", diags)
				}
				return
			}

			if !diags.HasError() {
				t.Fatal("expected an error")
			}
			for _, d := range diags.Errors() {
				if strings.Contains(d.Detail(), tc.wantError) {
					return
				}
			}
			t.Errorf("no error mentioned %q, got %+v", tc.wantError, diags.Errors())
		})
	}
}

// workingHoursList builds the working_hours attribute holding these ids, which is all the
// condition checks read out of it.
func workingHoursList(t *testing.T, ids []string) types.List {
	t.Helper()

	configType := types.ObjectType{AttrTypes: models.WeekdayIntervalConfigAttrTypes()}
	if len(ids) == 0 {
		return types.ListNull(configType)
	}

	configs := lo.Map(ids, func(id string, _ int) models.IncidentWeekdayIntervalConfig {
		return models.IncidentWeekdayIntervalConfig{
			ID:               types.StringValue(id),
			Name:             types.StringValue(id),
			Timezone:         types.StringValue("Europe/London"),
			WeekdayIntervals: types.ListNull(types.ObjectType{AttrTypes: models.WeekdayIntervalAttrTypes()}),
		}
	})

	list, diags := types.ListValueFrom(context.Background(), configType, configs)
	if diags.HasError() {
		t.Fatalf("building working hours: %+v", diags)
	}
	return list
}
