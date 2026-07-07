package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/provider/models"
)

// ValidateConfig enforces the relationships the static schema can't, and which
// differ between the v2 and v3 alert-route schemas. The merged schema makes
// most mode-specific attributes Optional; this restores the requiredness and
// mutual-exclusion rules per mode so users get plan-time errors rather than
// opaque API rejections (422s) or perpetual diffs.
//
// Mode is determined by the presence of the top-level grouping_config block.
func (r *IncidentAlertRouteResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	addErr := func(p path.Path, summary, detail string) {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(p, summary, detail))
	}

	// Helpers for presence checks. A value is "set" when it is neither null nor
	// unknown; unknown values (e.g. derived from another resource) are skipped to
	// avoid over-validating during planning.
	objectSet := func(p path.Path) bool {
		var v types.Object
		if d := req.Config.GetAttribute(ctx, p, &v); d.HasError() {
			return false
		}
		return !v.IsNull() && !v.IsUnknown()
	}
	boolSet := func(p path.Path) bool {
		var v types.Bool
		if d := req.Config.GetAttribute(ctx, p, &v); d.HasError() {
			return false
		}
		return !v.IsNull() && !v.IsUnknown()
	}
	int64Set := func(p path.Path) bool {
		var v types.Int64
		if d := req.Config.GetAttribute(ctx, p, &v); d.HasError() {
			return false
		}
		return !v.IsNull() && !v.IsUnknown()
	}
	// setPresent reports that a set attribute is configured: non-null and known,
	// regardless of how many elements it holds. Mutual-exclusion checks use this
	// (rather than a non-empty check) so an explicit empty set — e.g.
	// `channel_config = []` — still counts as using the attribute. Otherwise
	// validation would pass, apply would ignore it, and the config/state
	// mismatch would produce a perpetual diff.
	setPresent := func(p path.Path) bool {
		var v types.Set
		if d := req.Config.GetAttribute(ctx, p, &v); d.HasError() {
			return false
		}
		return !v.IsNull() && !v.IsUnknown()
	}
	// The *Missing helpers report that an attribute is definitively absent: known
	// to be null (not merely unknown). Required-attribute checks use these so we
	// don't flag a value that is unknown at plan time (e.g. derived from a
	// variable or another resource) and may well be present at apply.
	boolMissing := func(p path.Path) bool {
		var v types.Bool
		if d := req.Config.GetAttribute(ctx, p, &v); d.HasError() {
			return false
		}
		return !v.IsUnknown() && v.IsNull()
	}
	int64Missing := func(p path.Path) bool {
		var v types.Int64
		if d := req.Config.GetAttribute(ctx, p, &v); d.HasError() {
			return false
		}
		return !v.IsUnknown() && v.IsNull()
	}
	objectMissing := func(p path.Path) bool {
		var v types.Object
		if d := req.Config.GetAttribute(ctx, p, &v); d.HasError() {
			return false
		}
		return !v.IsUnknown() && v.IsNull()
	}
	// boolValue returns (value, known). known is false when the attribute is
	// null or unknown.
	boolValue := func(p path.Path) (bool, bool) {
		var v types.Bool
		if d := req.Config.GetAttribute(ctx, p, &v); d.HasError() || v.IsNull() || v.IsUnknown() {
			return false, false
		}
		return v.ValueBool(), true
	}

	groupingBase := path.Root("grouping_config")
	isV3 := objectSet(groupingBase)

	escalationBase := path.Root("escalation_config")
	incidentBase := path.Root("incident_config")

	if isV3 {
		// v2-only attributes must not be set in v3 mode.
		if setPresent(path.Root("channel_config")) {
			addErr(path.Root("channel_config"), "Invalid attribute combination",
				"`channel_config` can't be used together with `grouping_config`. Use `message_config.destinations` instead.")
		}
		if objectSet(path.Root("message_template")) {
			addErr(path.Root("message_template"), "Invalid attribute combination",
				"`message_template` can't be used together with `grouping_config`. Use `message_config.template` instead.")
		}
		if objectSet(path.Root("incident_template")) {
			addErr(path.Root("incident_template"), "Invalid attribute combination",
				"`incident_template` can't be used together with `grouping_config`. Use `incident_config.template` instead.")
		}
		if setPresent(incidentBase.AtName("grouping_keys")) {
			addErr(incidentBase.AtName("grouping_keys"), "Invalid attribute combination",
				"`incident_config.grouping_keys` can't be used together with `grouping_config`. Use `grouping_config.default.grouping_keys` instead.")
		}
		if int64Set(incidentBase.AtName("grouping_window_seconds")) {
			addErr(incidentBase.AtName("grouping_window_seconds"), "Invalid attribute combination",
				"`incident_config.grouping_window_seconds` can't be used together with `grouping_config`. Use `grouping_config.default.window_seconds` instead.")
		}
		if int64Set(incidentBase.AtName("defer_time_seconds")) {
			addErr(incidentBase.AtName("defer_time_seconds"), "Invalid attribute combination",
				"`incident_config.defer_time_seconds` can't be used with `grouping_config` and is deprecated - remove it")
		}
		if boolSet(incidentBase.AtName("auto_relate_grouped_alerts")) {
			addErr(incidentBase.AtName("auto_relate_grouped_alerts"), "Invalid attribute combination",
				"`incident_config.auto_relate_grouped_alerts` can't be used with `grouping_config` and is deprecated - remove it")
		}

		// message_config is required in v3 mode.
		if objectMissing(path.Root("message_config")) {
			addErr(path.Root("message_config"), "Missing required attribute",
				"`message_config` is required when `grouping_config` is set.")
		}

		r.validateV3Gating(ctx, req, resp, boolValue, boolSet, boolMissing, setPresent, objectSet, int64Set, int64Missing)
	} else {
		// v3-only attributes must not be set in v2 mode.
		if objectSet(path.Root("message_config")) {
			addErr(path.Root("message_config"), "Invalid attribute combination",
				"`message_config` can only be used when `grouping_config` is set. Set `grouping_config`, or use the deprecated `channel_config` / `message_template` instead.")
		}
		if objectSet(escalationBase.AtName("when_alert_joins_group")) {
			addErr(escalationBase.AtName("when_alert_joins_group"), "Invalid attribute combination",
				"`escalation_config.when_alert_joins_group` can only be used when `grouping_config` is set.")
		}
		if objectSet(incidentBase.AtName("template")) {
			addErr(incidentBase.AtName("template"), "Invalid attribute combination",
				"`incident_config.template` can only be used when `grouping_config` is set. Set `grouping_config`, or use the deprecated top-level `incident_template` instead.")
		}

		// Restore the v2 required fields that the merged schema relaxed to Optional
		// so that v3 mode can omit them. (auto_cancel_escalations and
		// escalation_targets are Required in the schema in both modes.)
		if objectMissing(path.Root("incident_template")) {
			addErr(path.Root("incident_template"), "Missing required attribute",
				"`incident_template` is required when `grouping_config` is not set.")
		}
		if boolMissing(incidentBase.AtName("auto_decline_enabled")) {
			addErr(incidentBase.AtName("auto_decline_enabled"), "Missing required attribute",
				"`incident_config.auto_decline_enabled` is required when `grouping_config` is not set.")
		}
		if int64Missing(incidentBase.AtName("grouping_window_seconds")) {
			addErr(incidentBase.AtName("grouping_window_seconds"), "Missing required attribute",
				"`incident_config.grouping_window_seconds` is required when `grouping_config` is not set.")
		}
		if int64Missing(incidentBase.AtName("defer_time_seconds")) {
			addErr(incidentBase.AtName("defer_time_seconds"), "Missing required attribute",
				"`incident_config.defer_time_seconds` is required when `grouping_config` is not set.")
		}
	}

	r.validateExpressions(ctx, req, resp)
}

