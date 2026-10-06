package models

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// IncidentFormResourceModel is one lifecycle incident form: the expressions its elements may
// reference, and the elements in display order. It is shared by the incident_incident_form
// resource and data source, so every field here needs an attribute in both schemas.
type IncidentFormResourceModel struct {
	UnlockInDashboard types.Bool   `tfsdk:"unlock_in_dashboard"`
	ID                types.String `tfsdk:"id"`
	FormType          types.String `tfsdk:"form_type"`
	IncidentTypeID    types.String `tfsdk:"incident_type_id"`

	Expressions       IncidentEngineExpressions           `tfsdk:"expressions"`
	LifecycleElements []IncidentFormLifecycleElementModel `tfsdk:"lifecycle_elements"`
}

// IncidentFormLifecycleElementModel is one element on a form. An element is identified by
// element_type plus whichever of custom_field_id, incident_role_id and incident_timestamp_id
// the type requires; divider and text elements have no such key and are identified by id.
type IncidentFormLifecycleElementModel struct {
	ID                  types.String `tfsdk:"id"`
	ElementType         types.String `tfsdk:"element_type"`
	CustomFieldID       types.String `tfsdk:"custom_field_id"`
	IncidentRoleID      types.String `tfsdk:"incident_role_id"`
	IncidentTimestampID types.String `tfsdk:"incident_timestamp_id"`

	ShowIfConditionGroups     IncidentEngineConditionGroups   `tfsdk:"show_if_condition_groups"`
	RequiredIf                types.String                    `tfsdk:"required_if"`
	RequiredIfConditionGroups IncidentEngineConditionGroups   `tfsdk:"required_if_condition_groups"`
	DefaultValue              *IncidentEngineParamBinding     `tfsdk:"default_value"`
	Placeholder               types.String                    `tfsdk:"placeholder"`
	Description               types.String                    `tfsdk:"description"`
	CanSelectNoValue          types.Bool                      `tfsdk:"can_select_no_value"`
	Config                    *IncidentFormElementConfigModel `tfsdk:"config"`
}

type IncidentFormElementConfigModel struct {
	RequireComment types.Bool `tfsdk:"require_comment"`
}

// IncidentFormElementTypesWithoutNaturalKey are the element types a form can have several
// of, so the API tells them apart by id rather than by type.
var IncidentFormElementTypesWithoutNaturalKey = []string{
	string(client.IncidentFormLifecycleElementV3ElementTypeDivider),
	string(client.IncidentFormLifecycleElementV3ElementTypeText),
}

// HasNaturalKey reports whether the API identifies this element by its type and resource
// IDs, rather than by id.
func (m IncidentFormLifecycleElementModel) HasNaturalKey() bool {
	return !lo.Contains(IncidentFormElementTypesWithoutNaturalKey, m.ElementType.ValueString())
}

// NaturalKey is the identity the API matches an element on: its type plus whichever resource
// ID it points at. Divider and text elements share a key per type, so callers check
// HasNaturalKey first.
func (m IncidentFormLifecycleElementModel) NaturalKey() string {
	return strings.Join([]string{
		m.ElementType.ValueString(),
		m.CustomFieldID.ValueString(),
		m.IncidentRoleID.ValueString(),
		m.IncidentTimestampID.ValueString(),
	}, ":")
}

