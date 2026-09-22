package provider

import (
	"fmt"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/timestamptypes"
)

// oneOffRule builds a one-off rule over a window written as two dates in December 2026,
// which is enough to say which rules are in each other's way.
func oneOffRule(name string, startDay, endDay int, rateCents int64) models.PayConfigOneOffRuleModel {
	day := func(of int) timestamptypes.Instant {
		return timestamptypes.NewInstantStringValue(fmt.Sprintf("2026-12-%02dT00:00:00Z", of))
	}

	return models.PayConfigOneOffRuleModel{
		ID:        types.StringValue("01" + name),
		Name:      types.StringValue(name),
		StartAt:   day(startDay),
		EndAt:     day(endDay),
		RateCents: types.Int64Value(rateCents),
	}
}

// TestPayConfigOneOffUpdateOrder covers the order the one-off rules are written in. The
// API rejects a rule that overlaps another on every write, not only once the config is
// whole, so a rule can only move when the window it is moving to is already clear.
func TestPayConfigOneOffUpdateOrder(t *testing.T) {
	for _, tc := range []struct {
		name             string
		current, planned []models.PayConfigOneOffRuleModel
		order            []int
		ok               bool
	}{
		{
			name:    "nothing changed",
			current: []models.PayConfigOneOffRuleModel{oneOffRule("first", 1, 5, 100), oneOffRule("second", 10, 15, 200)},
			planned: []models.PayConfigOneOffRuleModel{oneOffRule("first", 1, 5, 100), oneOffRule("second", 10, 15, 200)},
			order:   []int{},
			ok:      true,
		},
		{
			name:    "a rate change needs no room",
			current: []models.PayConfigOneOffRuleModel{oneOffRule("first", 1, 5, 100), oneOffRule("second", 10, 15, 200)},
			planned: []models.PayConfigOneOffRuleModel{oneOffRule("first", 1, 5, 100), oneOffRule("second", 10, 15, 300)},
			order:   []int{1},
			ok:      true,
		},
		{
			name:    "windows that are in nobody's way keep their position order",
			current: []models.PayConfigOneOffRuleModel{oneOffRule("first", 1, 5, 100), oneOffRule("second", 10, 15, 200)},
			planned: []models.PayConfigOneOffRuleModel{oneOffRule("first", 2, 6, 100), oneOffRule("second", 11, 16, 200)},
			order:   []int{0, 1},
			ok:      true,
		},
		{
			// The bug this ordering exists for: position 0 moves onto a window position 1
			// is itself about to leave, so position 1 has to be written first.
			name:    "a rule waits for the one whose window it is taking",
			current: []models.PayConfigOneOffRuleModel{oneOffRule("first", 1, 5, 100), oneOffRule("second", 10, 15, 200)},
			planned: []models.PayConfigOneOffRuleModel{oneOffRule("first", 11, 14, 100), oneOffRule("second", 20, 25, 200)},
			order:   []int{1, 0},
			ok:      true,
		},
		{
			name:    "an exchange has no order",
			current: []models.PayConfigOneOffRuleModel{oneOffRule("first", 1, 5, 100), oneOffRule("second", 10, 15, 200)},
			planned: []models.PayConfigOneOffRuleModel{oneOffRule("first", 11, 14, 100), oneOffRule("second", 2, 4, 200)},
			ok:      false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order, ok := oneOffUpdateOrder(tc.planned, tc.current)
			if ok != tc.ok {
				t.Fatalf("oneOffUpdateOrder ok = %v, want %v (order %v)", ok, tc.ok, order)
			}
			if !tc.ok {
				return
			}

			if !slices.Equal(order, tc.order) {
				t.Errorf("oneOffUpdateOrder = %v, want %v", order, tc.order)
			}
		})
	}
}
