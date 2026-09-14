package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
	"github.com/incident-io/terraform-provider-incident/v7/internal/provider/models"
)

// A templated escalation path is built from an incident_escalation_path_template: it names
// the template and binds each of the template's params, and takes its nodes, working hours
// and repeat config from the template. This file holds what incident_escalation_path
// does differently for one.

func escalationPathParamBindingsType() types.MapType {
	return types.MapType{ElemType: types.ObjectType{AttrTypes: models.ParamBindingAttrTypes()}}
}

// isTemplated reports whether the path is built from a template.
//
// kind decides this, not template_id: the API takes the two separately and validates the id
// against the kind, so a path carrying a template_id without saying it is templated is a
// contradiction rather than an inference to make on the caller's behalf. The schema defaults
// kind to standalone, so this is settled for any config that doesn't compute it.
func (m *escalationPathModel) isTemplated() bool {
	return m.Kind.ValueString() == string(client.EscalationPathV2KindTemplated)
}

// escalationPathKindRequiresReplace replaces the path when its kind changes. The API
// refuses to change a path's kind in place, since converting has to decide what happens to
// the nodes or bindings left behind. Switching from one template to another keeps the kind,
// so it stays an in-place update.
func escalationPathKindRequiresReplace() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		escalationPathKindChanged,
		"Replaces the escalation path when it changes between standalone and templated.",
		"Replaces the escalation path when it changes between standalone and templated. Switching from one template to another is an in-place update.",
	)
}

// escalationPathKindChanged decides the replacement, and is separate so a test can reach
// it without building a plan.
func escalationPathKindChanged(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	// A kind the config computes rather than writes reaches us unknown, and destroying a
	// live path on a guess that could settle either way is worse than the alternative: left
	// alone, an apply that does turn out to change the kind is refused by the API, which is
	// an error you re-run rather than a path you have to rebuild.
	if req.PlanValue.IsUnknown() || req.StateValue.IsNull() {
		return
	}

	resp.RequiresReplace = req.PlanValue.ValueString() != req.StateValue.ValueString()
}

// isSetInConfig reports whether the configuration definitely gives this attribute a value.
//
// ValidateConfig sees the configuration rather than the plan, so anything written as a
// reference arrives unknown even when it would resolve to null: `working_hours = var.hours`
// reads the same whether the variable holds a list or nothing. Only a value that is known
// and non-null is something the caller has actually set.
func isSetInConfig(value attr.Value) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// validateEscalationPathKind checks the config sets the attributes its kind takes and
// none of the other kind's, mirroring what the API rejects so the attribute is named at plan
// time rather than part way through an apply.
func validateEscalationPathKind(data *escalationPathModel, diags *diag.Diagnostics) {
	// Nothing below can pick a side until the kind settles.
	if data.Kind.IsUnknown() {
		return
	}

	if data.isTemplated() {
		if data.TemplateID.IsNull() {
			diags.AddAttributeError(
				path.Root("template_id"),
				"Missing template_id",
				`A templated escalation path is built from a template, so it needs template_id. Remove kind = "templated" to make this a standalone path.`,
			)
		}

		for name, value := range map[string]attr.Value{
			"start":         data.Start,
			"sequences":     data.Sequences,
			"working_hours": data.WorkingHours,
			"repeat_config": data.RepeatConfig,
		} {
			if isSetInConfig(value) {
				diags.AddAttributeError(
					path.Root(name),
					fmt.Sprintf("%s is not valid on a templated escalation path", name),
					fmt.Sprintf("A path built from a template takes its %s from the template. Remove %s, or remove kind = \"templated\" to make this a standalone path.", name, name),
				)
			}
		}
		return
	}

	// Standalone, whether the config said so or left kind out.
	for name, value := range map[string]attr.Value{
		"template_id":    data.TemplateID,
		"param_bindings": data.ParamBindings,
	} {
		if isSetInConfig(value) {
			diags.AddAttributeError(
				path.Root(name),
				fmt.Sprintf("%s needs kind = \"templated\"", name),
				fmt.Sprintf("%s only applies to a path built from a template. Set kind = \"templated\", or remove %s.", name, name),
			)
		}
	}

	if data.Start.IsNull() {
		diags.AddAttributeError(path.Root("start"), "Missing start",
			`A standalone escalation path needs start, naming the sequence it begins with. Set kind = "templated" to build the path from a template instead.`)
	}
	if data.Sequences.IsNull() {
		diags.AddAttributeError(path.Root("sequences"), "Missing sequences",
			`A standalone escalation path needs sequences. Set kind = "templated" to build the path from a template instead.`)
	}
}

func escalationPathParamBindingsToPayload(ctx context.Context, bindings types.Map, diags *diag.Diagnostics) *map[string]client.EngineParamBindingPayloadV2 {
	if bindings.IsNull() || bindings.IsUnknown() {
		return nil
	}

	var decoded map[string]models.IncidentEngineParamBinding
	diags.Append(bindings.ElementsAs(ctx, &decoded, false)...)
	if diags.HasError() {
		return nil
	}

	out := make(map[string]client.EngineParamBindingPayloadV2, len(decoded))
	for name, binding := range decoded {
		out[name] = binding.ToPayload()
	}
	return &out
}

// escalationPathParamBindingsFromAPI builds the param_bindings map for state, keeping
// the author's spelling of each binding from prior: the API returns the long form, and a
// config written as `value_literal` would otherwise plan a change forever.
func escalationPathParamBindingsFromAPI(ctx context.Context, bindings *map[string]client.EngineParamBindingV2, prior types.Map, diags *diag.Diagnostics) types.Map {
	// A path that binds nothing holds null, not an empty map, or state and config would
	// disagree over which of the two an unbound path has. The API omits the field today
	// rather than sending {}, so this guards a shape it could start sending.
	if bindings == nil || len(*bindings) == 0 {
		return types.MapNull(escalationPathParamBindingsType().ElemType)
	}

	var priorBindings map[string]models.IncidentEngineParamBinding
	if !prior.IsNull() && !prior.IsUnknown() {
		var ignored diag.Diagnostics
		ignored.Append(prior.ElementsAs(ctx, &priorBindings, false)...)
	}

	out := make(map[string]models.IncidentEngineParamBinding, len(*bindings))
	for name, binding := range *bindings {
		converted := models.IncidentEngineParamBinding{}.FromAPI(binding)
		if priorBinding, ok := priorBindings[name]; ok {
			converted = *models.ReconcileBindingSpelling(lo.ToPtr(converted), lo.ToPtr(priorBinding))
		}
		out[name] = converted
	}

	value, d := types.MapValueFrom(ctx, escalationPathParamBindingsType().ElemType, out)
	diags.Append(d...)
	return value
}
