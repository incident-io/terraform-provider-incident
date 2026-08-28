package provider

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

const (
	testAlertSourceID    = "01SOURCE"
	testAlertAttributeID = "01ATTRIBUTE"
)

// TestAlertSourceAttributeBetaResourceSchema builds the schema, which resolves every
// apischema.Docstring call against the embedded OpenAPI schema and panics if a definition or
// property is missing — the quickest way to catch the resource being built against a stale
// vendored schema.
func TestAlertSourceAttributeBetaResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewAlertSourceAttributeBetaResource()

	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_alert_source_attribute_beta" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}

	for _, name := range []string{
		"alert_source_id", "alert_attribute_id", "merge_strategy",
		"value_literal", "value_reference", "expression_ref", "values", "value", "array_value",
	} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing expected attribute %q", name)
		}
	}

	for _, name := range []string{"expression", "named_expression"} {
		if _, ok := schemaResp.Schema.Blocks[name]; !ok {
			t.Errorf("schema missing expected block %q", name)
		}
	}

	// The pair of ids is the identity, so there is no id to mint.
	if _, ok := schemaResp.Schema.Attributes["id"]; ok {
		t.Error("schema should not carry an id")
	}
}

// TestAlertSourceAttributeImportIDRoundTrips pins the two-part key: whatever the provider tells
// people to import has to be what ImportState accepts.
func TestAlertSourceAttributeImportIDRoundTrips(t *testing.T) {
	imported := importAlertSourceAttribute(t, alertSourceAttributeImportID(testAlertSourceID, testAlertAttributeID))

	if got := stateString(t, imported, "alert_source_id"); got != testAlertSourceID {
		t.Errorf("alert_source_id is %q, want %q", got, testAlertSourceID)
	}
	if got := stateString(t, imported, "alert_attribute_id"); got != testAlertAttributeID {
		t.Errorf("alert_attribute_id is %q, want %q", got, testAlertAttributeID)
	}
}

func TestAlertSourceAttributeImportIDRejectsOnePart(t *testing.T) {
	r, ok := NewAlertSourceAttributeBetaResource().(*alertSourceAttributeBetaResource)
	if !ok {
		t.Fatalf("NewAlertSourceAttributeBetaResource did not return a *alertSourceAttributeBetaResource")
	}

	for _, id := range []string{testAlertSourceID, ":" + testAlertAttributeID, testAlertSourceID + ":", ""} {
		resp := resource.ImportStateResponse{State: emptyAlertSourceAttributeState(t)}
		r.ImportState(
			context.Background(),
			resource.ImportStateRequest{ID: id},
			&resp,
		)

		if !resp.Diagnostics.HasError() {
			t.Errorf("import id %q was accepted, and names no attribute to read", id)
		}
	}
}

// TestAlertSourceAttributeConflict covers the two things a 409 means. The API answers it both
// for an already-bound attribute and for losing the race on the source's config lock, and
// telling someone to import a binding that was never made sends them somewhere useless.
func TestAlertSourceAttributeConflict(t *testing.T) {
	err := client.HTTPError{StatusCode: 409, Body: []byte(`{"type":"conflict"}`)}

	t.Run("points at import when the attribute really is bound", func(t *testing.T) {
		summary, detail := alertSourceAttributeConflict(true, testAlertSourceID, testAlertAttributeID, err)

		if !strings.Contains(summary, "already bound") {
			t.Errorf("summary is %q, want it to say the attribute is bound", summary)
		}
		if want := alertSourceAttributeImportID(testAlertSourceID, testAlertAttributeID); !strings.Contains(detail, want) {
			t.Errorf("detail does not carry the import id %q: %s", want, detail)
		}
	})

	t.Run("says to run again when nothing was bound", func(t *testing.T) {
		summary, detail := alertSourceAttributeConflict(false, testAlertSourceID, testAlertAttributeID, err)

		if strings.Contains(summary, "already bound") || strings.Contains(detail, "terraform import") {
			t.Errorf("a contended write was reported as an existing binding: %s / %s", summary, detail)
		}
		if !strings.Contains(detail, "again") {
			t.Errorf("detail does not say it is worth retrying: %s", detail)
		}
	})
}