// validateV3Gating enforces the conditional relationships within the v3 schema
// that the API checks at its boundary, surfacing them at plan time instead:
// grouping detail fields gated on grouping.enabled, incident template/decline
// gated on incident.enabled, and grace_period_seconds gated on the
// when_alert_joins_group mode.
func (r *IncidentAlertRouteResource) validateV3Gating(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
	boolValue func(path.Path) (bool, bool),
	boolSet func(path.Path) bool,
	boolMissing func(path.Path) bool,
	setPresent func(path.Path) bool,
	objectSet func(path.Path) bool,
	int64Set func(path.Path) bool,
	int64Missing func(path.Path) bool,
) {
	addErr := func(p path.Path, summary, detail string) {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(p, summary, detail))
	}

	groupingBase := path.Root("grouping_config").AtName("default")
	escalationBase := path.Root("escalation_config")
	incidentBase := path.Root("incident_config")
	whenJoinsBase := escalationBase.AtName("when_alert_joins_group")

	// when_alert_joins_group only applies when grouping is enabled, and its
	// grace_period_seconds only when re-escalating on each new alert; the API
	// rejects both otherwise.
	if objectSet(whenJoinsBase) {
		if enabled, known := boolValue(groupingBase.AtName("enabled")); known && !enabled {
			addErr(whenJoinsBase, "Invalid attribute combination",
				"`escalation_config.when_alert_joins_group` can only be set when `grouping_config.default.enabled` is true.")
		}

		var mode types.String
		if d := req.Config.GetAttribute(ctx, whenJoinsBase.AtName("mode"), &mode); !d.HasError() &&
			!mode.IsNull() && !mode.IsUnknown() && mode.ValueString() == "on_priority_increase" {
			if int64Set(whenJoinsBase.AtName("grace_period_seconds")) {
				addErr(whenJoinsBase.AtName("grace_period_seconds"), "Invalid attribute combination",
					"`grace_period_seconds` can only be set when `escalation_config.when_alert_joins_group.mode` is `on_each_new_alert`.")
			}
		}
	}

	// Grouping: when enabled, window_seconds and window_type are required; when
	// disabled, the detail fields must be unset.
	groupingEnabled, groupingKnown := boolValue(groupingBase.AtName("enabled"))
	if groupingKnown {
		if groupingEnabled {
			if int64Missing(groupingBase.AtName("window_seconds")) {
				addErr(groupingBase.AtName("window_seconds"), "Missing required attribute",
					"`window_seconds` is required when `grouping_config.default.enabled` is true.")
			}
			var windowType types.String
			req.Config.GetAttribute(ctx, groupingBase.AtName("window_type"), &windowType)
			if windowType.IsNull() {
				addErr(groupingBase.AtName("window_type"), "Missing required attribute",
					"`window_type` is required when `grouping_config.default.enabled` is true.")
			}
		} else {
			if int64Set(groupingBase.AtName("window_seconds")) {
				addErr(groupingBase.AtName("window_seconds"), "Invalid attribute combination",
					"`window_seconds` must not be set when `grouping_config.default.enabled` is false.")
			}
			var windowType types.String
			req.Config.GetAttribute(ctx, groupingBase.AtName("window_type"), &windowType)
			if !windowType.IsNull() && !windowType.IsUnknown() {
				addErr(groupingBase.AtName("window_type"), "Invalid attribute combination",
					"`window_type` must not be set when `grouping_config.default.enabled` is false.")
			}
			if setPresent(groupingBase.AtName("grouping_keys")) {
				addErr(groupingBase.AtName("grouping_keys"), "Invalid attribute combination",
					"`grouping_keys` must not be set when `grouping_config.default.enabled` is false.")
			}
		}
	}

	// Incident: auto_decline_enabled and condition_groups/template only apply when
	// incident creation is enabled.
	incidentEnabled, incidentKnown := boolValue(incidentBase.AtName("enabled"))
	if incidentKnown {
		if incidentEnabled {
			if boolMissing(incidentBase.AtName("auto_decline_enabled")) {
				addErr(incidentBase.AtName("auto_decline_enabled"), "Missing required attribute",
					"`incident_config.auto_decline_enabled` is required when `incident_config.enabled` is true.")
			}
		} else {
			if boolSet(incidentBase.AtName("auto_decline_enabled")) {
				addErr(incidentBase.AtName("auto_decline_enabled"), "Invalid attribute combination",
					"`incident_config.auto_decline_enabled` must not be set when `incident_config.enabled` is false.")
			}
			var conditionGroups types.List
			if d := req.Config.GetAttribute(ctx, incidentBase.AtName("condition_groups"), &conditionGroups); !d.HasError() &&
				!conditionGroups.IsNull() && !conditionGroups.IsUnknown() && len(conditionGroups.Elements()) > 0 {
				addErr(incidentBase.AtName("condition_groups"), "Invalid attribute combination",
					"`incident_config.condition_groups` must be empty when `incident_config.enabled` is false.")
			}
			if objectSet(incidentBase.AtName("template")) {
				addErr(incidentBase.AtName("template"), "Invalid attribute combination",
					"`incident_config.template` must not be set when `incident_config.enabled` is false.")
			}
		}
	}
}

// validateExpressions enforces that branches (if/else) operations use a valid
// root reference. This applies in both schemas.
func (r *IncidentAlertRouteResource) validateExpressions(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var expressions []models.IncidentEngineExpression

	diags := req.Config.GetAttribute(ctx, path.Root("expressions"), &expressions)
	if diags.HasError() {
		// If expressions is unknown (e.g., depends on another resource), skip validation.
		return
	}

	for i, expr := range expressions {
		hasBranches := false
		for _, op := range expr.Operations {
			if op.Branches != nil {
				hasBranches = true
				break
			}
		}

		if !hasBranches {
			continue
		}

		rootRef := expr.RootReference.ValueString()
		if rootRef != "" && rootRef != "." {
			resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
				path.Root("expressions").AtListIndex(i).AtName("root_reference"),
				"Invalid root_reference for branches operation",
				fmt.Sprintf(
					"Expression %q uses a branches (if/else) operation, which requires "+
						"root_reference to be \".\" (the whole scope). Got %q instead.\n\n"+
						"When using branches operations, set root_reference = \".\" and have "+
						"conditions reference absolute paths like \"alert.attributes.xxx\".",
					expr.Label.ValueString(),
					rootRef,
				),
			))
		}
	}
}
