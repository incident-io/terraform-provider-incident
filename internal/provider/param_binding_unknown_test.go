package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A param binding attribute a config points at anything other than a literal - a local, a
// for, a ternary, each.value - arrives unknown while Terraform validates the config, which
// it does before it has evaluated any of them. Reading such a config used to fail outright
// with "Received unknown value, however the target type cannot handle unknown values",
// because the model held these three as Go slices and pointers.
//
// The forms are covered together because they share one cause: value_literal and
// value_reference were always framework types and never had the problem.
var unknownBindingForms = []string{"array_value", "value", "values"}

// bindingWithUnknown builds one param binding object with field set to unknown and every
// other attribute null.
func bindingWithUnknown(t *testing.T, bindingType tftypes.Object, field string) tftypes.Value {
	t.Helper()

	attributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, bindingType).As(&attributes); err != nil {
		t.Fatalf("reading the binding back: %v", err)
	}

	fieldType, ok := bindingType.AttributeTypes[field]
	if !ok {
		t.Fatalf("the binding has no %s attribute", field)
	}
	attributes[field] = tftypes.NewValue(fieldType, tftypes.UnknownValue)

	return tftypes.NewValue(bindingType, attributes)
}

// conditionWithUnknownBinding builds one condition whose single param binding is unknown in
// the named form.
func conditionWithUnknownBinding(t *testing.T, conditionsType tftypes.List, subject, field string) tftypes.Value {
	t.Helper()

	conditionType, ok := conditionsType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("expected conditions to hold objects")
	}
	bindingsType, ok := conditionType.AttributeTypes["param_bindings"].(tftypes.List)
	if !ok {
		t.Fatal("expected param_bindings to be a list")
	}
	bindingType, ok := bindingsType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("expected param_bindings to hold objects")
	}

	attributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, conditionType).As(&attributes); err != nil {
		t.Fatalf("reading the condition back: %v", err)
	}
	attributes["subject"] = tftypes.NewValue(tftypes.String, subject)
	attributes["operation"] = tftypes.NewValue(tftypes.String, "one_of")
	attributes["param_bindings"] = tftypes.NewValue(bindingsType, []tftypes.Value{
		bindingWithUnknown(t, bindingType, field),
	})

	return tftypes.NewValue(conditionType, attributes)
}

// TestEscalationPathValidatesAnUnknownConditionBinding is the reported failure: an
// escalation path whose if_else condition binds `array_value = local.priorities` (or any
// other expression) failed to plan at all, with a value conversion error naming
// conditions[0].param_bindings[0].array_value.
func TestEscalationPathValidatesAnUnknownConditionBinding(t *testing.T) {
	for _, field := range unknownBindingForms {
		t.Run(field, func(t *testing.T) {
			objType := escalationPathSchemaType(t)

			pathType := objType.AttributeTypes["path"].(tftypes.List)
			nodeType := pathType.ElementType.(tftypes.Object)
			ifElseType := nodeType.AttributeTypes["if_else"].(tftypes.Object)
			conditionsType := ifElseType.AttributeTypes["conditions"].(tftypes.List)

			condition := conditionWithUnknownBinding(t, conditionsType, "escalation.priority", field)

			ifElseAttributes := map[string]tftypes.Value{}
			if err := nullFilledObject(t, ifElseType).As(&ifElseAttributes); err != nil {
				t.Fatalf("reading if_else back: %v", err)
			}
			ifElseAttributes["conditions"] = tftypes.NewValue(conditionsType, []tftypes.Value{condition})
			ifElseAttributes["then_path"] = tftypes.NewValue(ifElseType.AttributeTypes["then_path"], []tftypes.Value{})
			ifElseAttributes["else_path"] = tftypes.NewValue(ifElseType.AttributeTypes["else_path"], []tftypes.Value{})

			nodeAttributes := map[string]tftypes.Value{}
			if err := nullFilledObject(t, nodeType).As(&nodeAttributes); err != nil {
				t.Fatalf("reading the node back: %v", err)
			}
			nodeAttributes["type"] = tftypes.NewValue(tftypes.String, "if_else")
			nodeAttributes["if_else"] = tftypes.NewValue(ifElseType, ifElseAttributes)
			// nullFilledObject gives every nested block an object of nulls, which the
			// node/block agreement check reads as a block that is set.
			nodeAttributes["escalation_path"] = tftypes.NewValue(nodeType.AttributeTypes["escalation_path"], nil)

			attributes := map[string]tftypes.Value{}
			for name, attrType := range objType.AttributeTypes {
				attributes[name] = tftypes.NewValue(attrType, nil)
			}
			attributes["name"] = tftypes.NewValue(tftypes.String, "Paged by priority")
			attributes["path"] = tftypes.NewValue(pathType, []tftypes.Value{
				tftypes.NewValue(nodeType, nodeAttributes),
			})

			var schemaResp resource.SchemaResponse
			NewIncidentEscalationPathResource().Schema(t.Context(), resource.SchemaRequest{}, &schemaResp)

			var resp resource.ValidateConfigResponse
			(&IncidentEscalationPathResource{}).ValidateConfig(t.Context(), resource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, attributes)},
			}, &resp)

			assertNoDiagErrors(t, resp.Diagnostics)
		})
	}
}

