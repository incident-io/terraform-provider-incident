package models

import (
	"reflect"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// Reading back has one job beyond mapping: return the same spelling the config used, or the
// apply fails as an inconsistent result and every later plan shows a change.
//
// Several spellings serialise identically, so the payload alone can't say which one to return.
// Where the prior model is available it answers that — see ReconcileBinding. Where it isn't (an
// import, or a subtree that genuinely changed) prefer the sugar, so a config using the long
// form is the one rewritten rather than diffing forever.

// ExpressionsFromPayload projects the API's expressions onto the blocks.
//
// Names come back out of ns first, so everything below works in the names the config wrote. The
// unnamed block has one of its own, under which its expression folds back into the block rather
// than reading as a named_expression nobody wrote.
//
// priorBound and priorNamed are the plan on create and update, the prior state on read. They
// settle what the payload can't: the order the config wrote its named expressions in
// (named_expression is a list, so any other order is a diff, and the server won't preserve
// ours), and which spelling each nested binding used.
func ExpressionsFromPayload(
	expressions []client.ExpressionPayloadV3,
	ns ExpressionNamespace,
	priorBound *Expression,
	priorNamed []NamedExpression,
) (*Expression, []NamedExpression) {
	expressions = ns.localise(expressions)
	boundName := ns.boundLocalName()

	byName := map[string]client.ExpressionPayloadV3{}
	for _, expression := range expressions {
		byName[expression.Reference] = expression
	}

	var bound *Expression
	if payload, ok := byName[boundName]; ok {
		delete(byName, boundName)

		if priorBound != nil {
			if synthesized, matched := expressionMatchesPrior(
				boundName, boundName, priorBound.StartFrom, priorBound.Operations, priorBound.Fallback,
				payload, byName,
			); matched {
				delete(byName, synthesized)
				bound = priorBound
			}
		}

		if bound == nil {
			startFrom, operations, fallback := expressionBodyFromPayload(payload)
			bound = &Expression{
				StartFrom:  startFrom,
				Operations: operations,
				Fallback:   foldSynthesizedFallback(boundName, fallback, byName),
			}
		}
	}

	named := []NamedExpression{}

	// Config order first, so an unchanged config reads back identically.
	for _, prior := range priorNamed {
		name := prior.Name.ValueString()
		payload, ok := byName[name]
		if !ok {
			continue
		}
		delete(byName, name)

		if synthesized, matched := expressionMatchesPrior(
			name, prior.LabelOrName(), prior.StartFrom, prior.Operations, prior.Fallback,
			payload, byName,
		); matched {
			delete(byName, synthesized)
			named = append(named, prior)
			continue
		}

		named = append(named, namedExpressionFromPayload(payload, byName, prior.Label))
	}

	// Anything else — an import, say — sorted so it is at least stable.
	//
	// A synthesized fallback sorts after its parent, the parent being a prefix of it, so the
	// parent always folds it away before this loop reaches it.
	remaining := make([]string, 0, len(byName))
	for name := range byName {
		remaining = append(remaining, name)
	}
	sort.Strings(remaining)

	for _, name := range remaining {
		payload, ok := byName[name]
		if !ok {
			// Consumed as another expression's synthesized fallback.
			continue
		}
		delete(byName, name)

		// No prior: an import, so a label worth keeping is one that isn't just the reference.
		named = append(named, namedExpressionFromPayload(payload, byName, types.StringNull()))
	}

	if len(named) == 0 {
		return bound, nil
	}

	return bound, named
}

// expressionMatchesPrior reports whether the prior model still means what the API returned, and
// names the synthesized fallback expression it accounts for so the caller can consume it.
//
// The comparison is over payloads, because the payload is what "means the same thing" is defined
// against: the sugar spellings collapse onto it, so two models producing the same payload are
// interchangeable and the prior's is the one to keep. Comparing whole expressions rather than
// each nested binding costs a re-spelling of an expression's untouched parts when some other
// part of it genuinely drifted, and buys not having to thread the prior through every read
// helper.
//
// A field the model can't express is normalised away first. A field neither side knows about
// would fail the comparison, which is the safe direction — a re-spelling, never hidden drift.
func expressionMatchesPrior(
	name string,
	label string,
	startFrom types.String,
	operations []Operation,
	fallback *Fallback,
	payload client.ExpressionPayloadV3,
	byName map[string]client.ExpressionPayloadV3,
) (synthesized string, matched bool) {
	want, extra, err := expressionPayload(name, label, startFrom, operations, fallback)
	if err != nil {
		return "", false
	}

	if !reflect.DeepEqual(want, payload) {
		return "", false
	}

	if extra == nil {
		return "", true
	}

	// The shorthand's expression has to match too, or the fallback it folds back into wouldn't
	// be the one the config wrote.
	got, ok := byName[extra.Reference]
	if !ok {
		return "", false
	}
	extra.Label = got.Label
	if !reflect.DeepEqual(*extra, got) {
		return "", false
	}

	return extra.Reference, true
}

// foldSynthesizedFallback rewrites a fallback pointing at the generated expression back into
// the shorthand, dropping it from byName so it isn't also returned as a named_expression
// nobody wrote. The shorthand only generates branches-only expressions, so anything else
// under that name is left alone.
func foldSynthesizedFallback(
	parentName string,
	fallback *Fallback,
	byName map[string]client.ExpressionPayloadV3,
) *Fallback {
	if fallback == nil || fallback.ExpressionRef.IsNull() {
		return fallback
	}

	name := fallback.ExpressionRef.ValueString()
	if name != parentName+FallbackSuffix {
		return fallback
	}

	synthesized, ok := byName[name]
	if !ok {
		return fallback
	}

	if len(synthesized.Operations) != 1 || synthesized.Operations[0].Branches == nil {
		return fallback
	}

	branches := branchesFromPayload(synthesized.Operations[0].Branches)
	folded := &Fallback{
		ExpressionRef: types.StringNull(),
		If:            branches.If,
		ElseIf:        branches.ElseIf,
	}

	// A trailing `else` became the synthesized expression's own else branch.
	if synthesized.ElseBranch != nil {
		if result := BindingFromPayload(&synthesized.ElseBranch.Result); result != nil {
			folded.Else = &Else{Result: result}
		}
	}

	delete(byName, name)

	return folded
}

// namedExpressionFromPayload reads an expression back. priorLabel is what the config asked for,
// so a label it never set doesn't appear in state: the write mirrors the name when the config
// gives no label, and storing that back would diff against a config that omitted it. A label
// differing from the name is either an import or a rename elsewhere, and belongs in state.
func namedExpressionFromPayload(
	payload client.ExpressionPayloadV3,
	byName map[string]client.ExpressionPayloadV3,
	priorLabel types.String,
) NamedExpression {
	startFrom, operations, fallback := expressionBodyFromPayload(payload)

	label := types.StringNull()
	if !priorLabel.IsNull() || payload.Label != payload.Reference {
		label = types.StringValue(payload.Label)
	}

	return NamedExpression{
		Name:       types.StringValue(payload.Reference),
		Label:      label,
		StartFrom:  startFrom,
		Operations: operations,
		Fallback:   foldSynthesizedFallback(payload.Reference, fallback, byName),
	}
}

func expressionBodyFromPayload(payload client.ExpressionPayloadV3) (types.String, []Operation, *Fallback) {
	operations := []Operation{}
	for _, operation := range payload.Operations {
		operations = append(operations, operationFromPayload(operation))
	}
	if len(operations) == 0 {
		operations = nil
	}

	return types.StringValue(payload.RootReference), operations, fallbackFromPayload(payload.ElseBranch)
}

func operationFromPayload(operation client.ExpressionOperationPayloadV3) Operation {
	model := Operation{}

	switch {
	case operation.Parse != nil:
		model.Parse = &Parse{
			Function: types.StringValue(operation.Parse.Source),
			As:       types.StringValue(operation.Parse.Returns.Type),
			Array:    boolOrNull(operation.Parse.Returns.Array),
		}
	case operation.Navigate != nil:
		model.Navigate = &Navigate{To: types.StringValue(operation.Navigate.Reference)}
	case operation.Cast != nil:
		model.Cast = &Cast{As: types.StringValue(operation.Cast.Returns.Type)}
	case operation.Concatenate != nil:
		model.Concatenate = &Concatenate{With: types.StringValue(operation.Concatenate.Reference)}
	case operation.Filter != nil:
		conditions, groups := conditionGroupsFromPayload(operation.Filter.ConditionGroups)
		model.Filter = &Conditions{Conditions: conditions, ConditionGroups: groups}
	case operation.Branches != nil:
		model.Branches = branchesFromPayload(operation.Branches)
	default:
		empty := &EmptyOpts{}
		switch string(operation.OperationType) {
		case "first":
			model.First = empty
		case "count":
			model.Count = empty
		case "sum":
			model.Sum = empty
		case "min":
			model.Min = empty
		case "max":
			model.Max = empty
		case "random":
			model.Random = empty
		}
	}

	return model
}

func branchesFromPayload(branches *client.ExpressionBranchesOptsPayloadV3) *Branches {
	model := &Branches{
		As:    types.StringValue(branches.Returns.Type),
		Array: boolOrNull(branches.Returns.Array),
	}

	for idx := range branches.Branches {
		conditions, groups := conditionGroupsFromPayload(branches.Branches[idx].ConditionGroups)
		mapped := Branch{
			Conditions:      conditions,
			ConditionGroups: groups,
			Result:          BindingFromPayload(&branches.Branches[idx].Result),
		}

		// First branch is the `if`, the rest are `else_if` in order.
		if idx == 0 {
			model.If = &mapped
			continue
		}
		model.ElseIf = append(model.ElseIf, mapped)
	}

	return model
}

func fallbackFromPayload(elseBranch *client.ExpressionElseBranchPayloadV3) *Fallback {
	if elseBranch == nil {
		return nil
	}

	binding := BindingFromPayload(&elseBranch.Result)
	if binding == nil {
		return nil
	}

	if !binding.ExpressionRef.IsNull() {
		return &Fallback{ExpressionRef: binding.ExpressionRef}
	}

	return &Fallback{Result: binding}
}

// conditionGroupsFromPayload prefers the `conditions` sugar for a single group, that being
// the spelling a config almost always used.
func conditionGroupsFromPayload(groups []client.ConditionGroupPayloadV3) ([]Condition, []ConditionGroup) {
	if len(groups) == 0 {
		return nil, nil
	}

	if len(groups) == 1 {
		return conditionsFromPayload(groups[0].Conditions), nil
	}

	mapped := []ConditionGroup{}
	for _, group := range groups {
		mapped = append(mapped, ConditionGroup{Conditions: conditionsFromPayload(group.Conditions)})
	}

	return nil, mapped
}

func conditionsFromPayload(conditions []client.ConditionPayloadV3) []Condition {
	mapped := []Condition{}
	for _, condition := range conditions {
		params := []Binding{}
		for idx := range condition.ParamBindings {
			if binding := BindingFromPayload(&condition.ParamBindings[idx]); binding != nil {
				params = append(params, *binding)
			}
		}
		if len(params) == 0 {
			params = nil
		}

		mapped = append(mapped, Condition{
			Subject:   types.StringValue(condition.Subject),
			Operation: types.StringValue(condition.Operation),
			Params:    params,
		})
	}

	return mapped
}

// ReconcileBinding keeps the spelling the config used whenever it still means what the API
// returned, and is what a resource should call rather than BindingFromPayload.
//
// `value = { literal = "high" }` and `value_literal = "high"` are the same payload, so a read
// given only the payload has to guess which the config wrote. Guessing wrong fails the apply
// outright with "Provider produced inconsistent result after apply", after the write has landed.
// Round-tripping the prior forward answers what the payload can't.
//
// prior is the plan on create and update, the prior state on read.
func ReconcileBinding(prior *Binding, fromAPI *client.EngineParamBindingPayloadV3) *Binding {
	// Absent has a single spelling, so there is nothing to preserve.
	if fromAPI == nil {
		return nil
	}

	if prior != nil {
		if want, err := BindingToPayload(prior); err == nil && reflect.DeepEqual(want, fromAPI) {
			return prior
		}
	}

	return BindingFromPayload(fromAPI)
}

// BindingFromPayload picks the narrowest spelling the value fits. Nil means the binding
// holds nothing, which is how absent is stored.
func BindingFromPayload(binding *client.EngineParamBindingPayloadV3) *Binding {
	if binding == nil {
		return nil
	}

	model := &Binding{
		ValueLiteral:   types.StringNull(),
		ValueReference: types.StringNull(),
		ExpressionRef:  types.StringNull(),
	}

	if binding.Value != nil {
		if binding.Value.Literal != nil {
			model.ValueLiteral = types.StringValue(*binding.Value.Literal)
			return model
		}
		if binding.Value.Reference != nil {
			// Same value either way, so one spelling has to win; the sugar is the one
			// worth encouraging.
			if name := ExpressionNameFromReference(*binding.Value.Reference); name != "" {
				model.ExpressionRef = types.StringValue(name)
				return model
			}

			model.ValueReference = types.StringValue(*binding.Value.Reference)
			return model
		}
	}

	if binding.ArrayValue == nil || len(*binding.ArrayValue) == 0 {
		return nil
	}

	// All literals is the `values` sugar; mixed needs the long form.
	allLiteral := true
	for _, value := range *binding.ArrayValue {
		if value.Literal == nil {
			allLiteral = false
			break
		}
	}

	if allLiteral {
		for _, value := range *binding.ArrayValue {
			model.Values = append(model.Values, types.StringValue(*value.Literal))
		}
		return model
	}

	for _, value := range *binding.ArrayValue {
		model.ArrayValue = append(model.ArrayValue, BindingValue{
			Literal:   stringOrNull(value.Literal),
			Reference: stringOrNull(value.Reference),
		})
	}

	return model
}

// ExpressionNameFromReference returns "" for an ordinary scope path, and for a reference into an
// expression's result: folding `expressions["guess"].name` to an expression_ref would drop the path.
func ExpressionNameFromReference(reference string) string {
	name, tail, ok := splitExpressionReference(reference)
	if !ok || tail != "" {
		return ""
	}

	return name
}

func stringOrNull(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}

	return types.StringValue(*value)
}

// boolOrNull keeps false as absent: `array = false` and an omitted `array` mean the same
// thing, and storing the explicit false diffs on every plan.
func boolOrNull(value bool) types.Bool {
	if !value {
		return types.BoolNull()
	}

	return types.BoolValue(true)
}
