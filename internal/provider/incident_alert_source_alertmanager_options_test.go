package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// TestAlertSourceFromAPIAlertmanagerOptions covers the options only reaching state when the
// config manages them. A source whose options were set in the dashboard, and whose config has no
// block, must read back null: anything else is drift against a plan that never set them, and an
// apply that would then send the stored value back as if Terraform owned it.
func TestAlertSourceFromAPIAlertmanagerOptions(t *testing.T) {
	source := alertSourceV3("alertmanager")
	source.AlertmanagerOptions = &client.AlertSourceAlertmanagerOptionsV3{OneAlertPerGroup: false}

	t.Run("keeps them out of state when the config has no block", func(t *testing.T) {
		model := fromAPI(t, source, &alertSourceModel{})

		if model.AlertmanagerOptions != nil {
			t.Errorf("alertmanager_options should be null when unmanaged, got %+v", model.AlertmanagerOptions)
		}
	})

	t.Run("reads them back when the config has the block", func(t *testing.T) {
		model := fromAPI(t, source, &alertSourceModel{
			AlertmanagerOptions: &alertSourceAlertmanagerOptions{OneAlertPerGroup: types.BoolValue(false)},
		})

		if model.AlertmanagerOptions == nil {
			t.Fatal("alertmanager_options should be read back when managed")
		}
		if got := model.AlertmanagerOptions.OneAlertPerGroup; got.IsNull() || got.ValueBool() {
			t.Errorf("one_alert_per_group should be false, got %v", got)
		}
	})

	t.Run("reads null when the API returns none", func(t *testing.T) {
		model := fromAPI(t, alertSourceV3("alertmanager"), &alertSourceModel{
			AlertmanagerOptions: &alertSourceAlertmanagerOptions{OneAlertPerGroup: types.BoolValue(true)},
		})

		if model.AlertmanagerOptions != nil {
			t.Errorf("alertmanager_options should be null without API options, got %+v", model.AlertmanagerOptions)
		}
	})
}

// TestAlertmanagerOptionsPayload covers an absent block sending nothing, which leaves the stored
// options alone. Sending a default instead would undo a value set in the dashboard on the first
// apply after upgrading the provider.
func TestAlertmanagerOptionsPayload(t *testing.T) {
	var absent *alertSourceAlertmanagerOptions
	if payload := absent.toPayload(); payload != nil {
		t.Errorf("an absent block should send nothing, got %+v", payload)
	}

	set := &alertSourceAlertmanagerOptions{OneAlertPerGroup: types.BoolValue(false)}
	payload := set.toPayload()
	if payload == nil || payload.OneAlertPerGroup {
		t.Errorf("a set block should send one_alert_per_group = false, got %+v", payload)
	}
}

func TestAlertSourceValidateAlertmanagerOptions(t *testing.T) {
	options := objectWith(t, attributeType(t, "alertmanager_options"), map[string]tftypes.Value{
		"one_alert_per_group": tftypes.NewValue(tftypes.Bool, false),
	})

	t.Run("rejects them on another source type", func(t *testing.T) {
		diags := validateAlertSource(t, alertSourceConfig(t, map[string]tftypes.Value{
			"source_type":          stringValue("http"),
			"alertmanager_options": options,
		}))

		assertErrorContaining(t, diags, "alertmanager_options is only for alertmanager alert sources")
	})

	t.Run("does not require them for an alertmanager source", func(t *testing.T) {
		diags := validateAlertSource(t, alertSourceConfig(t, map[string]tftypes.Value{
			"source_type": stringValue("alertmanager"),
		}))

		assertNoErrorContaining(t, diags, "alertmanager_options")
	})

	t.Run("accepts them on an alertmanager source", func(t *testing.T) {
		diags := validateAlertSource(t, alertSourceConfig(t, map[string]tftypes.Value{
			"source_type":          stringValue("alertmanager"),
			"alertmanager_options": options,
		}))

		assertNoErrorContaining(t, diags, "alertmanager_options")
	})
}
