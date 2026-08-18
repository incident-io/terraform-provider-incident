package models

import (
	"strings"

	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// The server refuses an expression reference another resource on the same alert source already
// holds. So a resource sharing a source stores its expressions under names it owns alone, and maps
// back on the way in: the config only ever holds the name the user wrote.

// ExpressionNamespace is the set of names one resource's expressions are stored under. The zero
// value stores them as written and has no unnamed `expression { }` block.
type ExpressionNamespace struct {
	prefix   string
	hasBound bool
}

func AlertAttributeExpressions(alertAttributeID string) ExpressionNamespace {
	return ExpressionNamespace{
		prefix:   "attr_" + alertAttributeID + "_",
		hasBound: true,
	}
}

// boundExpressionLocalName stands in for the name the unnamed `expression { }` block hasn't got.
// Stored it ends in BoundExpressionSuffix, which ValidateExpressions reserves.
const boundExpressionLocalName = "_bound"

func (n ExpressionNamespace) boundLocalName() string {
	if !n.hasBound {
		return ""
	}

	return boundExpressionLocalName
}

func (n ExpressionNamespace) boundBinding() *client.EngineParamBindingValuePayloadV3 {
	return &client.EngineParamBindingValuePayloadV3{
		Reference: lo.ToPtr(ExpressionReference(n.stored(boundExpressionLocalName))),
	}
}

func (n ExpressionNamespace) stored(localName string) string {
	return n.prefix + localName
}

// reservesName reports whether name would be stored under a reference the resource keeps back for
// its own unnamed `expression { }` block.
//
// It compares local names rather than stored ones, so a namespace built from an id Terraform
// hasn't resolved yet gives the same answer as one built from the resolved id.
func (n ExpressionNamespace) reservesName(name string) bool {
	if n.hasBound {
		return name == boundExpressionLocalName
	}

	// Storing names as written, so a name ending the way a bound reference does could land on the
	// unnamed block of any attribute sharing the source.
	return strings.HasSuffix(name, BoundExpressionSuffix)
}

// local leaves a name without the prefix alone, so a dashboard-written expression keeps its name.
func (n ExpressionNamespace) local(storedName string) string {
	return strings.TrimPrefix(storedName, n.prefix)
}

// namespace renames only the names the resource declared: a reference to anything else is an
// ordinary scope path or another resource's expression.
func (n ExpressionNamespace) namespace(
	expressions []client.ExpressionPayloadV3,
	declared map[string]bool,
) []client.ExpressionPayloadV3 {
	if n.prefix == "" {
		return expressions
	}

	return renameExpressions(expressions, n.storing(declared))
}

func (n ExpressionNamespace) localise(expressions []client.ExpressionPayloadV3) []client.ExpressionPayloadV3 {
	if n.prefix == "" {
		return expressions
	}

	return renameExpressions(expressions, n.local)
}

// NamespaceBinding is for a value the resource binds outside its expression tree.
func (n ExpressionNamespace) NamespaceBinding(
	binding *client.EngineParamBindingPayloadV3,
	named []NamedExpression,
) *client.EngineParamBindingPayloadV3 {
	if n.prefix == "" {
		return binding
	}

	return renameBindingPayload(binding, n.storing(KnownExpressionNames(named)))
}

func (n ExpressionNamespace) LocaliseBinding(binding *client.EngineParamBindingPayloadV3) *client.EngineParamBindingPayloadV3 {
	if n.prefix == "" {
		return binding
	}

	return renameBindingPayload(binding, n.local)
}

func (n ExpressionNamespace) storing(declared map[string]bool) func(string) string {
	return func(name string) string {
		if !declared[name] {
			return name
		}

		return n.stored(name)
	}
}

// renameExpressions visits named fields rather than walking reflectively, because parse, cast and
// branches each carry an engine type name, and renaming one of those corrupts the type. A label is
// left alone, being the name as written.
func renameExpressions(expressions []client.ExpressionPayloadV3, rename func(string) string) []client.ExpressionPayloadV3 {
	renamed := make([]client.ExpressionPayloadV3, 0, len(expressions))
	for _, expression := range expressions {
		expression.Reference = rename(expression.Reference)
		expression.RootReference = renameReference(expression.RootReference, rename)
		expression.Operations = renameOperations(expression.Operations, rename)

		if expression.ElseBranch != nil {
			expression.ElseBranch = &client.ExpressionElseBranchPayloadV3{
				Result: renameBinding(expression.ElseBranch.Result, rename),
			}
		}

		renamed = append(renamed, expression)
	}

	return renamed
}

func renameOperations(
	operations []client.ExpressionOperationPayloadV3,
	rename func(string) string,
) []client.ExpressionOperationPayloadV3 {
	renamed := make([]client.ExpressionOperationPayloadV3, 0, len(operations))
	for _, operation := range operations {
		switch {
		case operation.Navigate != nil:
			operation.Navigate = &client.ExpressionNavigateOptsPayloadV3{
				Reference: renameReference(operation.Navigate.Reference, rename),
			}

		case operation.Concatenate != nil:
			operation.Concatenate = &client.ExpressionConcatenateOptsPayloadV3{
				Reference: renameReference(operation.Concatenate.Reference, rename),
			}

		case operation.Filter != nil:
			operation.Filter = &client.ExpressionFilterOptsPayloadV3{
				ConditionGroups: renameConditionGroups(operation.Filter.ConditionGroups, rename),
			}

		case operation.Branches != nil:
			branches := *operation.Branches
			branches.Branches = renameBranches(branches.Branches, rename)
			operation.Branches = &branches
		}

		renamed = append(renamed, operation)
	}

	return renamed
}

func renameBranches(
	branches []client.ExpressionBranchPayloadV3,
	rename func(string) string,
) []client.ExpressionBranchPayloadV3 {
	renamed := make([]client.ExpressionBranchPayloadV3, 0, len(branches))
	for _, branch := range branches {
		branch.ConditionGroups = renameConditionGroups(branch.ConditionGroups, rename)
		branch.Result = renameBinding(branch.Result, rename)
		renamed = append(renamed, branch)
	}

	return renamed
}

func renameConditionGroups(
	groups []client.ConditionGroupPayloadV3,
	rename func(string) string,
) []client.ConditionGroupPayloadV3 {
	renamed := make([]client.ConditionGroupPayloadV3, 0, len(groups))
	for _, group := range groups {
		renamed = append(renamed, client.ConditionGroupPayloadV3{
			Conditions: renameConditions(group.Conditions, rename),
		})
	}

	return renamed
}

func renameConditions(
	conditions []client.ConditionPayloadV3,
	rename func(string) string,
) []client.ConditionPayloadV3 {
	renamed := make([]client.ConditionPayloadV3, 0, len(conditions))
	for _, condition := range conditions {
		condition.Subject = renameReference(condition.Subject, rename)

		params := make([]client.EngineParamBindingPayloadV3, 0, len(condition.ParamBindings))
		for _, param := range condition.ParamBindings {
			params = append(params, renameBinding(param, rename))
		}
		condition.ParamBindings = params

		renamed = append(renamed, condition)
	}

	return renamed
}

func renameBindingPayload(
	binding *client.EngineParamBindingPayloadV3,
	rename func(string) string,
) *client.EngineParamBindingPayloadV3 {
	if binding == nil {
		return nil
	}

	return lo.ToPtr(renameBinding(*binding, rename))
}

func renameBinding(
	binding client.EngineParamBindingPayloadV3,
	rename func(string) string,
) client.EngineParamBindingPayloadV3 {
	if binding.Value != nil {
		binding.Value = lo.ToPtr(renameBindingValue(*binding.Value, rename))
	}

	if binding.ArrayValue != nil {
		values := make([]client.EngineParamBindingValuePayloadV3, 0, len(*binding.ArrayValue))
		for _, value := range *binding.ArrayValue {
			values = append(values, renameBindingValue(value, rename))
		}
		binding.ArrayValue = &values
	}

	return binding
}

func renameBindingValue(
	value client.EngineParamBindingValuePayloadV3,
	rename func(string) string,
) client.EngineParamBindingValuePayloadV3 {
	if value.Reference == nil {
		return value
	}

	value.Reference = lo.ToPtr(renameReference(*value.Reference, rename))

	return value
}

func renameReference(reference string, rename func(string) string) string {
	name, tail, ok := splitExpressionReference(reference)
	if !ok {
		return reference
	}

	return ExpressionReference(rename(name)) + tail
}

// splitExpressionReference splits `expressions["team"].name` into "team" and ".name". The trailing
// path is part of a real reference, so a rename has to keep it.
func splitExpressionReference(reference string) (name string, tail string, ok bool) {
	rest, ok := strings.CutPrefix(reference, `expressions["`)
	if !ok {
		return "", "", false
	}

	name, tail, ok = strings.Cut(rest, `"]`)
	if !ok {
		return "", "", false
	}

	return name, tail, true
}
