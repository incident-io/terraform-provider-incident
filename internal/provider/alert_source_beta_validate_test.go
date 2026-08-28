package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// alertSourceBetaSchemaType is the resource's schema as a tftypes.Object, so the config the
// tests build is the one Terraform would build.
func alertSourceBetaSchemaType(t *testing.T) (tfsdk.Config, tftypes.Object) {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewAlertSourceBetaResource().Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build failed: %+v", schemaResp.Diagnostics)
	}

	objType, ok := schemaResp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}

	return tfsdk.Config{Schema: schemaResp.Schema}, objType
}

// alertSourceBetaConfig builds a config with every attribute null, then applies overrides. The
// resource has around twenty attributes and only a few matter per test, so spelling them all
// out per case would bury what each one is actually checking.
func alertSourceBetaConfig(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Config {
	t.Helper()

	config, objType := alertSourceBetaSchemaType(t)

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

	config.Raw = tftypes.NewValue(objType, values)

	return config
}

// objectWith fills an object type with nulls and sets only the named attributes, which is how
// every options block and binding is written in a real config.
func objectWith(t *testing.T, objectType tftypes.Type, set map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	object, ok := objectType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected an object type, got %s", objectType)
	}

	values := map[string]tftypes.Value{}
	for name, attrType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	for name, value := range set {
		if _, ok := object.AttributeTypes[name]; !ok {
			t.Fatalf("attribute %q is not part of %s", name, object)
		}
		values[name] = value
	}

	return tftypes.NewValue(object, values)
}

// attributeType returns the schema type of a top-level attribute, for building a value to put
// in it.
func attributeType(t *testing.T, name string) tftypes.Type {
	t.Helper()

	_, objType := alertSourceBetaSchemaType(t)
	attrType, ok := objType.AttributeTypes[name]
	if !ok {
		t.Fatalf("schema has no attribute %q", name)
	}

	return attrType
}

func validateAlertSourceBeta(t *testing.T, config tfsdk.Config) diag.Diagnostics {
	t.Helper()

	r, ok := NewAlertSourceBetaResource().(*alertSourceBetaResource)
	if !ok {
		t.Fatalf("NewAlertSourceBetaResource did not return a *alertSourceBetaResource")
	}

	resp := resource.ValidateConfigResponse{}
	r.ValidateConfig(
		context.Background(),
		resource.ValidateConfigRequest{Config: config},
		&resp,
	)

	return resp.Diagnostics
}

func assertErrorContaining(t *testing.T, diags diag.Diagnostics, want string) {
	t.Helper()

	for _, d := range diags.Errors() {
		if strings.Contains(d.Summary(), want) || strings.Contains(d.Detail(), want) {
			return
		}
	}

	t.Errorf("expected an error mentioning %q, got %+v", want, diags.Errors())
}

func assertNoErrorContaining(t *testing.T, diags diag.Diagnostics, unwanted string) {
	t.Helper()

	for _, d := range diags.Errors() {
		if strings.Contains(d.Summary(), unwanted) || strings.Contains(d.Detail(), unwanted) {
			t.Errorf("did not expect an error mentioning %q, got %s: %s", unwanted, d.Summary(), d.Detail())
		}
	}
}

func stringValue(value string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, value)
}

// TestAlertSourceBetaValidateOptions covers each options block being tied to the one source
// type that reads it. The API rejects a mismatch too, but only at apply — and for a required
// block that means a create that was never going to work.
func TestAlertSourceBetaValidateOptions(t *testing.T) {
	projectIDs := func(ids ...tftypes.Value) tftypes.Value {
		return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, ids)
	}
	jiraOptions := objectWith(t, attributeType(t, "jira_options"), map[string]tftypes.Value{
		"project_ids": projectIDs(stringValue("ENG")),
	})

	t.Run("rejects options the source type doesn't read", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type":  stringValue("http"),
			"jira_options": jiraOptions,
		}))

		assertErrorContaining(t, diags, "jira_options is only for jira alert sources")
	})

	t.Run("requires the options a jira source can't be created without", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type": stringValue("jira"),
		}))

		assertErrorContaining(t, diags, "jira_options is required for jira alert sources")
	})

	t.Run("accepts the matching pair", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type":  stringValue("jira"),
			"jira_options": jiraOptions,
		}))

		assertNoErrorContaining(t, diags, "jira_options")
	})

	// An email source works with no options at all: it just gets no redactions and no
	// transform. Only jira, heartbeat and http_custom are rejected without theirs.
	t.Run("does not require email_options for an email source", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type": stringValue("email"),
		}))

		assertNoErrorContaining(t, diags, "email_options")
	})

	// An empty list stores no options server-side, so the block reads back absent and the apply
	// fails as an inconsistent result. Caught here instead.
	t.Run("rejects a jira source watching no projects", func(t *testing.T) {
		empty := objectWith(t, attributeType(t, "jira_options"), map[string]tftypes.Value{
			"project_ids": projectIDs(),
		})

		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type":  stringValue("jira"),
			"jira_options": empty,
		}))

		assertErrorContaining(t, diags, "No Jira projects")
	})

	// source_type computed from another resource isn't known at validate time, so the check
	// has to hold off rather than guess.
	t.Run("holds off while source_type is unknown", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"jira_options": jiraOptions,
		}))

		assertNoErrorContaining(t, diags, "jira_options")
	})
}

