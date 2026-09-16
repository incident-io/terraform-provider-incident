package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// The alert route schema has two Optional+Computed attributes that belong to
// only one of the two schemas: escalation_config.when_alert_joins_group (v3
// only) and incident_config.auto_relate_grouped_alerts (v2 only). The stock
// UseStateForUnknown plan modifier does nothing when the prior state is null,
// so in the mode where the attribute doesn't apply (and is therefore always
// null) it stays Computed-unknown and shows up as "(known after apply)" on
// every change. It also can't reconcile a v2<->v3 migration, where the prior
// state value contradicts what the API will return in the new mode.
//
// These mode-aware modifiers fix both: they plan a concrete null when the
// attribute doesn't apply to the planned mode, and otherwise fall back to
// UseStateForUnknown semantics (carry a known prior-state value, else leave it
// unknown for the API to compute).

// attrGetter is satisfied by both tfsdk.Plan and tfsdk.State.
type attrGetter interface {
	GetAttribute(ctx context.Context, p path.Path, target interface{}) diag.Diagnostics
}

// alertRoutePlanIsV3 reports whether the given plan/state is in v3 mode (the
// top-level grouping_config block is set) and whether that is known.
func alertRoutePlanIsV3(ctx context.Context, g attrGetter) (isV3 bool, known bool) {
	var groupingConfig types.Object
	if d := g.GetAttribute(ctx, path.Root("grouping_config"), &groupingConfig); d.HasError() {
		return false, false
	}
	if groupingConfig.IsUnknown() {
		return false, false
	}
	return !groupingConfig.IsNull(), true
}

// alertRoutePlanEnablesGrouping reports whether the plan turns route grouping on, or leaves
// that unknown. Only meaningful in v3 mode, where the grouping_config block exists.
func alertRoutePlanEnablesGrouping(ctx context.Context, g attrGetter) bool {
	var enabled types.Bool
	if d := g.GetAttribute(ctx, path.Root("grouping_config").AtName("default").AtName("enabled"), &enabled); d.HasError() {
		return true
	}
	return enabled.IsUnknown() || enabled.ValueBool()
}

// computedPlanAction is the decision made for a mode-specific Optional+Computed
// attribute whose config value is absent.
type computedPlanAction int

const (
	// planActionNone leaves the framework's default in place (the config value
	// when set, otherwise Computed-unknown).
	planActionNone computedPlanAction = iota
	// planActionSetNull plans a concrete null (the attribute doesn't apply).
	planActionSetNull
	// planActionUseState carries the prior state value (UseStateForUnknown).
	planActionUseState
)

// whenAlertJoinsGroupAction decides how to plan when_alert_joins_group, a v3-only
// attribute: null in v2 mode, prior state in v3, unknown when entering v3 or creating.
//
// Grouping being off does not null it: a team preference can group the route's alerts, so
// the API returns the mode for every v3 route once team grouping is on. Until then it
// returns none for a route that does not group, and such a route with a null state stays
// null rather than planning unknown on every run; the refresh before the next plan picks
// the mode up when the API sends it. Turning grouping on makes the API return a mode, so
// that plan leaves it unknown for the API to fill.
func whenAlertJoinsGroupAction(configNull, planV3, planKnown, stateV3, stateNull, planEnablesGrouping bool) computedPlanAction {
	if !configNull {
		return planActionNone // respect an explicit config value
	}
	if !planKnown {
		return planActionNone // mode unknown: leave computed-unknown
	}
	if !planV3 {
		return planActionSetNull // v2 mode: always null
	}
	if !stateNull {
		return planActionUseState // steady state: don't churn the server default
	}
	if stateV3 && !planEnablesGrouping {
		return planActionSetNull // v3 route the API returned no mode for, still not grouping: stay null
	}
	return planActionNone // entering v3, creating, or turning grouping on: let the API compute it
}

// autoRelateGroupedAlertsAction decides how to plan auto_relate_grouped_alerts
// (a v2-only attribute): null in v3 mode, otherwise UseStateForUnknown.
func autoRelateGroupedAlertsAction(configNull, planV3, planKnown, stateNull bool) computedPlanAction {
	if !configNull {
		return planActionNone
	}
	if !planKnown {
		return planActionNone
	}
	if planV3 {
		return planActionSetNull // v2-only field: null in v3 mode
	}
	if !stateNull {
		return planActionUseState
	}
	return planActionNone
}

// whenAlertJoinsGroupPlanModifier is a mode-aware plan modifier for
// escalation_config.when_alert_joins_group.
type whenAlertJoinsGroupPlanModifier struct{}

func (whenAlertJoinsGroupPlanModifier) Description(context.Context) string {
	return "Keeps the prior value of when_alert_joins_group, or plans null when the route has no grouping_config block. A route that is new, has just gained a grouping_config block, or is turning grouping on leaves it for the API to fill."
}

func (m whenAlertJoinsGroupPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (whenAlertJoinsGroupPlanModifier) PlanModifyObject(ctx context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	planV3, planKnown := alertRoutePlanIsV3(ctx, req.Plan)
	stateV3, _ := alertRoutePlanIsV3(ctx, req.State)
	planEnablesGrouping := planV3 && alertRoutePlanEnablesGrouping(ctx, req.Plan)

	switch whenAlertJoinsGroupAction(req.ConfigValue.IsNull(), planV3, planKnown, stateV3, req.StateValue.IsNull(), planEnablesGrouping) {
	case planActionSetNull:
		resp.PlanValue = types.ObjectNull(models.WhenAlertJoinsGroupAttrTypes())
	case planActionUseState:
		resp.PlanValue = req.StateValue
	case planActionNone:
	}
}

// autoRelateGroupedAlertsPlanModifier is a mode-aware plan modifier for
// incident_config.auto_relate_grouped_alerts.
type autoRelateGroupedAlertsPlanModifier struct{}

func (autoRelateGroupedAlertsPlanModifier) Description(context.Context) string {
	return "Plans auto_relate_grouped_alerts as null when grouping_config is set, otherwise uses prior state when known."
}

func (m autoRelateGroupedAlertsPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (autoRelateGroupedAlertsPlanModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	planV3, planKnown := alertRoutePlanIsV3(ctx, req.Plan)

	switch autoRelateGroupedAlertsAction(req.ConfigValue.IsNull(), planV3, planKnown, req.StateValue.IsNull()) {
	case planActionSetNull:
		resp.PlanValue = types.BoolNull()
	case planActionUseState:
		resp.PlanValue = req.StateValue
	case planActionNone:
	}
}
