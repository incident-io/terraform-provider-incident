package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// A branch tests one thing: the escalation's priority or urgency, or whether one of the
// path's working hours is active. Those are all an escalation knows about itself, and the
// API takes exactly one condition on a branch, so this resource takes one attribute per test
// and builds the condition from whichever the author set.
//
// V2 branches on priority and V3 on urgency, and neither accepts the other's. Both
// conditions are built and read the same way, so this file handles both, and
// escalationPathAPIVersion decides which API a path uses.

const (
	// The references the escalation scope holds, which is what a condition's subject names.
	escalationPrioritySubject = "escalation.priority"
	escalationUrgencySubject  = "escalation.urgency"

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

// escalationPathBranchIf is what a branch tests. Exactly one attribute is set, which
// validateSequenceConditions enforces, and which one it is takes the place of the subject
// and operation the API carries.
type escalationPathBranchIf struct {
	WorkingHoursActive types.String `tfsdk:"working_hours_active"`
	PriorityOneOf      types.Set    `tfsdk:"priority_one_of"`
	UrgencyOneOf       types.Set    `tfsdk:"urgency_one_of"`
}

// escalationPathBranchIfAttrTypes returns the attribute types for a branch's if object.
func escalationPathBranchIfAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"working_hours_active": types.StringType,
		"priority_one_of":      types.SetType{ElemType: types.StringType},
		"urgency_one_of":       types.SetType{ElemType: types.StringType},
	}
}

