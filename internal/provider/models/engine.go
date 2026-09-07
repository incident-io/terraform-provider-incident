package models

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/jsontypes"
)

// ParamBindingValueAttrTypes returns the attribute types for a param binding
// value object.
func ParamBindingValueAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"literal":   jsontypes.NormalizedJSONOrStringType{},
		"reference": types.StringType,
	}
}

// ParamBindingValueType is the type of one element of a binding's array_value, and of its
// value. It must match ParamBindingValueAttributes: literal is a NormalizedJSONOrString and
// not a plain string, and a list built with the wrong element type fails when the framework
// writes it to state.
func ParamBindingValueType() types.ObjectType {
	return types.ObjectType{AttrTypes: ParamBindingValueAttrTypes()}
}

// ParamBindingValueListType is the type of a binding's array_value.
func ParamBindingValueListType() types.ListType {
	return types.ListType{ElemType: ParamBindingValueType()}
}

// ParamBindingValuesListType is the type of a binding's values shorthand, which holds bare
// literals rather than value objects.
func ParamBindingValuesListType() types.ListType {
	return types.ListType{ElemType: jsontypes.NormalizedJSONOrStringType{}}
}

// ParamBindingArrayValue builds a binding's array_value from its values. No values at all
// reads as unset, which is what an array_value the API answers with but left empty means.
func ParamBindingArrayValue(values ...IncidentEngineParamBindingValue) types.List {
	if len(values) == 0 {
		return types.ListNull(ParamBindingValueType())
	}

	return types.ListValueMust(ParamBindingValueType(), lo.Map(values,
		func(v IncidentEngineParamBindingValue, _ int) attr.Value { return v.ToObject() }))
}

// ParamBindingValuesList builds a binding's values shorthand from its literals.
func ParamBindingValuesList(literals ...jsontypes.NormalizedJSONOrString) types.List {
	if len(literals) == 0 {
		return types.ListNull(jsontypes.NormalizedJSONOrStringType{})
	}

	return types.ListValueMust(jsontypes.NormalizedJSONOrStringType{}, lo.Map(literals,
		func(v jsontypes.NormalizedJSONOrString, _ int) attr.Value { return v }))
}

// NullParamBinding is a binding with nothing set. Use it rather than the zero value: a
// zero-value types.List carries no element type, and the framework can't write one to
// state.
func NullParamBinding() IncidentEngineParamBinding {
	return IncidentEngineParamBinding{
		ArrayValue:     types.ListNull(ParamBindingValueType()),
		Value:          types.ObjectNull(ParamBindingValueAttrTypes()),
		ValueLiteral:   jsontypes.NewNormalizedJSONOrStringNull(),
		ValueReference: types.StringNull(),
		ExpressionRef:  types.StringNull(),
		Values:         types.ListNull(jsontypes.NormalizedJSONOrStringType{}),
	}
}

// ParamBindingAttrTypes returns the attribute types for a param binding object. It has to list
// every attribute ParamBindingAttributes declares, including the shorthands: a binding built as
// an object from a shorter list fails at apply with "struct defines fields not found in object".
func ParamBindingAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"array_value": ParamBindingValueListType(),
		"value":       ParamBindingValueType(),

		"value_literal":   jsontypes.NormalizedJSONOrStringType{},
		"value_reference": types.StringType,
		"expression_ref":  types.StringType,
		"values":          ParamBindingValuesListType(),
	}
}

// ToObject renders a binding as an object, and ParamBindingFromObject reads one back. Alert route
// severity holds its binding as a types.Object rather than a struct, and going through here keeps
// the attribute list in one place.
func (binding IncidentEngineParamBinding) ToObject() types.Object {
	return types.ObjectValueMust(ParamBindingAttrTypes(), map[string]attr.Value{
		"array_value":     listOrNull(binding.ArrayValue, ParamBindingValueType()),
		"value":           objectOrNull(binding.Value, ParamBindingValueAttrTypes()),
		"value_literal":   binding.ValueLiteral,
		"value_reference": binding.ValueReference,
		"expression_ref":  binding.ExpressionRef,
		"values":          listOrNull(binding.Values, jsontypes.NormalizedJSONOrStringType{}),
	})
}

func ParamBindingFromObject(value attr.Value) IncidentEngineParamBinding {
	obj, ok := value.(types.Object)
	if !ok || obj.IsNull() || obj.IsUnknown() {
		return NullParamBinding()
	}

	attrs := obj.Attributes()
	binding := NullParamBinding()
	binding.ValueLiteral = stringOrJSONAttr(attrs["value_literal"])
	binding.ValueReference = stringAttr(attrs["value_reference"])
	binding.ExpressionRef = stringAttr(attrs["expression_ref"])

	if value, ok := attrs["value"].(types.Object); ok {
		binding.Value = value
	}
	if arrayValue, ok := attrs["array_value"].(types.List); ok {
		binding.ArrayValue = listOrNull(arrayValue, ParamBindingValueType())
	}
	if values, ok := attrs["values"].(types.List); ok {
		binding.Values = listOrNull(values, jsontypes.NormalizedJSONOrStringType{})
	}

	return binding
}

