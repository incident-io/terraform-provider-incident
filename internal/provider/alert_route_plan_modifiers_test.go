package provider

import "testing"

// TestWhenAlertJoinsGroupAction covers the mode-aware planning decision for the
// v3-only escalation_config.when_alert_joins_group attribute.
func TestWhenAlertJoinsGroupAction(t *testing.T) {
	cases := []struct {
		name                                                                     string
		configNull, planV3, planKnown, groupingEnabled, groupingKnown, stateNull bool
		want                                                                     computedPlanAction
	}{
		{
			// The reported bug: v2 route, computed field null in state, an
			// unrelated edit triggers an update. Must plan null, not unknown.
			name:       "v2 mode plans null",
			configNull: true, planV3: false, planKnown: true, stateNull: true,
			want: planActionSetNull,
		},
		{
			name:       "v3 grouping disabled plans null",
			configNull: true, planV3: true, planKnown: true, groupingEnabled: false, groupingKnown: true, stateNull: true,
			want: planActionSetNull,
		},
		{
			name:       "v3 grouping enabled steady state uses state",
			configNull: true, planV3: true, planKnown: true, groupingEnabled: true, groupingKnown: true, stateNull: false,
			want: planActionUseState,
		},
		{
			// Migrating v2 -> v3 with grouping enabled: prior state is null, so let
			// the API compute the default rather than pinning it.
			name:       "v3 grouping enabled entering leaves unknown",
			configNull: true, planV3: true, planKnown: true, groupingEnabled: true, groupingKnown: true, stateNull: true,
			want: planActionNone,
		},
		{
			name:       "explicit config value is respected",
			configNull: false, planV3: false, planKnown: true, stateNull: true,
			want: planActionNone,
		},
		{
			name:       "unknown mode leaves unknown",
			configNull: true, planV3: false, planKnown: false, stateNull: true,
			want: planActionNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := whenAlertJoinsGroupAction(tc.configNull, tc.planV3, tc.planKnown, tc.groupingEnabled, tc.groupingKnown, tc.stateNull)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAutoRelateGroupedAlertsAction covers the mode-aware planning decision for
// the v2-only incident_config.auto_relate_grouped_alerts attribute.
func TestAutoRelateGroupedAlertsAction(t *testing.T) {
	cases := []struct {
		name                                     string
		configNull, planV3, planKnown, stateNull bool
		want                                     computedPlanAction
	}{
		{
			// v2 -> v3 migration: the v2-only field must plan null in v3.
			name:       "v3 mode plans null",
			configNull: true, planV3: true, planKnown: true, stateNull: false,
			want: planActionSetNull,
		},
		{
			name:       "v2 steady state uses state",
			configNull: true, planV3: false, planKnown: true, stateNull: false,
			want: planActionUseState,
		},
		{
			name:       "v2 with null state leaves unknown",
			configNull: true, planV3: false, planKnown: true, stateNull: true,
			want: planActionNone,
		},
		{
			name:       "explicit config value is respected",
			configNull: false, planV3: true, planKnown: true, stateNull: true,
			want: planActionNone,
		},
		{
			name:       "unknown mode leaves unknown",
			configNull: true, planV3: false, planKnown: false, stateNull: true,
			want: planActionNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := autoRelateGroupedAlertsAction(tc.configNull, tc.planV3, tc.planKnown, tc.stateNull)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