// escalationPathBranchIfAttribute returns the if attribute's schema.
func escalationPathBranchIfAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "What the branch tests. Set exactly one of these: a branch tests one thing, so combining them means nesting a second branch inside the first.",
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"working_hours_active": schema.StringAttribute{
				MarkdownDescription: "The `id` of one of this escalation path's `working_hours`, met while those hours are active.",
				Optional:            true,
			},
			"priority_one_of": schema.SetAttribute{
				MarkdownDescription: "Deprecated: priority on escalations has been replaced with escalation urgency. Use `urgency_one_of` instead. " +
					"Alert priority ids, met when the escalation came in at one of them.",
				DeprecationMessage: priorityOneOfDeprecation,
				Optional:           true,
				ElementType:        types.StringType,
			},
			"urgency_one_of": schema.SetAttribute{
				MarkdownDescription: "Escalation urgency ids, met when the escalation came in at one of them.",
				Optional:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

// escalationPathTemplateBranchIfAttribute is the if attribute for templates, which go
// through V2 only, so priority_one_of isn't deprecated there. urgency_one_of stays in the
// schema because templates share escalationPathBranchIf with paths.
func escalationPathTemplateBranchIfAttribute() schema.SingleNestedAttribute {
	attribute := escalationPathBranchIfAttribute()
	attribute.Attributes["priority_one_of"] = schema.SetAttribute{
		MarkdownDescription: "Alert priority ids, met when the escalation came in at one of them.",
		Optional:            true,
		ElementType:         types.StringType,
	}
	attribute.Attributes["urgency_one_of"] = schema.SetAttribute{
		MarkdownDescription: "Not supported on templates, which branch on `priority_one_of`.",
		Optional:            true,
		ElementType:         types.StringType,
	}
	return attribute
}

// priorityOneOfDeprecation is the warning a config still branching on priority gets.
const priorityOneOfDeprecation = "Priority on escalations has been replaced with escalation urgency. " +
	"Please replace escalation path conditions on priority_one_of with urgency_one_of. " +
	escalationPathMigrateTogether

// escalationPathMigrateTogether follows both deprecation warnings, because a path that
// mixes the old and new attributes is rejected.
const escalationPathMigrateTogether = "Change every target and branch in the escalation path at the same time."

// toPayload builds the one condition the API stores, reporting a branch that tests more
// than one thing, or nothing, rather than sending a payload the API would reject.
//
// validateSequenceConditions reports the same, but only for a config it can read: one
// whose sequences another resource computes reaches here unchecked.
func (i *escalationPathBranchIf) toPayload(ctx context.Context, diags *diag.Diagnostics) []client.ConditionPayloadV2 {
	none := []client.ConditionPayloadV2{}
	nothingToTest := func() []client.ConditionPayloadV2 {
		diags.AddError(
			"Branch tests nothing",
			"A branch sets none of working_hours_active, urgency_one_of and priority_one_of, so it has nothing to choose on.",
		)
		return none
	}

	// if is Required, so a branch reaching here without one isn't something a config can
	// say. It would still leave the API with a branch testing nothing.
	if i == nil {
		return nothingToTest()
	}

	if set := i.testsSet(); len(set) > 1 {
		diags.AddError(
			"Branch tests more than one thing",
			fmt.Sprintf("A branch sets %s, and a branch tests one thing.", strings.Join(set, " and ")),
		)
		return none
	}

	workingHours := i.WorkingHoursActive.ValueString()
	priorities := escalationPathOneOfIDs(ctx, i.PriorityOneOf, diags)
	urgencies := escalationPathOneOfIDs(ctx, i.UrgencyOneOf, diags)
	if diags.HasError() {
		return none
	}

	switch {
	case workingHours != "":
		return []client.ConditionPayloadV2{{
			Subject:       fmt.Sprintf(escalationWorkingHoursSubject, workingHours),
			Operation:     escalationOperationIsActive,
			ParamBindings: []client.EngineParamBindingPayloadV2{},
		}}

	case len(priorities) > 0:
		return []client.ConditionPayloadV2{oneOfConditionPayload(escalationPrioritySubject, priorities)}

	case len(urgencies) > 0:
		return []client.ConditionPayloadV2{oneOfConditionPayload(escalationUrgencySubject, urgencies)}

	default:
		return nothingToTest()
	}
}

// testsSet names the tests this branch sets, in the order the schema declares them.
func (i *escalationPathBranchIf) testsSet() []string {
	var set []string
	if !i.WorkingHoursActive.IsNull() {
		set = append(set, "working_hours_active")
	}
	if !i.PriorityOneOf.IsNull() {
		set = append(set, "priority_one_of")
	}
	if !i.UrgencyOneOf.IsNull() {
		set = append(set, "urgency_one_of")
	}
	return set
}

// oneOfConditionPayload builds a condition met when subject is one of ids.
func oneOfConditionPayload(subject string, ids []string) client.ConditionPayloadV2 {
	values := lo.Map(ids, func(id string, _ int) client.EngineParamBindingValuePayloadV2 {
		return client.EngineParamBindingValuePayloadV2{Literal: lo.ToPtr(id)}
	})
	return client.ConditionPayloadV2{
		Subject:   subject,
		Operation: escalationOperationOneOf,
		ParamBindings: []client.EngineParamBindingPayloadV2{
			{ArrayValue: &values},
		},
	}
}

// escalationPathOneOfIDs decodes a priority or urgency set, treating an unset one as empty.
func escalationPathOneOfIDs(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
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

// escalationPathBranchIfFromAPI reads a branch's condition back into the attribute
// that holds it, reporting one it can't: the engine allows conditions neither this
// resource nor our dashboard builds.
func escalationPathBranchIfFromAPI(ctx context.Context, conditions []client.ConditionV2, diags *diag.Diagnostics) *escalationPathBranchIf {
	out := &escalationPathBranchIf{
		WorkingHoursActive: types.StringNull(),
		PriorityOneOf:      types.SetNull(types.StringType),
		UrgencyOneOf:       types.SetNull(types.StringType),
	}

	unsupported := func(detail string) *escalationPathBranchIf {
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

	oneOf := map[string]*types.Set{
		escalationPrioritySubject: &out.PriorityOneOf,
		escalationUrgencySubject:  &out.UrgencyOneOf,
	}
	if target, ok := oneOf[subject]; ok {
		name := strings.TrimPrefix(subject, "escalation.")
		if operation != escalationOperationOneOf {
			return unsupported(fmt.Sprintf("A branch tests whether the %s is %q.", name, operation))
		}
		ids, ok := escalationPathOneOfIDsFromAPI(condition.ParamBindings)
		if !ok {
			return unsupported(fmt.Sprintf("A branch tests the %s against something other than a list of %s ids.", name, name))
		}
		set, d := types.SetValueFrom(ctx, types.StringType, ids)
		diags.Append(d...)
		*target = set
		return out
	}

	return unsupported(fmt.Sprintf("A branch tests %s, which is neither the priority, the urgency nor a set of working hours.", subject))
}

// escalationPathOneOfIDsFromAPI pulls the priority or urgency ids out of a one_of's
// bindings, reporting false for a binding holding anything but literals.
func escalationPathOneOfIDsFromAPI(bindings []client.EngineParamBindingV2) ([]string, bool) {
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
// hours it names are ones this path declares. oneOf is the attribute to suggest for a
// branch that tests nothing: the deprecated priority_one_of for a path still using the
// deprecated attributes, or a template, and urgency_one_of otherwise.
func validateSequenceConditions(ctx context.Context, data *escalationPathModel, oneOf string, diags *diag.Diagnostics) {
	workingHoursIDs := escalationPathWorkingHoursIDs(ctx, data.WorkingHours, diags)
	sequences := decodeSequences(ctx, data.Sequences, diags)

	for _, key := range sortedKeys(sequences) {
		for index, node := range sequences[key] {
			if node.Branch == nil || node.Branch.If == nil {
				continue
			}
			ifPath := path.Root("sequences").AtMapKey(key).AtName("nodes").AtListIndex(index).AtName("branch").AtName("if")

			branchIf := node.Branch.If
			workingHours := branchIf.WorkingHoursActive

			// Report two tests only once all are known: a value another resource computes may
			// still resolve to null, and toPayload catches a branch that really sets two.
			switch set := branchIf.testsSet(); {
			case len(set) == 0:
				diags.AddAttributeError(
					ifPath,
					"Branch tests nothing",
					fmt.Sprintf("Set working_hours_active or %s, so the branch has something to choose on.", oneOf),
				)
			case len(set) > 1 && branchIf.testsKnown():
				diags.AddAttributeError(
					ifPath,
					"Branch tests more than one thing",
					fmt.Sprintf("A branch sets %s, and a branch tests one thing. Move the second test into a branch in the sequence this one names.", strings.Join(set, " and ")),
				)
			}

			for _, oneOf := range []struct {
				name string
				ids  types.Set
			}{
				{"priority_one_of", branchIf.PriorityOneOf},
				{"urgency_one_of", branchIf.UrgencyOneOf},
			} {
				name, ids := oneOf.name, oneOf.ids
				if !ids.IsNull() && !ids.IsUnknown() && len(ids.Elements()) == 0 {
					diags.AddAttributeError(
						ifPath.AtName(name),
						"Nothing to match",
						fmt.Sprintf("%s is empty, so no escalation could satisfy it. Give it an id, or leave it out.", name),
					)
				}
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

// testsKnown reports whether every test this branch could set is known.
func (i *escalationPathBranchIf) testsKnown() bool {
	return !i.WorkingHoursActive.IsUnknown() && !i.PriorityOneOf.IsUnknown() && !i.UrgencyOneOf.IsUnknown()
}

// escalationPathWorkingHoursIDs returns the ids of the working hours this path
// declares, or nil when they aren't all known yet.
func escalationPathWorkingHoursIDs(ctx context.Context, list types.List, diags *diag.Diagnostics) map[string]bool {
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
