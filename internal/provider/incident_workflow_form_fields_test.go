package provider

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
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

// TestWorkflowFormFieldsNilRoundTrip ensures that a workflow with no form fields
// configured stays nil in the model, so an unset attribute doesn't show a
// perpetual diff against an API that reports no fields — whether it says so with
// a nil list or an empty one.
func TestWorkflowFormFieldsNilRoundTrip(t *testing.T) {
	if got := buildFormFields(nil); got != nil {
		t.Errorf("expected nil model for nil API form fields, got %#v", got)
	}
	if got := buildFormFields(&[]client.WorkflowFormFieldV2{}); got != nil {
		t.Errorf("expected nil model for empty API form fields, got %#v", got)
	}
	if got := reconcileFormFields(nil, buildFormFields(nil)); got != nil {
		t.Errorf("expected unset form fields to stay nil, got %#v", got)
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

	// Once the attribute is managed, fields returned by the API win over the prior value.
	built := buildFormFields(&[]client.WorkflowFormFieldV2{{
		Id: "01FCNDV6P870EA6S7TK1DSYDG0", Key: "reason", Title: "Reason", Type: "Text",
	}})
	if got := reconcileFormFields(empty, built); len(got) != 1 {
		t.Errorf("expected API form fields to win, got %d", len(got))
	}
}

// TestWorkflowFormFieldsOmittedClears covers omitting form_fields on a workflow
// that already has some. The payload has to carry an empty list so the API clears
// them: leaving the key out of the body instead would strand the fields outside
// Terraform's control, and hand back a state contradicting a plan of null, which
// fails the apply with "Provider produced inconsistent result after apply".
func TestWorkflowFormFieldsOmittedClears(t *testing.T) {
	payload := toPayloadFormFields(nil)
	if payload == nil {
		t.Fatalf("expected a non-nil payload for unset form fields, so the API clears them")
	}
	if len(*payload) != 0 {
		t.Errorf("expected an empty payload, got %d fields", len(*payload))
	}

	// A nil pointer would be omitted by omitempty; an empty slice must survive
	// encoding as `[]`, or the API has nothing telling it to clear.
	body, err := json.Marshal(struct {
		FormFields *[]client.WorkflowFormFieldPayloadV2 `json:"form_fields,omitempty"`
	}{FormFields: payload})
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	if want := `{"form_fields":[]}`; string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}

	// Having cleared them, the API reports none and state goes back to null.
	if got := reconcileFormFields(nil, buildFormFields(&[]client.WorkflowFormFieldV2{})); got != nil {
		t.Errorf("expected cleared form fields to read back as null, got %#v", got)
	}
}

// TestWorkflowFormFieldsUnsetIDOmitted checks that a field with no id yet is sent
// without one, so the API creates it. types.String.ValueStringPointer returns a
// pointer to "" for an unknown value, which omitempty happily encodes as
// `"id": ""` — an id the API would reject.
func TestWorkflowFormFieldsUnsetIDOmitted(t *testing.T) {
	for name, id := range map[string]types.String{
		"unknown": types.StringUnknown(),
		"null":    types.StringNull(),
	} {
		t.Run(name, func(t *testing.T) {
			payload := toPayloadFormFields([]IncidentWorkflowFormField{{
				ID:       id,
				Key:      types.StringValue("reason"),
				Title:    types.StringValue("Reason"),
				Type:     types.StringValue("Text"),
				Array:    types.BoolValue(false),
				Required: types.BoolValue(true),
			}})
			if payload == nil {
				t.Fatalf("expected a non-nil payload")
			}
			if got := (*payload)[0].Id; got != nil {
				t.Errorf("expected id to be omitted, got %q", *got)
			}

			// Belt and braces: the encoded body must not carry an id at all.
			body, err := json.Marshal((*payload)[0])
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}
			if bytes.Contains(body, []byte(`"id"`)) {
				t.Errorf("expected no id in the request body, got %s", body)
			}
		})
	}
}
