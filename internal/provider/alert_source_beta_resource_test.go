package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/richtexttypes"
)

// TestAlertSourceBetaResourceSchema builds the schema, which resolves every apischema.Docstring
// call against the embedded OpenAPI schema and panics if a definition or property is missing.
// It's the quickest way to catch the resource being built against a stale vendored schema —
// which is exactly what it was before the V3 alert source endpoints were regenerated into it.
func TestAlertSourceBetaResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewAlertSourceBetaResource()

	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_alert_source_beta" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}

	for _, name := range []string{
		"id", "name", "source_type", "secret_token", "alert_events_url", "email_address",
		"owning_team_ids", "is_private", "title", "description", "priority", "visible_to_teams",
		"jira_options", "heartbeat_options", "email_options", "http_custom_options",
		"rate_limit_sharding", "fixed_team_id",
		"auto_resolve_timeout_minutes", "auto_resolve_incident_alerts", "version",
	} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing expected attribute %q", name)
		}
	}

	if _, ok := schemaResp.Schema.Blocks["named_expression"]; !ok {
		t.Error("schema missing expected block \"named_expression\"")
	}

	// The attributes each belong to their own resource, so holding them here would have an
	// apply of the source wipe whatever those manage.
	if _, ok := schemaResp.Schema.Attributes["attributes"]; ok {
		t.Error("schema should not carry attribute bindings")
	}

	// The custom type on literal is what carries "{{ }}" support and, through semantic equality,
	// what stops a template diffing against the document it produces.
	for _, name := range []string{"title", "description"} {
		nested, ok := schemaResp.Schema.Attributes[name].(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s should be a single nested attribute, got %T", name, schemaResp.Schema.Attributes[name])
		}

		literal, ok := nested.Attributes["literal"]
		if !ok {
			t.Fatalf("%s should have a literal attribute", name)
		}
		if got := literal.GetType(); got != (richtexttypes.TemplatedTextType{}) {
			t.Errorf("%s.literal should be templated text, got %s", name, got)
		}
		if _, ok := nested.Attributes["reference"]; !ok {
			t.Errorf("%s should have a reference attribute", name)
		}
	}
}

func alertSourceV3(sourceType string) client.AlertSourceV3 {
	return client.AlertSourceV3{
		Id:         "01SOURCE",
		Name:       "Prometheus",
		SourceType: client.AlertSourceV3SourceType(sourceType),
		Version:    7,
		IsPrivate:  false,
	}
}

// fromAPI projects a source and fails the test on an unexpected diagnostic, so each case below
// only says what it is actually asserting.
func fromAPI(t *testing.T, source client.AlertSourceV3, config *alertSourceBetaModel) *alertSourceBetaModel {
	t.Helper()

	diags := diag.Diagnostics{}
	model := alertSourceBetaFromAPI(source, config, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %+v", diags.Errors())
	}

	return model
}

func literalPayload(value string) *client.EngineParamBindingPayloadV3 {
	return &client.EngineParamBindingPayloadV3{
		Value: &client.EngineParamBindingValuePayloadV3{Literal: lo.ToPtr(value)},
	}
}