// listOrNull replaces a list that carries no element type - the zero value of types.List,
// which a binding built as a Go literal has - with a properly typed null. The framework
// can't write an untyped list to state, and the failure is a long way from the literal
// that caused it.
func listOrNull(list types.List, elemType attr.Type) types.List {
	if list.ElementType(context.Background()) == nil {
		return types.ListNull(elemType)
	}

	return list
}

// objectOrNull is listOrNull for an object.
func objectOrNull(obj types.Object, attrTypes map[string]attr.Type) types.Object {
	if obj.AttributeTypes(context.Background()) == nil {
		return types.ObjectNull(attrTypes)
	}

	return obj
}

// nullIfEmptyList reads a list holding nothing as unset. The API answers a binding it has no
// array for with an empty array, and a config that didn't write the attribute reads back
// null, so keeping the empty one would show as a diff on every plan.
func nullIfEmptyList(list types.List, elemType attr.Type) types.List {
	if isEmptyList(list) {
		return types.ListNull(elemType)
	}

	return listOrNull(list, elemType)
}

func (v IncidentEngineParamBindingValue) ToObject() types.Object {
	return types.ObjectValueMust(ParamBindingValueAttrTypes(), map[string]attr.Value{
		"literal":   v.Literal,
		"reference": v.Reference,
	})
}

// ParamBindingValueFromObject reads a binding value back out of the object a binding holds
// it as. A null or unknown object reads as a value with nothing set.
func ParamBindingValueFromObject(obj types.Object) IncidentEngineParamBindingValue {
	attrs := obj.Attributes()

	return IncidentEngineParamBindingValue{
		Literal:   stringOrJSONAttr(attrs["literal"]),
		Reference: stringAttr(attrs["reference"]),
	}
}

// isSet reports whether a config actually gave this attribute a value.
func isSet(value attr.Value) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func stringAttr(value attr.Value) types.String {
	if str, ok := value.(types.String); ok {
		return str
	}

	return types.StringNull()
}

func stringOrJSONAttr(value attr.Value) jsontypes.NormalizedJSONOrString {
	if str, ok := value.(jsontypes.NormalizedJSONOrString); ok {
		return str
	}

	return jsontypes.NewNormalizedJSONOrStringNull()
}

// ConditionAttrTypes returns the attribute types for a single condition object.
func ConditionAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"operation": types.StringType,
		"param_bindings": types.ListType{
			ElemType: types.ObjectType{AttrTypes: ParamBindingAttrTypes()},
		},
		"subject": types.StringType,
	}
}

// Types

type IncidentEngineConditionGroups []IncidentEngineConditionGroup

func (IncidentEngineConditionGroups) FromAPI(groups []client.ConditionGroupV2) IncidentEngineConditionGroups {
	out := IncidentEngineConditionGroups{}

	for _, g := range groups {
		out = append(out, IncidentEngineConditionGroup{
			Conditions: IncidentEngineConditions{}.FromAPI(g.Conditions),
		})
	}

	return out
}

type IncidentEngineConditionGroup struct {
	Conditions IncidentEngineConditions `tfsdk:"conditions"`
}

// serverOperationNormalisations maps operation aliases the API accepts to the
// canonical name it returns for them. Each is canonical on other subject types,
// so we match the API value exactly rather than rewriting on the alias alone.
var serverOperationNormalisations = map[string]string{
	"one_of":   "contains_one_of", // String, Link and TemplatedText subjects
	"contains": "name_contains",   // CatalogEntry subjects
}

// ReconcileSpelling restores what the config wrote wherever the API answers with something that
// means the same thing: the operation, where the API returned the canonical form of an alias we
// sent (ONC-12602), and each param binding, where the API only ever reports the two long forms.
//
// Condition groups are lists, so we correlate positionally and require the subject to match.
func (groups IncidentEngineConditionGroups) ReconcileSpelling(plan IncidentEngineConditionGroups) {
	for gi := range groups {
		if gi >= len(plan) {
			break
		}

		groups[gi].Conditions.ReconcileSpelling(plan[gi].Conditions)
	}
}

type IncidentEngineConditions []IncidentEngineCondition

func (IncidentEngineConditions) FromAPI(conditions []client.ConditionV2) IncidentEngineConditions {
	out := []IncidentEngineCondition{}
	for _, cond := range conditions {
		out = append(out, IncidentEngineCondition{}.FromAPI(cond))
	}
	return out
}

// ReconcileSpelling correlates positionally and requires the subject to match, so a condition
// that moved is left with whatever the API said.
func (conditions IncidentEngineConditions) ReconcileSpelling(plan IncidentEngineConditions) {
	for ci := range conditions {
		if ci >= len(plan) {
			break
		}

		planned, applied := plan[ci], conditions[ci]
		if !applied.Subject.Equal(planned.Subject) {
			continue
		}

		if serverOperationNormalisations[planned.Operation.ValueString()] == applied.Operation.ValueString() {
			conditions[ci].Operation = planned.Operation
		}
		conditions[ci].ParamBindings = applied.ParamBindings.ReconcileSpelling(planned.ParamBindings)
	}
}

type IncidentEngineCondition struct {
	Subject       types.String                `tfsdk:"subject"`
	Operation     types.String                `tfsdk:"operation"`
	ParamBindings IncidentEngineParamBindings `tfsdk:"param_bindings"`
}

