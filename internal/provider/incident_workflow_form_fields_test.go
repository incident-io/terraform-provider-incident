package provider

import (
	"testing"

	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// TestWorkflowFormFieldsRoundTrip exercises the conversion of workflow form
// fields from the API response type into the Terraform model
// (buildFormFields) and back into the create/update payload type
// (toPayloadFormFields), including the optional default_value param binding.
func TestWorkflowFormFieldsRoundTrip(t *testing.T) {
	apiFields := []client.WorkflowFormFieldV2{
		{
			Id:          "01FCNDV6P870EA6S7TK1DSYDG0",
			Name:        "Affected customer",
			Type:        "Text",
			Array:       false,
			Required:    true,
			Description: lo.ToPtr("The customer impacted by this incident"),
			Placeholder: lo.ToPtr("Select a customer"),
			DefaultValue: &client.EngineParamBindingV2{
				Value: &client.EngineParamBindingValueV2{
					Literal: lo.ToPtr("ACME Inc"),
				},
			},
		},
		{
			Id:       "01FCNDV6P870EA6S7TK1DSYDG1",
			Name:     "Notify oncall",
			Type:     "Bool",
			Array:    true,
			Required: false,
		},
	}

	model := buildFormFields(&apiFields)
	if len(model) != 2 {
		t.Fatalf("expected 2 form fields, got %d", len(model))
	}

	first := model[0]
	if first.ID.ValueString() != "01FCNDV6P870EA6S7TK1DSYDG0" {
		t.Errorf("unexpected id: %s", first.ID.ValueString())
	}
	if first.Name.ValueString() != "Affected customer" {
		t.Errorf("unexpected name: %s", first.Name.ValueString())
	}
	if first.Type.ValueString() != "Text" {
		t.Errorf("unexpected type: %s", first.Type.ValueString())
	}
	if first.Required.ValueBool() != true {
		t.Errorf("expected required to be true")
	}
	if first.Array.ValueBool() != false {
		t.Errorf("expected array to be false")
	}
	if first.Description.ValueString() != "The customer impacted by this incident" {
		t.Errorf("unexpected description: %s", first.Description.ValueString())
	}
	if first.Placeholder.ValueString() != "Select a customer" {
		t.Errorf("unexpected placeholder: %s", first.Placeholder.ValueString())
	}
	if first.DefaultValue == nil {
		t.Fatalf("expected default_value to be set")
	}
	if first.DefaultValue.Value == nil || first.DefaultValue.Value.Literal.ValueString() != "ACME Inc" {
		t.Errorf("unexpected default_value literal")
	}

	// Optional fields that were absent should be null.
	second := model[1]
	if !second.Description.IsNull() {
		t.Errorf("expected description to be null when absent")
	}
	if !second.Placeholder.IsNull() {
		t.Errorf("expected placeholder to be null when absent")
	}
	if second.DefaultValue != nil {
		t.Errorf("expected default_value to be nil when absent")
	}

	// Convert back to the payload type and assert the round trip preserves values.
	payloadPtr := toPayloadFormFields(model)
	if payloadPtr == nil {
		t.Fatalf("expected payload to be non-nil")
	}
	payload := *payloadPtr
	if len(payload) != 2 {
		t.Fatalf("expected 2 payload form fields, got %d", len(payload))
	}
	if lo.FromPtr(payload[0].Id) != "01FCNDV6P870EA6S7TK1DSYDG0" {
		t.Errorf("unexpected payload id: %v", payload[0].Id)
	}
	if payload[0].Name != "Affected customer" {
		t.Errorf("unexpected payload name: %s", payload[0].Name)
	}
	if payload[0].Type != "Text" {
		t.Errorf("unexpected payload type: %s", payload[0].Type)
	}
	if !payload[0].Required {
		t.Errorf("expected payload required to be true")
	}
	if lo.FromPtr(payload[0].Description) != "The customer impacted by this incident" {
		t.Errorf("unexpected payload description")
	}
	if payload[0].DefaultValue == nil || payload[0].DefaultValue.Value == nil ||
		lo.FromPtr(payload[0].DefaultValue.Value.Literal) != "ACME Inc" {
		t.Errorf("unexpected payload default_value")
	}
	if payload[1].DefaultValue != nil {
		t.Errorf("expected payload default_value to be nil when absent")
	}
}

// TestWorkflowFormFieldsNilRoundTrip ensures that a workflow with no form
// fields configured stays nil, so we don't send an empty list to the API or
// introduce spurious diffs.
func TestWorkflowFormFieldsNilRoundTrip(t *testing.T) {
	if got := buildFormFields(nil); got != nil {
		t.Errorf("expected nil model for nil API form fields, got %#v", got)
	}
	if got := toPayloadFormFields(nil); got != nil {
		t.Errorf("expected nil payload for nil model form fields, got %#v", got)
	}
}
