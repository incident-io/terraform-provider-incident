package models

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// BoundExpressionSuffix ends the name an unnamed `expression { }` is stored under, and is
// reserved so a named_expression can't take it.
const BoundExpressionSuffix = "__bound"

// FallbackSuffix names the expression `fallback { if … }` synthesizes, and is reserved so a
// sibling can't take it.
//
// A reference must match ^[a-zA-Z0-9-_]*$, so the separator comes from that set. The double
// hyphen is unlikely to collide with a name someone would write themselves.
const FallbackSuffix = "--fallback"

func ExpressionReference(name string) string {
	return fmt.Sprintf("expressions[%q]", name)
}

// ExpressionsToPayload maps a resource's expression blocks, returning the binding for the
// unnamed block as a reference to it. Resources owning only named expressions pass nil for
// bound and ignore that binding.
//
// Names are mapped into ns on the way out, so what the config wrote is not what gets stored.
func ExpressionsToPayload(ns ExpressionNamespace, bound *Expression, named []NamedExpression) (
	[]client.ExpressionPayloadV3,
	*client.EngineParamBindingValuePayloadV3,
	error,
) {
	expressions := []client.ExpressionPayloadV3{}

	// Written order is preserved only so a read comes back matching; the server sorts by
	// dependency.
	for _, expression := range named {
		payload, synthesized, err := expressionPayload(
			expression.Name.ValueString(), expression.LabelOrName(),
			expression.StartFrom, expression.Operations, expression.Fallback)
		if err != nil {
			return nil, nil, fmt.Errorf("named_expression %q: %w", expression.Name.ValueString(), err)
		}
		expressions = append(expressions, payload)
		if synthesized != nil {
			expressions = append(expressions, *synthesized)
		}
	}

	if bound != nil {
		// The unnamed block has no label of its own: nothing points at it by name, so the
		// name it is stored under is all there is to show.
		payload, synthesized, err := expressionPayload(
			boundExpressionLocalName, boundExpressionLocalName,
			bound.StartFrom, bound.Operations, bound.Fallback)
		if err != nil {
			return nil, nil, fmt.Errorf("expression: %w", err)
		}
		expressions = append(expressions, payload)
		if synthesized != nil {
			expressions = append(expressions, *synthesized)
		}
	}

	// Everything built above is this resource's own, which is what may be renamed.
	declared := map[string]bool{}
	for _, expression := range expressions {
		declared[expression.Reference] = true
	}
	expressions = ns.namespace(expressions, declared)

	if bound == nil {
		return expressions, nil, nil
	}

	return expressions, ns.boundBinding(), nil
}

// expressionPayload can produce a second expression: the fallback shorthand is sugar for a
// private branches-only expression, which the parent points its else branch at.
func expressionPayload(
	name string,
	label string,
	startFrom types.String,
	operations []Operation,
	fallback *Fallback,
) (client.ExpressionPayloadV3, *client.ExpressionPayloadV3, error) {
	payload := client.ExpressionPayloadV3{
		Label:         label,
		Reference:     name,
		RootReference: startFrom.ValueString(),
		Operations:    []client.ExpressionOperationPayloadV3{},
	}

	for idx, operation := range operations {
		mapped, err := operationPayload(operation)
		if err != nil {
			return payload, nil, fmt.Errorf("operation %d: %w", idx, err)
		}
		payload.Operations = append(payload.Operations, mapped)
	}

	if UsesBranchingFallback(fallback) {
		synthesized, err := synthesizeFallbackExpression(name, operations, fallback)
		if err != nil {
			return payload, nil, err
		}

		payload.ElseBranch = &client.ExpressionElseBranchPayloadV3{
			Result: client.EngineParamBindingPayloadV3{
				Value: &client.EngineParamBindingValuePayloadV3{
					Reference: lo.ToPtr(ExpressionReference(synthesized.Reference)),
				},
			},
		}

		return payload, synthesized, nil
	}

	elseBranch, err := elseBranchPayload(fallback)
	if err != nil {
		return payload, nil, err
	}
	payload.ElseBranch = elseBranch

	return payload, nil, nil
}

// UsesBranchingFallback reports whether a fallback used the shorthand, the form that
// synthesizes an expression rather than filling a field.
func UsesBranchingFallback(fallback *Fallback) bool {
	return fallback != nil && (fallback.If != nil || len(fallback.ElseIf) > 0 || fallback.Else != nil)
}

