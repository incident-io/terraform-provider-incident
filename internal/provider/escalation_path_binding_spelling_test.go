package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/jsontypes"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// A path is held as a types.List, so keeping a condition's binding spelling means decoding the
// prior state back into nodes. This covers that decode and the reconciliation it feeds: without
// either, a config writing `value_literal` under if_else fails the apply as an inconsistent
// result.
func TestEscalationPathKeepsAConditionsBindingSpelling(t *testing.T) {
	ctx := context.Background()
	r := &IncidentEscalationPathResource{}
	var diags diag.Diagnostics

	ep := client.EscalationPathV2{
		Id:   "01ABC",
		Name: "Paged by severity",
		Path: []client.EscalationPathNodeV2{
			{
				Id:   "node-if-else",
				Type: client.EscalationPathNodeV2TypeIfElse,
				IfElse: &client.EscalationPathNodeIfElseV2{
					Conditions: []client.ConditionV2{
						{
							Subject:   client.ConditionSubjectV2{Reference: "incident.severity"},
							Operation: client.ConditionOperationV2{Value: "is"},
							ParamBindings: []client.EngineParamBindingV2{
								{Value: &client.EngineParamBindingValueV2{Literal: lo.ToPtr("high")}},
							},
						},
					},
					ThenPath: []client.EscalationPathNodeV2{{
						Id:    "node-then",
						Type:  client.EscalationPathNodeV2TypeLevel,
						Level: &client.EscalationPathNodeLevelV2{},
					}},
					ElsePath: []client.EscalationPathNodeV2{{
						Id:    "node-else",
						Type:  client.EscalationPathNodeV2TypeLevel,
						Level: &client.EscalationPathNodeLevelV2{},
					}},
				},
			},
		},
	}

	onImport := r.buildModel(ctx, ep, nil, &diags)
	require.False(t, diags.HasError(), "buildModel: %#v", diags)

	nodes := priorPathNodes(ctx, onImport.Path, pathSchemaDepth)
	require.Len(t, nodes, 1, "the prior state has to decode back into nodes")
	require.NotNil(t, nodes[0].IfElse)
	require.Len(t, nodes[0].IfElse.Conditions, 1)

	// Stand in for a config that wrote the shorthand rather than the long form.
	nodes[0].IfElse.Conditions[0].ParamBindings = models.IncidentEngineParamBindings{
		{ValueLiteral: jsontypes.NewNormalizedJSONOrStringValue("high")},
	}
	prior := &IncidentEscalationPathResourceModel{
		Path: types.ListValueMust(
			types.ObjectType{AttrTypes: nodeAttrTypes(pathSchemaDepth)},
			lo.Map(nodes, func(node IncidentEscalationPathNode, _ int) attr.Value {
				return nodeToObject(ctx, node, pathSchemaDepth, &diags)
			}),
		),
	}

	reread := priorPathNodes(ctx, r.buildModel(ctx, ep, prior, &diags).Path, pathSchemaDepth)
	require.False(t, diags.HasError(), "buildModel with a prior: %#v", diags)
	require.Len(t, reread, 1)

	binding := reread[0].IfElse.Conditions[0].ParamBindings[0]
	assert.Equal(t, "high", binding.ValueLiteral.ValueString(), "should keep the shorthand")
	assert.Nil(t, binding.Value, "and not also report the long form")
}
