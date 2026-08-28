package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// formFieldObjectType mirrors the nested object in the form_fields schema.
var formFieldObjectType = tftypes.Object{
	AttributeTypes: map[string]tftypes.Type{
		"id":          tftypes.String,
		"key":         tftypes.String,
		"title":       tftypes.String,
		"type":        tftypes.String,
		"description": tftypes.String,
		"array":       tftypes.Bool,
		"required":    tftypes.Bool,
	},
}

// formFieldValue builds one configured form field. id is always null, matching a
// user's config: it's Computed, so they can't set it.
func formFieldValue(key string) tftypes.Value {
	return tftypes.NewValue(formFieldObjectType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nil),
		"key":         tftypes.NewValue(tftypes.String, key),
		"title":       tftypes.NewValue(tftypes.String, strings.ToTitle(key)),
		"type":        tftypes.NewValue(tftypes.String, "Text"),
		"description": tftypes.NewValue(tftypes.String, nil),
		"array":       tftypes.NewValue(tftypes.Bool, false),
		"required":    tftypes.NewValue(tftypes.Bool, true),
	})
}

func formFieldsValue(fields ...tftypes.Value) tftypes.Value {
	return tftypes.NewValue(tftypes.List{ElementType: formFieldObjectType}, fields)
}

// validateWorkflowFormFields runs the real resource's ValidateConfig against a
// config whose form_fields is the value under test and whose every other
// attribute is null, so only the form field checks have anything to say.
// overrides sets other attributes, for cases that need a second check to fire.
func validateWorkflowFormFields(t *testing.T, formFields tftypes.Value, overrides ...map[string]tftypes.Value) resource.ValidateConfigResponse {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentWorkflowResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}

	attributes := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attrType, nil)
	}
	if _, ok := attributes["form_fields"]; !ok {
		t.Fatalf("form_fields isn't an attribute on the workflow schema")
	}
	attributes["form_fields"] = formFields
	for _, override := range overrides {
		for name, value := range override {
			if _, ok := attributes[name]; !ok {
				t.Fatalf("override %q isn't an attribute on the workflow schema", name)
			}
			attributes[name] = value
		}
	}

	r, ok := NewIncidentWorkflowResource().(*IncidentWorkflowResource)
	if !ok {
		t.Fatalf("NewIncidentWorkflowResource did not return an *IncidentWorkflowResource")
	}

	var resp resource.ValidateConfigResponse
	r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(objType, attributes),
		},
	}, &resp)

	return resp
}

// TestWorkflowFormFieldKeysValidateConfig covers rejecting two form fields that
// share a key. A key has to be unique for form.<key> to resolve, and
// it's how a field is correlated with its prior id (see formFieldIDPlanModifier),
// so a duplicate would hand the same id to two fields.
func TestWorkflowFormFieldKeysValidateConfig(t *testing.T) {
	t.Run("distinct keys are valid", func(t *testing.T) {
		resp := validateWorkflowFormFields(t, formFieldsValue(
			formFieldValue("reason"), formFieldValue("responders"),
		))
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("a duplicate key is rejected", func(t *testing.T) {
		resp := validateWorkflowFormFields(t, formFieldsValue(
			formFieldValue("reason"), formFieldValue("responders"), formFieldValue("reason"),
		))
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error, got none")
		}
		if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "Duplicate form_fields key" {
			t.Errorf("unexpected diagnostic: %q", summary)
		}
	})

	t.Run("an unknown key is skipped rather than compared", func(t *testing.T) {
		unknown := tftypes.NewValue(formFieldObjectType, map[string]tftypes.Value{
			"id":          tftypes.NewValue(tftypes.String, nil),
			"key":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"title":       tftypes.NewValue(tftypes.String, "Reason"),
			"type":        tftypes.NewValue(tftypes.String, "Text"),
			"description": tftypes.NewValue(tftypes.String, nil),
			"array":       tftypes.NewValue(tftypes.Bool, false),
			"required":    tftypes.NewValue(tftypes.Bool, true),
		})
		resp := validateWorkflowFormFields(t, formFieldsValue(unknown, unknown))
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	// GetAttribute into []IncidentWorkflowFormField fails when the whole list is
	// unknown (a variable, or another resource). That conversion error must not
	// be reported as a config problem — the list isn't known enough to check yet.
	t.Run("an entirely unknown form_fields is skipped", func(t *testing.T) {
		unknown := tftypes.NewValue(tftypes.List{ElementType: formFieldObjectType}, tftypes.UnknownValue)
		resp := validateWorkflowFormFields(t, unknown)
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	// Each check judges its own diagnostics, so a duplicate key doesn't mask an
	// unrelated mistake elsewhere in the config.
	t.Run("a duplicate key and a bad scope both report", func(t *testing.T) {
		resp := validateWorkflowFormFields(t,
			formFieldsValue(formFieldValue("reason"), formFieldValue("reason")),
			map[string]tftypes.Value{
				"private_incident_scope": tftypes.NewValue(tftypes.String, "sometimes"),
			})
		if len(resp.Diagnostics.Errors()) != 2 {
			t.Errorf("expected both errors, got %+v", resp.Diagnostics)
		}
	})

	t.Run("no form fields is valid", func(t *testing.T) {
		for name, value := range map[string]tftypes.Value{
			"null":  tftypes.NewValue(tftypes.List{ElementType: formFieldObjectType}, nil),
			"empty": formFieldsValue(),
		} {
			t.Run(name, func(t *testing.T) {
				if resp := validateWorkflowFormFields(t, value); resp.Diagnostics.HasError() {
					t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
				}
			})
		}
	})
}

// TestWorkflowFormFieldIDIsComputedOnly pins the id attribute to Computed. Making
// it Optional would let a config supply an id, which formFieldIDPlanModifier
// would then have to defer to, and hand users a way to point two fields at the
// same one.
func TestWorkflowFormFieldIDIsComputedOnly(t *testing.T) {
	var schemaResp resource.SchemaResponse
	NewIncidentWorkflowResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	attr, diags := schemaResp.Schema.AttributeAtPath(
		context.Background(), path.Root("form_fields").AtListIndex(0).AtName("id"))
	if diags.HasError() {
		t.Fatalf("failed to look up form_fields id: %+v", diags)
	}
	if !attr.IsComputed() {
		t.Error("expected form_fields id to be computed")
	}
	if attr.IsOptional() {
		t.Error("expected form_fields id not to be optional: the API assigns it")
	}
}