// synthesizeFallbackExpression builds the expression behind `fallback { if … }`. A trailing
// `else` becomes its else branch, never a catch-all branch — an unconditional default is
// what the server stores an else branch as.
//
// The shorthand has nowhere to write the type its branches return, and needs none: the
// server checks the fallback against the PARENT's result type, so it returns whatever the
// parent does. Read off the parent's operations.
func synthesizeFallbackExpression(
	parentName string,
	parentOperations []Operation,
	fallback *Fallback,
) (*client.ExpressionPayloadV3, error) {
	resultType, resultArray, ok := inferResultType(parentOperations)
	if !ok {
		return nil, fmt.Errorf(
			"cannot tell what this expression returns, so the fallback's branches have nothing " +
				"to declare. They need both a type and a cardinality, and take both from the " +
				"expression they fall back from. Add an operation that states them — parse, or " +
				"first or concatenate to settle cardinality — or use " +
				"fallback { expression_ref = ... }, pointing at a named_expression whose " +
				"branches state `as` and `array`",
		)
	}

	payload, err := branchesPayload(&Branches{
		As:     types.StringValue(resultType),
		Array:  types.BoolValue(resultArray),
		If:     fallback.If,
		ElseIf: fallback.ElseIf,
	})
	if err != nil {
		return nil, err
	}

	synthesized := &client.ExpressionPayloadV3{
		Label:         parentName + FallbackSuffix,
		Reference:     parentName + FallbackSuffix,
		RootReference: ".",
		Operations: []client.ExpressionOperationPayloadV3{
			{OperationType: "branches", Branches: payload},
		},
	}

	if fallback.Else != nil {
		result, err := BindingToPayload(fallback.Else.Result)
		if err != nil {
			return nil, fmt.Errorf("fallback else: %w", err)
		}
		if result == nil {
			return nil, fmt.Errorf("fallback else needs a result")
		}
		synthesized.ElseBranch = &client.ExpressionElseBranchPayloadV3{Result: *result}
	}

	return synthesized, nil
}

// inferResultType walks the pipeline forwards, letting operations that assert a type set it
// and ones that only reshape cardinality adjust the array flag.
//
// It gives up rather than guess: navigate resolves against the catalog, count/sum produce a
// number whose engine type name is not ours to assume. Guessing is worse than failing,
// because the server accepts a wrong-but-valid asserted type without complaint.
//
// The two halves are tracked separately because operations clear and restore them
// independently: cast names the type but leaves cardinality following its input, and an
// unknown type mid-pipeline is not the final answer if a later parse asserts one.
//
// Both start unknown, cardinality included. The root is whatever start_from names, and
// `payload.teams` is an array where `payload` is not — the provider cannot tell them apart,
// and this is the half the server will not catch us guessing at. Its assignability check
// takes a scalar for an array param happily, so an else branch we wrongly declare scalar is
// accepted and simply disagrees with its parent from then on.
func inferResultType(operations []Operation) (string, bool, bool) {
	var (
		resultType string
		array      bool
		typeKnown  bool
		arrayKnown bool
	)

	for _, operation := range operations {
		switch {
		case operation.Parse != nil:
			resultType, typeKnown = operation.Parse.As.ValueString(), true
			array, arrayKnown = operation.Parse.Array.ValueBool(), true
		case operation.Branches != nil:
			resultType, typeKnown = operation.Branches.As.ValueString(), true
			array, arrayKnown = operation.Branches.Array.ValueBool(), true

		// Names the type, cardinality follows the input.
		case operation.Cast != nil:
			resultType, typeKnown = operation.Cast.As.ValueString(), true

		// Cardinality-changing, type-preserving.
		case operation.First != nil, operation.Min != nil, operation.Max != nil, operation.Random != nil:
			array, arrayKnown = false, true
		case operation.Concatenate != nil:
			// Always a deduped array of the same type.
			array, arrayKnown = true, true
		case operation.Filter != nil:
			// Changes neither.

		// One number, so the cardinality is known and only the engine's name for the type
		// is not ours to assume.
		case operation.Count != nil, operation.Sum != nil:
			resultType, typeKnown = "", false
			array, arrayKnown = false, true

		default:
			// navigate resolves against the catalog, which can change both.
			resultType, typeKnown = "", false
			arrayKnown = false
		}
	}

	return resultType, array, typeKnown && arrayKnown
}