// FromAPI builds the model from a form the API returned. The plan, when given, is what the
// config said: it restores the spelling of bindings and conditions the API answers
// differently but equivalently, and decides whether an absent optional collection reads back
// as null or empty.
func (IncidentFormResourceModel) FromAPI(form client.IncidentFormV3, plan *IncidentFormResourceModel) IncidentFormResourceModel {
	result := IncidentFormResourceModel{
		ID:             types.StringValue(form.Id),
		FormType:       types.StringValue(string(form.FormType)),
		IncidentTypeID: types.StringPointerValue(form.IncidentTypeId),
	}
	if plan != nil {
		result.UnlockInDashboard = plan.UnlockInDashboard
	}

	// A form with no expressions reads back the way the config wrote it: null when the
	// attribute was left out, empty when it said `expressions = []`.
	if len(form.Expressions) > 0 {
		result.Expressions = expressionsFromV3(form.Expressions)
		if plan != nil {
			result.Expressions.ReconcileSpelling(plan.Expressions)
		}
	} else if plan != nil && plan.Expressions != nil {
		result.Expressions = IncidentEngineExpressions{}
	}

	elements := lo.FromPtr(form.LifecycleElements)
	if len(elements) > 0 || (plan != nil && plan.LifecycleElements != nil) {
		result.LifecycleElements = make([]IncidentFormLifecycleElementModel, 0, len(elements))
	}
	// Unkeyed elements with no ID yet match the plan by their position among elements of the
	// same type, which the API keeps even as it moves pinned elements to the front.
	ordinals := map[string]int{}
	for _, element := range elements {
		ordinal := ordinals[string(element.ElementType)]
		ordinals[string(element.ElementType)]++

		var planned *IncidentFormLifecycleElementModel
		if plan != nil {
			planned = plan.matchingElement(element, ordinal)
		}
		result.LifecycleElements = append(result.LifecycleElements, IncidentFormLifecycleElementModel{}.FromAPI(element, planned))
	}
	if plan != nil {
		result.LifecycleElements = restorePinnedElementPositions(result.LifecycleElements, plan.LifecycleElements)
	}

	return result
}

// IncidentFormPinnedElementTypes are the elements the API always lists first on a form,
// in this order, wherever a write puts them.
var IncidentFormPinnedElementTypes = []string{
	string(client.IncidentFormLifecycleElementV3ElementTypeName),
	string(client.IncidentFormLifecycleElementV3ElementTypeIncidentType),
	string(client.IncidentFormLifecycleElementV3ElementTypeStatus),
}

// restorePinnedElementPositions puts each pinned element back in the slot where the plan
// listed that type. The API fixes those positions and ranks everything else by list order,
// so this difference isn't drift, and keeping it would fail every apply that lists a pinned
// element anywhere but first.
func restorePinnedElementPositions(applied, plan []IncidentFormLifecycleElementModel) []IncidentFormLifecycleElementModel {
	isPinned := func(element IncidentFormLifecycleElementModel) bool {
		return lo.Contains(IncidentFormPinnedElementTypes, element.ElementType.ValueString())
	}

	// Only when the plan and the API hold the same pinned elements: anything else is a
	// real difference, shown as the API has it.
	appliedPinned := map[string]IncidentFormLifecycleElementModel{}
	for _, element := range applied {
		if isPinned(element) {
			appliedPinned[element.ElementType.ValueString()] = element
		}
	}
	plannedPinned := map[string]bool{}
	for _, element := range plan {
		if isPinned(element) {
			plannedPinned[element.ElementType.ValueString()] = true
		}
	}
	if len(plannedPinned) != len(appliedPinned) {
		return applied
	}
	for elementType := range plannedPinned {
		if _, ok := appliedPinned[elementType]; !ok {
			return applied
		}
	}

	// Walk the plan's shape: a pinned slot takes the applied element of that type, every
	// other slot takes the next unpinned applied element in order.
	unpinned := lo.Filter(applied, func(element IncidentFormLifecycleElementModel, _ int) bool { return !isPinned(element) })
	result := make([]IncidentFormLifecycleElementModel, 0, len(applied))
	nextUnpinned := 0
	for _, planned := range plan {
		if isPinned(planned) {
			result = append(result, appliedPinned[planned.ElementType.ValueString()])
			continue
		}
		if nextUnpinned < len(unpinned) {
			result = append(result, unpinned[nextUnpinned])
			nextUnpinned++
		}
	}
	// Elements the plan didn't have, which only an import or a refresh after drift sees.
	result = append(result, unpinned[nextUnpinned:]...)

	return result
}

