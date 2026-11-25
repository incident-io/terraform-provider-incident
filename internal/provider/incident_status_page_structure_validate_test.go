package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestStatusPageStructureValidateConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	NewIncidentStatusPageStructureResource().Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	itemsType, ok := objType.AttributeTypes["items"].(tftypes.List)
	if !ok {
		t.Fatal("items is not a list")
	}
	itemType, ok := itemsType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("an item is not an object")
	}
	groupType, ok := itemType.AttributeTypes["group"].(tftypes.Object)
	if !ok {
		t.Fatal("group is not an object")
	}

	// nullAll gives an object of the given type with every attribute null, then applies
	// overrides.
	nullAll := func(typ tftypes.Object, overrides map[string]tftypes.Value) tftypes.Value {
		values := map[string]tftypes.Value{}
		for name, attrType := range typ.AttributeTypes {
			values[name] = tftypes.NewValue(attrType, nil)
		}
		for name, value := range overrides {
			values[name] = value
		}
		return tftypes.NewValue(typ, values)
	}

	item := func(overrides map[string]tftypes.Value) tftypes.Value {
		return nullAll(itemType, overrides)
	}
	group := func(overrides map[string]tftypes.Value) tftypes.Value {
		return nullAll(groupType, overrides)
	}
	list := func(items ...tftypes.Value) tftypes.Value {
		return tftypes.NewValue(itemsType, items)
	}
	str := func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
	unknownString := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	componentsType := groupType.AttributeTypes["components"]

	for _, tc := range []struct {
		name      string
		items     tftypes.Value
		wantError string
	}{
		{
			name:  "an unknown items list is skipped",
			items: tftypes.NewValue(itemsType, tftypes.UnknownValue),
		},
		{
			name:  "an unknown item is skipped",
			items: list(tftypes.NewValue(itemType, tftypes.UnknownValue)),
		},
		{
			name:  "a null item is skipped",
			items: list(tftypes.NewValue(itemType, nil)),
		},
		{
			name:  "a component with an unknown id is fine",
			items: list(item(map[string]tftypes.Value{"component_id": unknownString})),
		},
		{
			name: "a group with unknown components is fine",
			items: list(item(map[string]tftypes.Value{
				"group": group(map[string]tftypes.Value{
					"name":       str("Core"),
					"components": tftypes.NewValue(componentsType, tftypes.UnknownValue),
				}),
			})),
		},
		{
			name: "an unknown group is not an empty item",
			items: list(item(map[string]tftypes.Value{
				"group": tftypes.NewValue(groupType, tftypes.UnknownValue),
			})),
		},
		{
			name: "display flags on a group still fail when its components are unknown",
			items: list(item(map[string]tftypes.Value{
				"hidden": tftypes.NewValue(tftypes.Bool, true),
				"group": group(map[string]tftypes.Value{
					"name":       str("Core"),
					"components": tftypes.NewValue(componentsType, tftypes.UnknownValue),
				}),
			})),
			wantError: "Display flags on a group item",
		},
		{
			name:      "an empty item still fails",
			items:     list(item(nil)),
			wantError: "Empty item",
		},
		{
			name: "a component and a group still fail",
			items: list(item(map[string]tftypes.Value{
				"component_id": str("01ABC"),
				"group":        group(map[string]tftypes.Value{"name": str("Core")}),
			})),
			wantError: "Ambiguous item",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := nullAll(objType, map[string]tftypes.Value{
				"status_page_id": str("01PAGE"),
				"items":          tc.items,
			})

			var resp resource.ValidateConfigResponse
			(&IncidentStatusPageStructureResource{}).ValidateConfig(
				ctx,
				resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}},
				&resp,
			)

			if tc.wantError == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected error: %+v", resp.Diagnostics)
				}
				return
			}

			for _, diag := range resp.Diagnostics.Errors() {
				if strings.Contains(diag.Summary(), tc.wantError) {
					return
				}
			}
			t.Fatalf("want an error containing %q, got %+v", tc.wantError, resp.Diagnostics)
		})
	}
}
