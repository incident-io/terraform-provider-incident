package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// TestEscalationPathTemplateResourceSchema builds the schema, which resolves every
// apischema.Docstring call against the embedded OpenAPI schema and panics if one is missing.
func TestEscalationPathTemplateResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewEscalationPathTemplateResource()

	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_escalation_path_template" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}

	for _, name := range []string{"id", "name", "description", "params", "expressions", "start", "sequences", "working_hours", "repeat_config"} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing expected attribute %q", name)
		}
	}
}

// TestEscalationPathTemplateNodeAttrTypesMatchSchema checks the template node's attribute
// types against its schema, the way the beta resource's test does for its own. The two are
// derived from the beta resource's, so what this really checks is that the targets swap
// happened on both sides.
func TestEscalationPathTemplateNodeAttrTypesMatchSchema(t *testing.T) {
	attrTypes := escalationPathTemplateNodeAttrTypes()
	attributes := escalationPathTemplateNodeSchema().Attributes

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

	levelType, ok := attrTypes["level"].(types.ObjectType)
	if !ok {
		t.Fatalf("level is not an object type: %T", attrTypes["level"])
	}
	targetsType, ok := levelType.AttrTypes["targets"].(types.ListType)
	if !ok {
		t.Fatalf("level.targets is not a list type: %T", levelType.AttrTypes["targets"])
	}
	targetType, ok := targetsType.ElemType.(types.ObjectType)
	if !ok {
		t.Fatalf("level.targets element is not an object type: %T", targetsType.ElemType)
	}
	if _, ok := targetType.AttrTypes["binding"]; !ok {
		t.Errorf("template level targets have no binding attribute type")
	}
}

// TestEscalationPathTemplateSequencesRoundTrip converts a template's sequences to the API
// payload and back, through the template codec, and checks the binding survives.
func TestEscalationPathTemplateSequencesRoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	target := escalationPathTemplateTarget{
		ID:             types.StringNull(),
		Type:           types.StringValue("schedule"),
		Urgency:        types.StringValue("high"),
		ScheduleMode:   types.StringValue("currently_on_call"),
		SelectedRotaID: types.StringNull(),
		Binding: lo.ToPtr(models.IncidentEngineParamBinding{
			ArrayValue:     types.ListNull(models.ParamBindingValueType()),
			Value:          types.ObjectNull(models.ParamBindingValueAttrTypes()),
			ValueReference: types.StringValue("primary_schedule"),
			Values:         types.ListNull(models.ParamBindingValuesListType().ElemType),
		}),
	}
	targets, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: escalationPathTemplateTargetAttrTypes()}, []escalationPathTemplateTarget{target})
	diags.Append(d...)

	want := map[string][]escalationPathBetaNode{
		"main": {{
			ID: types.StringNull(),
			Level: &IncidentEscalationPathNodeLevel{
				Targets:          targets,
				TimeToAckSeconds: types.Int64Value(300),
				AckMode:          types.StringValue("first"),
			},
		}},
	}

	payload := unflattenSequencesWith(ctx, templateSequenceCodec{}, "main", want, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected errors: %+v", diags)
	}
	if len(payload) != 1 || payload[0].Level == nil || len(payload[0].Level.Targets) != 1 {
		t.Fatalf("unexpected payload shape: %+v", payload)
	}
	binding := payload[0].Level.Targets[0].Binding
	if binding == nil || binding.Value == nil || binding.Value.Reference == nil || *binding.Value.Reference != "primary_schedule" {
		t.Fatalf("binding did not reach the payload: %+v", binding)
	}
	if payload[0].Level.Targets[0].Id != nil {
		t.Errorf("a bound target should send no id, got %q", *payload[0].Level.Targets[0].Id)
	}

	// What the API would hand back for that payload.
	response := []client.EscalationPathTemplateNodeV2{{
		Id:   payload[0].Id,
		Type: client.EscalationPathTemplateNodeV2Type(payload[0].Type),
		Level: &client.EscalationPathNodeLevelWithBindingV2{
			TimeToAckSeconds: payload[0].Level.TimeToAckSeconds,
			AckMode:          lo.ToPtr(client.EscalationPathNodeLevelWithBindingV2AckMode("first")),
			Targets: []client.EscalationPathTargetWithBindingV2{{
				Type:         client.EscalationPathTargetWithBindingV2Type("schedule"),
				Urgency:      client.EscalationPathTargetWithBindingV2Urgency("high"),
				ScheduleMode: lo.ToPtr(client.EscalationPathTargetWithBindingV2ScheduleMode("currently_on_call")),
				Binding: &client.EngineParamBindingV2{Value: &client.EngineParamBindingValueV2{
					Label:     "Primary schedule",
					Reference: lo.ToPtr("primary_schedule"),
				}},
			}},
		},
	}}

	start, got := flattenSequencesWith(ctx, templateSequenceCodec{}, response, escalationPathBetaPriorNames{start: "main", sequences: want}, &diags)
	reconcileTemplateBindingSpelling(ctx, got, want, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected errors: %+v", diags)
	}
	if start != "main" || len(got["main"]) != 1 || got["main"][0].Level == nil {
		t.Fatalf("unexpected sequences: start=%q %+v", start, got)
	}

	gotTargets := decodeTemplateTargets(ctx, got["main"][0].Level.Targets, &diags)
	if len(gotTargets) != 1 || gotTargets[0].Binding == nil {
		t.Fatalf("binding did not survive the read: %+v", gotTargets)
	}
	if !gotTargets[0].ID.IsNull() {
		t.Errorf("a bound target should read back with a null id, got %v", gotTargets[0].ID)
	}
	// The author wrote value_reference, so that is what state should hold rather than the
	// long form the API returns.
	if gotTargets[0].Binding.ValueReference.ValueString() != "primary_schedule" {
		t.Errorf("binding spelling not kept: %+v", gotTargets[0].Binding)
	}
}

