package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// filter_condition_groups is a plain Optional (not Computed) attribute, so Terraform's plan for
// it is null whenever config omits it, and the post-apply value has to match. These cover the
// write-side mapping that would otherwise fail that consistency check: sending nil here, rather
// than an empty slice, would let the API's real (non-null) answer diverge from a null plan.
func TestAlertSourceFilterConditionGroupsToPayloadPtr(t *testing.T) {
	t.Run("sends an empty slice, never nil, when the config has no filters", func(t *testing.T) {
		payload := IncidentEngineConditionGroups(nil).ToPayloadPtr()
		if payload == nil {
			t.Fatal("expected a non-nil payload, got nil")
		}
		if len(*payload) != 0 {
			t.Errorf("expected an empty slice, got %+v", *payload)
		}
	})

	t.Run("sends the configured filters", func(t *testing.T) {
		groups := IncidentEngineConditionGroups{
			{Conditions: IncidentEngineConditions{
				{
					Subject:   types.StringValue(`expressions["severity_expr"]`),
					Operation: types.StringValue("is_set"),
				},
			}},
		}

		payload := groups.ToPayloadPtr()
		if payload == nil || len(*payload) != 1 {
			t.Fatalf("unexpected payload %+v", payload)
		}
		if got := (*payload)[0].Conditions[0].Subject; got != `expressions["severity_expr"]` {
			t.Errorf("unexpected subject %q", got)
		}
	})
}
