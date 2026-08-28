package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/richtexttypes"
)

func literalValue(literal string) *TemplatedTextValue {
	return &TemplatedTextValue{
		Literal:   richtexttypes.NewTemplatedTextValue(literal),
		Reference: types.StringNull(),
	}
}

func referenceValue(reference string) *TemplatedTextValue {
	return &TemplatedTextValue{
		Literal:   richtexttypes.NewTemplatedTextNull(),
		Reference: types.StringValue(reference),
	}
}

// TestTemplatedTextRoundTrip is the property that keeps a plan empty: whatever a config wrote
// must read back as itself. A template is the one form that doesn't survive byte-for-byte, going
// out as a document and back in canonical spelling, so each case pins both.
func TestTemplatedTextRoundTrip(t *testing.T) {
	document := func(template string) string {
		t.Helper()

		doc, err := richtexttypes.ToDocument(template)
		if err != nil {
			t.Fatalf("building the document for %q: %v", template, err)
		}

		return string(doc)
	}

	for _, tc := range []struct {
		name  string
		value *TemplatedTextValue
		// Expected on the wire, "" for a reference-only binding.
		literal string
		// What it reads back as, where that differs from value.
		reads *TemplatedTextValue
	}{
		{
			name:    "a fixed string",
			value:   literalValue("Prometheus alert"),
			literal: document("Prometheus alert"),
		},
		{
			name:    "a variable",
			value:   literalValue("{{payload.service}}"),
			literal: document("{{payload.service}}"),
		},
		{
			// Spacing isn't preserved by emission and doesn't need to be: semantic equality
			// compares the documents, so Terraform keeps the config's value.
			name:    "a variable with the spacing a human writes",
			value:   literalValue("Alert on {{ payload.service }}"),
			literal: document("Alert on {{ payload.service }}"),
			reads:   literalValue("Alert on {{payload.service}}"),
		},
		{
			// Filters emit in the spelling they're written in.
			name:    "a filtered variable",
			value:   literalValue("{{payload.summary | truncate: 100}}"),
			literal: document("{{payload.summary | truncate: 100}}"),
		},
		{
			// What data.incident_rich_text produces, and the fallback for anything the grammar
			// can't spell. Verbatim both ways, so it never diffs.
			name:    "a raw document",
			value:   literalValue(document("Prometheus alert")),
			literal: document("Prometheus alert"),
			reads:   literalValue("Prometheus alert"),
		},
		{
			name:  "a reference",
			value: referenceValue("payload.summary"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := TemplatedTextValueToPayload(tc.value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if payload == nil || payload.Value == nil {
				t.Fatalf("expected a value binding, got %+v", payload)
			}
			if got := lo.FromPtr(payload.Value.Literal); got != tc.literal {
				t.Errorf("literal sent\n got %q\nwant %q", got, tc.literal)
			}

			got, err := TemplatedTextValueFromPayload(payload)
			if err != nil {
				t.Fatalf("unexpected error reading back: %v", err)
			}

			want := tc.value
			if tc.reads != nil {
				want = tc.reads
			}
			if got == nil {
				t.Fatalf("expected a value, got nil")
			}
			if got.Literal.ValueString() != want.Literal.ValueString() ||
				!got.Reference.Equal(want.Reference) {
				t.Errorf("read back\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

// TestTemplatedTextValueToPayload covers the forms the schema can't express as a conflict.
func TestTemplatedTextValueToPayload(t *testing.T) {
	t.Run("sends nothing when absent", func(t *testing.T) {
		payload, err := TemplatedTextValueToPayload(nil)
		if payload != nil || err != nil {
			t.Errorf("an absent value should send no binding, got %+v (%v)", payload, err)
		}
	})

	t.Run("rejects both forms at once", func(t *testing.T) {
		value := literalValue("Prometheus alert")
		value.Reference = types.StringValue("payload.summary")

		if _, err := TemplatedTextValueToPayload(value); err == nil {
			t.Error("a value holding both forms should be rejected")
		}
	})

	t.Run("rejects a value holding nothing", func(t *testing.T) {
		if _, err := TemplatedTextValueToPayload(&TemplatedTextValue{
			Literal:   richtexttypes.NewTemplatedTextNull(),
			Reference: types.StringNull(),
		}); err == nil {
			t.Error("an empty value should be rejected rather than silently dropped")
		}
	})

	// An unresolved interpolation isn't an empty object, so it must not be reported as one — and
	// Terraform's placeholder must not reach the API.
	t.Run("sends nothing for an unknown", func(t *testing.T) {
		payload, err := TemplatedTextValueToPayload(&TemplatedTextValue{
			Literal:   richtexttypes.NewTemplatedTextUnknown(),
			Reference: types.StringNull(),
		})
		if payload != nil || err != nil {
			t.Errorf("an unknown value should send no binding, got %+v (%v)", payload, err)
		}
	})

	// The type reports these at plan time, so reaching the mapping with one is a provider bug:
	// fail the apply rather than send nonsense.
	t.Run("rejects an unparseable template", func(t *testing.T) {
		if _, err := TemplatedTextValueToPayload(literalValue("Alert on {{ payload.service")); err == nil {
			t.Error("an unclosed variable should be rejected")
		}
	})
}

// TestTemplatedTextValueFromPayload covers the shapes the API answers with that a single rich
// text value has no way to hold.
func TestTemplatedTextValueFromPayload(t *testing.T) {
	t.Run("reads absent as absent", func(t *testing.T) {
		value, err := TemplatedTextValueFromPayload(nil)
		if value != nil || err != nil {
			t.Errorf("no binding should read as no value, got %+v (%v)", value, err)
		}
	})

	// The API spells "holds nothing" as an object with neither field set, and storing that
	// against a config that omitted the attribute diffs on every plan.
	t.Run("reads an empty binding as absent", func(t *testing.T) {
		value, err := TemplatedTextValueFromPayload(&client.EngineParamBindingPayloadV3{
			Value: &client.EngineParamBindingValuePayloadV3{},
		})
		if value != nil || err != nil {
			t.Errorf("an empty binding should read as no value, got %+v (%v)", value, err)
		}
	})

	t.Run("reports several values", func(t *testing.T) {
		_, err := TemplatedTextValueFromPayload(&client.EngineParamBindingPayloadV3{
			ArrayValue: &[]client.EngineParamBindingValuePayloadV3{{Literal: lo.ToPtr("one")}},
		})
		if err == nil {
			t.Error("an array-valued binding should be reported, not dropped")
		}
	})
}

func TestValidateTemplatedTextValue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *TemplatedTextValue
		want  string
	}{
		{name: "absent", value: nil},
		{name: "a literal", value: literalValue("Prometheus alert")},
		{name: "a reference", value: referenceValue("payload.summary")},
		{
			name: "neither form",
			value: &TemplatedTextValue{
				Literal:   richtexttypes.NewTemplatedTextNull(),
				Reference: types.StringNull(),
			},
			want: "Missing value",
		},
		{
			name: "both forms",
			value: &TemplatedTextValue{
				Literal:   richtexttypes.NewTemplatedTextValue("Prometheus alert"),
				Reference: types.StringValue("payload.summary"),
			},
			want: "Ambiguous value",
		},
		{
			// Nothing is decided yet, so neither error is honest.
			name: "an unknown literal",
			value: &TemplatedTextValue{
				Literal:   richtexttypes.NewTemplatedTextUnknown(),
				Reference: types.StringNull(),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diags := diag.Diagnostics{}
			ValidateTemplatedTextValue(tc.value, path.Root("title"), &diags)

			switch {
			case tc.want == "" && diags.HasError():
				t.Errorf("expected no error, got %+v", diags.Errors())
			case tc.want != "" && !diags.HasError():
				t.Errorf("expected an error mentioning %q", tc.want)
			case tc.want != "" && diags.Errors()[0].Summary() != tc.want:
				t.Errorf("expected %q, got %q", tc.want, diags.Errors()[0].Summary())
			}
		})
	}
}