// TestValidateEscalationPathBetaKind covers the standalone/templated attribute rules.
func TestValidateEscalationPathBetaKind(t *testing.T) {
	sequences := types.MapValueMust(sequenceMapType(escalationPathBetaNodeAttrTypes()).ElemType, map[string]attrValue(nil))
	bindings := types.MapValueMust(escalationPathBetaParamBindingsType().ElemType, map[string]attrValue(nil))

	cases := map[string]struct {
		model     escalationPathBetaModel
		wantError bool
	}{
		"standalone": {
			model: escalationPathBetaModel{Start: types.StringValue("main"), Sequences: sequences,
				TemplateID: types.StringNull(), ParamBindings: types.MapNull(escalationPathBetaParamBindingsType().ElemType)},
		},
		"standalone missing sequences": {
			model:     escalationPathBetaModel{Start: types.StringValue("main"), Sequences: types.MapNull(sequenceMapType(escalationPathBetaNodeAttrTypes()).ElemType), TemplateID: types.StringNull()},
			wantError: true,
		},
		"standalone with param_bindings": {
			model:     escalationPathBetaModel{Start: types.StringValue("main"), Sequences: sequences, TemplateID: types.StringNull(), ParamBindings: bindings},
			wantError: true,
		},
		"templated": {
			model: escalationPathBetaModel{TemplateID: types.StringValue("tmpl"), ParamBindings: bindings,
				Sequences: types.MapNull(sequenceMapType(escalationPathBetaNodeAttrTypes()).ElemType)},
		},
		// A config writing template_id as a reference or a variable reaches ValidateConfig
		// unknown, where it could still settle either way, so neither kind's rules apply.
		"unknown template_id with sequences": {
			model: escalationPathBetaModel{TemplateID: types.StringUnknown(), Start: types.StringValue("main"), Sequences: sequences},
		},
		"unknown template_id with nothing else": {
			model: escalationPathBetaModel{TemplateID: types.StringUnknown(),
				Sequences: types.MapNull(sequenceMapType(escalationPathBetaNodeAttrTypes()).ElemType)},
		},
		"templated with unknown template_id": {
			model: escalationPathBetaModel{TemplateID: types.StringUnknown(), ParamBindings: bindings,
				Sequences: types.MapNull(sequenceMapType(escalationPathBetaNodeAttrTypes()).ElemType)},
		},
		"templated with sequences": {
			model:     escalationPathBetaModel{TemplateID: types.StringValue("tmpl"), Start: types.StringValue("main"), Sequences: sequences},
			wantError: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateEscalationPathBetaKind(&tc.model, &diags)
			if diags.HasError() != tc.wantError {
				t.Errorf("got errors=%v (%+v), want %v", diags.HasError(), diags, tc.wantError)
			}
		})
	}
}

type attrValue = attr.Value
