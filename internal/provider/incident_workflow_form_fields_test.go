package provider

import (
	"testing"

	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// TestWorkflowFormFieldsRoundTrip exercises the conversion of workflow form
// fields from the API response type into the Terraform model
// (buildFormFields) and back into the create/update payload type
// (toPayloadFormFields).
func TestWorkflowFormFieldsRoundTrip(t *testing.T) {
	apiFields := []client.WorkflowFormFieldV2{
		{
			Id:          "01FCNDV6P870EA6S7TK1DSYDG0",
			Key:         "affected_customer",
			Title:       "Affected customer",
			Type:        "User",
			Array:       true,
			Required:    true,
			Description: lo.ToPtr("The customer affected by this incident"),
		},
		{
			Id:       "01FCNDV6P870EA6S7TK1DSYDG1",
			Key:      "reason",
			Title:    "Reason",
			Type:     "Text",
			Array:    false,
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
	if first.Key.ValueString() != "affected_customer" {
		t.Errorf("unexpected key: %s", first.Key.ValueString())
	}
	if first.Title.ValueString() != "Affected customer" {
		t.Errorf("unexpected title: %s", first.Title.ValueString())
	}
	if first.Type.ValueString() != "User" {
		t.Errorf("unexpected type: %s", first.Type.ValueString())
	}
	if !first.Array.ValueBool() {
		t.Errorf("expected array to be true")
	}
	if !first.Required.ValueBool() {
		t.Errorf("expected required to be true")
	}
	if first.Description.ValueString() != "The customer affected by this incident" {
		t.Errorf("unexpected description: %s", first.Description.ValueString())
	}

	// Optional description that was absent should be null.
	second := model[1]
	if !second.Description.IsNull() {
		t.Errorf("expected description to be null when absent")
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
	if payload[0].Key != "affected_customer" {
		t.Errorf("unexpected payload key: %s", payload[0].Key)
	}
	if payload[0].Title != "Affected customer" {
		t.Errorf("unexpected payload title: %s", payload[0].Title)
	}
	if payload[0].Type != "User" {
		t.Errorf("unexpected payload type: %s", payload[0].Type)
	}
	if !lo.FromPtr(payload[0].Array) {
		t.Errorf("expected payload array to be true")
	}
	if !lo.FromPtr(payload[0].Required) {
		t.Errorf("expected payload required to be true")
	}
	if lo.FromPtr(payload[0].Description) != "The customer affected by this incident" {
		t.Errorf("unexpected payload description")
	}
	if payload[1].Description != nil {
		t.Errorf("expected payload description to be nil when absent")
	}
}

// TestWorkflowFormFieldsNilRoundTrip ensures that a workflow with no form
// fields configured stays nil, so we don't send an empty list to the API or
// introduce spurious diffs. The API returning an empty array is also treated
// as nil to avoid a perpetual diff against unset config.
func TestWorkflowFormFieldsNilRoundTrip(t *testing.T) {
	if got := buildFormFields(nil); got != nil {
		t.Errorf("expected nil model for nil API form fields, got %#v", got)
	}
	if got := buildFormFields(&[]client.WorkflowFormFieldV2{}); got != nil {
		t.Errorf("expected nil model for empty API form fields, got %#v", got)
	}
	if got := toPayloadFormFields(nil); got != nil {
		t.Errorf("expected nil payload for nil model form fields, got %#v", got)
	}
}

// TestWorkflowFormFieldsEmptyListStaysEmpty covers `form_fields = []`. The API
// can't tell an empty list apart from no form fields, so buildFormFields
// collapses both to nil; reconcileFormFields has to put the empty list back when
// that's what the user actually planned, or Terraform fails the apply with
// "Provider produced inconsistent result after apply".
func TestWorkflowFormFieldsEmptyListStaysEmpty(t *testing.T) {
	empty := []IncidentWorkflowFormField{}

	// An explicit empty list survives an API response with no form fields.
	got := reconcileFormFields(empty, buildFormFields(&[]client.WorkflowFormFieldV2{}))
	if got == nil {
		t.Errorf("expected empty list to stay empty, got nil")
	} else if len(got) != 0 {
		t.Errorf("expected 0 form fields, got %d", len(got))
	}

	// ...and an omitted attribute stays null rather than becoming an empty list.
	if got := reconcileFormFields(nil, buildFormFields(nil)); got != nil {
		t.Errorf("expected unset form fields to stay nil, got %#v", got)
	}

	// An empty list still sends an empty array, so the API clears any existing fields.
	payload := toPayloadFormFields(empty)
	if payload == nil {
		t.Fatalf("expected empty model to produce a non-nil payload, so the API clears fields")
	}
	if len(*payload) != 0 {
		t.Errorf("expected empty payload, got %d fields", len(*payload))
	}

	built := buildFormFields(&[]client.WorkflowFormFieldV2{{
		Id: "01FCNDV6P870EA6S7TK1DSYDG0", Key: "reason", Title: "Reason", Type: "Text",
	}})

	// Omitting form_fields leaves API fields unmanaged: keep state null even
	// when the API still has fields (payload omitted them, so they weren't cleared).
	if got := reconcileFormFields(nil, built); got != nil {
		t.Errorf("expected omitted form fields to stay nil when API still has fields, got %#v", got)
	}

	// When form_fields is set, fields returned by the API win over the prior value.
	if got := reconcileFormFields(empty, built); len(got) != 1 {
		t.Errorf("expected API form fields to win, got %d", len(got))
	}
}
