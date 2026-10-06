package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/jsontypes"
)

func apiElement(elementType string, id string) client.IncidentFormLifecycleElementV3 {
	return client.IncidentFormLifecycleElementV3{
		Id:                        lo.ToPtr(id),
		ElementType:               client.IncidentFormLifecycleElementV3ElementType(elementType),
		RequiredIf:                lo.ToPtr(client.IncidentFormLifecycleElementV3RequiredIfNeverRequire),
		ShowIfConditionGroups:     &[]client.ConditionGroupV3{},
		RequiredIfConditionGroups: &[]client.ConditionGroupV3{},
		CanSelectNoValue:          lo.ToPtr(false),
		Config:                    &client.IncidentFormLifecycleElementConfigV3{RequireComment: lo.ToPtr(false)},
	}
}

func apiForm(elements ...client.IncidentFormLifecycleElementV3) client.IncidentFormV3 {
	return client.IncidentFormV3{
		Id:                "form",
		FormType:          client.IncidentFormV3FormTypeDeclare,
		Expressions:       []client.ExpressionV3{},
		LifecycleElements: &elements,
	}
}

func planElement(elementType string) IncidentFormLifecycleElementModel {
	return IncidentFormLifecycleElementModel{
		ID:          types.StringUnknown(),
		ElementType: types.StringValue(elementType),
		RequiredIf:  types.StringValue("never_require"),
	}
}

func elementTypes(elements []IncidentFormLifecycleElementModel) []string {
	return lo.Map(elements, func(element IncidentFormLifecycleElementModel, _ int) string {
		return element.ElementType.ValueString()
	})
}

func TestIncidentFormFromAPIWithoutPlan(t *testing.T) {
	custom := apiElement("custom_field", "el-cf")
	custom.CustomFieldId = lo.ToPtr("cf")
	custom.Description = lo.ToPtr("The **team**")
	custom.DefaultValue = &client.EngineParamBindingV3{Value: &client.EngineParamBindingValueV3{Literal: lo.ToPtr("Payments")}}

	model := IncidentFormResourceModel{}.FromAPI(apiForm(apiElement("name", "el-name"), custom), nil)

	if model.Expressions != nil {
		t.Errorf("expected no expressions to read as null, got %v", model.Expressions)
	}
	if got := elementTypes(model.LifecycleElements); len(got) != 2 || got[0] != "name" || got[1] != "custom_field" {
		t.Fatalf("unexpected elements: %v", got)
	}

	name := model.LifecycleElements[0]
	if name.ShowIfConditionGroups != nil || name.RequiredIfConditionGroups != nil {
		t.Errorf("expected empty condition groups to read as null without a plan")
	}
	if name.Config != nil {
		t.Errorf("expected a default config to read as null, got %v", name.Config)
	}
	if name.RequiredIf.ValueString() != "never_require" || name.CanSelectNoValue.ValueBool() {
		t.Errorf("unexpected defaults: %v %v", name.RequiredIf, name.CanSelectNoValue)
	}

	field := model.LifecycleElements[1]
	if field.CustomFieldID.ValueString() != "cf" || field.Description.ValueString() != "The **team**" {
		t.Errorf("unexpected custom field element: %+v", field)
	}
	if field.DefaultValue == nil || field.DefaultValue.Value.IsNull() {
		t.Fatalf("expected the default value to be read as the long form, got %+v", field.DefaultValue)
	}
}

func TestIncidentFormFromAPIKeepsPlanSpelling(t *testing.T) {
	custom := apiElement("custom_field", "el-cf")
	custom.CustomFieldId = lo.ToPtr("cf")
	custom.Description = lo.ToPtr("The team.")
	custom.DefaultValue = &client.EngineParamBindingV3{Value: &client.EngineParamBindingValueV3{Literal: lo.ToPtr("Payments")}}

	plannedBinding := NullParamBinding()
	plannedBinding.ValueLiteral = jsontypes.NewNormalizedJSONOrStringValue("Payments")
	plannedField := planElement("custom_field")
	plannedField.CustomFieldID = types.StringValue("cf")
	plannedField.Description = types.StringValue("The team.\n")
	plannedField.DefaultValue = &plannedBinding
	plannedField.Config = &IncidentFormElementConfigModel{RequireComment: types.BoolValue(false)}
	plannedField.ShowIfConditionGroups = IncidentEngineConditionGroups{}

	plan := &IncidentFormResourceModel{
		UnlockInDashboard: types.BoolValue(true),
		Expressions:       IncidentEngineExpressions{},
		LifecycleElements: []IncidentFormLifecycleElementModel{plannedField},
	}

	model := IncidentFormResourceModel{}.FromAPI(apiForm(custom), plan)

	if !model.UnlockInDashboard.ValueBool() {
		t.Errorf("expected unlock_in_dashboard to carry over from the plan")
	}
	if model.Expressions == nil || len(model.Expressions) != 0 {
		t.Errorf("expected `expressions = []` to read back empty, not null: %v", model.Expressions)
	}

	field := model.LifecycleElements[0]
	if field.Description.ValueString() != "The team.\n" {
		t.Errorf("expected the planned description to win over a whitespace-only difference, got %q", field.Description.ValueString())
	}
	if field.DefaultValue == nil || field.DefaultValue.ValueLiteral.ValueString() != "Payments" {
		t.Errorf("expected the value_literal shorthand to be restored, got %+v", field.DefaultValue)
	}
	if field.Config == nil || field.Config.RequireComment.ValueBool() {
		t.Errorf("expected a planned config to read back, got %+v", field.Config)
	}
	if field.ShowIfConditionGroups == nil || len(field.ShowIfConditionGroups) != 0 {
		t.Errorf("expected planned empty condition groups to read back empty, not null")
	}
	if field.RequiredIfConditionGroups != nil {
		t.Errorf("expected unplanned empty condition groups to read back null")
	}
}

