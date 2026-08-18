package richtexttypes

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The grammar fixtures are shared with the product's own template parser. See
// testdata/README.md for the schema.

// templateCase asserts both directions: template parses to document, document emits
// canonical (defaulting to template when emission matches the input).
type templateCase struct {
	Name      string          `json:"name"`
	Template  string          `json:"template"`
	Canonical *string         `json:"canonical"`
	Document  json.RawMessage `json:"document"`
}

func (c templateCase) want() string {
	if c.Canonical != nil {
		return *c.Canonical
	}

	return c.Template
}

// documentCase asserts the emit direction only, for ASTs no template produces.
type documentCase struct {
	Name        string          `json:"name"`
	Document    json.RawMessage `json:"document"`
	Expressible bool            `json:"expressible"`
	Template    *string         `json:"template"`
	Reason      string          `json:"reason"`
}

// invalidCase asserts a template fails to parse. The slug rather than message text, so
// each implementation can word its own diagnostics.
type invalidCase struct {
	Name     string `json:"name"`
	Template string `json:"template"`
	Error    string `json:"error"`
}

// loadFixtures reads one fixture file, rejecting unknown fields so a typo'd key can't
// silently produce a vacuously passing case.
func loadFixtures[T any](t *testing.T, file string) []T {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var cases []T
	if err := dec.Decode(&cases); err != nil {
		t.Fatalf("unmarshalling %s: %v", file, err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s is empty", file)
	}

	return cases
}
