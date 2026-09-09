package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestEscalationPathResourceSchema builds the schema, which resolves every
// apischema.Docstring and EnumValuesDescription call against the embedded OpenAPI schema and
// panics if a definition or property is missing. Building it without a panic or an error
// diagnostic is the quickest check that every lookup the resource makes resolves.
func TestEscalationPathResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewEscalationPathResource()

	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_escalation_path" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}

	for _, name := range []string{"id", "name", "start", "sequences", "working_hours", "repeat_config", "team_ids"} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing expected attribute %q", name)
		}
	}
}

// TestEscalationPathNodeAttrTypesMatchSchema checks the node object's attribute types
// against the node schema. The two are written out separately, and a name in one that isn't
// in the other panics the framework at runtime rather than failing a build.
func TestEscalationPathNodeAttrTypesMatchSchema(t *testing.T) {
	attrTypes := escalationPathNodeAttrTypes()
	attributes := escalationPathNodeSchema().Attributes

	for name := range attrTypes {
		if _, ok := attributes[name]; !ok {
			t.Errorf("attr types have %q, which the node schema doesn't", name)
		}
	}
	for name := range attributes {
		if _, ok := attrTypes[name]; !ok {
			t.Errorf("node schema has %q, which the attr types don't", name)
		}
	}
}
