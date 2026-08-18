package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The V3 expression and binding grammar: sugar over the engine schema, which is positional
// and deeply nested and so reads badly in HCL.
//
// Blocks for the skeleton, attributes for the payloads underneath. The framework forces
// that split rather than letting us choose per field: a block may contain attributes and
// blocks, a nested attribute may only contain attributes.

// Expression is an unnamed expression block. Declaring one binds its result to whatever
// field it hangs off, so it is mutually exclusive with that field's direct value forms.
type Expression struct {
	StartFrom  types.String `tfsdk:"start_from"`
	Operations []Operation  `tfsdk:"operation"`
	Fallback   *Fallback    `tfsdk:"fallback"`
}

// NamedExpression is Expression plus a name. Keeping the two shapes identical is what makes
// import 1:1 — one expression row becomes one block.
//
// Label is what the dashboard shows. It is separate from Name because Name is the reference
// everything else points at — a binding, another expression's operand, a varSpec inside a
// templated title — so an imported expression has to keep the reference it already has, and
// without a label of its own the next write would overwrite "Team" with "823b1cac".
type NamedExpression struct {
	Name       types.String `tfsdk:"name"`
	Label      types.String `tfsdk:"label"`
	StartFrom  types.String `tfsdk:"start_from"`
	Operations []Operation  `tfsdk:"operation"`
	Fallback   *Fallback    `tfsdk:"fallback"`
}

// LabelOrName is what to write as the label: the name, unless the config gave one.
func (e NamedExpression) LabelOrName() string {
	if e.Label.IsNull() || e.Label.IsUnknown() {
		return e.Name.ValueString()
	}

	return e.Label.ValueString()
}

// Operation holds one option set, and which one is set is what decides the operation type —
// so the type is never written out.
type Operation struct {
	Parse       *Parse       `tfsdk:"parse"`
	Navigate    *Navigate    `tfsdk:"navigate"`
	Filter      *Conditions  `tfsdk:"filter"`
	Cast        *Cast        `tfsdk:"cast"`
	Concatenate *Concatenate `tfsdk:"concatenate"`
	First       *EmptyOpts   `tfsdk:"first"`
	Count       *EmptyOpts   `tfsdk:"count"`
	Sum         *EmptyOpts   `tfsdk:"sum"`
	Min         *EmptyOpts   `tfsdk:"min"`
	Max         *EmptyOpts   `tfsdk:"max"`
	Random      *EmptyOpts   `tfsdk:"random"`
	Branches    *Branches    `tfsdk:"branches"`
}

type Parse struct {
	Function types.String `tfsdk:"function"`
	As       types.String `tfsdk:"as"`
	Array    types.Bool   `tfsdk:"array"`
}

type Navigate struct {
	To types.String `tfsdk:"to"`
}

type Cast struct {
	As types.String `tfsdk:"as"`
}

type Concatenate struct {
	With types.String `tfsdk:"with"`
}

// EmptyOpts selects a no-option operation as `first = {}` rather than a bool, so they read
// the same way as every other operation.
type EmptyOpts struct{}

// Conditions is the two spellings: `conditions` as sugar for a single group, and
// `condition_groups` for real OR-of-ANDs. At most one is set.
type Conditions struct {
	Conditions      []Condition      `tfsdk:"conditions"`
	ConditionGroups []ConditionGroup `tfsdk:"condition_groups"`
}

type ConditionGroup struct {
	Conditions []Condition `tfsdk:"conditions"`
}

type Condition struct {
	Subject   types.String `tfsdk:"subject"`
	Operation types.String `tfsdk:"operation"`
	Params    []Binding    `tfsdk:"params"`
}

// Branches is first-match-wins over `if` then each `else_if` in order.
//
// Deliberately no `else`: an unconditional default is the expression's fallback, because
// that is what the server stores it as. A branch with no conditions is a different row, and
// folding the two together would diff forever.
type Branches struct {
	As     types.String `tfsdk:"as"`
	Array  types.Bool   `tfsdk:"array"`
	If     *Branch      `tfsdk:"if"`
	ElseIf []Branch     `tfsdk:"else_if"`
}

type Branch struct {
	Conditions      []Condition      `tfsdk:"conditions"`
	ConditionGroups []ConditionGroup `tfsdk:"condition_groups"`
	Result          *Binding         `tfsdk:"result"`
}

// Fallback nests inside each expression rather than alongside them, because the server
// stores the else branch per expression — hoisting it would cap chains at one hop.
type Fallback struct {
	Result        *Binding     `tfsdk:"result"`
	ExpressionRef types.String `tfsdk:"expression_ref"`
	If            *Branch      `tfsdk:"if"`
	ElseIf        []Branch     `tfsdk:"else_if"`
	Else          *Else        `tfsdk:"else"`
}

type Else struct {
	Result *Binding `tfsdk:"result"`
}

// Binding carries the sugar spellings alongside the full forms. `values` covers the
// all-literal array; array_value stays because a mixed literal/reference array is real.
type Binding struct {
	ValueLiteral   types.String   `tfsdk:"value_literal"`
	ValueReference types.String   `tfsdk:"value_reference"`
	ExpressionRef  types.String   `tfsdk:"expression_ref"`
	Values         []types.String `tfsdk:"values"`
	Value          *BindingValue  `tfsdk:"value"`
	ArrayValue     []BindingValue `tfsdk:"array_value"`
}

type BindingValue struct {
	Literal   types.String `tfsdk:"literal"`
	Reference types.String `tfsdk:"reference"`
}
