package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// StatusPageStructureModel is the Terraform model for a status page's structure: the
// components it shows, in order, how they are grouped, and how each is displayed. The
// resource's id is the page's, since a page has exactly one structure.
type StatusPageStructureModel struct {
	ID                types.String                   `tfsdk:"id"`
	StatusPageID      types.String                   `tfsdk:"status_page_id"`
	DisplayUptimeMode types.String                   `tfsdk:"display_uptime_mode"`
	Items             []StatusPageStructureItemModel `tfsdk:"items"`
	UnlockInDashboard types.Bool                     `tfsdk:"unlock_in_dashboard"`
}

// StatusPageStructureItemModel is one entry on the page: a component on its own, or a group
// of them. Exactly one of the two is set, and the display flags belong to the component.
type StatusPageStructureItemModel struct {
	ComponentID   types.String                   `tfsdk:"component_id"`
	Hidden        types.Bool                     `tfsdk:"hidden"`
	DisplayUptime types.Bool                     `tfsdk:"display_uptime"`
	Group         *StatusPageStructureGroupModel `tfsdk:"group"`
}

type StatusPageStructureGroupModel struct {
	ID                      types.String                             `tfsdk:"id"`
	Name                    types.String                             `tfsdk:"name"`
	Description             types.String                             `tfsdk:"description"`
	Hidden                  types.Bool                               `tfsdk:"hidden"`
	DisplayAggregatedUptime types.Bool                               `tfsdk:"display_aggregated_uptime"`
	Components              []StatusPageStructureGroupComponentModel `tfsdk:"components"`
}

// StatusPageStructureGroupComponentModel is a component's place in a group, with the
// display settings of that placement.
type StatusPageStructureGroupComponentModel struct {
	ComponentID   types.String `tfsdk:"component_id"`
	Hidden        types.Bool   `tfsdk:"hidden"`
	DisplayUptime types.Bool   `tfsdk:"display_uptime"`
}

// FromAPI converts an API structure into the Terraform model. A sub-page item only appears
// on a parent page, which this resource cannot manage, so it is dropped. Whether Terraform
// claims the structure is configuration the API does not report, so the caller passes it
// through from the prior state.
func (StatusPageStructureModel) FromAPI(statusPageID string, structure client.StatusPageStructureV2, displayUptimeMode string, unlockInDashboard types.Bool) StatusPageStructureModel {
	items := make([]StatusPageStructureItemModel, 0, len(structure.Items))
	for _, item := range structure.Items {
		switch {
		case item.Component != nil:
			items = append(items, StatusPageStructureItemModel{
				ComponentID:   types.StringValue(item.Component.ComponentId),
				Hidden:        types.BoolValue(item.Component.Hidden),
				DisplayUptime: types.BoolValue(item.Component.DisplayUptime),
			})
		case item.Group != nil:
			items = append(items, StatusPageStructureItemModel{
				Hidden:        types.BoolNull(),
				DisplayUptime: types.BoolNull(),
				Group: &StatusPageStructureGroupModel{
					ID:                      types.StringValue(item.Group.Id),
					Name:                    types.StringValue(item.Group.Name),
					Description:             types.StringPointerValue(item.Group.Description),
					Hidden:                  types.BoolValue(item.Group.Hidden),
					DisplayAggregatedUptime: types.BoolValue(item.Group.DisplayAggregatedUptime),
					Components: lo.Map(item.Group.Components, func(component client.StatusPageStructureComponentV2, _ int) StatusPageStructureGroupComponentModel {
						return StatusPageStructureGroupComponentModel{
							ComponentID:   types.StringValue(component.ComponentId),
							Hidden:        types.BoolValue(component.Hidden),
							DisplayUptime: types.BoolValue(component.DisplayUptime),
						}
					}),
				},
			})
		}
	}

	return StatusPageStructureModel{
		ID:                types.StringValue(statusPageID),
		StatusPageID:      types.StringValue(statusPageID),
		DisplayUptimeMode: types.StringValue(displayUptimeMode),
		Items:             items,
		UnlockInDashboard: unlockInDashboard,
	}
}

// ToPayload converts the configured model to the API's set-structure payload. It expects the
// configuration rather than the plan: a display flag the configuration leaves out is left
// out of the payload too, which the API reads as "keep what this placement already has",
// whereas the plan carries the current value and would pin it.
//
// A group's ID is assigned by the API, so the configuration never carries one. Each
// configured group takes the ID of a same-named group on the page's current structure,
// which keeps it rather than creating a new one. Two groups sharing a name are matched in
// order.
func (m StatusPageStructureModel) ToPayload(current client.StatusPageStructureV2) client.StatusPagesSetStatusPageStructurePayloadV2 {
	idsByName := map[string][]string{}
	for _, item := range current.Items {
		if item.Group != nil {
			idsByName[item.Group.Name] = append(idsByName[item.Group.Name], item.Group.Id)
		}
	}

	component := func(id types.String, hidden, displayUptime types.Bool) client.StatusPageStructureComponentPayloadV2 {
		return client.StatusPageStructureComponentPayloadV2{
			ComponentId:   id.ValueString(),
			Hidden:        configuredBool(hidden),
			DisplayUptime: configuredBool(displayUptime),
		}
	}

	payload := client.StatusPagesSetStatusPageStructurePayloadV2{
		Items: lo.Map(m.Items, func(item StatusPageStructureItemModel, _ int) client.StatusPageStructureItemPayloadV2 {
			if item.Group == nil {
				return client.StatusPageStructureItemPayloadV2{
					Component: lo.ToPtr(component(item.ComponentID, item.Hidden, item.DisplayUptime)),
				}
			}

			group := client.StatusPageStructureGroupPayloadV2{
				Name: item.Group.Name.ValueString(),
				// Left out, a description means none: an empty string tells the API to
				// clear whatever a kept group had.
				Description:             lo.ToPtr(item.Group.Description.ValueString()),
				Hidden:                  configuredBool(item.Group.Hidden),
				DisplayAggregatedUptime: configuredBool(item.Group.DisplayAggregatedUptime),
				Components: lo.Map(item.Group.Components, func(member StatusPageStructureGroupComponentModel, _ int) client.StatusPageStructureComponentPayloadV2 {
					return component(member.ComponentID, member.Hidden, member.DisplayUptime)
				}),
			}
			if ids := idsByName[group.Name]; len(ids) > 0 {
				group.Id = lo.ToPtr(ids[0])
				idsByName[group.Name] = ids[1:]
			}

			return client.StatusPageStructureItemPayloadV2{Group: &group}
		}),
	}

	if !m.DisplayUptimeMode.IsNull() && !m.DisplayUptimeMode.IsUnknown() {
		payload.DisplayUptimeMode = lo.ToPtr(client.StatusPagesSetStatusPageStructurePayloadV2DisplayUptimeMode(m.DisplayUptimeMode.ValueString()))
	}

	return payload
}