// TestAlertSourceAttributeBetaFromAPIValue covers the value spellings living at the top level:
// a read has to put them back in the spelling the config wrote.
func TestAlertSourceAttributeBetaFromAPIValue(t *testing.T) {
	t.Run("reads a literal back as value_literal", func(t *testing.T) {
		model := alertSourceAttributeBetaFromAPI(alertSourceAttributeV3(func(attribute *client.AlertSourceAttributeV3) {
			attribute.Value = &client.EngineParamBindingValuePayloadV3{Literal: lo.ToPtr("high")}
		}), &alertSourceAttributeBetaModel{})

		if model.ValueLiteral.ValueString() != "high" {
			t.Errorf("value_literal is %q, want %q", model.ValueLiteral.ValueString(), "high")
		}
		if !model.ValueReference.IsNull() || !model.ExpressionRef.IsNull() {
			t.Error("a literal set another spelling too, so every plan would diff")
		}
	})

	t.Run("reads an all-literal array back as values", func(t *testing.T) {
		model := alertSourceAttributeBetaFromAPI(alertSourceAttributeV3(func(attribute *client.AlertSourceAttributeV3) {
			attribute.ArrayValue = &[]client.EngineParamBindingValuePayloadV3{
				{Literal: lo.ToPtr("one")}, {Literal: lo.ToPtr("two")},
			}
		}), &alertSourceAttributeBetaModel{})

		want := []types.String{types.StringValue("one"), types.StringValue("two")}
		if !reflect.DeepEqual(model.Values, want) {
			t.Errorf("values is %#v, want %#v", model.Values, want)
		}
	})

	// `value = { literal = "high" }` and `value_literal = "high"` are the same payload, so the
	// read has to take the spelling from the prior or the apply fails as an inconsistent result.
	t.Run("keeps the long form the config wrote", func(t *testing.T) {
		prior := &alertSourceAttributeBetaModel{
			Value: &models.BindingValue{Literal: types.StringValue("high")},
		}

		model := alertSourceAttributeBetaFromAPI(alertSourceAttributeV3(func(attribute *client.AlertSourceAttributeV3) {
			attribute.Value = &client.EngineParamBindingValuePayloadV3{Literal: lo.ToPtr("high")}
		}), prior)

		if !model.ValueLiteral.IsNull() {
			t.Errorf("read rewrote the long form to value_literal = %q", model.ValueLiteral.ValueString())
		}
		if model.Value == nil || model.Value.Literal.ValueString() != "high" {
			t.Errorf("value did not round trip, got %#v", model.Value)
		}
	})

	t.Run("takes the merge strategy the API settled on", func(t *testing.T) {
		model := alertSourceAttributeBetaFromAPI(alertSourceAttributeV3(nil), &alertSourceAttributeBetaModel{})

		if model.MergeStrategy.ValueString() != "first_wins" {
			t.Errorf("merge_strategy is %q, want the API's answer", model.MergeStrategy.ValueString())
		}
	})
}

// TestAlertSourceAttributeBetaBoundExpressionRoundTrip is the property the expression block
// rests on: declaring it binds its result, so the reference the provider minted has to fold
// back into the block rather than reading as a value or a named_expression.
func TestAlertSourceAttributeBetaBoundExpressionRoundTrip(t *testing.T) {
	config := &alertSourceAttributeBetaModel{
		AlertSourceID:    types.StringValue(testAlertSourceID),
		AlertAttributeID: types.StringValue(testAlertAttributeID),
		Expression: &models.Expression{
			StartFrom:  types.StringValue("payload"),
			Operations: []models.Operation{{Cast: &models.Cast{As: types.StringValue("Text")}}},
		},
	}

	binding := (&alertSourceAttributeBetaResource{}).toPayload(config, &diag.Diagnostics{})

	model := alertSourceAttributeBetaFromAPI(alertSourceAttributeV3(func(attribute *client.AlertSourceAttributeV3) {
		attribute.Value = binding.value
		attribute.Expressions = binding.expressions
	}), config)

	if !reflect.DeepEqual(model.Expression, config.Expression) {
		t.Errorf("expression did not round trip\n got: %#v\nwant: %#v", model.Expression, config.Expression)
	}
	if len(model.NamedExpressions) > 0 {
		t.Errorf("the minted expression surfaced as a named_expression nobody wrote: %#v", model.NamedExpressions)
	}
	if !model.ExpressionRef.IsNull() || !model.ValueReference.IsNull() {
		t.Error("the minted reference surfaced as a value, which the config has no field for")
	}
}

// TestAlertSourceAttributeBetaMintsPerAttribute is why the minted name isn't a constant: two
// resources on one alert source would claim the same reference, and the API refuses the second
// as somebody else's.
func TestAlertSourceAttributeBetaMintsPerAttribute(t *testing.T) {
	expression := &models.Expression{
		StartFrom:  types.StringValue("payload"),
		Operations: []models.Operation{{Cast: &models.Cast{As: types.StringValue("Text")}}},
	}

	references := []string{}
	for _, attributeID := range []string{"01TEAM", "01SERVICE"} {
		binding := (&alertSourceAttributeBetaResource{}).toPayload(&alertSourceAttributeBetaModel{
			AlertAttributeID: types.StringValue(attributeID),
			Expression:       expression,
		}, &diag.Diagnostics{})

		if len(binding.expressions) != 1 {
			t.Fatalf("got %d expressions, want the one the block declares", len(binding.expressions))
		}
		references = append(references, binding.expressions[0].Reference)
	}

	if references[0] == references[1] {
		t.Errorf("both attributes minted %q, so the second write would be refused", references[0])
	}
}

