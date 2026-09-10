package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// V2 does not return disabled, so FromAPI has to keep the planned value (or settle as
// null when the plan was unknown) rather than inventing one.
func TestAlertSourceDisabledFromAPI(t *testing.T) {
	source := client.AlertSourceV2{
		Id:         "01SOURCE",
		Name:       "Nightly backup",
		SourceType: client.AlertSourceV2SourceTypeHeartbeat,
	}

	t.Run("keeps a known planned value", func(t *testing.T) {
		got := AlertSourceResourceModel{}.FromAPIWithPlan(source, &AlertSourceResourceModel{
			Disabled: types.BoolValue(true),
		})
		if !got.Disabled.ValueBool() {
			t.Errorf("planned disabled = true should be kept, got %v", got.Disabled)
		}
	})

	t.Run("settles unknown as null", func(t *testing.T) {
		got := AlertSourceResourceModel{}.FromAPIWithPlan(source, &AlertSourceResourceModel{
			Disabled: types.BoolUnknown(),
		})
		if got.Disabled.IsUnknown() {
			t.Error("an unknown planned value must not be written into state")
		}
		if !got.Disabled.IsNull() {
			t.Errorf("it should settle as null, got %v", got.Disabled)
		}
	})

	t.Run("import with no plan is null", func(t *testing.T) {
		got := AlertSourceResourceModel{}.FromAPI(source)
		if !got.Disabled.IsNull() {
			t.Errorf("import should leave disabled unset, got %v", got.Disabled)
		}
	})
}