// TestAlertSourceBetaTemplatedText covers both directions: the API stores a document, while a
// config usually writes a template.
func TestAlertSourceBetaTemplatedText(t *testing.T) {
	const template = "Alert on {{ payload.service }}"

	t.Run("sends a template as the document the API stores", func(t *testing.T) {
		data := &alertSourceBetaModel{
			Title: &models.TemplatedTextValue{
				Literal:   richtexttypes.NewTemplatedTextValue(template),
				Reference: types.StringNull(),
			},
		}

		diags := diag.Diagnostics{}
		_, bindings := (&alertSourceBetaResource{}).toPayloads(data, &diags)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %+v", diags.Errors())
		}

		document, err := richtexttypes.ToDocument(template)
		if err != nil {
			t.Fatalf("building the expected document: %v", err)
		}
		if bindings.title == nil || bindings.title.Value == nil ||
			lo.FromPtr(bindings.title.Value.Literal) != string(document) {
			t.Errorf("title should be sent as its document, got %+v", bindings.title)
		}
	})

	t.Run("reads a document back as its template", func(t *testing.T) {
		document, err := richtexttypes.ToDocument(template)
		if err != nil {
			t.Fatalf("building the stored document: %v", err)
		}

		source := alertSourceV3("http")
		source.Title = literalPayload(string(document))

		title := fromAPI(t, source, &alertSourceBetaModel{}).Title
		if title == nil {
			t.Fatal("title should be read back")
		}
		// Emission is canonical, so spacing isn't preserved here. Semantic equality is what
		// keeps the config's own spelling in state.
		if got := title.Literal.ValueString(); got != "Alert on {{payload.service}}" {
			t.Errorf("title should collapse to its template, got %q", got)
		}
	})

	// Semantic equality reconciles the AST against whichever form the config wrote, so keeping it
	// loses nothing — while a lossy template would silently drop the mention.
	t.Run("keeps a document no template can spell", func(t *testing.T) {
		const mention = `{"type":"doc","content":[{"type":"paragraph","content":` +
			`[{"type":"user","attrs":{"id":"01USER"}}]}]}`

		source := alertSourceV3("http")
		source.Description = literalPayload(mention)

		description := fromAPI(t, source, &alertSourceBetaModel{}).Description
		if description == nil || description.Literal.ValueString() != mention {
			t.Errorf("an unexpressible document should be kept verbatim, got %+v", description)
		}
	})

	// The point of the object shape: a source bound to a scope reference is manageable rather
	// than an error, which is what a bare string left it as.
	t.Run("reads a reference", func(t *testing.T) {
		source := alertSourceV3("http")
		source.Title = &client.EngineParamBindingPayloadV3{
			Value: &client.EngineParamBindingValuePayloadV3{Reference: lo.ToPtr("payload.summary")},
		}

		title := fromAPI(t, source, &alertSourceBetaModel{}).Title
		if title == nil || title.Reference.ValueString() != "payload.summary" {
			t.Fatalf("a reference-bound title should be read back, got %+v", title)
		}
		if !title.Literal.IsNull() {
			t.Errorf("no literal was stored, so it should read null, got %v", title.Literal)
		}
	})

	// The API assigns title unconditionally, so reading a value we can't hold as absent would
	// have the next apply delete it.
	t.Run("reports a binding no single value can hold", func(t *testing.T) {
		source := alertSourceV3("http")
		source.Title = &client.EngineParamBindingPayloadV3{
			ArrayValue: &[]client.EngineParamBindingValuePayloadV3{
				{Literal: lo.ToPtr("one")},
				{Literal: lo.ToPtr("two")},
			},
		}

		diags := diag.Diagnostics{}
		model := alertSourceBetaFromAPI(source, &alertSourceBetaModel{}, &diags)

		if !diags.HasError() {
			t.Fatal("an array-valued title should be reported, not silently dropped")
		}
		if !strings.Contains(diags.Errors()[0].Detail(), "several values") {
			t.Errorf("the error should say what it holds, got %q", diags.Errors()[0].Detail())
		}
		if model.Title != nil {
			t.Errorf("the unrepresentable value should not be stored, got %+v", model.Title)
		}
	})

	t.Run("sends nothing for an unset value", func(t *testing.T) {
		payload, err := models.TemplatedTextValueToPayload(nil)
		if err != nil || payload != nil {
			t.Errorf("an unset value should send no binding, got %+v (%v)", payload, err)
		}
	})
}