// configuredBool is a pointer to a flag the configuration sets, and nil for one it leaves
// out or that is not yet known.
func configuredBool(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	return lo.ToPtr(value.ValueBool())
}

// CarryDisplaySettings fills the plan's unknown display settings and group IDs from the
// prior state, matching a component by its ID and a group by its name rather than by
// position, so a reorder plans no change to settings it does not touch.
//
// The API derives a group's flags from its members when the payload leaves them out, so a
// carried group flag is corrected the same way where the members are known: a group of
// hidden members is hidden, and a group with no visible member showing uptime shows none.
func (m *StatusPageStructureModel) CarryDisplaySettings(state StatusPageStructureModel) {
	type flags struct{ hidden, displayUptime types.Bool }
	componentFlags := map[string]flags{}
	groupsByName := map[string][]*StatusPageStructureGroupModel{}
	for _, item := range state.Items {
		if item.Group == nil {
			componentFlags[item.ComponentID.ValueString()] = flags{item.Hidden, item.DisplayUptime}
			continue
		}
		name := item.Group.Name.ValueString()
		groupsByName[name] = append(groupsByName[name], item.Group)
		for _, member := range item.Group.Components {
			componentFlags[member.ComponentID.ValueString()] = flags{member.Hidden, member.DisplayUptime}
		}
	}

	carry := func(planned types.Bool, prior types.Bool) types.Bool {
		if planned.IsUnknown() && !prior.IsNull() {
			return prior
		}

		return planned
	}
	carryComponent := func(id types.String, hidden, displayUptime *types.Bool) {
		prior, ok := componentFlags[id.ValueString()]
		if !ok || id.IsUnknown() {
			return
		}
		*hidden = carry(*hidden, prior.hidden)
		*displayUptime = carry(*displayUptime, prior.displayUptime)
	}
	known := func(value types.Bool) bool { return !value.IsUnknown() && !value.IsNull() }

	if m.DisplayUptimeMode.IsUnknown() {
		m.DisplayUptimeMode = state.DisplayUptimeMode
	}

	for idx := range m.Items {
		item := &m.Items[idx]
		if item.Group == nil {
			carryComponent(item.ComponentID, &item.Hidden, &item.DisplayUptime)
			continue
		}

		// A group item has no component flags of its own.
		item.Hidden = types.BoolNull()
		item.DisplayUptime = types.BoolNull()

		group := item.Group
		for memberIdx := range group.Components {
			member := &group.Components[memberIdx]
			carryComponent(member.ComponentID, &member.Hidden, &member.DisplayUptime)
		}

		// Only a carried flag is corrected below. One the configuration sets must reach
		// the API as written, so a contradiction is the API's 422 and not a silent edit.
		carriedHidden := group.Hidden.IsUnknown()
		carriedAggregatedUptime := group.DisplayAggregatedUptime.IsUnknown()

		name := group.Name.ValueString()
		if priors := groupsByName[name]; len(priors) > 0 && !group.Name.IsUnknown() {
			prior := priors[0]
			groupsByName[name] = priors[1:]
			if group.ID.IsUnknown() {
				group.ID = prior.ID
			}
			group.Hidden = carry(group.Hidden, prior.Hidden)
			group.DisplayAggregatedUptime = carry(group.DisplayAggregatedUptime, prior.DisplayAggregatedUptime)
		}

		allHidden := lo.EveryBy(group.Components, func(member StatusPageStructureGroupComponentModel) bool {
			return known(member.Hidden) && member.Hidden.ValueBool()
		})
		if carriedHidden && allHidden && known(group.Hidden) && !group.Hidden.ValueBool() {
			group.Hidden = types.BoolValue(true)
		}

		noneShowUptime := lo.EveryBy(group.Components, func(member StatusPageStructureGroupComponentModel) bool {
			return (known(member.Hidden) && member.Hidden.ValueBool()) ||
				(known(member.DisplayUptime) && !member.DisplayUptime.ValueBool())
		})
		if carriedAggregatedUptime && noneShowUptime && known(group.DisplayAggregatedUptime) && group.DisplayAggregatedUptime.ValueBool() {
			group.DisplayAggregatedUptime = types.BoolValue(false)
		}
	}
}
