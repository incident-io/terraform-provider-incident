package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

// A branch tests one thing: the escalation's priority, or whether one of the path's
// working hours is active. Those are all an escalation knows about itself, and the API
// takes exactly one condition on a branch, so this resource takes one attribute per test
// and builds the condition from whichever the author set.

const (
	// The references the escalation scope holds, which is what a condition's subject names.
	escalationPrioritySubject = "escalation.priority"

	// Built and read exactly as the escalation scope registers it, quotes and all. %q
	// would escape an id the scope leaves alone, giving a subject that matches nothing.
	escalationWorkingHoursSubject = `escalation.working_hours["%s"]`

	escalationOperationOneOf    = "one_of"
	escalationOperationIsActive = "is_active"
)

// escalationWorkingHoursSubjectPattern reads the working hours id back out of a subject.
// It matches what the API parses, so an id we couldn't hand back to it reads as a
// subject we don't recognise rather than as working hours we'd then fail to find.
var escalationWorkingHoursSubjectPattern = regexp.MustCompile(`^escalation\.working_hours\["([^"]+)"\]$`)

// escalationPathBetaBranchIf is what a branch tests. Exactly one attribute is set, which
// validateSequenceConditions enforces, and which one it is takes the place of the subject
// and operation the API carries.
type escalationPathBetaBranchIf struct {
	WorkingHoursActive types.String `tfsdk:"working_hours_active"`
	PriorityOneOf      types.Set    `tfsdk:"priority_one_of"`
}

// escalationPathBetaBranchIfAttrTypes returns the attribute types for a branch's if object.
func escalationPathBetaBranchIfAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"working_hours_active": types.StringType,
		"priority_one_of":      types.SetType{ElemType: types.StringType},
	}
}

