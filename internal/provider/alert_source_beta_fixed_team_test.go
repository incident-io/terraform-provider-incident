package provider

import (
	"testing"

	"github.com/samber/lo"
)

// The mapping helpers are covered in models, alongside the V2 resource that shares them. What
// this adds is that the projection actually wires them up: a fixed team reaching state, and
// "not fixed" reading back as null rather than diffing against a config that never set it.
func TestAlertSourceBetaFixedTeamID(t *testing.T) {
	t.Run("reads a fixed team", func(t *testing.T) {
		source := alertSourceV3("http")
		source.FixedTeamId = lo.ToPtr("01TEAM")

		model := fromAPI(t, source, &alertSourceBetaModel{})
		if got := model.FixedTeamID.ValueString(); got != "01TEAM" {
			t.Errorf("unexpected fixed team %q", got)
		}
	})

	t.Run("reads an absent field as null", func(t *testing.T) {
		model := fromAPI(t, alertSourceV3("http"), &alertSourceBetaModel{})
		if !model.FixedTeamID.IsNull() {
			t.Errorf("expected null, got %v", model.FixedTeamID)
		}
	})
}