func (IncidentEngineCondition) FromAPI(condition client.ConditionV2) IncidentEngineCondition {
	return IncidentEngineCondition{
		Subject:       types.StringValue(condition.Subject.Reference),
		Operation:     types.StringValue(condition.Operation.Value),
		ParamBindings: IncidentEngineParamBindings{}.FromAPI(condition.ParamBindings),
	}
}

type IncidentEngineParamBindings []IncidentEngineParamBinding

func (IncidentEngineParamBindings) FromAPI(pbs []client.EngineParamBindingV2) IncidentEngineParamBindings {
	out := []IncidentEngineParamBinding{}

	for _, pb := range pbs {
		out = append(out, IncidentEngineParamBinding{}.FromAPI(pb))
	}

	return out
}

// ReconcileSpelling puts back the shorthand the config used: a read only sees the stored form, so
// `value_literal = "x"` would otherwise read back as `value = { literal = "x" }` and fail the
// apply as an inconsistent result.
//
// The prior only wins while it still means what came back, so genuine drift isn't hidden.
func (binding IncidentEngineParamBinding) ReconcileSpelling(prior IncidentEngineParamBinding) IncidentEngineParamBinding {
	if binding.meansTheSameAs(prior) {
		return prior
	}

	return binding
}

func (binding IncidentEngineParamBinding) meansTheSameAs(other IncidentEngineParamBinding) bool {
	return binding.resolved().meansTheSameAs(other.resolved())
}

// meansTheSameAs compares two bindings by what they bind rather than how they were written.
// A binding Terraform hasn't settled means nothing yet, so it matches nothing - including
// another unsettled one, which may land somewhere else entirely.
func (resolved resolvedBinding) meansTheSameAs(other resolvedBinding) bool {
	if resolved.unsettled || other.unsettled {
		return false
	}

	if (resolved.Value == nil) != (other.Value == nil) {
		return false
	}
	if resolved.Value != nil && !resolved.Value.meansTheSameAs(*other.Value) {
		return false
	}
	if len(resolved.ArrayValue) != len(other.ArrayValue) {
		return false
	}
	for idx := range resolved.ArrayValue {
		if !resolved.ArrayValue[idx].meansTheSameAs(other.ArrayValue[idx]) {
			return false
		}
	}

	return true
}

// meansTheSameAs compares literals the way NormalizedJSONOrString compares them in state, rather
// than byte for byte. The API re-encodes a JSON literal, so a difference in key order or escaping
// would otherwise read as a changed value and drop the shorthand the config wrote.
func (v IncidentEngineParamBindingValue) meansTheSameAs(other IncidentEngineParamBindingValue) bool {
	if !v.Reference.Equal(other.Reference) {
		return false
	}
	if v.Literal.IsNull() != other.Literal.IsNull() || v.Literal.IsUnknown() != other.Literal.IsUnknown() {
		return false
	}

	return jsontypes.JSONStringsEqual(v.Literal.ValueString(), other.Literal.ValueString())
}

// ReconcileBindingSpelling is ReconcileSpelling for the standalone bindings a resource holds as a
// pointer, rather than as part of a list.
func ReconcileBindingSpelling(applied, prior *IncidentEngineParamBinding) *IncidentEngineParamBinding {
	if applied == nil || prior == nil {
		return applied
	}

	return lo.ToPtr(applied.ReconcileSpelling(*prior))
}

// ReconcileSpelling correlates positionally: param bindings are a list, and the API returns them
// in the order it was given them.
func (pbs IncidentEngineParamBindings) ReconcileSpelling(prior IncidentEngineParamBindings) IncidentEngineParamBindings {
	for idx := range pbs {
		if idx >= len(prior) {
			break
		}
		pbs[idx] = pbs[idx].ReconcileSpelling(prior[idx])
	}

	return pbs
}

// ReconcileScalarBinding is ReconcileSpelling for an endpoint that stores a scalar binding
// as a one-element array and answers with the array. The policies API does this to assignee
// bindings, because it binds them against an array param, so a config written as
// value_literal reads back as array_value and fails the apply as an inconsistent result.
//
// meansTheSameAs cannot absorb that on its own: a scalar and an array are different shapes,
// and for every other endpoint the difference is real drift. Restoring the config's
// spelling hides nothing here, because the server treats the two as one binding.
func ReconcileScalarBinding(applied, prior IncidentEngineParamBinding) IncidentEngineParamBinding {
	if applied.meansTheSameAs(prior) {
		return prior
	}

	resolved := applied.resolved()
	if resolved.Value == nil && len(resolved.ArrayValue) == 1 {
		scalar := resolvedBinding{Value: &resolved.ArrayValue[0]}
		if scalar.meansTheSameAs(prior.resolved()) {
			return prior
		}
	}

	return applied
}

// ReconcileScalarSpelling is ReconcileSpelling using ReconcileScalarBinding, correlating
// positionally for the same reason.
func (pbs IncidentEngineParamBindings) ReconcileScalarSpelling(prior IncidentEngineParamBindings) IncidentEngineParamBindings {
	for idx := range pbs {
		if idx >= len(prior) {
			break
		}
		pbs[idx] = ReconcileScalarBinding(pbs[idx], prior[idx])
	}

	return pbs
}