// escalationPathBetaBranchIfAttribute returns the if attribute's schema.
func escalationPathBetaBranchIfAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "What the branch tests. Set exactly one of these: a branch tests one thing, so combining them means nesting a second branch inside the first.",
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"working_hours_active": schema.StringAttribute{
				MarkdownDescription: "The `id` of one of this escalation path's `working_hours`, met while those hours are active.",
				Optional:            true,
			},
			"priority_one_of": schema.SetAttribute{
				MarkdownDescription: "Alert priority ids, met when the escalation came in at one of them.",
				Optional:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

// toPayload builds the one condition the API stores, reporting a branch that tests both
// things or neither rather than sending a payload the API would reject.
//
// validateSequenceConditions reports the same two, but only for a config it can read: one
// whose sequences another resource computes reaches here unchecked.
func (i *escalationPathBetaBranchIf) toPayload(ctx context.Context, diags *diag.Diagnostics) []client.ConditionPayloadV2 {
	none := []client.ConditionPayloadV2{}
	nothingToTest := func() []client.ConditionPayloadV2 {
		diags.AddError(
			"Branch tests nothing",
			"A branch sets neither working_hours_active nor priority_one_of, so it has nothing to choose on.",
		)
		return none
	}

	// if is Required, so a branch reaching here without one isn't something a config can
	// say. It would still leave the API with a branch testing nothing.
	if i == nil {
		return nothingToTest()
	}

	workingHours := i.WorkingHoursActive.ValueString()
	priorities := escalationPathBetaPriorityIDs(ctx, i.PriorityOneOf, diags)
	if diags.HasError() {
		return none
	}

	switch {
	case workingHours != "" && !i.PriorityOneOf.IsNull():
		diags.AddError(
			"Branch tests more than one thing",
			"A branch sets working_hours_active and priority_one_of, and a branch tests one thing.",
		)
		return none

	case workingHours != "":
		return []client.ConditionPayloadV2{{
			Subject:       fmt.Sprintf(escalationWorkingHoursSubject, workingHours),
			Operation:     escalationOperationIsActive,
			ParamBindings: []client.EngineParamBindingPayloadV2{},
		}}

	case len(priorities) > 0:
		values := lo.Map(priorities, func(id string, _ int) client.EngineParamBindingValuePayloadV2 {
			return client.EngineParamBindingValuePayloadV2{Literal: lo.ToPtr(id)}
		})
		return []client.ConditionPayloadV2{{
			Subject:   escalationPrioritySubject,
			Operation: escalationOperationOneOf,
			ParamBindings: []client.EngineParamBindingPayloadV2{
				{ArrayValue: &values},
			},
		}}

	default:
		return nothingToTest()
	}
}

// escalationPathBetaPriorityIDs decodes the priority set, treating an unset one as empty.
func escalationPathBetaPriorityIDs(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	ids := []string{}
	diags.Append(set.ElementsAs(ctx, &ids, false)...)
	if diags.HasError() {
		return nil
	}
	return ids
}

// escalationPathBetaBranchIfFromAPI reads a branch's condition back into the attribute
// that holds it, reporting one it can't: the engine allows conditions neither this
// resource nor our dashboard builds.
func escalationPathBetaBranchIfFromAPI(ctx context.Context, conditions []client.ConditionV2, diags *diag.Diagnostics) *escalationPathBetaBranchIf {
	out := &escalationPathBetaBranchIf{
		WorkingHoursActive: types.StringNull(),
		PriorityOneOf:      types.SetNull(types.StringType),
	}

	unsupported := func(detail string) *escalationPathBetaBranchIf {
		diags.AddError(
			"Escalation path branch can't be represented",
			fmt.Sprintf("%s Use incident_escalation_path for this path.", detail),
		)
		return out
	}

	// The API takes one condition on a branch, so this isn't a limit of the flat shape.
	if len(conditions) != 1 {
		return unsupported(fmt.Sprintf("A branch tests %d conditions, and a branch tests one thing.", len(conditions)))
	}

	condition := conditions[0]
	subject, operation := condition.Subject.Reference, condition.Operation.Value

	if workingHours := escalationWorkingHoursSubjectPattern.FindStringSubmatch(subject); workingHours != nil {
		if operation != escalationOperationIsActive {
			return unsupported(fmt.Sprintf("A branch tests whether working hours are %q.", operation))
		}
		out.WorkingHoursActive = types.StringValue(workingHours[1])
		return out
	}

	if subject == escalationPrioritySubject {
		if operation != escalationOperationOneOf {
			return unsupported(fmt.Sprintf("A branch tests whether the priority is %q.", operation))
		}
		ids, ok := escalationPathBetaPriorityIDsFromAPI(condition.ParamBindings)
		if !ok {
			return unsupported("A branch tests the priority against something other than a list of priority ids.")
		}
		set, d := types.SetValueFrom(ctx, types.StringType, ids)
		diags.Append(d...)
		out.PriorityOneOf = set
		return out
	}

	return unsupported(fmt.Sprintf("A branch tests %s, which is neither the priority nor a set of working hours.", subject))
}

// escalationPathBetaPriorityIDsFromAPI pulls the priority ids out of a one_of's bindings,
// reporting false for a binding holding anything but literals.
func escalationPathBetaPriorityIDsFromAPI(bindings []client.EngineParamBindingV2) ([]string, bool) {
	ids := []string{}
	for _, binding := range bindings {
		if binding.Value != nil || binding.ArrayValue == nil {
			return nil, false
		}
		for _, value := range *binding.ArrayValue {
			if value.Literal == nil {
				return nil, false
			}
			ids = append(ids, *value.Literal)
		}
	}
	if len(ids) == 0 {
		return nil, false
	}
	return ids, true
}

// validateSequenceConditions checks each branch tests one thing, and that the working
// hours it names are ones this path declares.
func validateSequenceConditions(ctx context.Context, data *escalationPathBetaModel, diags *diag.Diagnostics) {
	workingHoursIDs := escalationPathBetaWorkingHoursIDs(ctx, data.WorkingHours, diags)
	sequences := decodeSequences(ctx, data.Sequences, diags)

	for _, key := range sortedKeys(sequences) {
		for index, node := range sequences[key] {
			if node.Branch == nil || node.Branch.If == nil {
				continue
			}
			ifPath := path.Root("sequences").AtMapKey(key).AtName("nodes").AtListIndex(index).AtName("branch").AtName("if")

			workingHours := node.Branch.If.WorkingHoursActive
			priorities := node.Branch.If.PriorityOneOf

			switch {
			case workingHours.IsNull() && priorities.IsNull():
				diags.AddAttributeError(
					ifPath,
					"Branch tests nothing",
					"Set working_hours_active or priority_one_of, so the branch has something to choose on.",
				)
			// Only once both are known: a value another resource computes may still
			// resolve to null, and toPayload catches a branch that really does set both.
			case !workingHours.IsNull() && !workingHours.IsUnknown() &&
				!priorities.IsNull() && !priorities.IsUnknown():
				diags.AddAttributeError(
					ifPath,
					"Branch tests more than one thing",
					"A branch sets working_hours_active and priority_one_of, and a branch tests one thing. Move the second test into a branch in the sequence this one names.",
				)
			}

			if !priorities.IsNull() && !priorities.IsUnknown() && len(priorities.Elements()) == 0 {
				diags.AddAttributeError(
					ifPath.AtName("priority_one_of"),
					"No priorities to match",
					"priority_one_of is empty, so no escalation could satisfy it. Give it a priority, or leave it out.",
				)
			}

			// An empty string is set as far as the switch above is concerned, but names no
			// working hours, so it would reach the API as a branch testing nothing.
			if !workingHours.IsNull() && !workingHours.IsUnknown() && workingHours.ValueString() == "" {
				diags.AddAttributeError(
					ifPath.AtName("working_hours_active"),
					"No working hours named",
					"working_hours_active is empty. Name one of this escalation path's working_hours, or leave it out.",
				)
			}

			// The path's own working hours are all the escalation knows about, so any
			// other name could never be active.
			if id := workingHours.ValueString(); id != "" && workingHoursIDs != nil && !workingHoursIDs[id] {
				diags.AddAttributeError(
					ifPath.AtName("working_hours_active"),
					"Unknown working hours",
					fmt.Sprintf("This escalation path declares no working hours with the id %q.", id),
				)
			}
		}
	}
}

// escalationPathBetaWorkingHoursIDs returns the ids of the working hours this path
// declares, or nil when they aren't all known yet.
func escalationPathBetaWorkingHoursIDs(ctx context.Context, list types.List, diags *diag.Diagnostics) map[string]bool {
	// A null list declares no working hours, so a branch naming any is wrong. An unknown
	// one is a list another resource computes, which we can't check yet.
	if list.IsUnknown() {
		return nil
	}
	if list.IsNull() {
		return map[string]bool{}
	}

	var configs []models.IncidentWeekdayIntervalConfig
	diags.Append(list.ElementsAs(ctx, &configs, false)...)
	if diags.HasError() {
		return nil
	}

	ids := map[string]bool{}
	for _, config := range configs {
		if config.ID.IsUnknown() {
			return nil
		}
		ids[config.ID.ValueString()] = true
	}
	return ids
}
