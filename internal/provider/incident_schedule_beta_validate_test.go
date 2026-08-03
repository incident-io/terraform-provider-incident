package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// scheduleV3ConfigFor builds a tfsdk.Config against the real resource schema, so
// these tests exercise ValidateConfig exactly as Terraform calls it on every plan.
// holidays is the value for holidays_public_config: nil for absent, otherwise an
// object whose country_codes is the given value.
func scheduleV3ConfigFor(t *testing.T, holidays tftypes.Value) tfsdk.Config {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentScheduleBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)

	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
			"id":                     tftypes.NewValue(tftypes.String, "01SCHED"),
			"name":                   tftypes.NewValue(tftypes.String, "Platform on-call"),
			"timezone":               tftypes.NewValue(tftypes.String, "Europe/London"),
			"team_ids":               tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, nil),
			"holidays_public_config": holidays,
		}),
	}
}

// holidaysObjectType mirrors the SingleNestedAttribute in the schema.
var holidaysObjectType = tftypes.Object{
	AttributeTypes: map[string]tftypes.Type{
		"country_codes": tftypes.List{ElementType: tftypes.String},
	},
}

func validateScheduleV3(t *testing.T, holidays tftypes.Value) resource.ValidateConfigResponse {
	t.Helper()

	r := NewIncidentScheduleBetaResource().(*IncidentScheduleBetaResource)
	var resp resource.ValidateConfigResponse
	r.ValidateConfig(
		context.Background(),
		resource.ValidateConfigRequest{Config: scheduleV3ConfigFor(t, holidays)},
		&resp,
	)
	return resp
}