// matchingElement finds the planned element an API element corresponds to: by natural key
// where it has one, otherwise by id, and failing both as the ordinal-th planned element of
// its type.
func (m IncidentFormResourceModel) matchingElement(element client.IncidentFormLifecycleElementV3, ordinal int) *IncidentFormLifecycleElementModel {
	api := IncidentFormLifecycleElementModel{}.FromAPI(element, nil)

	for i := range m.LifecycleElements {
		planned := &m.LifecycleElements[i]
		if api.HasNaturalKey() {
			if planned.HasNaturalKey() && planned.NaturalKey() == api.NaturalKey() {
				return planned
			}
			continue
		}

		if element.Id != nil && planned.ID.ValueString() == *element.Id {
			return planned
		}
	}

	if !api.HasNaturalKey() {
		seen := 0
		for i := range m.LifecycleElements {
			if !m.LifecycleElements[i].ElementType.Equal(api.ElementType) {
				continue
			}
			if seen == ordinal {
				return &m.LifecycleElements[i]
			}
			seen++
		}
	}

	return nil
}

func (IncidentFormLifecycleElementModel) FromAPI(element client.IncidentFormLifecycleElementV3, plan *IncidentFormLifecycleElementModel) IncidentFormLifecycleElementModel {
	result := IncidentFormLifecycleElementModel{
		ID:                  types.StringPointerValue(element.Id),
		ElementType:         types.StringValue(string(element.ElementType)),
		CustomFieldID:       types.StringPointerValue(element.CustomFieldId),
		IncidentRoleID:      types.StringPointerValue(element.IncidentRoleId),
		IncidentTimestampID: types.StringPointerValue(element.IncidentTimestampId),
		Placeholder:         types.StringPointerValue(element.Placeholder),
		Description:         types.StringPointerValue(element.Description),
		CanSelectNoValue:    types.BoolValue(lo.FromPtr(element.CanSelectNoValue)),
	}

	// The API always answers with a required_if; a config that left it out gets the API's
	// default back, which the schema default matches.
	result.RequiredIf = types.StringValue(string(client.IncidentFormLifecycleElementV3RequiredIfNeverRequire))
	if element.RequiredIf != nil {
		result.RequiredIf = types.StringValue(string(*element.RequiredIf))
	}

	var plannedShowIf, plannedRequiredIf IncidentEngineConditionGroups
	if plan != nil {
		plannedShowIf = plan.ShowIfConditionGroups
		plannedRequiredIf = plan.RequiredIfConditionGroups
	}
	result.ShowIfConditionGroups = optionalConditionGroupsFromV3(lo.FromPtr(element.ShowIfConditionGroups), plannedShowIf)
	result.RequiredIfConditionGroups = optionalConditionGroupsFromV3(lo.FromPtr(element.RequiredIfConditionGroups), plannedRequiredIf)

	if element.DefaultValue != nil {
		binding := paramBindingFromV3(*element.DefaultValue)
		result.DefaultValue = &binding
		if plan != nil {
			result.DefaultValue = ReconcileBindingSpelling(result.DefaultValue, plan.DefaultValue)
		}
	}

	// The API stores the description as a document and renders it back as markdown, which
	// can differ from the config in whitespace alone. Keep what the config wrote when the
	// two say the same thing.
	if plan != nil && !plan.Description.IsNull() && !plan.Description.IsUnknown() && element.Description != nil &&
		strings.TrimSpace(plan.Description.ValueString()) == strings.TrimSpace(*element.Description) {
		result.Description = plan.Description
	}

	// config is always returned, so an element whose config was never set reads it back as
	// null unless the API has something non-default to report.
	if element.Config != nil && lo.FromPtr(element.Config.RequireComment) {
		result.Config = &IncidentFormElementConfigModel{RequireComment: types.BoolValue(true)}
	} else if plan != nil && plan.Config != nil {
		result.Config = &IncidentFormElementConfigModel{RequireComment: types.BoolValue(false)}
	}

	return result
}

// optionalConditionGroupsFromV3 reads condition groups the config may have left out. The
// API answers an element with no conditions with an empty list, so an empty answer reads
// back as whatever the config said: null when unset, empty when it wrote `[]`.
func optionalConditionGroupsFromV3(groups []client.ConditionGroupV3, plan IncidentEngineConditionGroups) IncidentEngineConditionGroups {
	if len(groups) == 0 {
		if plan != nil {
			return IncidentEngineConditionGroups{}
		}

		return nil
	}

	result := conditionGroupsFromV3(groups)
	result.ReconcileSpelling(plan)

	return result
}

