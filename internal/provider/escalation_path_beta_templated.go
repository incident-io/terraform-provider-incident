package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
	"github.com/incident-io/terraform-provider-incident/v6/internal/provider/models"
)

// A templated escalation path is built from an incident_escalation_path_template: it names
// the template and binds each of the template's params, and takes its nodes, working hours
// and repeat config from the template. This file holds what incident_escalation_path_beta
// does differently for one.

func escalationPathBetaParamBindingsType() types.MapType {
	return types.MapType{ElemType: types.ObjectType{AttrTypes: models.ParamBindingAttrTypes()}}
}

// isTemplated reports whether the config describes a templated path. An unknown template_id
// still counts: the value is another resource's to fill in, but the attribute is set.
func (m *escalationPathBetaModel) isTemplated() bool {
	return !m.TemplateID.IsNull()
}

// escalationPathBetaTemplateIDRequiresReplace replaces the path when template_id goes from
// unset to set or back. The API refuses to change a path's kind in place, since converting
// has to decide what happens to the nodes or bindings left behind. Switching between two
// templates is an in-place update.
func escalationPathBetaTemplateIDRequiresReplace() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = req.StateValue.IsNull() != req.ConfigValue.IsNull()
		},
		"Replaces the escalation path when it changes between standalone and templated.",
		"Replaces the escalation path when it changes between standalone and templated. Switching from one template to another is an in-place update.",
	)
}

// validateEscalationPathBetaKind checks the config sets the attributes its kind takes and
// none of the other kind's. The API rejects the same combinations; catching them here names
// the attribute at plan time.
func validateEscalationPathBetaKind(data *escalationPathBetaModel, diags *diag.Diagnostics) {
	if data.isTemplated() {
		for name, value := range map[string]interface{ IsNull() bool }{
			"start":         data.Start,
			"sequences":     data.Sequences,
			"working_hours": data.WorkingHours,
			"repeat_config": data.RepeatConfig,
		} {
			if !value.IsNull() {
				diags.AddAttributeError(
					path.Root(name),
					fmt.Sprintf("%s is not valid on a templated escalation path", name),
					fmt.Sprintf("A path built from a template takes its %s from the template. Remove %s, or remove template_id to make this a standalone path.", name, name),
				)
			}
		}
		return
	}

	if data.Start.IsNull() {
		diags.AddAttributeError(path.Root("start"), "Missing start",
			"A standalone escalation path needs start, naming the sequence it begins with. Set template_id instead to build the path from a template.")
	}
	if data.Sequences.IsNull() {
		diags.AddAttributeError(path.Root("sequences"), "Missing sequences",
			"A standalone escalation path needs sequences. Set template_id instead to build the path from a template.")
	}
	if !data.ParamBindings.IsNull() {
		diags.AddAttributeError(path.Root("param_bindings"), "param_bindings needs template_id",
			"param_bindings binds the params of the template named by template_id, so it has nothing to bind without one.")
	}
}

func escalationPathBetaParamBindingsToPayload(ctx context.Context, bindings types.Map, diags *diag.Diagnostics) *map[string]client.EngineParamBindingPayloadV2 {
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

// escalationPathBetaParamBindingsFromAPI builds the param_bindings map for state, keeping
// the author's spelling of each binding from prior: the API returns the long form, and a
// config written as `value_literal` would otherwise plan a change forever.
func escalationPathBetaParamBindingsFromAPI(ctx context.Context, bindings *map[string]client.EngineParamBindingV2, prior types.Map, diags *diag.Diagnostics) types.Map {
	if bindings == nil {
		return types.MapNull(escalationPathBetaParamBindingsType().ElemType)
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

	value, d := types.MapValueFrom(ctx, escalationPathBetaParamBindingsType().ElemType, out)
	diags.Append(d...)
	return value
}