// TestAlertSourceBetaFromAPIEmailAddress covers email_address being lifted out of the options
// the API nests it in. It sits at the top level because we mint it rather than accepting it.
func TestAlertSourceBetaFromAPIEmailAddress(t *testing.T) {
	source := alertSourceV3("email")
	source.EmailOptions = &client.AlertSourceEmailOptionsV3{
		EmailAddress: "alerts@example.incident.io",
		Redactions:   []client.AlertSourceEmailOptionsV3Redactions{"phone_numbers"},
	}

	model := fromAPI(t, source, &alertSourceBetaModel{
		EmailOptions: &alertSourceEmailOptions{},
	})

	if got := model.EmailAddress.ValueString(); got != "alerts@example.incident.io" {
		t.Errorf("email_address should come from email_options, got %q", got)
	}
	if model.EmailOptions == nil || len(model.EmailOptions.Redactions) != 1 {
		t.Errorf("email_options should keep its redactions, got %+v", model.EmailOptions)
	}

	// Every other source type has no email options, so the attribute has to read back null
	// rather than empty — an empty string would diff against a config that never set it.
	withoutEmail := fromAPI(t, alertSourceV3("http"), &alertSourceBetaModel{})
	if !withoutEmail.EmailAddress.IsNull() {
		t.Errorf("email_address should be null for a non-email source, got %v", withoutEmail.EmailAddress)
	}
}

// TestAlertSourceBetaFromAPIEmailOptionsSpelling covers an email source configured with no
// email_options block. The API always answers with options for one, because the address we mint
// lives in them, so storing them would take the attribute from null to an object — which
// Terraform rejects as an inconsistent result after apply.
func TestAlertSourceBetaFromAPIEmailOptionsSpelling(t *testing.T) {
	source := alertSourceV3("email")
	source.EmailOptions = &client.AlertSourceEmailOptionsV3{
		EmailAddress: "alerts@example.incident.io",
		Redactions:   []client.AlertSourceEmailOptionsV3Redactions{},
	}

	model := fromAPI(t, source, &alertSourceBetaModel{EmailOptions: nil})
	if model.EmailOptions != nil {
		t.Errorf("options carrying nothing the config set should stay unset, got %+v", model.EmailOptions)
	}
	// The address is still captured, so nothing is lost by dropping the block.
	if model.EmailAddress.ValueString() != "alerts@example.incident.io" {
		t.Errorf("email_address should still be read, got %v", model.EmailAddress)
	}

	// Options carrying something the config could have set are real drift, so they're kept.
	source.EmailOptions.Redactions = []client.AlertSourceEmailOptionsV3Redactions{"phone_numbers"}
	if fromAPI(t, source, &alertSourceBetaModel{EmailOptions: nil}).EmailOptions == nil {
		t.Error("redactions added out of band should be read back, so the diff shows")
	}
}

// TestAlertSourceBetaFromAPIAutoResolve covers the API ignoring auto_resolve_incident_alerts
// where there's no timeout to resolve against, and never sending it for a heartbeat source.
func TestAlertSourceBetaFromAPIAutoResolve(t *testing.T) {
	t.Run("keeps the configured value when the API omits it", func(t *testing.T) {
		config := &alertSourceBetaModel{AutoResolveIncidentAlerts: types.BoolValue(true)}

		if !fromAPI(t, alertSourceV3("heartbeat"), config).AutoResolveIncidentAlerts.ValueBool() {
			t.Error("the configured value should be kept when the API omits it")
		}
	})

	// The attribute is Optional+Computed, so a config that omits it plans unknown, and on create
	// there's no prior state for the plan modifier to substitute. Storing that unknown fails the
	// apply with "provider produced inconsistent result".
	t.Run("never stores an unknown", func(t *testing.T) {
		config := &alertSourceBetaModel{AutoResolveIncidentAlerts: types.BoolUnknown()}

		got := fromAPI(t, alertSourceV3("heartbeat"), config).AutoResolveIncidentAlerts
		if got.IsUnknown() {
			t.Error("an unknown planned value must not be written into state")
		}
		if !got.IsNull() {
			t.Errorf("it should settle as null, got %v", got)
		}
	})

	t.Run("takes the API's answer when it gives one", func(t *testing.T) {
		source := alertSourceV3("http")
		source.AutoResolveTimeoutMinutes = lo.ToPtr(int64(30))
		source.AutoResolveIncidentAlerts = lo.ToPtr(false)

		config := &alertSourceBetaModel{AutoResolveIncidentAlerts: types.BoolValue(true)}
		if fromAPI(t, source, config).AutoResolveIncidentAlerts.ValueBool() {
			t.Error("the API's answer should win when it gives one")
		}
	})
}

