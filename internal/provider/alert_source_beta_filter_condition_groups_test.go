package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// The drift harness in examples/experiment is what proves the round-trip end to end. These cover
// the three mappings it would fail on, so a break points at the direction that caused it.
func TestAlertSourceBetaFilterConditionGroups(t *testing.T) {
	t.Run("reads filter conditions", func(t *testing.T) {
		source := alertSourceV3("http")
		source.FilterConditionGroups = &[]client.ConditionGroupPayloadV3{
			{Conditions: []client.ConditionPayloadV3{
				{
					Subject:       `expressions["severity_expr"]`,
					Operation:     "is_set",
					ParamBindings: []client.EngineParamBindingPayloadV3{},
				},
			}},
		}

		model := fromAPI(t, source, &alertSourceBetaModel{})
		if len(model.FilterConditionGroups) != 1 {
			t.Fatalf("expected 1 filter condition group, got %d", len(model.FilterConditionGroups))
		}

		conditions := model.FilterConditionGroups[0].Conditions
		if len(conditions) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(conditions))
		}
		if got := conditions[0].Subject.ValueString(); got != `expressions["severity_expr"]` {
			t.Errorf("unexpected subject %q", got)
		}
		if got := conditions[0].Operation.ValueString(); got != "is_set" {
			t.Errorf("unexpected operation %q", got)
		}
	})

	// The API omits the field whenever the source has no filters, so this has to read back as
	// no blocks, or it would diff against a config that never set the attribute and fail the
	// apply as an inconsistent result.
	t.Run("reads an absent field as no blocks", func(t *testing.T) {
		model := fromAPI(t, alertSourceV3("http"), &alertSourceBetaModel{})
		if model.FilterConditionGroups != nil {
			t.Errorf("expected nil, got %+v", model.FilterConditionGroups)
		}
	})

	// Nil leaves any stored filters alone, matching a config that omits the attribute: the API
	// reads an omitted field as "leave the stored filters alone", so removing the attribute from
	// HCL doesn't clear filters set elsewhere (e.g. the dashboard).
	t.Run("update sends nothing when the config has no blocks", func(t *testing.T) {
		if payload := filterConditionGroupsToPayload(nil); payload != nil {
			t.Errorf("expected no payload, got %+v", payload)
		}
	})

	// An explicit empty list is how a config clears filters, so it has to send a non-nil empty
	// slice rather than being treated the same as omitting the attribute.
	t.Run("update sends an empty slice to clear filters", func(t *testing.T) {
		payload := filterConditionGroupsToPayload(models.IncidentEngineConditionGroups{})
		if payload == nil {
			t.Fatal("expected a payload, got nil")
		}
		if len(*payload) != 0 {
			t.Errorf("expected an empty slice, got %+v", *payload)
		}
	})

	t.Run("update sends the configured filters", func(t *testing.T) {
		groups := models.IncidentEngineConditionGroups{
			{Conditions: models.IncidentEngineConditions{
				{
					Subject:   types.StringValue(`expressions["severity_expr"]`),
					Operation: types.StringValue("is_set"),
				},
			}},
		}

		payload := filterConditionGroupsToPayload(groups)
		if payload == nil || len(*payload) != 1 {
			t.Fatalf("unexpected payload %+v", payload)
		}
		if got := (*payload)[0].Conditions[0].Subject; got != `expressions["severity_expr"]` {
			t.Errorf("unexpected subject %q", got)
		}
	})

	// Create differs from update in intent, though not in code: a new source has nothing to
	// clear, so sending nothing just means no filters rather than needing an explicit empty list.
	t.Run("create sends nothing when the config has no blocks", func(t *testing.T) {
		if payload := filterConditionGroupsToPayload(nil); payload != nil {
			t.Errorf("expected no payload, got %+v", payload)
		}
	})
}