func operationPayload(operation Operation) (client.ExpressionOperationPayloadV3, error) {
	switch {
	case operation.Parse != nil:
		return client.ExpressionOperationPayloadV3{
			OperationType: "parse",
			Parse: &client.ExpressionParseOptsPayloadV3{
				Source: operation.Parse.Function.ValueString(),
				// Required provider-side though the API schema isn't: the server
				// dereferences returns with no nil check, so omitting it panics there
				// rather than being refused.
				Returns: client.ReturnsMetaV3{
					Type:  operation.Parse.As.ValueString(),
					Array: operation.Parse.Array.ValueBool(),
				},
			},
		}, nil

	case operation.Navigate != nil:
		return client.ExpressionOperationPayloadV3{
			OperationType: "navigate",
			Navigate: &client.ExpressionNavigateOptsPayloadV3{
				Reference: operation.Navigate.To.ValueString(),
			},
		}, nil

	case operation.Cast != nil:
		return client.ExpressionOperationPayloadV3{
			OperationType: "cast",
			Cast: &client.ExpressionCastOptsPayloadV3{
				// Cardinality follows the input; the server accepts an array flag here
				// and ignores it.
				Returns: client.ReturnsMetaV3{Type: operation.Cast.As.ValueString()},
			},
		}, nil

	case operation.Concatenate != nil:
		return client.ExpressionOperationPayloadV3{
			OperationType: "concatenate",
			Concatenate: &client.ExpressionConcatenateOptsPayloadV3{
				Reference: operation.Concatenate.With.ValueString(),
			},
		}, nil

	case operation.Filter != nil:
		groups, err := ConditionGroupsToPayload(operation.Filter.Conditions, operation.Filter.ConditionGroups)
		if err != nil {
			return client.ExpressionOperationPayloadV3{}, err
		}
		return client.ExpressionOperationPayloadV3{
			OperationType: "filter",
			Filter:        &client.ExpressionFilterOptsPayloadV3{ConditionGroups: groups},
		}, nil

	case operation.Branches != nil:
		branches, err := branchesPayload(operation.Branches)
		if err != nil {
			return client.ExpressionOperationPayloadV3{}, err
		}
		return client.ExpressionOperationPayloadV3{
			OperationType: "branches",
			Branches:      branches,
		}, nil
	}

	// Fixed order, not a map: an operation setting two of these (which ValidateExpressions
	// rejects) must not pick a different one run to run.
	for _, candidate := range []struct {
		operationType string
		set           *EmptyOpts
	}{
		{"first", operation.First},
		{"count", operation.Count},
		{"sum", operation.Sum},
		{"min", operation.Min},
		{"max", operation.Max},
		{"random", operation.Random},
	} {
		if candidate.set != nil {
			return client.ExpressionOperationPayloadV3{
				OperationType: client.ExpressionOperationPayloadV3OperationType(candidate.operationType),
			}, nil
		}
	}

	return client.ExpressionOperationPayloadV3{}, fmt.Errorf(
		"no operation set. Give it one of parse, navigate, filter, cast, concatenate, branches, first, count, sum, min, max or random")
}

// branchesPayload emits `if` then each `else_if`. Evaluation is first-match-wins over the
// slice and nothing reorders it, so position is the whole contract.
func branchesPayload(branches *Branches) (*client.ExpressionBranchesOptsPayloadV3, error) {
	payload := &client.ExpressionBranchesOptsPayloadV3{
		Branches: []client.ExpressionBranchPayloadV3{},
		Returns: client.ReturnsMetaV3{
			Type:  branches.As.ValueString(),
			Array: branches.Array.ValueBool(),
		},
	}

	for idx, branch := range OrderedBranches(branches) {
		groups, err := ConditionGroupsToPayload(branch.Conditions, branch.ConditionGroups)
		if err != nil {
			return nil, fmt.Errorf("branch %d: %w", idx, err)
		}

		result, err := BindingToPayload(branch.Result)
		if err != nil {
			return nil, fmt.Errorf("branch %d: %w", idx, err)
		}
		if result == nil {
			return nil, fmt.Errorf("branch %d: needs a result", idx)
		}

		payload.Branches = append(payload.Branches, client.ExpressionBranchPayloadV3{
			ConditionGroups: groups,
			Result:          *result,
		})
	}

	return payload, nil
}

func OrderedBranches(branches *Branches) []Branch {
	ordered := []Branch{}
	if branches.If != nil {
		ordered = append(ordered, *branches.If)
	}

	return append(ordered, branches.ElseIf...)
}

// elseBranchPayload maps a fallback onto the else branch. `result` and `expression_ref`
// collapse to the same shape, because a reference to an expression is an ordinary scope
// reference. The shorthand is the caller's job — it adds an expression, not a field.
func elseBranchPayload(fallback *Fallback) (*client.ExpressionElseBranchPayloadV3, error) {
	if fallback == nil || UsesBranchingFallback(fallback) {
		return nil, nil
	}

	if !fallback.ExpressionRef.IsNull() {
		return &client.ExpressionElseBranchPayloadV3{
			Result: client.EngineParamBindingPayloadV3{
				Value: &client.EngineParamBindingValuePayloadV3{
					Reference: lo.ToPtr(ExpressionReference(fallback.ExpressionRef.ValueString())),
				},
			},
		}, nil
	}

	result, err := BindingToPayload(fallback.Result)
	if err != nil {
		return nil, fmt.Errorf("fallback: %w", err)
	}
	if result == nil {
		// An empty else branch reads back as absent and would diff forever.
		return nil, nil
	}

	return &client.ExpressionElseBranchPayloadV3{Result: *result}, nil
}

