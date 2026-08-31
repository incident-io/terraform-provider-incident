package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// TestWorkflowBuildModelReconcilesConditionOperation covers ONC-12602 for
// workflows, which carry condition groups just like alert routes do.
func TestWorkflowBuildModelReconcilesConditionOperation(t *testing.T) {
	workflow := client.WorkflowV2{
		Id:   "01ABC",
		Name: "workflow",
		ConditionGroups: []client.ConditionGroupV2{
			{
				Conditions: []client.ConditionV2{
					{
						Subject:   client.ConditionSubjectV2{Reference: "incident.custom_field.01GH"},
						Operation: client.ConditionOperationV2{Value: "contains_one_of"},
					},
				},
			},
		},
	}

	prior := &IncidentWorkflowResourceModel{
		ConditionGroups: models.IncidentEngineConditionGroups{
			{
				Conditions: models.IncidentEngineConditions{
					{
						Subject:   types.StringValue("incident.custom_field.01GH"),
						Operation: types.StringValue("one_of"),
					},
				},
			},
		},
	}

	r := &IncidentWorkflowResource{}

	withPrior := r.buildModel(workflow, prior)
	assert.Equal(t, "one_of", withPrior.ConditionGroups[0].Conditions[0].Operation.ValueString(),
		"with a plan the operation should be reconciled")

	noPrior := r.buildModel(workflow, nil)
	assert.Equal(t, "contains_one_of", noPrior.ConditionGroups[0].Conditions[0].Operation.ValueString(),
		"with no plan the operation should stay verbatim")
}
