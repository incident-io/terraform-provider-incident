package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// A `moved` block from incident_team_grouping_preference carries the object across as a
// team's preference, so the plan after it updates nothing.
func TestGroupingPreferenceMovesFromTheTeamResource(t *testing.T) {
	ctx := t.Context()
	source := NewIncidentTeamGroupingPreferenceResource()
	sourceSchema := declaredResourceSchema(ctx, source)
	targetSchema := groupingPreferenceResourceSchema(ctx)

	movers := (&IncidentAlertGroupingPreferenceResource{}).MoveState(ctx)
	require.Len(t, movers, 1)

	old := models.TeamGroupingPreferenceResourceModel{
		ID:                types.StringValue("01TEAMPREF"),
		UnlockInDashboard: types.BoolValue(true),
		TeamID:            types.StringValue("01TEAM"),
		Version:           types.Int64Value(3),
		Default:           &models.TeamGroupingPreferenceBranchModel{Settings: groupingOffSettings()},
	}
	sourceState := tfsdk.State{Schema: sourceSchema, Raw: tftypes.NewValue(sourceSchema.Type().TerraformType(ctx), nil)}
	require.False(t, sourceState.Set(ctx, &old).HasError())

	resp := moveStateResponse(ctx, t, targetSchema)
	movers[0].StateMover(ctx, resource.MoveStateRequest{
		SourceTypeName:      resourceTypeName(ctx, source),
		SourceSchemaVersion: sourceSchema.Version,
		SourceState:         &sourceState,
	}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

	var moved models.AlertGroupingPreferenceResourceModel
	require.False(t, resp.TargetState.Get(ctx, &moved).HasError())
	assert.Equal(t, types.StringValue(models.GroupingKindTeam), moved.Kind)
	assert.Equal(t, old.ID, moved.ID)
	assert.Equal(t, old.TeamID, moved.TeamID)
	assert.Equal(t, old.Version, moved.Version)
	assert.Equal(t, old.UnlockInDashboard, moved.UnlockInDashboard)
	assert.Equal(t, old.Default, moved.Default)

	t.Run("skips any other source", func(t *testing.T) {
		resp := moveStateResponse(ctx, t, targetSchema)
		movers[0].StateMover(ctx, resource.MoveStateRequest{
			SourceTypeName:      "incident_alert_route",
			SourceSchemaVersion: sourceSchema.Version,
			SourceState:         &sourceState,
		}, &resp)
		assert.Empty(t, resp.Diagnostics)
		assert.True(t, resp.TargetState.Raw.IsNull())
	})
}

// TestAccIncidentGroupingPreferenceMovedFromTeamResource moves a preference from the
// resource this one replaces with a `moved` block, and expects the move to plan no change.
// It writes only a team's default, which the API allows whatever the organisation routes
// alerts with.
func TestAccIncidentGroupingPreferenceMovedFromTeamResource(t *testing.T) {
	preferenceID := statecheck.CompareValue(compare.ValuesSame())

	tfresource.Test(t, tfresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []tfresource.TestStep{
			{
				Config: testAccGroupingPreferenceMoveConfig(`
resource "incident_team_grouping_preference" "moving" {
  team_id = incident_catalog_entry.team.id
  default = { settings = { enabled = false } }
}
`),
				ConfigStateChecks: []statecheck.StateCheck{
					preferenceID.AddStateValue("incident_team_grouping_preference.moving", tfjsonpath.New("id")),
				},
			},
			{
				Config: testAccGroupingPreferenceMoveConfig(`
resource "incident_alert_grouping_preference" "moving" {
  kind    = "team"
  team_id = incident_catalog_entry.team.id
  default = { settings = { enabled = false } }
}

moved {
  from = incident_team_grouping_preference.moving
  to   = incident_alert_grouping_preference.moving
}
`),
				ConfigPlanChecks: tfresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("incident_alert_grouping_preference.moving", plancheck.ResourceActionNoop),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					preferenceID.AddStateValue("incident_alert_grouping_preference.moving", tfjsonpath.New("id")),
				},
			},
		},
	})
}

func testAccGroupingPreferenceMoveConfig(preference string) string {
	return testRunTemplate("incident_alert_grouping_preference_move", `
data "incident_catalog_type" "team" {
  name = {{ .TeamTypeName | quote }}
}

resource "incident_catalog_entry" "team" {
  catalog_type_id    = data.incident_catalog_type.team.id
  external_id        = {{ .TeamName | quote }}
  name               = {{ .TeamName | quote }}
  attribute_values   = []
  managed_attributes = []
}
`, struct {
		TeamTypeName string
		TeamName     string
	}{
		TeamTypeName: teamTypeName(),
		TeamName:     StableSuffix("tf-acceptance-test-grouping-move-team"),
	}) + preference
}