func TestScheduleV3ValidateConfig(t *testing.T) {
	codesList := tftypes.List{ElementType: tftypes.String}

	t.Run("no holidays block is fine", func(t *testing.T) {
		resp := validateScheduleV3(t, tftypes.NewValue(holidaysObjectType, nil))
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("a populated holidays block is fine", func(t *testing.T) {
		resp := validateScheduleV3(t, tftypes.NewValue(holidaysObjectType, map[string]tftypes.Value{
			"country_codes": tftypes.NewValue(codesList, []tftypes.Value{
				tftypes.NewValue(tftypes.String, "GB"),
			}),
		}))
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("an empty country_codes list is rejected", func(t *testing.T) {
		resp := validateScheduleV3(t, tftypes.NewValue(holidaysObjectType, map[string]tftypes.Value{
			"country_codes": tftypes.NewValue(codesList, []tftypes.Value{}),
		}))
		if !resp.Diagnostics.HasError() {
			t.Error("expected an error for an empty country_codes list")
		}
	})

	// An unknown list can't be judged yet. Rejecting it — or failing to decode it —
	// would break any plan that takes country_codes from another resource's output.
	t.Run("an unknown country_codes list is left alone", func(t *testing.T) {
		resp := validateScheduleV3(t, tftypes.NewValue(holidaysObjectType, map[string]tftypes.Value{
			"country_codes": tftypes.NewValue(codesList, tftypes.UnknownValue),
		}))
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("an unknown holidays block is left alone", func(t *testing.T) {
		resp := validateScheduleV3(t, tftypes.NewValue(holidaysObjectType, tftypes.UnknownValue))
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})
}

// TestScheduleV3ValidateCountryCodes covers the codes the API would accept and
// then silently ignore, which is why they're worth catching locally.
func TestScheduleV3ValidateCountryCodes(t *testing.T) {
	codesList := tftypes.List{ElementType: tftypes.String}

	withCodes := func(t *testing.T, values ...tftypes.Value) resource.ValidateConfigResponse {
		t.Helper()
		return validateScheduleV3(t, tftypes.NewValue(holidaysObjectType, map[string]tftypes.Value{
			"country_codes": tftypes.NewValue(codesList, values),
		}))
	}
	code := func(v string) tftypes.Value { return tftypes.NewValue(tftypes.String, v) }

	for _, tc := range []struct {
		name    string
		values  []tftypes.Value
		wantErr bool
	}{
		{name: "uppercase alpha-2", values: []tftypes.Value{code("GB"), code("FR")}},
		{name: "lowercase", values: []tftypes.Value{code("gb")}, wantErr: true},
		{name: "alpha-3", values: []tftypes.Value{code("GBR")}, wantErr: true},
		{name: "country name", values: []tftypes.Value{code("United Kingdom")}, wantErr: true},
		{name: "empty string", values: []tftypes.Value{code("")}, wantErr: true},
		{name: "padded", values: []tftypes.Value{code(" GB")}, wantErr: true},
		{name: "digits", values: []tftypes.Value{code("12")}, wantErr: true},
		{name: "duplicate", values: []tftypes.Value{code("GB"), code("GB")}, wantErr: true},
		{name: "unknown element is skipped", values: []tftypes.Value{
			tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := withCodes(t, tc.values...)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Errorf("HasError() = %v, want %v (diagnostics: %+v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestScheduleV3ValidateTimezone checks the typo class the API only rejects at
// apply, by which point Terraform has already planned a replacement.
func TestScheduleV3ValidateTimezone(t *testing.T) {
	validateTimezone := func(t *testing.T, timezone tftypes.Value) resource.ValidateConfigResponse {
		t.Helper()

		var schemaResp resource.SchemaResponse
		NewIncidentScheduleBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
		objType := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)

		config := tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
				"id":                     tftypes.NewValue(tftypes.String, "01SCHED"),
				"name":                   tftypes.NewValue(tftypes.String, "Platform on-call"),
				"timezone":               timezone,
				"team_ids":               tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, nil),
				"holidays_public_config": tftypes.NewValue(holidaysObjectType, nil),
			}),
		}

		r := NewIncidentScheduleBetaResource().(*IncidentScheduleBetaResource)
		var resp resource.ValidateConfigResponse
		r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: config}, &resp)
		return resp
	}

	for _, tc := range []struct {
		name     string
		timezone tftypes.Value
		wantErr  bool
	}{
		{name: "valid", timezone: tftypes.NewValue(tftypes.String, "Europe/London")},
		{name: "also valid", timezone: tftypes.NewValue(tftypes.String, "America/Los_Angeles")},
		{name: "typo", timezone: tftypes.NewValue(tftypes.String, "Europe/Londn"), wantErr: true},
		{name: "abbreviation", timezone: tftypes.NewValue(tftypes.String, "GMT+1"), wantErr: true},
		{name: "empty", timezone: tftypes.NewValue(tftypes.String, ""), wantErr: true},
		{name: "Local", timezone: tftypes.NewValue(tftypes.String, "Local"), wantErr: true},
		{name: "unknown is skipped", timezone: tftypes.NewValue(tftypes.String, tftypes.UnknownValue)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := validateTimezone(t, tc.timezone)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Errorf("HasError() = %v, want %v (diagnostics: %+v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

// validateScheduleV3DataSource runs the data source's ValidateConfig against the
// real schema, with id and name set to the given raw values.
func validateScheduleV3DataSource(t *testing.T, id, name tftypes.Value) datasource.ValidateConfigResponse {
	t.Helper()

	var schemaResp datasource.SchemaResponse
	NewIncidentScheduleBetaDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	config := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
			"id":       id,
			"name":     name,
			"timezone": tftypes.NewValue(tftypes.String, "Europe/London"),
			"team_ids": tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, nil),
		}),
	}

	d := NewIncidentScheduleBetaDataSource().(*IncidentScheduleBetaDataSource)
	var resp datasource.ValidateConfigResponse
	d.ValidateConfig(context.Background(), datasource.ValidateConfigRequest{Config: config}, &resp)
	return resp
}

func TestScheduleV3DataSourceValidateConfig(t *testing.T) {
	null := tftypes.NewValue(tftypes.String, nil)
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	value := tftypes.NewValue(tftypes.String, "01SCHED")

	t.Run("id alone is fine", func(t *testing.T) {
		if resp := validateScheduleV3DataSource(t, value, null); resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("name alone is fine", func(t *testing.T) {
		if resp := validateScheduleV3DataSource(t, null, value); resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("both set is rejected", func(t *testing.T) {
		if resp := validateScheduleV3DataSource(t, value, value); !resp.Diagnostics.HasError() {
			t.Error("expected an error when both id and name are set")
		}
	})

	t.Run("neither set is rejected", func(t *testing.T) {
		if resp := validateScheduleV3DataSource(t, null, null); !resp.Diagnostics.HasError() {
			t.Error("expected an error when neither id nor name is set")
		}
	})

	// An unknown value is non-null, so without a guard these would look like "both
	// set" and fail a plan that's actually fine.
	t.Run("both unknown is left alone", func(t *testing.T) {
		if resp := validateScheduleV3DataSource(t, unknown, unknown); resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})

	t.Run("one unknown alongside a value is left alone", func(t *testing.T) {
		if resp := validateScheduleV3DataSource(t, unknown, value); resp.Diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %+v", resp.Diagnostics)
		}
	})
}