// TrimAppendedEmpty drops trailing empty bindings the API padded onto a step that has
// gained params, beyond the priorLen we sent. Stopping at priorLen keeps a configured
// empty binding, which means "skip this optional param".
func (pbs IncidentEngineParamBindings) TrimAppendedEmpty(priorLen int) IncidentEngineParamBindings {
	out := pbs
	for len(out) > priorLen && out[len(out)-1].IsEmpty() {
		out = out[:len(out)-1]
	}

	return out
}

// A binding has two forms the API understands, `value` and `array_value`, and four shorthands.
// The shorthands carry no meaning of their own: ToPayload folds them onto the two real forms, and
// a read puts back whichever spelling the config used, because Terraform compares the spelling
// and not the meaning.
// The framework types matter: array_value, value and values are all attributes a config can
// point at an expression Terraform hasn't settled yet - a local, a for, a ternary, anything
// under for_each - and a plain Go slice or pointer can't hold the unknown that arrives while
// validating such a config. Everything downstream works on the resolved form instead, which
// is settled by construction.
type IncidentEngineParamBinding struct {
	ArrayValue types.List   `tfsdk:"array_value"`
	Value      types.Object `tfsdk:"value"`

	ValueLiteral   jsontypes.NormalizedJSONOrString `tfsdk:"value_literal"`
	ValueReference types.String                     `tfsdk:"value_reference"`
	ExpressionRef  types.String                     `tfsdk:"expression_ref"`
	Values         types.List                       `tfsdk:"values"`
}

func (binding IncidentEngineParamBinding) IsEmpty() bool {
	return binding.Value.IsNull() && isEmptyList(binding.ArrayValue) &&
		binding.ValueLiteral.IsNull() && binding.ValueReference.IsNull() &&
		binding.ExpressionRef.IsNull() && isEmptyList(binding.Values)
}

// isEmptyList reports a list holding nothing. Null and empty both count, which is what the
// slice this used to be said: an unset attribute and an `= []` both read as len 0. An
// unknown doesn't, because it may yet turn out to hold something.
func isEmptyList(list types.List) bool {
	if list.IsUnknown() {
		return false
	}

	return list.IsNull() || len(list.Elements()) == 0
}

// resolvedBinding is a binding folded onto the two forms the API understands, holding plain
// Go values rather than framework ones.
//
// Comparison, payloads and spelling all work on this rather than on the model, so each of
// them asks about null and unknown once, here, instead of at every use.
type resolvedBinding struct {
	ArrayValue []IncidentEngineParamBindingValue
	Value      *IncidentEngineParamBindingValue

	// unsettled marks a binding whose form Terraform hasn't decided yet, so there is
	// nothing to compare and nothing to send. Only a config or plan can hold one: every
	// binding attribute is settled by the time an apply reads it.
	unsettled bool
}

// resolved folds the shorthands onto the two forms the API understands, so a shorthand and the
// long form it stands for are the same binding from here on.
// An unknown shorthand counts as unset, so the value it stands for stays unknown rather than
// becoming whatever ValueString reads off an unknown, which is the empty string.
func (binding IncidentEngineParamBinding) resolved() resolvedBinding {
	switch {
	case isSet(binding.ValueLiteral):
		return resolvedBinding{Value: &IncidentEngineParamBindingValue{
			Literal:   binding.ValueLiteral,
			Reference: types.StringNull(),
		}}

	case isSet(binding.ValueReference):
		return resolvedBinding{Value: &IncidentEngineParamBindingValue{
			Literal:   jsontypes.NewNormalizedJSONOrStringNull(),
			Reference: binding.ValueReference,
		}}

	case isSet(binding.ExpressionRef):
		return resolvedBinding{Value: &IncidentEngineParamBindingValue{
			Literal:   jsontypes.NewNormalizedJSONOrStringNull(),
			Reference: types.StringValue(ExpressionReference(binding.ExpressionRef.ValueString())),
		}}

	case isSet(binding.Values) && len(binding.Values.Elements()) > 0:
		values := []IncidentEngineParamBindingValue{}
		for _, literal := range binding.Values.Elements() {
			values = append(values, IncidentEngineParamBindingValue{
				Literal:   stringOrJSONAttr(literal),
				Reference: types.StringNull(),
			})
		}

		return resolvedBinding{ArrayValue: values}

	// Checked after every shorthand, so a form the config did write wins over one it left
	// for Terraform to settle - the same way a set shorthand wins over a set long form.
	case binding.Values.IsUnknown() || binding.ArrayValue.IsUnknown() || binding.Value.IsUnknown():
		return resolvedBinding{unsettled: true}
	}

	out := resolvedBinding{}
	if isSet(binding.Value) {
		out.Value = lo.ToPtr(ParamBindingValueFromObject(binding.Value))
	}
	for _, element := range binding.ArrayValue.Elements() {
		value, ok := element.(types.Object)
		if !ok {
			continue
		}

		// An element Terraform hasn't settled leaves the whole binding unsettled. The list
		// around it can be known while an element isn't - a ternary whose branches are the
		// same length gives exactly that, which is how the reported failure arrived - and
		// reading such an element off would produce a value with no literal and no
		// reference, indistinguishable from one the config left empty.
		if value.IsUnknown() {
			return resolvedBinding{unsettled: true}
		}

		out.ArrayValue = append(out.ArrayValue, ParamBindingValueFromObject(value))
	}

	return out
}