func TestIncidentFormFromAPIRestoresPinnedPositions(t *testing.T) {
	plan := &IncidentFormResourceModel{LifecycleElements: []IncidentFormLifecycleElementModel{
		planElement("severity"),
		planElement("name"),
		planElement("summary"),
	}}

	// The API puts name first whatever the plan said.
	model := IncidentFormResourceModel{}.FromAPI(apiForm(
		apiElement("name", "el-name"),
		apiElement("severity", "el-sev"),
		apiElement("summary", "el-sum"),
	), plan)

	if got := elementTypes(model.LifecycleElements); got[0] != "severity" || got[1] != "name" || got[2] != "summary" {
		t.Errorf("expected the plan's order back, got %v", got)
	}
	if model.LifecycleElements[1].ID.ValueString() != "el-name" {
		t.Errorf("expected the name element to keep its ID through the reorder")
	}

	// Real drift in the unpinned elements still shows.
	drifted := IncidentFormResourceModel{}.FromAPI(apiForm(
		apiElement("name", "el-name"),
		apiElement("summary", "el-sum"),
		apiElement("severity", "el-sev"),
	), plan)
	if got := elementTypes(drifted.LifecycleElements); got[0] != "summary" || got[1] != "name" || got[2] != "severity" {
		t.Errorf("expected only the pinned element to move, got %v", got)
	}

	// Several pinned elements each go back to the slot where the plan listed their type.
	retro := IncidentFormResourceModel{}.FromAPI(apiForm(
		apiElement("name", "el-name"),
		apiElement("incident_type", "el-type"),
		apiElement("severity", "el-sev"),
	), &IncidentFormResourceModel{LifecycleElements: []IncidentFormLifecycleElementModel{
		planElement("incident_type"),
		planElement("severity"),
		planElement("name"),
	}})
	if got := elementTypes(retro.LifecycleElements); got[0] != "incident_type" || got[1] != "severity" || got[2] != "name" {
		t.Errorf("expected each pinned element back in its planned slot, got %v", got)
	}

	// Without a pinned element in the plan nothing moves.
	imported := IncidentFormResourceModel{}.FromAPI(apiForm(
		apiElement("name", "el-name"),
		apiElement("severity", "el-sev"),
	), &IncidentFormResourceModel{LifecycleElements: []IncidentFormLifecycleElementModel{planElement("severity")}})
	if got := elementTypes(imported.LifecycleElements); got[0] != "name" || got[1] != "severity" {
		t.Errorf("expected the API's order when the plan has no pinned element, got %v", got)
	}
}

func TestIncidentFormElementMatching(t *testing.T) {
	firstText := planElement("text")
	firstText.ID = types.StringValue("el-text-1")
	firstText.Description = types.StringValue("one")
	secondText := planElement("text")
	secondText.ID = types.StringValue("el-text-2")
	secondText.Description = types.StringValue("two")

	plan := &IncidentFormResourceModel{LifecycleElements: []IncidentFormLifecycleElementModel{secondText, firstText}}

	// The API answers in its own order; each text element matches its planned self by ID,
	// so the planned description spelling is kept for each.
	apiFirst := apiElement("text", "el-text-1")
	apiFirst.Description = lo.ToPtr("one\n")
	apiSecond := apiElement("text", "el-text-2")
	apiSecond.Description = lo.ToPtr("two\n")

	model := IncidentFormResourceModel{}.FromAPI(apiForm(apiFirst, apiSecond), plan)
	if model.LifecycleElements[0].Description.ValueString() != "one" || model.LifecycleElements[1].Description.ValueString() != "two" {
		t.Errorf("expected text elements to match on ID, got %q and %q",
			model.LifecycleElements[0].Description.ValueString(), model.LifecycleElements[1].Description.ValueString())
	}
}

