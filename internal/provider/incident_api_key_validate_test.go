package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// apiKeySchema builds the resource's schema, so the configs these tests assemble are the
// ones Terraform would assemble.
func apiKeySchema(t *testing.T) schema.Schema {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentAPIKeyResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	return schemaResp.Schema
}

// apiKeyObject builds an object with every attribute null, then applies overrides.
func apiKeyObject(t *testing.T, overrides map[string]tftypes.Value) (tftypes.Object, tftypes.Value) {
	t.Helper()

	objType, ok := apiKeySchema(t).Type().TerraformType(context.Background()).(tftypes.Object)
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

	return objType, tftypes.NewValue(objType, values)
}

// apiKeyConfig builds a config with every attribute null, then applies overrides.
func apiKeyConfig(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Config {
	t.Helper()

	_, raw := apiKeyObject(t, overrides)

	return tfsdk.Config{Schema: apiKeySchema(t), Raw: raw}
}

// stringSetValue builds a set of strings as Terraform would hold one.
func stringSetValue(values ...string) tftypes.Value {
	elements := make([]tftypes.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, tftypes.NewValue(tftypes.String, value))
	}

	return tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, elements)
}

func TestAPIKeyValidateConfig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		overrides   map[string]tftypes.Value
		wantError   string
		wantWarning string
	}{
		{
			// An account-scoped key: roles across the whole account, no teams.
			name: "account roles only",
			overrides: map[string]tftypes.Value{
				"role_names":      stringSetValue("viewer"),
				"team_ids":        stringSetValue(),
				"team_role_names": stringSetValue(),
			},
		},
		{
			// A team-scoped key: no account roles at all, which is why role_names has to be
			// allowed to be empty.
			name: "team roles only",
			overrides: map[string]tftypes.Value{
				"role_names":      stringSetValue(),
				"team_ids":        stringSetValue("01G0J1EXE7AXZ2C93K61WBPYEH"),
				"team_role_names": stringSetValue("schedules_editor"),
			},
		},
		{
			name: "both kinds of role",
			overrides: map[string]tftypes.Value{
				"role_names":      stringSetValue("viewer"),
				"team_ids":        stringSetValue("01G0J1EXE7AXZ2C93K61WBPYEH"),
				"team_role_names": stringSetValue("schedules_editor"),
			},
		},
		{
			// Everything left out, which the schema defaults to empty sets.
			name:      "nothing set",
			overrides: map[string]tftypes.Value{},
		},
		{
			name: "teams without team roles",
			overrides: map[string]tftypes.Value{
				"team_ids":        stringSetValue("01G0J1EXE7AXZ2C93K61WBPYEH"),
				"team_role_names": stringSetValue(),
			},
			wantError: "needs at least one team role",
		},
		{
			name: "team roles without teams",
			overrides: map[string]tftypes.Value{
				"team_ids":        stringSetValue(),
				"team_role_names": stringSetValue("schedules_editor"),
			},
			wantError: "it needs team_ids saying which",
		},
		{
			// A team list built from another resource isn't known until apply, so there's
			// nothing to count yet and the config must not be rejected.
			name: "unknown teams with team roles",
			overrides: map[string]tftypes.Value{
				"team_ids":        tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, tftypes.UnknownValue),
				"team_role_names": stringSetValue("schedules_editor"),
			},
		},
		{
			name: "unknown team roles with teams",
			overrides: map[string]tftypes.Value{
				"team_ids":        stringSetValue("01G0J1EXE7AXZ2C93K61WBPYEH"),
				"team_role_names": tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, tftypes.UnknownValue),
			},
		},
		{
			// A grace period with no version to change can never be read, so say so rather
			// than let it sit there looking effective.
			name: "grace period without a token version",
			overrides: map[string]tftypes.Value{
				"rotation_grace_period_minutes": tftypes.NewValue(tftypes.Number, 60),
			},
			wantWarning: "no token_version",
		},
		{
			name: "grace period with a token version",
			overrides: map[string]tftypes.Value{
				"token_version":                 tftypes.NewValue(tftypes.Number, 2),
				"rotation_grace_period_minutes": tftypes.NewValue(tftypes.Number, 60),
			},
		},
		{
			// The common case: rotate with whatever the default grace period is.
			name: "token version alone",
			overrides: map[string]tftypes.Value{
				"token_version": tftypes.NewValue(tftypes.Number, 2),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var resp resource.ValidateConfigResponse
			(&IncidentAPIKeyResource{}).ValidateConfig(
				context.Background(),
				resource.ValidateConfigRequest{Config: apiKeyConfig(t, tc.overrides)},
				&resp,
			)

			switch {
			case tc.wantError != "":
				if !resp.Diagnostics.HasError() {
					t.Fatalf("expected an error containing %q, got: %+v", tc.wantError, resp.Diagnostics)
				}
				if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), tc.wantError) {
					t.Fatalf("expected an error containing %q, got: %+v", tc.wantError, resp.Diagnostics)
				}

			case tc.wantWarning != "":
				if resp.Diagnostics.HasError() {
					t.Fatalf("expected no error, got: %+v", resp.Diagnostics)
				}
				if len(resp.Diagnostics.Warnings()) == 0 {
					t.Fatalf("expected a warning containing %q, got none", tc.wantWarning)
				}
				if !strings.Contains(resp.Diagnostics.Warnings()[0].Detail(), tc.wantWarning) {
					t.Fatalf("expected a warning containing %q, got: %+v", tc.wantWarning, resp.Diagnostics)
				}

			default:
				if resp.Diagnostics.HasError() {
					t.Fatalf("expected no error, got: %+v", resp.Diagnostics)
				}
				if len(resp.Diagnostics.Warnings()) > 0 {
					t.Fatalf("expected no warnings, got: %+v", resp.Diagnostics.Warnings())
				}
			}
		})
	}
}