func (IncidentEngineParamBinding) FromAPI(pb client.EngineParamBindingV2) IncidentEngineParamBinding {
	binding := NullParamBinding()

	if pb.ArrayValue != nil {
		values := make([]IncidentEngineParamBindingValue, 0, len(*pb.ArrayValue))
		for _, v := range *pb.ArrayValue {
			values = append(values, IncidentEngineParamBindingValue{
				Literal:   jsontypes.NewNormalizedJSONOrStringPointerValue(v.Literal),
				Reference: types.StringPointerValue(v.Reference),
			})
		}
		binding.ArrayValue = ParamBindingArrayValue(values...)
	}

	if pb.Value != nil {
		binding.Value = IncidentEngineParamBindingValue{}.FromAPI(*pb.Value).ToObject()
	}

	return binding
}

type IncidentEngineParamBindingValue struct {
	Literal   jsontypes.NormalizedJSONOrString `tfsdk:"literal"`
	Reference types.String                     `tfsdk:"reference"`
}

func (IncidentEngineParamBindingValue) FromAPI(pbv client.EngineParamBindingValueV2) IncidentEngineParamBindingValue {
	// The Literal field is a jsontypes.NormalizedJSONOrString. Its semantic
	// equality is what prevents diffs and inconsistent-result errors: when the
	// planned and applied values are equivalent JSON (ignoring key order and
	// HTML escaping), Terraform keeps the user's own value and never compares
	// bytes. So we deliberately store the API value verbatim here and let
	// semantic equality absorb any byte differences, rather than re-encoding it.
	return IncidentEngineParamBindingValue{
		Literal:   jsontypes.NewNormalizedJSONOrStringPointerValue(pbv.Literal),
		Reference: types.StringPointerValue(pbv.Reference),
	}
}

type IncidentEngineExpressions []IncidentEngineExpression

// ReconcileOperations does the same for filter and branch conditions. Expressions
// are a set, so we correlate by reference rather than position.
func (expressions IncidentEngineExpressions) ReconcileSpelling(plan IncidentEngineExpressions) {
	planByReference := map[string]IncidentEngineExpression{}
	for _, expression := range plan {
		planByReference[expression.Reference.ValueString()] = expression
	}

	for ei := range expressions {
		planExpression, ok := planByReference[expressions[ei].Reference.ValueString()]
		if !ok {
			continue
		}

		if expressions[ei].ElseBranch != nil && planExpression.ElseBranch != nil {
			expressions[ei].ElseBranch.Result = expressions[ei].ElseBranch.Result.
				ReconcileSpelling(planExpression.ElseBranch.Result)
		}

		for oi := range expressions[ei].Operations {
			if oi >= len(planExpression.Operations) {
				break
			}

			op := expressions[ei].Operations[oi]
			planOp := planExpression.Operations[oi]

			if op.Filter != nil && planOp.Filter != nil {
				op.Filter.ConditionGroups.ReconcileSpelling(planOp.Filter.ConditionGroups)
			}

			if op.Branches != nil && planOp.Branches != nil {
				for bi := range op.Branches.Branches {
					if bi >= len(planOp.Branches.Branches) {
						break
					}
					op.Branches.Branches[bi].ConditionGroups.ReconcileSpelling(planOp.Branches.Branches[bi].ConditionGroups)
					op.Branches.Branches[bi].Result = op.Branches.Branches[bi].Result.
						ReconcileSpelling(planOp.Branches.Branches[bi].Result)
				}
			}
		}
	}
}

func (IncidentEngineExpressions) FromAPI(expressions []client.ExpressionV2) IncidentEngineExpressions {
	out := IncidentEngineExpressions{}

	for _, e := range expressions {
		expression := IncidentEngineExpression{
			Label:         types.StringValue(e.Label),
			Operations:    IncidentEngineExpressionOperation{}.FromAPI(e.Operations),
			Reference:     types.StringValue(e.Reference),
			RootReference: types.StringValue(e.RootReference),
		}
		if e.ElseBranch != nil {
			expression.ElseBranch = &IncidentEngineElseBranch{
				Result: IncidentEngineParamBinding{}.FromAPI(e.ElseBranch.Result),
			}
		}
		out = append(out, expression)
	}

	return out
}

type IncidentEngineExpression struct {
	ElseBranch    *IncidentEngineElseBranch          `tfsdk:"else_branch"`
	Label         types.String                       `tfsdk:"label"`
	Operations    IncidentEngineExpressionOperations `tfsdk:"operations"`
	Reference     types.String                       `tfsdk:"reference"`
	RootReference types.String                       `tfsdk:"root_reference"`
}

type IncidentEngineElseBranch struct {
	Result IncidentEngineParamBinding `tfsdk:"result"`
}

type IncidentEngineExpressionOperation struct {
	Branches    *IncidentEngineExpressionBranchesOpts    `tfsdk:"branches"`
	Cast        *IncidentEngineExpressionCastOpts        `tfsdk:"cast"`
	Concatenate *IncidentEngineExpressionConcatenateOpts `tfsdk:"concatenate"`
	Filter      *IncidentEngineExpressionFilterOpts      `tfsdk:"filter"`
	Navigate    *IncidentEngineExpressionNavigateOpts    `tfsdk:"navigate"`
	Parse       *IncidentEngineExpressionParseOpts       `tfsdk:"parse"`

	OperationType types.String `tfsdk:"operation_type"`
}