func (m IncidentFormResourceModel) ToCreatePayload() client.IncidentFormsCreatePayloadV3 {
	return client.IncidentFormsCreatePayloadV3{
		FormType:          client.IncidentFormsCreatePayloadV3FormType(m.FormType.ValueString()),
		IncidentTypeId:    m.IncidentTypeID.ValueStringPointer(),
		Expressions:       expressionsToV3Payload(m.Expressions),
		LifecycleElements: lo.ToPtr(m.lifecycleElementsPayload()),
	}
}

func (m IncidentFormResourceModel) ToUpdatePayload() client.IncidentFormsUpdatePayloadV3 {
	return client.IncidentFormsUpdatePayloadV3{
		FormType:          client.IncidentFormsUpdatePayloadV3FormType(m.FormType.ValueString()),
		IncidentTypeId:    m.IncidentTypeID.ValueStringPointer(),
		Expressions:       expressionsToV3Payload(m.Expressions),
		LifecycleElements: lo.ToPtr(m.lifecycleElementsPayload()),
	}
}

// ToValidatePayload checks this config as a replacement for the form with the given ID, or
// as a new form when the ID is null.
func (m IncidentFormResourceModel) ToValidatePayload(id types.String) client.IncidentFormsValidatePayloadV3 {
	payload := client.IncidentFormsValidatePayloadV3{
		FormType:          client.IncidentFormsValidatePayloadV3FormType(m.FormType.ValueString()),
		IncidentTypeId:    m.IncidentTypeID.ValueStringPointer(),
		Expressions:       expressionsToV3Payload(m.Expressions),
		LifecycleElements: lo.ToPtr(m.lifecycleElementsPayload()),
	}
	if !id.IsNull() && !id.IsUnknown() {
		payload.Id = id.ValueStringPointer()
	}

	return payload
}

func (m IncidentFormResourceModel) lifecycleElementsPayload() []client.IncidentFormLifecycleElementPayloadV3 {
	return lo.Map(m.LifecycleElements, func(element IncidentFormLifecycleElementModel, _ int) client.IncidentFormLifecycleElementPayloadV3 {
		return element.ToPayload()
	})
}

func (m IncidentFormLifecycleElementModel) ToPayload() client.IncidentFormLifecycleElementPayloadV3 {
	payload := client.IncidentFormLifecycleElementPayloadV3{
		ElementType:         client.IncidentFormLifecycleElementPayloadV3ElementType(m.ElementType.ValueString()),
		CustomFieldId:       m.CustomFieldID.ValueStringPointer(),
		IncidentRoleId:      m.IncidentRoleID.ValueStringPointer(),
		IncidentTimestampId: m.IncidentTimestampID.ValueStringPointer(),
		Placeholder:         m.Placeholder.ValueStringPointer(),
		Description:         m.Description.ValueStringPointer(),
		CanSelectNoValue:    lo.ToPtr(m.CanSelectNoValue.ValueBool()),
	}

	// The API matches every other element on its natural key, and rejects an id that names a
	// different element, so only the types with no key are sent one.
	if !m.HasNaturalKey() && !m.ID.IsNull() && !m.ID.IsUnknown() {
		payload.Id = m.ID.ValueStringPointer()
	}

	if !m.RequiredIf.IsNull() && !m.RequiredIf.IsUnknown() {
		payload.RequiredIf = lo.ToPtr(client.IncidentFormLifecycleElementPayloadV3RequiredIf(m.RequiredIf.ValueString()))
	}
	if m.ShowIfConditionGroups != nil {
		payload.ShowIfConditionGroups = lo.ToPtr(conditionGroupsToV3Payload(m.ShowIfConditionGroups))
	}
	if m.RequiredIfConditionGroups != nil {
		payload.RequiredIfConditionGroups = lo.ToPtr(conditionGroupsToV3Payload(m.RequiredIfConditionGroups))
	}
	if m.DefaultValue != nil && !m.DefaultValue.IsEmpty() {
		payload.DefaultValue = lo.ToPtr(paramBindingToV3Payload(*m.DefaultValue))
	}
	if m.Config != nil {
		payload.Config = &client.IncidentFormLifecycleElementConfigV3{
			RequireComment: lo.ToPtr(m.Config.RequireComment.ValueBool()),
		}
	}

	return payload
}
