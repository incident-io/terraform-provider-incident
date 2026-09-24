package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
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

	want := map[string][]escalationPathNode{
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

	start, got := flattenSequencesWith(ctx, templateSequenceCodec{}, response, escalationPathPriorNames{start: "main", sequences: want}, &diags)
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

// TestValidateEscalationPathKind covers the rules the API enforces on the two kinds,
// which the provider mirrors so the attribute is named at plan time.
func TestValidateEscalationPathKind(t *testing.T) {
	sequences := types.MapValueMust(sequenceMapType(escalationPathNodeAttrTypes()).ElemType, map[string]attrValue(nil))
	bindings := types.MapValueMust(escalationPathParamBindingsType().ElemType, map[string]attrValue(nil))
	noSequences := types.MapNull(sequenceMapType(escalationPathNodeAttrTypes()).ElemType)
	noBindings := types.MapNull(escalationPathParamBindingsType().ElemType)

	standalone := func(m escalationPathModel) escalationPathModel {
		m.Kind = types.StringValue("standalone")
		return m
	}
	templated := func(m escalationPathModel) escalationPathModel {
		m.Kind = types.StringValue("templated")
		return m
	}

	cases := map[string]struct {
		model     escalationPathModel
		wantError bool
	}{
		"standalone": {
			model: standalone(escalationPathModel{Start: types.StringValue("main"), Sequences: sequences,
				TemplateID: types.StringNull(), ParamBindings: noBindings}),
		},
		"standalone missing sequences": {
			model: standalone(escalationPathModel{Start: types.StringValue("main"), Sequences: noSequences,
				TemplateID: types.StringNull(), ParamBindings: noBindings}),
			wantError: true,
		},
		"standalone carrying a template_id": {
			model: standalone(escalationPathModel{Start: types.StringValue("main"), Sequences: sequences,
				TemplateID: types.StringValue("tmpl"), ParamBindings: noBindings}),
			wantError: true,
		},
		"standalone carrying param_bindings": {
			model: standalone(escalationPathModel{Start: types.StringValue("main"), Sequences: sequences,
				TemplateID: types.StringNull(), ParamBindings: bindings}),
			wantError: true,
		},
		"templated": {
			model: templated(escalationPathModel{TemplateID: types.StringValue("tmpl"), ParamBindings: bindings,
				Sequences: noSequences}),
		},
		"templated with a template_id another resource fills in": {
			model: templated(escalationPathModel{TemplateID: types.StringUnknown(), ParamBindings: bindings,
				Sequences: noSequences}),
		},
		"templated without a template_id": {
			model: templated(escalationPathModel{TemplateID: types.StringNull(), ParamBindings: bindings,
				Sequences: noSequences}),
			wantError: true,
		},
		"templated carrying sequences": {
			model: templated(escalationPathModel{TemplateID: types.StringValue("tmpl"),
				Start: types.StringValue("main"), Sequences: sequences}),
			wantError: true,
		},
		// ValidateConfig sees a reference as unknown even when it resolves to null, so an
		// attribute written that way is not something the caller has set.
		"templated with working_hours another value fills in": {
			model: templated(escalationPathModel{TemplateID: types.StringValue("tmpl"), ParamBindings: bindings,
				Sequences: noSequences, WorkingHours: types.ListUnknown(types.StringType)}),
		},
		"standalone with a template_id another value fills in": {
			model: standalone(escalationPathModel{Start: types.StringValue("main"), Sequences: sequences,
				TemplateID: types.StringUnknown(), ParamBindings: noBindings}),
		},
		// Nothing can pick a side until the kind settles, so neither kind's rules apply.
		"kind the config computes": {
			model: escalationPathModel{Kind: types.StringUnknown(), TemplateID: types.StringValue("tmpl"),
				Start: types.StringValue("main"), Sequences: sequences},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateEscalationPathKind(&tc.model, &diags)
			if diags.HasError() != tc.wantError {
				t.Errorf("got errors=%v (%+v), want %v", diags.HasError(), diags, tc.wantError)
			}
		})
	}
}

// TestEscalationPathKindChanged covers the replacement decision. A path is one kind for
// life, so a change is a destroy and create, and anything else must not be.
func TestEscalationPathKindChanged(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state types.String
		plan  types.String
		want  bool
	}{
		{"standalone becoming templated replaces", types.StringValue("standalone"), types.StringValue("templated"), true},
		{"templated becoming standalone replaces", types.StringValue("templated"), types.StringValue("standalone"), true},
		{"a standalone path left alone does not", types.StringValue("standalone"), types.StringValue("standalone"), false},
		{"swapping one template for another does not", types.StringValue("templated"), types.StringValue("templated"), false},
		{"an unsettled kind does not", types.StringValue("standalone"), types.StringUnknown(), false},
		{"a path being created does not", types.StringNull(), types.StringValue("templated"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &stringplanmodifier.RequiresReplaceIfFuncResponse{}
			escalationPathKindChanged(context.Background(), planmodifier.StringRequest{
				StateValue: tc.state,
				PlanValue:  tc.plan,
			}, resp)

			if resp.RequiresReplace != tc.want {
				t.Errorf("RequiresReplace is %v, want %v", resp.RequiresReplace, tc.want)
			}
		})
	}
}

type attrValue = attr.Value

func TestValidateEscalationPathTemplateTargetRota(t *testing.T) {
	rotaTarget := func(id, rotaID string, binding *models.IncidentEngineParamBinding) escalationPathTemplateTarget {
		target := escalationPathTemplateTarget{
			ID:             types.StringNull(),
			Type:           types.StringValue("schedule"),
			Urgency:        types.StringValue("high"),
			ScheduleMode:   types.StringValue("currently_on_call_for_rota"),
			SelectedRotaID: types.StringNull(),
			Binding:        binding,
		}
		if id != "" {
			target.ID = types.StringValue(id)
		}
		if rotaID != "" {
			target.SelectedRotaID = types.StringValue(rotaID)
		}
		return target
	}
	expressionBinding := func() *models.IncidentEngineParamBinding {
		binding := models.NullParamBinding()
		binding.ExpressionRef = types.StringValue("rotation_schedule_1_primary")
		return &binding
	}

	cases := []struct {
		name    string
		target  escalationPathTemplateTarget
		wantErr string
	}{
		{name: "bound target takes its rota from the binding", target: rotaTarget("", "", expressionBinding())},
		{name: "bound target may still name a rota", target: rotaTarget("", "01ROTA", expressionBinding())},
		{name: "concrete target needs a rota", target: rotaTarget("01SCHEDULE", "", nil), wantErr: "Missing selected_rota_id"},
		{name: "concrete target with a rota", target: rotaTarget("01SCHEDULE", "01ROTA", nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateEscalationPathTemplateTarget(tc.target, &diags)
			if tc.wantErr == "" {
				if diags.HasError() {
					t.Fatalf("unexpected errors: %+v", diags)
				}
				return
			}
			if !diags.HasError() || diags.Errors()[0].Summary() != tc.wantErr {
				t.Fatalf("want %q, got %+v", tc.wantErr, diags)
			}
		})
	}
}
