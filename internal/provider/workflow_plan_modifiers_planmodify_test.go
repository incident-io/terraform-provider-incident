package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// formFieldStateValue builds one form field as it appears in state, with the id
// the API assigned it.
func formFieldStateValue(id, key string) tftypes.Value {
	return tftypes.NewValue(formFieldObjectType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, id),
		"key":         tftypes.NewValue(tftypes.String, key),
		"title":       tftypes.NewValue(tftypes.String, key),
		"type":        tftypes.NewValue(tftypes.String, "Text"),
		"description": tftypes.NewValue(tftypes.String, nil),
		"array":       tftypes.NewValue(tftypes.Bool, false),
		"required":    tftypes.NewValue(tftypes.Bool, true),
	})
}

// workflowRawValue wraps a form_fields list in an otherwise-null workflow object.
func workflowRawValue(t *testing.T, objType tftypes.Object, formFields tftypes.Value) tftypes.Value {
	t.Helper()

	attributes := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attrType, nil)
	}
	attributes["form_fields"] = formFields

	return tftypes.NewValue(objType, attributes)
}

// TestFormFieldIDPlanModify drives formFieldIDPlanModifier the way the framework
// does, to check the id it plans for a field at a given index.
//
// The setup reproduces what Terraform hands a plan modifier: it fills a null
// computed attribute from the prior state at the *same index* before modifiers
// run, so the id arriving in the plan is whichever field used to sit there.
// That's the value the modifier has to correct.
func TestFormFieldIDPlanModify(t *testing.T) {
	var schemaResp resource.SchemaResponse
	NewIncidentWorkflowResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}
	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}

	// Two fields were applied: reason at index 0, responders at index 1.
	priorFields := formFieldsValue(
		formFieldStateValue("01A", "reason"),
		formFieldStateValue("01B", "responders"),
	)

	// modifyAt runs the modifier for the field at index, given the planned list
	// and the id Terraform prefilled positionally.
	modifyAt := func(t *testing.T, plannedFields tftypes.Value, index int, prefilledID types.String) planmodifier.StringResponse {
		t.Helper()

		req := planmodifier.StringRequest{
			Path:        path.Root("form_fields").AtListIndex(index).AtName("id"),
			ConfigValue: types.StringNull(), // id is Computed, so config never sets it.
			PlanValue:   prefilledID,
			State: tfsdk.State{
				Schema: schemaResp.Schema,
				Raw:    workflowRawValue(t, objType, priorFields),
			},
			Plan: tfsdk.Plan{
				Schema: schemaResp.Schema,
				Raw:    workflowRawValue(t, objType, plannedFields),
			},
		}
		resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
		formFieldIDPlanModifier{}.PlanModifyString(context.Background(), req, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %+v", resp.Diagnostics)
		}

		return resp
	}

	t.Run("a surviving field keeps its own id when an earlier one is deleted", func(t *testing.T) {
		// reason is gone, so responders moves to index 0 and inherits reason's 01A.
		resp := modifyAt(t, formFieldsValue(formFieldStateValue("01A", "responders")), 0, types.StringValue("01A"))
		if got := resp.PlanValue; got.ValueString() != "01B" {
			t.Errorf("planned id = %v, want 01B: ids must follow the key, not the position", got)
		}
	})

	t.Run("a field inserted at the front gets a fresh id", func(t *testing.T) {
		// impact is new at index 0, and inherits reason's 01A.
		planned := formFieldsValue(
			formFieldStateValue("01A", "impact"),
			formFieldStateValue("01B", "reason"),
		)
		resp := modifyAt(t, planned, 0, types.StringValue("01A"))
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("planned id = %v, want unknown so the API assigns one", resp.PlanValue)
		}
	})

	t.Run("a shifted field keeps its own id", func(t *testing.T) {
		// reason moved to index 1, where it inherited responders' 01B.
		planned := formFieldsValue(
			formFieldStateValue("01A", "impact"),
			formFieldStateValue("01B", "reason"),
		)
		resp := modifyAt(t, planned, 1, types.StringValue("01B"))
		if got := resp.PlanValue; got.ValueString() != "01A" {
			t.Errorf("planned id = %v, want 01A", got)
		}
	})

	t.Run("an unmoved field keeps its id", func(t *testing.T) {
		resp := modifyAt(t, priorFields, 1, types.StringValue("01B"))
		if got := resp.PlanValue; got.ValueString() != "01B" {
			t.Errorf("planned id = %v, want 01B", got)
		}
	})

	t.Run("a configured id is left alone", func(t *testing.T) {
		// id is Computed today, so this can't happen — but if it's ever made
		// Optional, a value the user wrote must win over key correlation.
		req := planmodifier.StringRequest{
			Path:        path.Root("form_fields").AtListIndex(0).AtName("id"),
			ConfigValue: types.StringUnknown(), // e.g. a reference to another resource
			PlanValue:   types.StringUnknown(),
			State: tfsdk.State{
				Schema: schemaResp.Schema,
				Raw:    workflowRawValue(t, objType, priorFields),
			},
			Plan: tfsdk.Plan{
				Schema: schemaResp.Schema,
				Raw:    workflowRawValue(t, objType, formFieldsValue(formFieldStateValue("01A", "responders"))),
			},
		}
		resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
		formFieldIDPlanModifier{}.PlanModifyString(context.Background(), req, &resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("planned id = %v, want the configured unknown left untouched", resp.PlanValue)
		}
	})

	t.Run("creating the workflow leaves every id unknown", func(t *testing.T) {
		req := planmodifier.StringRequest{
			Path:        path.Root("form_fields").AtListIndex(0).AtName("id"),
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			State:       tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, nil)},
			Plan: tfsdk.Plan{
				Schema: schemaResp.Schema,
				Raw:    workflowRawValue(t, objType, formFieldsValue(formFieldStateValue("01A", "reason"))),
			},
		}
		resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
		formFieldIDPlanModifier{}.PlanModifyString(context.Background(), req, &resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("planned id = %v, want unknown on create", resp.PlanValue)
		}
	})
}