// TestConditionGroupsReadAnUnknownBinding covers the resources that hold condition groups in
// their top-level model. Those fail earlier than the escalation path does - inside
// Config.Get itself, which takes no options - so nothing but the model's types can fix them.
func TestConditionGroupsReadAnUnknownBinding(t *testing.T) {
	for name, tc := range map[string]struct {
		schema    func(*testing.T) resource.SchemaResponse
		attribute string
		subject   string
		model     func() any
	}{
		"workflow": {
			schema:    resourceSchema(NewIncidentWorkflowResource),
			attribute: "condition_groups",
			subject:   "incident.severity",
			model:     func() any { return &IncidentWorkflowResourceModel{} },
		},
		"policy": {
			schema:    resourceSchema(NewIncidentPolicyResource),
			attribute: "condition_groups",
			subject:   "incident.severity",
			model:     func() any { return &incidentPolicyResourceModel{} },
		},
		"maintenance window": {
			schema:    resourceSchema(NewIncidentMaintenanceWindowResource),
			attribute: "alert_condition_groups",
			subject:   "alert.priority",
			model:     func() any { return &MaintenanceWindowResourceModel{} },
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, field := range unknownBindingForms {
				t.Run(field, func(t *testing.T) {
					schemaResp := tc.schema(t)
					objType := schemaResp.Schema.Type().TerraformType(t.Context()).(tftypes.Object)

					groupsType, ok := objType.AttributeTypes[tc.attribute].(tftypes.List)
					if !ok {
						t.Fatalf("expected %s to be a list", tc.attribute)
					}
					groupType := groupsType.ElementType.(tftypes.Object)
					conditionsType := groupType.AttributeTypes["conditions"].(tftypes.List)

					condition := conditionWithUnknownBinding(t, conditionsType, tc.subject, field)

					groupAttributes := map[string]tftypes.Value{}
					if err := nullFilledObject(t, groupType).As(&groupAttributes); err != nil {
						t.Fatalf("reading the group back: %v", err)
					}
					groupAttributes["conditions"] = tftypes.NewValue(conditionsType, []tftypes.Value{condition})

					attributes := map[string]tftypes.Value{}
					for attrName, attrType := range objType.AttributeTypes {
						attributes[attrName] = tftypes.NewValue(attrType, nil)
					}
					attributes["name"] = tftypes.NewValue(tftypes.String, "Test")
					attributes[tc.attribute] = tftypes.NewValue(groupsType, []tftypes.Value{
						tftypes.NewValue(groupType, groupAttributes),
					})

					config := tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objType, attributes)}

					model := tc.model()
					assertNoDiagErrors(t, config.Get(t.Context(), model))
				})
			}
		})
	}
}

// resourceSchema builds a resource's schema, for the table above.
func resourceSchema(new func() resource.Resource) func(*testing.T) resource.SchemaResponse {
	return func(t *testing.T) resource.SchemaResponse {
		t.Helper()

		var resp resource.SchemaResponse
		new().Schema(t.Context(), resource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("building the schema: %+v", resp.Diagnostics)
		}

		return resp
	}
}

func assertNoDiagErrors(t *testing.T, diags diag.Diagnostics) {
	t.Helper()

	for _, d := range diags.Errors() {
		t.Errorf("%s: %s", d.Summary(), d.Detail())
	}
}
