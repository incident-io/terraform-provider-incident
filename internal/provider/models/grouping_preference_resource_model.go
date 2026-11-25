package models

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// AlertGroupingPreferenceResourceModel is incident_alert_grouping_preference.
type AlertGroupingPreferenceResourceModel struct {
	ID                types.String `tfsdk:"id"`
	UnlockInDashboard types.Bool   `tfsdk:"unlock_in_dashboard"`
	Kind              types.String `tfsdk:"kind"`
	TeamID            types.String `tfsdk:"team_id"`
	Version           types.Int64  `tfsdk:"version"`
	GroupingPreferenceModel
}

// FromAPI keeps unlock_in_dashboard from the prior, as no read returns it.
func (AlertGroupingPreferenceResourceModel) FromAPI(preference client.AlertGroupingPreferenceV3, prior *AlertGroupingPreferenceResourceModel) AlertGroupingPreferenceResourceModel {
	result := AlertGroupingPreferenceResourceModel{
		ID:                types.StringValue(preference.Id),
		UnlockInDashboard: types.BoolNull(),
		Kind:              types.StringValue(string(preference.Kind)),
		TeamID:            types.StringPointerValue(preference.TeamId),
		Version:           types.Int64Value(preference.Version),
	}

	var priorPreference *GroupingPreferenceModel
	if prior != nil {
		result.UnlockInDashboard = prior.UnlockInDashboard
		priorPreference = &prior.GroupingPreferenceModel
	}
	result.GroupingPreferenceModel = GroupingPreferenceModel{}.FromAPI(preference, priorPreference)

	return result
}

// ValidateGroupingPreference applies the API's rules at plan time, and no more: the provider
// must never refuse what the API accepts. An unknown value may still resolve to anything, so
// only a known one is checked.
func ValidateGroupingPreference(m AlertGroupingPreferenceResourceModel, diags *diag.Diagnostics) {
	switch {
	case m.Kind.ValueString() == GroupingKindTeam && m.TeamID.IsNull():
		diags.AddAttributeError(path.Root("team_id"), "Missing team_id",
			"A team's grouping preference needs the team_id of the team it belongs to.")
	case m.Kind.ValueString() == GroupingKindDefault && !m.TeamID.IsNull() && !m.TeamID.IsUnknown():
		diags.AddAttributeError(path.Root("team_id"), "Unexpected team_id",
			"The organisation default belongs to no team. Remove team_id, or set kind to \"team\".")
	}

	if m.Default != nil {
		validateGroupingSettings(m.Default.Settings, path.Root("default").AtName("settings"), diags)
	}

	seen := map[string]bool{}
	for idx, branch := range m.Branches {
		at := path.Root("branch").AtListIndex(idx)
		validateGroupingSettings(branch.Settings, at.AtName("settings"), diags)

		if branch.ID.IsNull() || branch.ID.IsUnknown() {
			continue
		}
		if seen[branch.ID.ValueString()] {
			diags.AddAttributeError(at.AtName("id"), "Duplicate id", fmt.Sprintf(
				"Another branch in this preference already uses the id %q. Each branch needs its own id.",
				branch.ID.ValueString()))
		}
		seen[branch.ID.ValueString()] = true
	}

	ValidateExpressions(ExpressionNamespace{}, nil, path.Empty(), m.NamedExpressions, path.Root("named_expression"), diags)
}

// validateGroupingSettings is the API's rule for one set of settings: an enabled one needs
// its window, and a disabled one carries no window, no keys and no AI grouping. The API
// accepts empty grouping_keys when disabled, so this does too.
func validateGroupingSettings(settings *TeamGroupingPreferenceSettingsModel, at path.Path, diags *diag.Diagnostics) {
	if settings == nil || settings.Enabled.IsNull() || settings.Enabled.IsUnknown() {
		return
	}

	if settings.Enabled.ValueBool() {
		if settings.WindowSeconds.IsNull() {
			diags.AddAttributeError(at.AtName("window_seconds"), "Missing required attribute",
				"`window_seconds` is required when grouping is enabled.")
		}
		if settings.WindowType.IsNull() {
			diags.AddAttributeError(at.AtName("window_type"), "Missing required attribute",
				"`window_type` is required when grouping is enabled.")
		}
		return
	}

	if !settings.WindowSeconds.IsNull() && !settings.WindowSeconds.IsUnknown() {
		diags.AddAttributeError(at.AtName("window_seconds"), "Invalid attribute combination",
			"`window_seconds` must not be set when grouping is disabled.")
	}
	if !settings.WindowType.IsNull() && !settings.WindowType.IsUnknown() {
		diags.AddAttributeError(at.AtName("window_type"), "Invalid attribute combination",
			"`window_type` must not be set when grouping is disabled.")
	}
	if len(settings.GroupingKeys) > 0 {
		diags.AddAttributeError(at.AtName("grouping_keys"), "Invalid attribute combination",
			"`grouping_keys` must be empty when grouping is disabled.")
	}
	if settings.AIEnabled.ValueBool() {
		diags.AddAttributeError(at.AtName("ai_enabled"), "Invalid attribute combination",
			"`ai_enabled` can only be true when grouping is enabled.")
	}
}