// TestAlertSourceBetaFromAPIHeartbeatTemplate covers dropping the title and description a
// heartbeat source generates for itself. ValidateConfig rejects setting either, so storing what
// the API returned would leave a diff the config has no way to satisfy.
func TestAlertSourceBetaFromAPIHeartbeatTemplate(t *testing.T) {
	source := alertSourceV3("heartbeat")
	source.Title = literalPayload("Heartbeat missed")
	source.Description = literalPayload("No ping received")

	model := fromAPI(t, source, &alertSourceBetaModel{})

	if model.Title != nil {
		t.Errorf("a heartbeat source's generated title should be dropped, got %+v", model.Title)
	}
	if model.Description != nil {
		t.Errorf("a heartbeat source's generated description should be dropped, got %+v", model.Description)
	}

	// The same value on a type that does take one is kept.
	http := alertSourceV3("http")
	http.Title = literalPayload("Heartbeat missed")
	if fromAPI(t, http, &alertSourceBetaModel{}).Title == nil {
		t.Error("a title should be kept for a source type that takes one")
	}
}

// TestAlertSourceBetaFromAPIOwningTeamIDs covers the null-versus-empty distinction: the API
// always answers with a list, while HCL can omit the attribute, and storing [] against an
// omitted attribute shows a diff on every plan.
func TestAlertSourceBetaFromAPIOwningTeamIDs(t *testing.T) {
	source := alertSourceV3("http")
	source.OwningTeamIds = &[]string{}

	unset := fromAPI(t, source, &alertSourceBetaModel{
		OwningTeamIDs: types.SetNull(types.StringType),
	})
	if !unset.OwningTeamIDs.IsNull() {
		t.Errorf("no teams with the attribute unset should stay null, got %v", unset.OwningTeamIDs)
	}

	explicit := fromAPI(t, source, &alertSourceBetaModel{
		OwningTeamIDs: types.SetValueMust(types.StringType, []attr.Value{}),
	})
	if explicit.OwningTeamIDs.IsNull() {
		t.Error("no teams with the attribute set to [] should stay empty, not null")
	}
}

// TestAlertSourceBetaFromAPINamedExpressionOrder covers named_expression being a list, so any
// order but the config's own reads as a diff. The server sorts by dependency and won't preserve
// what was written.
func TestAlertSourceBetaFromAPINamedExpressionOrder(t *testing.T) {
	source := alertSourceV3("http")
	source.Expressions = []client.ExpressionPayloadV3{
		{Reference: "team_lookup", Label: "team_lookup", RootReference: "payload"},
		{Reference: "severity_lookup", Label: "severity_lookup", RootReference: "payload"},
	}

	config := &alertSourceBetaModel{
		NamedExpressions: []models.NamedExpression{
			{Name: types.StringValue("severity_lookup")},
			{Name: types.StringValue("team_lookup")},
		},
	}

	got := lo.Map(fromAPI(t, source, config).NamedExpressions, func(expression models.NamedExpression, _ int) string {
		return expression.Name.ValueString()
	})
	if len(got) != 2 || got[0] != "severity_lookup" || got[1] != "team_lookup" {
		t.Errorf("expressions should read back in the order the config wrote them, got %v", got)
	}
}