// TestAlertSourceBetaValidateHeartbeatTemplate covers the title and description a heartbeat
// source writes for itself. Left to the API these are silently replaced, so the config would
// keep planning a change that never lands.
func TestAlertSourceBetaValidateHeartbeatTemplate(t *testing.T) {
	title := func(literal string) tftypes.Value {
		return objectWith(t, attributeType(t, "title"), map[string]tftypes.Value{
			"literal": stringValue(literal),
		})
	}

	diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
		"source_type": stringValue("heartbeat"),
		"title":       title("Heartbeat missed"),
	}))

	assertErrorContaining(t, diags, "title can't be set on a heartbeat alert source")

	// A source type that does take one is left alone.
	diags = validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
		"source_type": stringValue("http"),
		"title":       title("Prometheus alert"),
	}))

	assertNoErrorContaining(t, diags, "title")
}

// TestAlertSourceBetaValidateTemplatedText covers a title carrying neither form or both. The
// mapping reads neither as absent, silently dropping the field and planning it again next time,
// and reads both as a conflict the API rejects only at apply.
func TestAlertSourceBetaValidateTemplatedText(t *testing.T) {
	t.Run("rejects a value holding nothing", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type": stringValue("http"),
			"title":       objectWith(t, attributeType(t, "title"), nil),
		}))

		assertErrorContaining(t, diags, "Set either literal or reference.")
	})

	t.Run("rejects both forms at once", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type": stringValue("http"),
			"title": objectWith(t, attributeType(t, "title"), map[string]tftypes.Value{
				"literal":   stringValue("Prometheus alert"),
				"reference": stringValue("payload.summary"),
			}),
		}))

		assertErrorContaining(t, diags, "Set either literal or reference, not both.")
	})

	// The type reports syntax errors on every attribute using it, with no resource wiring. This
	// proves that reaches the config.
	t.Run("reports a template syntax error", func(t *testing.T) {
		config := alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type": stringValue("http"),
			"title": objectWith(t, attributeType(t, "title"), map[string]tftypes.Value{
				"literal": stringValue("Alert on {{ payload.service"),
			}),
		})

		var value models.TemplatedTextValue
		diags := config.GetAttribute(context.Background(), path.Root("title"), &value)

		assertErrorContaining(t, diags, `Unclosed "{{"`)
	})
}

// TestAlertSourceBetaValidatePrivacy covers visible_to_teams and is_private, which are only
// meaningful together: one says who can see the alerts, the other that they're restricted at
// all.
func TestAlertSourceBetaValidatePrivacy(t *testing.T) {
	visibleToTeams := objectWith(t, attributeType(t, "visible_to_teams"), map[string]tftypes.Value{
		"value_reference": stringValue("payload.team"),
	})

	t.Run("rejects visible_to_teams on a source that isn't private", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type":      stringValue("http"),
			"is_private":       tftypes.NewValue(tftypes.Bool, false),
			"visible_to_teams": visibleToTeams,
		}))

		assertErrorContaining(t, diags, "visible_to_teams needs is_private")
	})

	t.Run("rejects a private source nobody can see", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type": stringValue("http"),
			"is_private":  tftypes.NewValue(tftypes.Bool, true),
		}))

		assertErrorContaining(t, diags, "visible_to_teams is required when is_private is true")
	})

	t.Run("accepts the two together", func(t *testing.T) {
		diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
			"source_type":      stringValue("http"),
			"is_private":       tftypes.NewValue(tftypes.Bool, true),
			"visible_to_teams": visibleToTeams,
		}))

		assertNoErrorContaining(t, diags, "visible_to_teams")
	})
}

// TestAlertSourceBetaValidateBindings checks the source's own bindings reach the shared
// expression validation. A reference to an expression this resource doesn't own can't be
// resolved by anyone — there is no shared pool — so it has to fail at plan time.
func TestAlertSourceBetaValidateBindings(t *testing.T) {
	priority := objectWith(t, attributeType(t, "priority"), map[string]tftypes.Value{
		"expression_ref": stringValue("severity_lookup"),
	})

	diags := validateAlertSourceBeta(t, alertSourceBetaConfig(t, map[string]tftypes.Value{
		"source_type": stringValue("http"),
		"priority":    priority,
	}))

	assertErrorContaining(t, diags, "Unknown expression_ref")
}