// TestAlertSourceAttributeBetaValueReachesItsExpression covers the value that sits outside the
// expression tree and still names something in it. Left unrenamed it round trips happily and fails
// at apply.
func TestAlertSourceAttributeBetaValueReachesItsExpression(t *testing.T) {
	binding := (&alertSourceAttributeBetaResource{}).toPayload(&alertSourceAttributeBetaModel{
		AlertAttributeID: types.StringValue(testAlertAttributeID),
		ExpressionRef:    types.StringValue("zebra"),
		NamedExpressions: []models.NamedExpression{{
			Name:       types.StringValue("zebra"),
			StartFrom:  types.StringValue("payload"),
			Operations: []models.Operation{{Cast: &models.Cast{As: types.StringValue("Text")}}},
		}},
	}, &diag.Diagnostics{})

	if binding.value == nil || binding.value.Reference == nil {
		t.Fatal("expected the value to reference the named expression")
	}

	name := models.ExpressionNameFromReference(*binding.value.Reference)
	if !slices.ContainsFunc(binding.expressions, func(payload client.ExpressionPayloadV3) bool {
		return payload.Reference == name
	}) {
		t.Errorf("the value references %q, which none of the expressions written is called",
			*binding.value.Reference)
	}
}

// TestAlertSourceAttributeBetaNamedExpressionOrder covers named_expression being a list: the API
// returns them sorted by reference, so a read has to restore the config's order or a config
// nobody touched plans a change.
func TestAlertSourceAttributeBetaNamedExpressionOrder(t *testing.T) {
	named := func(names ...string) []models.NamedExpression {
		return lo.Map(names, func(name string, _ int) models.NamedExpression {
			return models.NamedExpression{
				Name:       types.StringValue(name),
				StartFrom:  types.StringValue("payload"),
				Operations: []models.Operation{{Cast: &models.Cast{As: types.StringValue("Text")}}},
			}
		})
	}

	config := &alertSourceAttributeBetaModel{
		AlertAttributeID: types.StringValue(testAlertAttributeID),
		ExpressionRef:    types.StringValue("zebra"),
		NamedExpressions: named("zebra", "aardvark"),
	}

	binding := (&alertSourceAttributeBetaResource{}).toPayload(config, &diag.Diagnostics{})

	model := alertSourceAttributeBetaFromAPI(alertSourceAttributeV3(func(attribute *client.AlertSourceAttributeV3) {
		attribute.Value = binding.value
		// Sorted by reference, which is the order the API reads them back in.
		attribute.Expressions = []client.ExpressionPayloadV3{binding.expressions[1], binding.expressions[0]}
	}), config)

	if !reflect.DeepEqual(model.NamedExpressions, config.NamedExpressions) {
		t.Errorf("named expressions came back in a different order\n got: %#v\nwant: %#v",
			model.NamedExpressions, config.NamedExpressions)
	}
}

// alertSourceAttributeV3 is a bound attribute with no value, for a test to fill in the one part
// it cares about.
func alertSourceAttributeV3(with func(*client.AlertSourceAttributeV3)) client.AlertSourceAttributeV3 {
	attribute := client.AlertSourceAttributeV3{
		AlertSourceId:    testAlertSourceID,
		AlertAttributeId: testAlertAttributeID,
		MergeStrategy:    client.AlertSourceAttributeV3MergeStrategyFirstWins,
		Expressions:      []client.ExpressionPayloadV3{},
	}
	if with != nil {
		with(&attribute)
	}

	return attribute
}

func emptyAlertSourceAttributeState(t *testing.T) tfsdk.State {
	t.Helper()

	config := alertSourceAttributeConfig(t, nil)

	return tfsdk.State(config)
}

func importAlertSourceAttribute(t *testing.T, id string) tfsdk.State {
	t.Helper()

	r, ok := NewAlertSourceAttributeBetaResource().(*alertSourceAttributeBetaResource)
	if !ok {
		t.Fatalf("NewAlertSourceAttributeBetaResource did not return a *alertSourceAttributeBetaResource")
	}

	resp := resource.ImportStateResponse{State: emptyAlertSourceAttributeState(t)}
	r.ImportState(
		context.Background(),
		resource.ImportStateRequest{ID: id},
		&resp,
	)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import of %q failed: %+v", id, resp.Diagnostics.Errors())
	}

	return resp.State
}

func stateString(t *testing.T, state tfsdk.State, name string) string {
	t.Helper()

	var value types.String
	if diags := state.GetAttribute(context.Background(), path.Root(name), &value); diags.HasError() {
		t.Fatalf("reading %q: %+v", name, diags.Errors())
	}

	return value.ValueString()
}