func (IncidentEngineExpressionOperation) FromAPI(operations []client.ExpressionOperationV2) []IncidentEngineExpressionOperation {
	out := []IncidentEngineExpressionOperation{}

	for _, o := range operations {
		operation := IncidentEngineExpressionOperation{
			OperationType: types.StringValue(string(o.OperationType)),
		}
		if o.Branches != nil {
			operation.Branches = &IncidentEngineExpressionBranchesOpts{
				Branches: IncidentEngineBranches{}.fromAPI(o.Branches.Branches),
				Returns:  IncidentEngineReturnsMeta{}.fromAPI(o.Branches.Returns),
			}
		}
		if o.Cast != nil {
			operation.Cast = &IncidentEngineExpressionCastOpts{
				Returns: IncidentEngineReturnsMeta{}.fromAPI(o.Cast.Returns),
			}
		} else if o.OperationType == client.ExpressionOperationV2OperationTypeCast {
			// Older API versions omit cast's options and expose the target as the
			// operation's own returns instead, which holds the same values.
			operation.Cast = &IncidentEngineExpressionCastOpts{
				Returns: IncidentEngineReturnsMeta{}.fromAPI(o.Returns),
			}
		}
		if o.Concatenate != nil {
			operation.Concatenate = &IncidentEngineExpressionConcatenateOpts{
				Reference: types.StringValue(o.Concatenate.Reference),
			}
		}
		if o.Filter != nil {
			operation.Filter = &IncidentEngineExpressionFilterOpts{
				ConditionGroups: IncidentEngineConditionGroups{}.FromAPI(o.Filter.ConditionGroups),
			}
		}
		if o.Navigate != nil {
			operation.Navigate = &IncidentEngineExpressionNavigateOpts{
				Reference: types.StringValue(o.Navigate.Reference),
			}
		}
		if o.Parse != nil {
			operation.Parse = &IncidentEngineExpressionParseOpts{
				Returns: IncidentEngineReturnsMeta{}.fromAPI(o.Parse.Returns),
				Source:  types.StringValue(o.Parse.Source),
			}
		}
		out = append(out, operation)
	}

	return out
}

type IncidentEngineExpressionBranchesOpts struct {
	Branches IncidentEngineBranches    `tfsdk:"branches"`
	Returns  IncidentEngineReturnsMeta `tfsdk:"returns"`
}

type IncidentEngineBranch struct {
	ConditionGroups IncidentEngineConditionGroups `tfsdk:"condition_groups"`
	Result          IncidentEngineParamBinding    `tfsdk:"result"`
}

func (IncidentEngineBranches) fromAPI(branches []client.ExpressionBranchV2) IncidentEngineBranches {
	out := IncidentEngineBranches{}

	for _, b := range branches {
		out = append(out, IncidentEngineBranch{
			ConditionGroups: IncidentEngineConditionGroups{}.FromAPI(b.ConditionGroups),
			Result:          IncidentEngineParamBinding{}.FromAPI(b.Result),
		})
	}

	return out
}

type IncidentEngineReturnsMeta struct {
	Array types.Bool   `tfsdk:"array"`
	Type  types.String `tfsdk:"type"`
}

func (IncidentEngineReturnsMeta) fromAPI(returns client.ReturnsMetaV2) IncidentEngineReturnsMeta {
	return IncidentEngineReturnsMeta{
		Array: types.BoolValue(returns.Array),
		Type:  types.StringValue(returns.Type),
	}
}

type IncidentEngineExpressionCastOpts struct {
	Returns IncidentEngineReturnsMeta `tfsdk:"returns"`
}

type IncidentEngineExpressionConcatenateOpts struct {
	Reference types.String `tfsdk:"reference"`
}

type IncidentEngineExpressionFilterOpts struct {
	ConditionGroups IncidentEngineConditionGroups `tfsdk:"condition_groups"`
}

type IncidentEngineExpressionNavigateOpts struct {
	Reference types.String `tfsdk:"reference"`
}

type IncidentEngineExpressionParseOpts struct {
	Returns IncidentEngineReturnsMeta `tfsdk:"returns"`
	Source  types.String              `tfsdk:"source"`
}

// Attributes

func ParamBindingValueAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"literal": schema.StringAttribute{
			CustomType:          jsontypes.NormalizedJSONOrStringType{},
			MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "literal"),
			Optional:            true,
		},
		"reference": schema.StringAttribute{
			MarkdownDescription: apischema.Docstring("EngineParamBindingValueV2", "reference"),
			Optional:            true,
		},
	}
}

func ParamBindingAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"array_value": schema.ListNestedAttribute{
			MarkdownDescription: "The array of literal or reference parameter values",
			Optional:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: ParamBindingValueAttributes(),
			},
		},
		"value": schema.SingleNestedAttribute{
			MarkdownDescription: "The literal or reference parameter value",
			Optional:            true,
			Attributes:          ParamBindingValueAttributes(),
		},

		// Shorthands for the two above. ConflictsWith keeps them exclusive wherever the binding
		// is used, so no resource has to wire up its own check.
		"value_literal": schema.StringAttribute{
			CustomType:          jsontypes.NormalizedJSONOrStringType{},
			MarkdownDescription: "A fixed value, shorthand for `value = { literal = ... }`. A catalog entry ID is a literal, not a reference.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.ConflictsWith(bindingFormsOtherThan("value_literal")...)},
		},
		"value_reference": schema.StringAttribute{
			MarkdownDescription: "A reference into the scope, shorthand for `value = { reference = ... }`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.ConflictsWith(bindingFormsOtherThan("value_reference")...)},
		},
		"expression_ref": schema.StringAttribute{
			MarkdownDescription: "The name of an expression on this resource, whose result becomes the value. Shorthand for referencing `expressions[\"name\"]`.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.ConflictsWith(bindingFormsOtherThan("expression_ref")...)},
		},
		"values": schema.ListAttribute{
			ElementType:         jsontypes.NormalizedJSONOrStringType{},
			MarkdownDescription: "Several fixed values, shorthand for an `array_value` of literals. For a mix of literals and references, use `array_value`.",
			Optional:            true,
			Validators:          []validator.List{listvalidator.ConflictsWith(bindingFormsOtherThan("values")...)},
		},
	}
}

// bindingForms is every way of writing a binding's value. Exactly one may be set.
var bindingForms = []string{"value", "array_value", "value_literal", "value_reference", "expression_ref", "values"}

// bindingFormsOtherThan names the sibling attributes a given form conflicts with. The paths are
// relative to the binding object, so they resolve wherever the binding is nested.
func bindingFormsOtherThan(self string) []path.Expression {
	out := []path.Expression{}
	for _, form := range bindingForms {
		if form != self {
			out = append(out, path.MatchRelative().AtParent().AtName(form))
		}
	}

	return out
}

func ParamBindingsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: apischema.Docstring("ConditionV2", "param_bindings"),
		Required:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: ParamBindingAttributes(),
		},
	}
}

func ConditionsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: "The prerequisite conditions that must all be satisfied",
		Required:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"operation": schema.StringAttribute{
					MarkdownDescription: "The logical operation to be applied",
					Required:            true,
				},
				"param_bindings": ParamBindingsAttribute(),
				"subject": schema.StringAttribute{
					MarkdownDescription: "The subject of the condition, on which the operation is applied",
					Required:            true,
				},
			},
		},
	}
}

func ConditionGroupsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		MarkdownDescription: "Groups of prerequisite conditions. All conditions in at least one group must be satisfied",
		Required:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"conditions": ConditionsAttribute(),
			},
		},
	}
}

func ReturnsAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "The return type of an operation",
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"array": schema.BoolAttribute{
				MarkdownDescription: apischema.Docstring("ReturnsMetaV2", "array"),
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: apischema.Docstring("ReturnsMetaV2", "type"),
				Required:            true,
			},
		},
	}
}