func TestIncidentFormElementMatchingOnCreate(t *testing.T) {
	// On create no element has an ID yet, and the API has moved name to the front, so each
	// text element matches the plan by its position among the text elements.
	first := planElement("text")
	first.Description = types.StringValue("one")
	second := planElement("text")
	second.Description = types.StringValue("two")
	plan := &IncidentFormResourceModel{LifecycleElements: []IncidentFormLifecycleElementModel{first, second, planElement("name")}}

	apiFirst := apiElement("text", "el-text-1")
	apiFirst.Description = lo.ToPtr("one\n")
	apiSecond := apiElement("text", "el-text-2")
	apiSecond.Description = lo.ToPtr("two\n")

	model := IncidentFormResourceModel{}.FromAPI(apiForm(apiElement("name", "el-name"), apiFirst, apiSecond), plan)
	if got := elementTypes(model.LifecycleElements); got[0] != "text" || got[1] != "text" || got[2] != "name" {
		t.Fatalf("expected the plan's order back, got %v", got)
	}
	if model.LifecycleElements[0].Description.ValueString() != "one" || model.LifecycleElements[1].Description.ValueString() != "two" {
		t.Errorf("expected text elements to keep the planned description, got %q and %q",
			model.LifecycleElements[0].Description.ValueString(), model.LifecycleElements[1].Description.ValueString())
	}
	if model.LifecycleElements[0].ID.ValueString() != "el-text-1" || model.LifecycleElements[1].ID.ValueString() != "el-text-2" {
		t.Errorf("expected the API's IDs on the matched elements")
	}
}

func TestIncidentFormToPayload(t *testing.T) {
	text := planElement("text")
	text.ID = types.StringValue("el-text")
	text.Description = types.StringValue("Read me")

	field := planElement("custom_field")
	field.ID = types.StringValue("el-cf")
	field.CustomFieldID = types.StringValue("cf")
	field.RequiredIf = types.StringValue("check_engine_config")
	field.RequiredIfConditionGroups = IncidentEngineConditionGroups{{Conditions: IncidentEngineConditions{{
		Subject:       types.StringValue("incident.severity"),
		Operation:     types.StringValue("is_set"),
		ParamBindings: IncidentEngineParamBindings{},
	}}}}
	binding := NullParamBinding()
	binding.ValueLiteral = jsontypes.NewNormalizedJSONOrStringValue("Payments")
	field.DefaultValue = &binding
	field.Config = &IncidentFormElementConfigModel{RequireComment: types.BoolValue(true)}

	newText := planElement("text")

	model := IncidentFormResourceModel{
		FormType:          types.StringValue("declare"),
		IncidentTypeID:    types.StringValue("it"),
		LifecycleElements: []IncidentFormLifecycleElementModel{text, field, newText},
	}

	payload := model.ToUpdatePayload()
	if payload.FormType != "declare" || lo.FromPtr(payload.IncidentTypeId) != "it" {
		t.Errorf("unexpected form fields: %+v", payload)
	}
	if payload.Expressions == nil || len(payload.Expressions) != 0 {
		t.Errorf("expected no expressions to be sent as an empty list, got %v", payload.Expressions)
	}

	elements := lo.FromPtr(payload.LifecycleElements)
	if len(elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elements))
	}
	if lo.FromPtr(elements[0].Id) != "el-text" {
		t.Errorf("expected a text element to send its ID, got %v", elements[0].Id)
	}
	if elements[1].Id != nil {
		t.Errorf("expected a keyed element not to send its ID, the API matches it on its key; got %v", *elements[1].Id)
	}
	if lo.FromPtr(elements[1].RequiredIf) != "check_engine_config" || elements[1].RequiredIfConditionGroups == nil ||
		len(*elements[1].RequiredIfConditionGroups) != 1 {
		t.Errorf("unexpected required_if on the custom field: %+v", elements[1])
	}
	if elements[1].DefaultValue == nil || elements[1].DefaultValue.Value == nil || lo.FromPtr(elements[1].DefaultValue.Value.Literal) != "Payments" {
		t.Errorf("expected value_literal to fold onto value.literal, got %+v", elements[1].DefaultValue)
	}
	if elements[1].Config == nil || !lo.FromPtr(elements[1].Config.RequireComment) {
		t.Errorf("expected config to be sent, got %+v", elements[1].Config)
	}
	if elements[1].ShowIfConditionGroups != nil {
		t.Errorf("expected unset condition groups to be left out, got %v", elements[1].ShowIfConditionGroups)
	}
	if elements[2].Id != nil {
		t.Errorf("expected a new text element with an unknown ID not to send one")
	}

	validate := model.ToValidatePayload(types.StringUnknown())
	if validate.Id != nil {
		t.Errorf("expected an unknown ID to validate as a new form")
	}
	validate = model.ToValidatePayload(types.StringValue("form"))
	if lo.FromPtr(validate.Id) != "form" {
		t.Errorf("expected a known ID to validate as a replacement")
	}
}
