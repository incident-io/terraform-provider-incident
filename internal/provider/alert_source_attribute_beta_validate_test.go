package provider

import (
	"context"
	"maps"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func alertSourceAttributeSchemaType(t *testing.T) (tfsdk.Config, tftypes.Object) {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewAlertSourceAttributeBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}

	return tfsdk.Config{Schema: schemaResp.Schema}, objType
}

// alertSourceAttributeConfig builds a config with the two ids set and everything else null,
// then applies overrides — the ids being on every valid config, and never what is under test.
func alertSourceAttributeConfig(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Config {
	t.Helper()

	config, objType := alertSourceAttributeSchemaType(t)

	values := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	values["alert_source_id"] = stringValue("01ALERTSOURCE")
	values["alert_attribute_id"] = stringValue("01ALERTATTRIBUTE")

	for name, value := range overrides {
		if _, ok := objType.AttributeTypes[name]; !ok {
			t.Fatalf("override %q is not an attribute of the schema", name)
		}
		values[name] = value
	}

	config.Raw = tftypes.NewValue(objType, values)

	return config
}

func alertSourceAttributeType(t *testing.T, name string) tftypes.Type {
	t.Helper()

	_, objType := alertSourceAttributeSchemaType(t)
	attrType, ok := objType.AttributeTypes[name]
	if !ok {
		t.Fatalf("schema has no attribute %q", name)
	}

	return attrType
}

// expressionBlockValue builds the smallest expression the grammar accepts, so a test setting
// the block is testing the block being set rather than what is in it.
func expressionBlockValue(t *testing.T, blockType tftypes.Type, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	object, ok := blockType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected an object type, got %s", blockType)
	}

	operations, ok := object.AttributeTypes["operation"].(tftypes.List)
	if !ok {
		t.Fatalf("expected operation to be a list, got %s", object.AttributeTypes["operation"])
	}
	operation, ok := operations.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected an operation object, got %s", operations.ElementType)
	}

	set := map[string]tftypes.Value{
		"start_from": stringValue("payload"),
		"operation": tftypes.NewValue(operations, []tftypes.Value{
			objectWith(t, operation, map[string]tftypes.Value{
				"cast": objectWith(t, operation.AttributeTypes["cast"], map[string]tftypes.Value{
					"as": stringValue("Text"),
				}),
			}),
		}),
	}
	maps.Copy(set, overrides)

	return objectWith(t, object, set)
}

// namedExpressionsValue is a one-entry named_expression list under the given name.
func namedExpressionsValue(t *testing.T, name string) tftypes.Value {
	t.Helper()

	blockType := alertSourceAttributeType(t, "named_expression")
	list, ok := blockType.(tftypes.List)
	if !ok {
		t.Fatalf("expected named_expression to be a list, got %s", blockType)
	}

	return tftypes.NewValue(list, []tftypes.Value{
		expressionBlockValue(t, list.ElementType, map[string]tftypes.Value{
			"name": stringValue(name),
		}),
	})
}

func validateAlertSourceAttributeBeta(t *testing.T, config tfsdk.Config) diag.Diagnostics {
	t.Helper()

	r, ok := NewAlertSourceAttributeBetaResource().(*alertSourceAttributeBetaResource)
	if !ok {
		t.Fatalf("NewAlertSourceAttributeBetaResource did not return a *alertSourceAttributeBetaResource")
	}

	resp := resource.ValidateConfigResponse{}
	r.ValidateConfig(
		context.Background(),
		resource.ValidateConfigRequest{Config: config},
		&resp,
	)

	return resp.Diagnostics
}

// TestAlertSourceAttributeBetaValidateValue covers the exclusive group, which spans a block and
// its sibling attributes and so can't be a schema validator.
func TestAlertSourceAttributeBetaValidateValue(t *testing.T) {
	expressionType := alertSourceAttributeType(t, "expression")

	t.Run("rejects an attribute nothing fills in", func(t *testing.T) {
		diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, nil))

		assertErrorContaining(t, diags, "Missing value")
	})

	t.Run("rejects a value and an expression block together", func(t *testing.T) {
		diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, map[string]tftypes.Value{
			"value_literal": stringValue("high"),
			"expression":    expressionBlockValue(t, expressionType, nil),
		}))

		assertErrorContaining(t, diags, "Two values for one attribute")
	})

	t.Run("accepts a value on its own", func(t *testing.T) {
		diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, map[string]tftypes.Value{
			"value_literal": stringValue("high"),
		}))

		if diags.HasError() {
			t.Fatalf("expected no error, got %+v", diags.Errors())
		}
	})

	t.Run("accepts an expression block on its own", func(t *testing.T) {
		diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, map[string]tftypes.Value{
			"expression": expressionBlockValue(t, expressionType, nil),
		}))

		if diags.HasError() {
			t.Fatalf("expected no error, got %+v", diags.Errors())
		}
	})
}

// TestAlertSourceAttributeBetaValidateMergeStrategy reads the allowed values from the API
// schema, so the check can't fall behind the API and start rejecting one it has since accepted.
func TestAlertSourceAttributeBetaValidateMergeStrategy(t *testing.T) {
	t.Run("rejects a strategy that isn't one", func(t *testing.T) {
		diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, map[string]tftypes.Value{
			"value_literal":  stringValue("high"),
			"merge_strategy": stringValue("latest_wins"),
		}))

		assertErrorContaining(t, diags, "Unknown merge_strategy")
	})

	t.Run("accepts every strategy the API lists", func(t *testing.T) {
		for _, strategy := range alertSourceAttributeMergeStrategies {
			diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, map[string]tftypes.Value{
				"value_literal":  stringValue("high"),
				"merge_strategy": stringValue(strategy),
			}))

			assertNoErrorContaining(t, diags, "merge_strategy")
		}
	})
}

// TestAlertSourceAttributeBetaValidateExpressions checks the shared expression checks run
// against this resource's paths. The checks themselves are covered in the models package.
func TestAlertSourceAttributeBetaValidateExpressions(t *testing.T) {
	t.Run("rejects an expression_ref naming nothing", func(t *testing.T) {
		diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, map[string]tftypes.Value{
			"expression_ref": stringValue("severity"),
		}))

		assertErrorContaining(t, diags, "Unknown expression_ref")
	})

	t.Run("accepts an expression_ref naming a named_expression", func(t *testing.T) {
		diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, map[string]tftypes.Value{
			"expression_ref":   stringValue("severity"),
			"named_expression": namedExpressionsValue(t, "severity"),
		}))

		if diags.HasError() {
			t.Fatalf("expected no error, got %+v", diags.Errors())
		}
	})

	// A named_expression taking the local name of the unnamed block would be stored under its
	// reference.
	t.Run("rejects a named_expression using the name kept for the unnamed block", func(t *testing.T) {
		diags := validateAlertSourceAttributeBeta(t, alertSourceAttributeConfig(t, map[string]tftypes.Value{
			"value_literal":    stringValue("high"),
			"named_expression": namedExpressionsValue(t, "_bound"),
		}))

		assertErrorContaining(t, diags, "Reserved name")
	})
}