func ExpressionsAttribute() schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		MarkdownDescription: "The expressions to be prepared for use by steps and conditions",
		Required:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"label": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("ExpressionV2", "label"),
					Required:            true,
				},
				"reference": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("ExpressionV2", "reference"),
					Required:            true,
				},
				"root_reference": schema.StringAttribute{
					MarkdownDescription: apischema.Docstring("ExpressionV2", "root_reference"),
					Required:            true,
				},
				"else_branch": schema.SingleNestedAttribute{
					MarkdownDescription: "The else branch to resort to if all operations fail",
					Optional:            true,
					Attributes: map[string]schema.Attribute{
						"result": schema.SingleNestedAttribute{
							MarkdownDescription: "The result assumed if the else branch is reached",
							Required:            true,
							Attributes:          ParamBindingAttributes(),
						},
					},
				},
				"operations": schema.ListNestedAttribute{
					MarkdownDescription: "The operations to execute in sequence for this expression",
					Required:            true,
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"branches": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that allows for a value to be set conditionally by a series of logical branches",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"branches": schema.ListNestedAttribute{
										MarkdownDescription: apischema.Docstring("ExpressionBranchesOptsV2", "branches"),
										Required:            true,
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"condition_groups": ConditionGroupsAttribute(),
												"result": schema.SingleNestedAttribute{
													MarkdownDescription: "The result assumed if the condition groups are satisfied",
													Required:            true,
													Attributes:          ParamBindingAttributes(),
												},
											},
										},
									},
									"returns": ReturnsAttribute(),
								},
							},
							"cast": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that converts a value into another type. Only valid on values that can be represented as text. The returned `array` follows the value being cast, so it must match the cardinality of the previous operation",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"returns": ReturnsAttribute(),
								},
							},
							"concatenate": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that adds the values behind another reference to the current value, keeping each value once. There is no delimiter, despite the name",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"reference": schema.StringAttribute{
										MarkdownDescription: apischema.Docstring("ExpressionConcatenateOptsV2", "reference"),
										Required:            true,
									},
								},
							},
							"filter": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that allows values to be filtered out by conditions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"condition_groups": ConditionGroupsAttribute(),
								},
							},
							"navigate": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that allows attributes of a type to be accessed by reference",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"reference": schema.StringAttribute{
										Required: true,
									},
								},
							},
							"operation_type": schema.StringAttribute{
								MarkdownDescription: apischema.DescribeEnumValues(
									"Indicates which operation type to execute",
									"ExpressionOperationV2", "operation_type"),
								Required: true,
							},
							"parse": schema.SingleNestedAttribute{
								MarkdownDescription: "An operation type that allows a value to parsed from within a JSON object",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"returns": ReturnsAttribute(),
									"source": schema.StringAttribute{
										MarkdownDescription: "The ES5 Javascript expression to execute",
										Required:            true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ToPayloadConditionGroups converts from the terraform model to the http payload type.
// The payload type is different from the response type, which includes more information such as labels.
func (groups IncidentEngineConditionGroups) ToPayload() []client.ConditionGroupPayloadV2 {
	out := []client.ConditionGroupPayloadV2{}

	for _, group := range groups {
		out = append(out, client.ConditionGroupPayloadV2{
			Conditions: group.Conditions.ToPayload(),
		})
	}

	return out
}

func (conditions IncidentEngineConditions) ToPayload() []client.ConditionPayloadV2 {
	out := []client.ConditionPayloadV2{}

	for _, c := range conditions {
		out = append(out, client.ConditionPayloadV2{
			Subject:       c.Subject.ValueString(),
			Operation:     c.Operation.ValueString(),
			ParamBindings: (c.ParamBindings).ToPayload(),
		})
	}

	return out
}

func (pbs IncidentEngineParamBindings) ToPayload() []client.EngineParamBindingPayloadV2 {
	paramBindings := []client.EngineParamBindingPayloadV2{}

	for _, binding := range pbs {
		paramBindings = append(paramBindings, binding.ToPayload())
	}

	return paramBindings
}

func (binding IncidentEngineParamBinding) ToPayload() client.EngineParamBindingPayloadV2 {
	resolved := binding.resolved()

	arrayValue := []client.EngineParamBindingValuePayloadV2{}
	for _, v := range resolved.ArrayValue {
		arrayValue = append(arrayValue, v.ToPayload())
	}

	var value *client.EngineParamBindingValuePayloadV2
	if resolved.Value != nil {
		value = lo.ToPtr(resolved.Value.ToPayload())
	}

	return client.EngineParamBindingPayloadV2{
		ArrayValue: &arrayValue,
		Value:      value,
	}
}

func (v IncidentEngineParamBindingValue) ToPayload() client.EngineParamBindingValuePayloadV2 {
	return client.EngineParamBindingValuePayloadV2{
		Literal:   v.Literal.ValueStringPointer(),
		Reference: v.Reference.ValueStringPointer(),
	}
}

func (expressions IncidentEngineExpressions) ToPayload() []client.ExpressionPayloadV2 {
	out := []client.ExpressionPayloadV2{}

	for _, e := range expressions {
		expression := client.ExpressionPayloadV2{
			Label:         e.Label.ValueString(),
			Operations:    e.Operations.toPayload(),
			Reference:     e.Reference.ValueString(),
			RootReference: e.RootReference.ValueString(),
		}
		if e.ElseBranch != nil {
			expression.ElseBranch = &client.ExpressionElseBranchPayloadV2{
				Result: e.ElseBranch.Result.ToPayload(),
			}
		}
		out = append(out, expression)
	}

	return out
}

type IncidentEngineExpressionOperations []IncidentEngineExpressionOperation

func (operations IncidentEngineExpressionOperations) toPayload() []client.ExpressionOperationPayloadV2 {
	out := []client.ExpressionOperationPayloadV2{}

	for _, o := range operations {
		operation := client.ExpressionOperationPayloadV2{
			OperationType: client.ExpressionOperationPayloadV2OperationType(o.OperationType.ValueString()),
		}
		if o.Branches != nil {
			operation.Branches = &client.ExpressionBranchesOptsPayloadV2{
				Branches: o.Branches.Branches.toPayload(),
				Returns:  o.Branches.Returns.toPayload(),
			}
		}
		if o.Cast != nil {
			operation.Cast = &client.ExpressionCastOptsPayloadV2{
				Returns: o.Cast.Returns.toPayload(),
			}
		}
		if o.Concatenate != nil {
			operation.Concatenate = &client.ExpressionConcatenateOptsPayloadV2{
				Reference: o.Concatenate.Reference.ValueString(),
			}
		}
		if o.Filter != nil {
			operation.Filter = &client.ExpressionFilterOptsPayloadV2{
				ConditionGroups: o.Filter.ConditionGroups.ToPayload(),
			}
		}
		if o.Navigate != nil {
			operation.Navigate = &client.ExpressionNavigateOptsPayloadV2{
				Reference: o.Navigate.Reference.ValueString(),
			}
		}
		if o.Parse != nil {
			operation.Parse = &client.ExpressionParseOptsPayloadV2{
				Returns: o.Parse.Returns.toPayload(),
				Source:  o.Parse.Source.ValueString(),
			}
		}
		out = append(out, operation)
	}

	return out
}

type IncidentEngineBranches []IncidentEngineBranch

func (branches IncidentEngineBranches) toPayload() []client.ExpressionBranchPayloadV2 {
	out := []client.ExpressionBranchPayloadV2{}

	for _, b := range branches {
		out = append(out, client.ExpressionBranchPayloadV2{
			ConditionGroups: b.ConditionGroups.ToPayload(),
			Result:          b.Result.ToPayload(),
		})
	}

	return out
}

func (returns IncidentEngineReturnsMeta) toPayload() client.ReturnsMetaV2 {
	return client.ReturnsMetaV2{
		Array: returns.Array.ValueBool(),
		Type:  returns.Type.ValueString(),
	}
}
