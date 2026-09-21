package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
)

// OwningTeamIDsToState maps an owning_team_ids API response onto state, keyed off what
// the practitioner asked for.
//
// The API omits owning_team_ids for a resource nobody owns, so the response on its own
// can't tell "never set" from "explicitly empty". Answering null for both breaks an
// `owning_team_ids = []` config, which fails apply with "Provider produced inconsistent
// result after apply" (issue #595); answering an empty set for both invents a value for
// a config that never mentioned the attribute.
//
// planned is the plan on create and update, and prior state on read. It is null when
// there is neither — an import — where absent is the safer answer, matching how the
// other owning_team_ids resources import.
func OwningTeamIDsToState(ids *[]string, planned types.Set) types.Set {
	teamIDs := lo.FromPtr(ids)
	if len(teamIDs) == 0 && (planned.IsNull() || planned.IsUnknown()) {
		return types.SetNull(types.StringType)
	}

	elements := make([]attr.Value, len(teamIDs))
	for i, id := range teamIDs {
		elements[i] = types.StringValue(id)
	}

	return types.SetValueMust(types.StringType, elements)
}
