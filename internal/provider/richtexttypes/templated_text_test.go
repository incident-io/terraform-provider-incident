package richtexttypes

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr/xattr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/jsontypes"
)

const (
	bareVariable      = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"name":"description"}}]}]}`
	dashboardVariable = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"label":"Description","missing":false,"name":"description","omitIfUnset":false,"truncateTo":null}}]}]}`
	titleVariable     = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"name":"title"}}]}]}`
	plainParagraph    = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Payments alert"}]}]}`
	mentionDocument   = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"user","attrs":{"id":"01USER"}}]}]}`
	legacyEnvelope    = `{"root":{"type":"doc","content":[]},"value_markdown":"Payments alert"}`

	// What the dashboard's template editor actually stores: the tree wrapped in
	// {text_node, schema_version}, carrying the display attrs an editor load rewrites.
	// Sources built there are the common case.
	dashboardEnvelope = `{"schema_version":"v1.0.0","text_node":` + dashboardVariable + `}`
)

// The whole point of the custom type: a raw-AST config and a {{ }} config must both
// compare equal to the same stored document, or one of them gets a permanent diff.
func TestStringSemanticEquals(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		want bool
	}{
		{"a template and the document it produces", "{{description}}", bareVariable, true},
		{"a template and a dashboard-authored document", "{{description}}", dashboardVariable, true},
		{"a document and an equivalent dashboard-authored one", bareVariable, dashboardVariable, true},
		// Without unwrapping the envelope these compare unequal, which is a diff on the first
		// plan after importing a source built in the dashboard.
		{"a template and the enveloped document the dashboard stores", "{{description}}", dashboardEnvelope, true},
		{"an enveloped document and its bare equivalent", dashboardEnvelope, bareVariable, true},
		{"two spellings of the same template", "{{description}}", "{{ description }}", true},
		{"the same document with keys in a different order", bareVariable, `{"content":[{"content":[{"attrs":{"name":"description"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`, true},
		{"two different templates", "{{description}}", "{{title}}", false},
		{"a template and a different document", "{{description}}", titleVariable, false},
		{"plain text and a template", "Payments alert", "{{description}}", false},
		{"identical plain text", "Payments alert", "Payments alert", true},
		// Neither side normalises, so this falls back to exact string equality.
		{"identical unparseable strings", "{{unclosed", "{{unclosed", true},
		{"differing unparseable strings", "{{unclosed", "{{other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := NewTemplatedTextValue(tc.a).StringSemanticEquals(
				context.Background(), NewTemplatedTextValue(tc.b))
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got != tc.want {
				t.Errorf("StringSemanticEquals(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}

			// Equality must not depend on which side Terraform happens to hold.
			reverse, _ := NewTemplatedTextValue(tc.b).StringSemanticEquals(
				context.Background(), NewTemplatedTextValue(tc.a))
			if reverse != got {
				t.Errorf("StringSemanticEquals is not symmetric for (%q, %q): %v vs %v", tc.a, tc.b, got, reverse)
			}
		})
	}
}

// Without the Equal override, a TemplatedText reports itself equal to a plain
// types.String with the same contents.
func TestEqualDistinguishesTemplatedTextFromString(t *testing.T) {
	value := NewTemplatedTextValue("{{description}}")

	if value.Equal(types.StringValue("{{description}}")) {
		t.Error("TemplatedText should not equal a plain types.String with the same contents")
	}
	if !value.Equal(NewTemplatedTextValue("{{description}}")) {
		t.Error("TemplatedText should equal another TemplatedText with the same contents")
	}
	if value.Equal(NewTemplatedTextValue("{{title}}")) {
		t.Error("TemplatedText should not equal a TemplatedText with different contents")
	}
}

func TestTypeEqualDistinguishesTemplatedTextTypeFromStringType(t *testing.T) {
	if (TemplatedTextType{}).Equal(basetypes.StringType{}) {
		t.Error("TemplatedTextType should not equal a plain basetypes.StringType")
	}
	if !(TemplatedTextType{}).Equal(TemplatedTextType{}) {
		t.Error("TemplatedTextType should equal itself")
	}
}

func TestValidateAttribute(t *testing.T) {
	for _, tc := range []struct {
		name      string
		value     TemplatedText
		wantError bool
	}{
		{"a valid template", NewTemplatedTextValue("{{description}}"), false},
		{"a filtered template", NewTemplatedTextValue("{{description | truncate: 100}}"), false},
		{"plain text", NewTemplatedTextValue("Payments alert"), false},
		// Raw AST is not our grammar, so it passes through untouched.
		{"a raw document", NewTemplatedTextValue(bareVariable), false},
		{"null", NewTemplatedTextNull(), false},
		{"unknown", NewTemplatedTextUnknown(), false},
		{"an unclosed variable", NewTemplatedTextValue("{{description"), true},
		{"an unknown filter", NewTemplatedTextValue("{{description | upcase}}"), true},
		{"a bad truncate argument", NewTemplatedTextValue("{{description | truncate: 0}}"), true},
		{"an empty variable name", NewTemplatedTextValue("{{}}"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &xattr.ValidateAttributeResponse{}
			tc.value.ValidateAttribute(context.Background(),
				xattr.ValidateAttributeRequest{Path: path.Root("literal")}, resp)

			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Fatalf("ValidateAttribute(%q): got error=%v, want %v (%v)",
					tc.value.ValueString(), got, tc.wantError, resp.Diagnostics)
			}
			if !tc.wantError {
				return
			}

			// Must anchor to the attribute so it lands on the right line.
			at, ok := resp.Diagnostics.Errors()[0].(interface{ Path() path.Path })
			if !ok {
				t.Fatalf("diagnostic does not carry a Path()")
			}
			if at.Path().String() != "literal" {
				t.Errorf("diagnostic anchored to %q, want %q", at.Path(), "literal")
			}
		})
	}
}

// TestLiteral covers what goes on the wire: always a document. The server uplifts a non-JSON
// literal into one before storing it, so a plain string would read back as something we never
// wrote.
func TestLiteral(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value TemplatedText
		want  *string
	}{
		{"null", NewTemplatedTextNull(), nil},
		{"unknown", NewTemplatedTextUnknown(), nil},
		{"a template", NewTemplatedTextValue("{{description}}"), lo.ToPtr(bareVariable)},
		{"plain text", NewTemplatedTextValue("Payments alert"), lo.ToPtr(plainParagraph)},
		// The bytes a config wrote must be the bytes stored, or it diffs.
		{"a raw document", NewTemplatedTextValue(dashboardVariable), lo.ToPtr(dashboardVariable)},
		// Including an envelope: we read through one, but we never rewrite what a config sends.
		{"a raw enveloped document", NewTemplatedTextValue(dashboardEnvelope), lo.ToPtr(dashboardEnvelope)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.value.Literal()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			switch {
			case tc.want == nil && got != nil:
				t.Errorf("expected nothing to send, got %q", *got)
			case tc.want != nil && got == nil:
				t.Errorf("expected %q, got nothing", *tc.want)
			case tc.want != nil && !jsontypes.JSONStringsEqual(*got, *tc.want):
				t.Errorf("literal\n got: %s\nwant: %s", *got, *tc.want)
			}
		})
	}

	// Reported at plan time by ValidateAttribute, so reaching here with one is a provider bug.
	t.Run("an unparseable template", func(t *testing.T) {
		if _, err := NewTemplatedTextValue("{{description").Literal(); err == nil {
			t.Error("an unclosed variable should be rejected")
		}
	})
}

// TestNewTemplatedTextFromLiteral covers the read direction: prefer the form a human would have
// written, fall back to the AST where that would lose content.
func TestNewTemplatedTextFromLiteral(t *testing.T) {
	for _, tc := range []struct {
		name    string
		literal string
		want    string
	}{
		{"a document", bareVariable, "{{description}}"},
		// The display attrs a dashboard edit adds must be dropped, or config and state diff forever.
		{"a dashboard-authored document", dashboardVariable, "{{description}}"},
		{"plain text in a paragraph", plainParagraph, "Payments alert"},
		// A mention carries an ID the grammar has no syntax for, so a template would delete it.
		{"a document no template can spell", mentionDocument, mentionDocument},
		// Read through the envelope, so an imported source's state holds the template rather
		// than a wall of AST.
		{"an enveloped document the dashboard stored", dashboardEnvelope, "{{description}}"},
		// An envelope is no guarantee of expressibility: this one wraps a doc with no blocks.
		{"a legacy envelope around an empty document", legacyEnvelope, legacyEnvelope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewTemplatedTextFromLiteral(tc.literal).ValueString(); got != tc.want {
				t.Errorf("NewTemplatedTextFromLiteral(%s)\n got: %q\nwant: %q", tc.literal, got, tc.want)
			}
		})
	}
}
