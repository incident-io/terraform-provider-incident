package models

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/apischema"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

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

type IncidentEngineConditions []IncidentEngineCondition

func (IncidentEngineConditions) FromAPI(conditions []client.ConditionV2) IncidentEngineConditions {
	out := []IncidentEngineCondition{}
	for _, cond := range conditions {
		out = append(out, IncidentEngineCondition{}.FromAPI(cond))
	}
	return out
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

type IncidentEngineParamBinding struct {
	ArrayValue []IncidentEngineParamBindingValue `tfsdk:"array_value"`
	Value      *IncidentEngineParamBindingValue  `tfsdk:"value"`
}

func (IncidentEngineParamBinding) FromAPI(pb client.EngineParamBindingV2) IncidentEngineParamBinding {
	var arrayValue []IncidentEngineParamBindingValue
	if pb.ArrayValue != nil {
		for _, v := range *pb.ArrayValue {
			arrayValue = append(arrayValue, IncidentEngineParamBindingValue{
				Literal:   types.StringPointerValue(v.Literal),
				Reference: types.StringPointerValue(v.Reference),
			})
		}
	}

	var value *IncidentEngineParamBindingValue
	if pb.Value != nil {
		value = lo.ToPtr(IncidentEngineParamBindingValue{}.FromAPI(*pb.Value))
	}

	return IncidentEngineParamBinding{
		ArrayValue: arrayValue,
		Value:      value,
	}
}

type IncidentEngineParamBindingValue struct {
	Literal   types.String `tfsdk:"literal"`
	Reference types.String `tfsdk:"reference"`
}

func (IncidentEngineParamBindingValue) FromAPI(pbv client.EngineParamBindingValueV2) IncidentEngineParamBindingValue {
	literal := pbv.Literal
	if literal != nil {
		// If we have a literal engine value (that is JSON), we'll attempt to normalise it to
		// provide consistent key ordering.
		//
		// Most places where we initialise engine values should already sort keys alphabetically,
		// but there are the odd places where this is not the case.
		normalisedJSON, err := normaliseJSON(*literal)
		if err == nil {
			// Given not every engine value is JSON, we'll lean on the presence (or lack of) an error here
			// rather than looking for characters that indicate JSON.
			literal = &normalisedJSON
		}
	}

	return IncidentEngineParamBindingValue{
		Literal:   types.StringPointerValue(literal),
		Reference: types.StringPointerValue(pbv.Reference),
	}
}

// normaliseJSON normalises JSON strings to ensure consistent key ordering.
func normaliseJSON(jsonString string) (string, error) {
	if jsonString == "" {
		return "", nil
	}

	var data any
	err := json.Unmarshal([]byte(jsonString), &data)
	if err != nil {
		return "", err
	}

	// Use encoder with HTML escaping disabled to preserve special characters like >
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	err = encoder.Encode(data)
	if err != nil {
		return "", err
	}

	// Remove the trailing newline that Encode adds
	normalisedJSON := strings.TrimSuffix(buf.String(), "\n")

	return normalisedJSON, nil
}

type IncidentEngineExpressions []IncidentEngineExpression

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
	Branches *IncidentEngineExpressionBranchesOpts `tfsdk:"branches"`
	Filter   *IncidentEngineExpressionFilterOpts   `tfsdk:"filter"`
	Navigate *IncidentEngineExpressionNavigateOpts `tfsdk:"navigate"`
	Parse    *IncidentEngineExpressionParseOpts    `tfsdk:"parse"`

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
	}
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

func ExpressionsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
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
								MarkdownDescription: "Indicates which operation type to execute",
								Required:            true,
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
	arrayValue := []client.EngineParamBindingValuePayloadV2{}

	for _, v := range binding.ArrayValue {
		arrayValue = append(arrayValue, v.ToPayload())
	}

	var value *client.EngineParamBindingValuePayloadV2
	if binding.Value != nil {
		value = lo.ToPtr(binding.Value.ToPayload())
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
