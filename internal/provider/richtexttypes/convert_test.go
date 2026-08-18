package richtexttypes

import (
	"errors"
	"strings"
	"testing"

	"github.com/incident-io/terraform-provider-incident/internal/provider/jsontypes"
)

func TestToDocument(t *testing.T) {
	for _, tc := range loadFixtures[templateCase](t, "templates.json") {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := ToDocument(tc.Template)
			if err != nil {
				t.Fatalf("ToDocument(%q): unexpected error: %v", tc.Template, err)
			}

			if !jsontypes.JSONStringsEqual(string(got), string(tc.Document)) {
				t.Errorf("ToDocument(%q)\n got: %s\nwant: %s", tc.Template, got, tc.Document)
			}
		})
	}
}

func TestToDocumentRejectsInvalidTemplates(t *testing.T) {
	for _, tc := range loadFixtures[invalidCase](t, "invalid.json") {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := ToDocument(tc.Template)
			if err == nil {
				t.Fatalf("ToDocument(%q): want error %q, got none", tc.Template, tc.Error)
			}

			var parseErr *ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("ToDocument(%q): want *ParseError, got %T: %v", tc.Template, err, err)
			}
			if parseErr.Slug != tc.Error {
				t.Errorf("ToDocument(%q): got slug %q, want %q", tc.Template, parseErr.Slug, tc.Error)
			}
		})
	}
}

func TestFromDocumentEmitsTemplates(t *testing.T) {
	for _, tc := range loadFixtures[templateCase](t, "templates.json") {
		t.Run(tc.Name, func(t *testing.T) {
			got, ok := FromDocument(tc.Document)
			if !ok {
				t.Fatalf("FromDocument(%s): not expressible, want %q", tc.Document, tc.want())
			}
			if got != tc.want() {
				t.Errorf("FromDocument(%s)\n got: %q\nwant: %q", tc.Document, got, tc.want())
			}
		})
	}
}

func TestFromDocument(t *testing.T) {
	for _, tc := range loadFixtures[documentCase](t, "documents.json") {
		t.Run(tc.Name, func(t *testing.T) {
			got, ok := FromDocument(tc.Document)
			if ok != tc.Expressible {
				t.Fatalf("FromDocument(%s): got expressible=%v, want %v (%s)",
					tc.Document, ok, tc.Expressible, tc.Reason)
			}
			if !tc.Expressible {
				return
			}
			if got != *tc.Template {
				t.Errorf("FromDocument(%s)\n got: %q\nwant: %q", tc.Document, got, *tc.Template)
			}
		})
	}
}

func TestIsDocument(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{"bare prosemirror document", `{"type":"doc","content":[]}`, true},
		{"legacy envelope", `{"root":{},"value_markdown":"hi"}`, true},
		{"empty object", `{}`, true},
		{"template", "{{description}}", false},
		{"plain text", "Payments alert", false},
		{"empty string", "", false},
		// json.Valid accepts all of these, which is why we can't gate on it.
		{"number", "123", false},
		{"boolean", "true", false},
		{"null", "null", false},
		{"quoted string", `"a title"`, false},
		{"array", `[{"type":"doc"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDocument(tc.input); got != tc.want {
				t.Errorf("isDocument(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalise(t *testing.T) {
	const canonical = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"name":"description"}}]}]}`

	for _, tc := range []struct {
		name  string
		input string
	}{
		{"a template", "{{description}}"},
		{"the same template with whitespace", "{{ description }}"},
		{"the document it produces", canonical},
		{"a dashboard document carrying label and missing", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"label":"Description","missing":false,"name":"description"}}]}]}`},
		{"a document with omitIfUnset explicitly false", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"name":"description","omitIfUnset":false}}]}]}`},
		{"a document with truncateTo explicitly null", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"varSpec","attrs":{"name":"description","truncateTo":null}}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalise(tc.input)
			if err != nil {
				t.Fatalf("normalise(%q): unexpected error: %v", tc.input, err)
			}
			if !jsontypes.JSONStringsEqual(string(got), canonical) {
				t.Errorf("normalise(%q)\n got: %s\nwant: %s", tc.input, got, canonical)
			}
		})
	}
}

func TestNormaliseTreatsEmptyContentAsAbsent(t *testing.T) {
	withEmpty := `{"type":"doc","content":[{"type":"paragraph","content":[]}]}`

	fromTemplate, err := normalise("")
	if err != nil {
		t.Fatalf("normalise(%q): unexpected error: %v", "", err)
	}
	fromDocument, err := normalise(withEmpty)
	if err != nil {
		t.Fatalf("normalise(%q): unexpected error: %v", withEmpty, err)
	}

	if !jsontypes.JSONStringsEqual(string(fromTemplate), string(fromDocument)) {
		t.Errorf("an empty template and an empty-content document should normalise alike\n empty template: %s\n empty content:  %s", fromTemplate, fromDocument)
	}
}

func TestNormaliseRejectsInvalidTemplates(t *testing.T) {
	if _, err := normalise("{{description"); err == nil {
		t.Error("normalise: want error for an unparseable template, got none")
	}
}

// The offset points at the unclosed "{{", so a long heredoc template is navigable.
func TestUnclosedVariableReportsItsOffset(t *testing.T) {
	for _, tc := range []struct {
		name     string
		template string
		want     string
	}{
		{"at the start", "{{description", "offset 0"},
		{"after leading text", "Payments: {{description", "offset 10"},
		{"after an earlier variable", "{{title}} and {{description", "offset 14"},
		{"in a later paragraph", "first\n\n{{description", "offset 7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ToDocument(tc.template)
			if err == nil {
				t.Fatalf("ToDocument(%q): want an error, got none", tc.template)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ToDocument(%q): error %q does not mention %q", tc.template, err, tc.want)
			}
		})
	}
}