// ConditionGroupsToPayload wraps `conditions` into one group; `condition_groups` passes
// through.
func ConditionGroupsToPayload(conditions []Condition, groups []ConditionGroup) ([]client.ConditionGroupPayloadV3, error) {
	if len(conditions) > 0 && len(groups) > 0 {
		return nil, fmt.Errorf("set either conditions or condition_groups, not both")
	}

	if len(conditions) > 0 {
		groups = []ConditionGroup{{Conditions: conditions}}
	}

	out := []client.ConditionGroupPayloadV3{}
	for _, group := range groups {
		mapped := client.ConditionGroupPayloadV3{Conditions: []client.ConditionPayloadV3{}}
		for _, condition := range group.Conditions {
			params := []client.EngineParamBindingPayloadV3{}
			for idx := range condition.Params {
				binding, err := BindingToPayload(&condition.Params[idx])
				if err != nil {
					return nil, fmt.Errorf("param %d: %w", idx, err)
				}
				if binding == nil {
					return nil, fmt.Errorf("param %d: needs a value", idx)
				}
				params = append(params, *binding)
			}

			mapped.Conditions = append(mapped.Conditions, client.ConditionPayloadV3{
				Subject:       condition.Subject.ValueString(),
				Operation:     condition.Operation.ValueString(),
				ParamBindings: params,
			})
		}
		out = append(out, mapped)
	}

	return out, nil
}

// BindingToPayload normalises the sugar spellings onto the two forms the API has. Nil means
// absent, not empty.
func BindingToPayload(binding *Binding) (*client.EngineParamBindingPayloadV3, error) {
	if binding == nil {
		return nil, nil
	}

	switch set := SetBindingForms(binding); {
	case set == 0:
		return nil, nil
	case set > 1:
		return nil, fmt.Errorf(
			"set exactly one of value_literal, value_reference, expression_ref, values, value or array_value")
	}

	switch {
	case !binding.ValueLiteral.IsNull():
		return &client.EngineParamBindingPayloadV3{
			Value: &client.EngineParamBindingValuePayloadV3{Literal: binding.ValueLiteral.ValueStringPointer()},
		}, nil

	case !binding.ValueReference.IsNull():
		return &client.EngineParamBindingPayloadV3{
			Value: &client.EngineParamBindingValuePayloadV3{Reference: binding.ValueReference.ValueStringPointer()},
		}, nil

	// Purely a spelling: this is an ordinary scope reference underneath.
	case !binding.ExpressionRef.IsNull():
		return &client.EngineParamBindingPayloadV3{
			Value: &client.EngineParamBindingValuePayloadV3{
				Reference: lo.ToPtr(ExpressionReference(binding.ExpressionRef.ValueString())),
			},
		}, nil

	case len(binding.Values) > 0:
		values := []client.EngineParamBindingValuePayloadV3{}
		for _, value := range binding.Values {
			values = append(values, client.EngineParamBindingValuePayloadV3{Literal: value.ValueStringPointer()})
		}
		return &client.EngineParamBindingPayloadV3{ArrayValue: &values}, nil

	case binding.Value != nil:
		value, err := bindingValuePayload(*binding.Value)
		if err != nil {
			return nil, err
		}
		return &client.EngineParamBindingPayloadV3{Value: value}, nil

	default:
		values := []client.EngineParamBindingValuePayloadV3{}
		for idx, element := range binding.ArrayValue {
			value, err := bindingValuePayload(element)
			if err != nil {
				return nil, fmt.Errorf("array_value %d: %w", idx, err)
			}
			values = append(values, *value)
		}
		return &client.EngineParamBindingPayloadV3{ArrayValue: &values}, nil
	}
}

// SetBindingForms counts the value forms in use. The schema can't express the group — it
// spans attributes of different types.
func SetBindingForms(binding *Binding) int {
	set := 0
	for _, isSet := range []bool{
		!binding.ValueLiteral.IsNull(),
		!binding.ValueReference.IsNull(),
		!binding.ExpressionRef.IsNull(),
		len(binding.Values) > 0,
		binding.Value != nil,
		len(binding.ArrayValue) > 0,
	} {
		if isSet {
			set++
		}
	}

	return set
}

func bindingValuePayload(value BindingValue) (*client.EngineParamBindingValuePayloadV3, error) {
	if !value.Literal.IsNull() && !value.Reference.IsNull() {
		return nil, fmt.Errorf("set either literal or reference, not both")
	}
	if value.Literal.IsNull() && value.Reference.IsNull() {
		return nil, fmt.Errorf("set either literal or reference")
	}

	return &client.EngineParamBindingValuePayloadV3{
		Literal:   value.Literal.ValueStringPointer(),
		Reference: value.Reference.ValueStringPointer(),
	}, nil
}
