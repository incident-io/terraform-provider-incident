package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// secretSchema builds the resource's schema, so the configs these tests assemble are the
// ones Terraform would assemble.
func secretSchema(t *testing.T) schema.Schema {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentSecretResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	return schemaResp.Schema
}

// secretConfig builds a config with every attribute null, then applies overrides.
func secretConfig(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Config {
	t.Helper()

	resourceSchema := secretSchema(t)
	objType, ok := resourceSchema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}

	values := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	for name, value := range overrides {
		if _, ok := objType.AttributeTypes[name]; !ok {
			t.Fatalf("override %q is not an attribute of the schema", name)
		}
		values[name] = value
	}

	return tfsdk.Config{
		Schema: resourceSchema,
		Raw:    tftypes.NewValue(objType, values),
	}
}

func TestSecretValidateConfig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		overrides map[string]tftypes.Value
		wantError string
	}{
		{
			name: "value and version together",
			overrides: map[string]tftypes.Value{
				"value_wo":         tftypes.NewValue(tftypes.String, "sk_live_abc123"),
				"value_wo_version": tftypes.NewValue(tftypes.Number, 1),
			},
		},
		{
			// Setting an initial value without a version is fine: it's a secret that has
			// never been rotated. Rotating it later means adding value_wo_version.
			name: "value without a version",
			overrides: map[string]tftypes.Value{
				"value_wo": tftypes.NewValue(tftypes.String, "sk_live_abc123"),
			},
		},
		{
			// Managing a secret's metadata without owning its value.
			name:      "neither set",
			overrides: map[string]tftypes.Value{},
		},
		{
			name: "version without a value",
			overrides: map[string]tftypes.Value{
				"value_wo_version": tftypes.NewValue(tftypes.Number, 2),
			},
			wantError: "value_wo_version is set but value_wo is not",
		},
		{
			// A value taken from another resource isn't known until apply, so there's
			// nothing to judge yet and the config must not be rejected.
			name: "unknown value",
			overrides: map[string]tftypes.Value{
				"value_wo":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				"value_wo_version": tftypes.NewValue(tftypes.Number, 1),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var resp resource.ValidateConfigResponse
			(&IncidentSecretResource{}).ValidateConfig(
				context.Background(),
				resource.ValidateConfigRequest{Config: secretConfig(t, tc.overrides)},
				&resp,
			)

			if tc.wantError == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("expected no error, got: %+v", resp.Diagnostics)
				}

				return
			}

			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected an error containing %q, got none", tc.wantError)
			}
			if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), tc.wantError) {
				t.Fatalf("expected an error containing %q, got: %+v", tc.wantError, resp.Diagnostics)
			}
		})
	}
}

// TestSecretNameRejectsPaddedName covers the name validator: incident.io trims a name
// before storing it, so a padded one would read back as a different string and fail the
// post-apply consistency check. It has to be rejected at plan time instead.
func TestSecretNameRejectsPaddedName(t *testing.T) {
	t.Parallel()

	nameAttribute, ok := secretSchema(t).Attributes["name"].(schema.StringAttribute)
	if !ok {
		t.Fatal("name is not a string attribute")
	}

	for _, tc := range []struct {
		name      string
		value     string
		wantError bool
	}{
		{name: "plain", value: "PagerDuty webhook token"},
		{name: "single character", value: "x"},
		{name: "leading whitespace", value: " PagerDuty webhook token", wantError: true},
		{name: "trailing newline", value: "PagerDuty webhook token\n", wantError: true},
		{name: "only whitespace", value: "   ", wantError: true},
		{name: "empty", value: "", wantError: true},
		// The API caps the name at 1024 bytes, so the provider says so at plan time rather
		// than letting the create fail.
		{name: "at the byte limit", value: strings.Repeat("a", secretNameMaxLengthBytes)},
		{name: "one byte over", value: strings.Repeat("a", secretNameMaxLengthBytes+1), wantError: true},
		{
			// The limit counts bytes, so a name of multi-byte characters hits it at far
			// fewer characters than 1024. This one is 1026 bytes in 342 characters.
			name:      "under the limit in characters but over it in bytes",
			value:     strings.Repeat("日", 342),
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var resp validator.StringResponse
			for _, nameValidator := range nameAttribute.Validators {
				nameValidator.ValidateString(
					context.Background(),
					validator.StringRequest{ConfigValue: types.StringValue(tc.value)},
					&resp,
				)
			}

			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Fatalf("value %q: expected error=%v, got error=%v (%+v)", tc.value, tc.wantError, got, resp.Diagnostics)
			}
		})
	}
}
