package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
)

// The V2 resource has no acceptance case setting fixed_team_id — it needs a team, and the
// organisation's team alert attribute configured — so these are the only cover its mapping has.
func TestAlertSourceFixedTeamID(t *testing.T) {
	t.Run("reads a fixed team", func(t *testing.T) {
		if got := FixedTeamIDFromAPI(lo.ToPtr("01TEAM")).ValueString(); got != "01TEAM" {
			t.Errorf("unexpected fixed team %q", got)
		}
	})

	// Both spellings of "not fixed" have to read back as null, or they diff against a config
	// that never set the field and fail the apply as an inconsistent result.
	t.Run("reads absent and empty as null", func(t *testing.T) {
		if got := FixedTeamIDFromAPI(nil); !got.IsNull() {
			t.Errorf("absent should read as null, got %v", got)
		}

		if got := FixedTeamIDFromAPI(lo.ToPtr("")); !got.IsNull() {
			t.Errorf("empty should read as null, got %v", got)
		}
	})

	// Update always sends the field, because the API reads an omission as "leave the stored team
	// alone" and the value would otherwise be unclearable from HCL.
	t.Run("update sends an empty string when the config has no value", func(t *testing.T) {
		payload := FixedTeamIDUpdatePayload(types.StringNull())
		if payload == nil {
			t.Fatal("update should always send the field")
		}
		if *payload != "" {
			t.Errorf("expected an empty string, got %q", *payload)
		}
	})

	t.Run("update sends the configured team", func(t *testing.T) {
		payload := FixedTeamIDUpdatePayload(types.StringValue("01TEAM"))
		if payload == nil || *payload != "01TEAM" {
			t.Errorf("expected 01TEAM, got %v", payload)
		}
	})
}
