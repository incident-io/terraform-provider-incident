package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// form_fields has to be a list, because the order of the fields is the order
// they're shown in the form. That makes the stock UseStateForUnknown plan
// modifier the wrong tool for the nested computed id: it correlates with the
// prior state *by position*, so the ids silently shift as soon as the list does.
// Delete or insert anywhere but the end and every later field inherits its
// neighbour's id, so the update payload rewrites an existing form field (giving
// it a different key, and possibly a different type) instead of creating one and
// deleting the other.
//
// incident_catalog_entries avoids this by keying its nested collection on a
// stable external id (a MapNestedAttribute), but form fields have to stay
// ordered, so we correlate on `key` instead. It's unique within a workflow and
// is the identifier users already reference as form.<key>.

// formFieldIDForKey finds the prior form field with the given key and returns
// its id. found is false when no prior field has that key (a newly added field,
// whose id the API assigns) or when the matched id isn't usable.
func formFieldIDForKey(prior []IncidentWorkflowFormField, key types.String) (id types.String, found bool) {
	if key.IsNull() || key.IsUnknown() {
		return types.StringNull(), false
	}

	for _, field := range prior {
		if !field.Key.Equal(key) {
			continue
		}
		if field.ID.IsNull() || field.ID.IsUnknown() {
			return types.StringNull(), false
		}

		return field.ID, true
	}

	return types.StringNull(), false
}

// formFieldIDPlanModifier plans a form field's id from the prior field sharing
// its key, rather than the prior field in the same list position.
type formFieldIDPlanModifier struct{}

func (formFieldIDPlanModifier) Description(context.Context) string {
	return "Carries over the id of the prior form field with the same key, so ids follow the field rather than its position in the list."
}

func (m formFieldIDPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (formFieldIDPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// id is Computed and not Optional, so config never carries a value. Bail out
	// anyway if that ever changes: a value the user wrote — including one that's
	// still unknown, like a reference to another resource — is authoritative and
	// must not be replaced by a correlated id.
	if !req.ConfigValue.IsNull() {
		return
	}

	// Nothing to carry over when creating (or destroying) the workflow.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	// The key this field will have once applied.
	var plannedKey types.String
	if diags := req.Plan.GetAttribute(ctx, req.Path.ParentPath().AtName("key"), &plannedKey); diags.HasError() {
		return
	}

	var prior []IncidentWorkflowFormField
	if diags := req.State.GetAttribute(ctx, path.Root("form_fields"), &prior); diags.HasError() {
		return
	}

	// Terraform fills a null computed value from the prior state at the same index
	// before plan modifiers run, so an unmatched key has to be reset to unknown —
	// leaving it be would keep the positionally-inherited id. A key that's itself
	// unknown at plan time can't be correlated, so it also lands here and the API
	// assigns a fresh id.
	if id, found := formFieldIDForKey(prior, plannedKey); found {
		resp.PlanValue = id
	} else {
		resp.PlanValue = types.StringUnknown()
	}
}
