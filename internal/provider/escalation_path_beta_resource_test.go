package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestEscalationPathBetaResourceSchema builds the schema, which resolves every
// apischema.Docstring and EnumValuesDescription call against the embedded OpenAPI schema and
// panics if a definition or property is missing. Building it without a panic or an error
// diagnostic is the quickest check that every lookup the resource makes resolves.
func TestEscalationPathBetaResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewEscalationPathBetaResource()

	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_escalation_path_beta" {
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

// TestEscalationPathBetaNodeAttrTypesMatchSchema checks the node object's attribute types
// against the node schema. The two are written out separately, and a name in one that isn't
// in the other panics the framework at runtime rather than failing a build.
func TestEscalationPathBetaNodeAttrTypesMatchSchema(t *testing.T) {
	attrTypes := escalationPathBetaNodeAttrTypes()
	attributes := escalationPathBetaNodeSchema().Attributes

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

// TestEscalationPathNodeBlocksSharedWithGA checks the level, notify_channel and delay blocks
// really are one definition: both resources build them from the same helpers, so the beta
// resource picking up a new field on one of them shouldn't need a second edit.
func TestEscalationPathNodeBlocksSharedWithGA(t *testing.T) {
	ga := (&IncidentEscalationPathResource{}).getPathSchema(1).Attributes
	beta := escalationPathBetaNodeSchema().Attributes

	for _, name := range []string{"notify_channel", "delay"} {
		gaBlock, ok := ga[name].(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s is not a single nested attribute on incident_escalation_path", name)
		}
		betaBlock, ok := beta[name].(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s is not a single nested attribute on incident_escalation_path_beta", name)
		}

		for attr := range gaBlock.Attributes {
			if _, ok := betaBlock.Attributes[attr]; !ok {
				t.Errorf("%s.%s is on incident_escalation_path but not incident_escalation_path_beta", name, attr)
			}
		}
	}

	// level differs in one attribute only: the GA resource defaults ack_mode to "all",
	// which the beta resource defaults to "first".
	gaLevel, ok := ga["level"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("level is not a single nested attribute on incident_escalation_path")
	}
	betaLevel, ok := beta["level"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("level is not a single nested attribute on incident_escalation_path_beta")
	}
	for attr := range gaLevel.Attributes {
		if _, ok := betaLevel.Attributes[attr]; !ok {
			t.Errorf("level.%s is on incident_escalation_path but not incident_escalation_path_beta", attr)
		}
	}
}
