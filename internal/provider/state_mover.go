package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// migrateStateMover moves state from an older resource that manages the same
// object, through the same API, under a schema that says it differently.
//
// Terraform asks the provider to move state whenever a `moved` block changes a
// resource's type, because it cannot know whether the two schemas are
// compatible - and getting that wrong loses somebody's state. This is the case
// where they are not compatible but the object underneath is the same one, so
// what the move has to carry is the object's identity:
//
//	moved {
//	  from = incident_schedule.primary
//	  to   = incident_schedule_beta.primary
//	}
//
// which is the difference between a migration somebody can review in a pull
// request and one they run by hand against production state with
// `terraform state rm` and an import.
//
// The state it writes is deliberately sparse. Attributes both schemas hold
// under the same name with the same type are copied across, and every other
// attribute is left null, because Terraform refreshes a moved resource before
// it plans it: whatever the mover leaves null is read back from the API by the
// new resource's own Read, in the shape the new schema wants. Copying is for
// the attributes a Read cannot recover - the ones the new resource keeps from
// prior state rather than from the API, like a schedule's `team_ids` or an
// alert source's `owning_team_ids`, which describe how the author wrote
// something rather than what the API holds.
//
// A rewrite carries one of those attributes anyway, for the case where the two
// schemas hold the same value in the same shape under different names - an
// alert source's title, which the v6 resource keeps under `template`. Reading
// one of those back is not enough: the framework only applies a type's semantic
// equality when the prior value is not null, so a refresh onto the null a move
// leaves stores the API's spelling of the value rather than the author's, and
// the next plan compares that against the configuration byte for byte. Carrying
// it keeps the author's spelling, which is what makes the plan after the move
// empty.
//
// It follows that a value the new resource derives, rather than reads, is
// whatever the refresh derives it as. An escalation path's sequence names are
// the case that matters: the API doesn't store them, so a move leaves the
// provider to name them, and a configuration that calls them something else
// plans a change. `sequences` and `start` are documented with the names to
// expect.
//
// A mover that does not recognise its source returns no state and no
// diagnostics, which the framework reads as "skipped": it tries the next mover
// the resource offers, and if none of them take the move Terraform reports an
// error naming the source and target types. That is a better error than any we
// could write here, so recognising the source is all this has to do.
func migrateStateMover(
	sourceTypeName string,
	sourceSchema, targetSchema schema.Schema,
	rewrites ...attributeRewrite,
) resource.StateMover {
	return resource.StateMover{
		// Setting this asks the framework to decode the source state against the old
		// resource's own schema, which is the only schema it was ever written with.
		SourceSchema: &sourceSchema,
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if req.SourceTypeName != sourceTypeName {
				return // not a source we know: skip, and let the framework try the next mover
			}

			// Schemas are versioned separately from the provider, and the resources this
			// moves from have only ever had the one version, so a source at another
			// version is state we have never written and cannot vouch for.
			if req.SourceSchemaVersion != sourceSchema.Version {
				return
			}

			// The framework logs its decoding error at debug level and leaves this nil,
			// so a nil here is the only sign we get that state we were meant to be able
			// to read did not read. Refuse the move rather than write a partial value.
			if req.SourceState == nil || req.SourceState.Raw.IsNull() {
				resp.Diagnostics.AddError(
					"Unable to Move Resource State",
					fmt.Sprintf(
						"The state held by %s could not be read using that resource's own schema, "+
							"which is the schema it was written with. This is always a problem with the "+
							"provider: please report it to us, with the output of `terraform plan` run "+
							"with TF_LOG=debug.\n\nSource resource type: %s\nSource schema version: %d",
						sourceTypeName, req.SourceTypeName, req.SourceSchemaVersion,
					),
				)
				return
			}

			var sourceAttributes map[string]tftypes.Value
			if err := req.SourceState.Raw.As(&sourceAttributes); err != nil {
				resp.Diagnostics.AddError(
					"Unable to Move Resource State",
					fmt.Sprintf(
						"The state held by %s is not an object, which the schema says it must be. "+
							"This is always a problem with the provider: please report it to us.\n\nError: %s",
						sourceTypeName, err,
					),
				)
				return
			}

			targetType, ok := targetSchema.Type().TerraformType(ctx).(tftypes.Object)
			if !ok {
				resp.Diagnostics.AddError(
					"Unable to Move Resource State",
					"The resource being moved to does not have an object schema, which every resource must. "+
						"This is always a problem with the provider: please report it to us.",
				)
				return
			}

			carried := map[string]bool{}
			for _, name := range sharedAttributes(ctx, sourceSchema, targetSchema) {
				carried[name] = true
			}

			rewritten := map[string]attributeRewrite{}
			for _, rewrite := range rewrites {
				rewritten[rewrite.target] = rewrite
			}

			values := make(map[string]tftypes.Value, len(targetType.AttributeTypes))
			for name, attributeType := range targetType.AttributeTypes {
				if carried[name] {
					values[name] = sourceAttributes[name]
					continue
				}

				if rewrite, isRewritten := rewritten[name]; isRewritten {
					if value, held := rewrite.value(sourceAttributes); held && value.Type().Equal(attributeType) {
						values[name] = value
						continue
					}

					// Nothing there to carry, or a shape the target attribute can't hold,
					// which means one of the two schemas has moved. Fall through to the
					// null below: the refresh still fills the attribute in, so the
					// migration plans a change rather than failing, and the unit test
					// naming the rewrite is what catches the drift.
				}

				// Null rather than unknown: state holds no unknowns, and null is what the
				// new resource's Read will overwrite on the refresh that follows.
				values[name] = tftypes.NewValue(attributeType, nil)
			}

			// Without the ID the moved resource names no object, so the refresh that would
			// have filled the rest in has nothing to ask about. Better to refuse a move
			// somebody can undo than to write state that reads as a resource to create.
			if id, held := values["id"]; !held || id.IsNull() {
				resp.Diagnostics.AddError(
					"Unable to Move Resource State",
					fmt.Sprintf(
						"The state held by %s has no ID to move, so the resource it is moving to "+
							"would not know which object it manages.\n\nIf the resource was never "+
							"applied there is nothing to move: remove the `moved` block and apply the "+
							"new resource instead.",
						sourceTypeName,
					),
				)
				return
			}

			resp.TargetState.Raw = tftypes.NewValue(targetType, values)

			// Private state is not carried across. It belongs to the resource that wrote
			// it rather than to the object, and the resource being moved to has never read
			// the old one's, so anything kept there would be a value it cannot interpret.
		},
	}
}

// attributeRewrite says where in the source state a target attribute's value comes
// from, for a value both schemas hold the same way but under different names. The path
// is attribute names from the root of the source state, walked one at a time, so an
// alert source's title reads `rewrite("title", "template", "title")`.
//
// The value is copied as it stands rather than translated: a rewrite is for the case
// where the two attributes hold the same Terraform type, and migrateStateMover checks
// that before writing it. Anything needing translation belongs in the new resource's
// Read, against the API, rather than here against state.
type attributeRewrite struct {
	target string
	from   []string
}

func rewrite(target string, from ...string) attributeRewrite {
	return attributeRewrite{target: target, from: from}
}

// value walks the source state to the value being carried. Not held means there is
// nothing there to carry: an attribute the old configuration never set, or a whole
// object it left out, which a move leaves null the same way it leaves the rest.
func (r attributeRewrite) value(sourceAttributes map[string]tftypes.Value) (tftypes.Value, bool) {
	attributes := sourceAttributes

	for idx, name := range r.from {
		value, held := attributes[name]
		if !held || value.IsNull() || !value.IsKnown() {
			return tftypes.Value{}, false
		}

		if idx == len(r.from)-1 {
			return value, true
		}

		// Another step to walk, so this one has to be an object to walk into.
		attributes = map[string]tftypes.Value{}
		if err := value.As(&attributes); err != nil {
			return tftypes.Value{}, false
		}
	}

	return tftypes.Value{}, false
}

// sharedAttributes returns the attributes the two schemas hold in common: the same
// name, and the same underlying type. Those are the ones a move can copy without
// translating anything, so they are the ones migrateStateMover carries across.
//
// Comparing types rather than trusting the name is what makes the copy safe. Two
// schemas can call something the same thing and hold it differently - an alert
// source's title lives under `template` in one and at the top level in the other -
// and copying between those would write a value the new resource cannot read.
// Anything left out here is filled in by the refresh instead.
func sharedAttributes(ctx context.Context, source, target schema.Schema) []string {
	sourceType, sourceIsObject := source.Type().TerraformType(ctx).(tftypes.Object)
	targetType, targetIsObject := target.Type().TerraformType(ctx).(tftypes.Object)
	if !sourceIsObject || !targetIsObject {
		return nil
	}

	shared := []string{}
	for name, targetAttribute := range targetType.AttributeTypes {
		sourceAttribute, inSource := sourceType.AttributeTypes[name]
		if inSource && sourceAttribute.Equal(targetAttribute) {
			shared = append(shared, name)
		}
	}
	sort.Strings(shared)

	return shared
}

// declaredResourceSchema returns the schema a resource declares, for the places that
// need it outside of the framework's own call to Schema - a state mover, which has to
// describe both the state it is moving from and the state it is writing.
func declaredResourceSchema(ctx context.Context, r resource.Resource) schema.Schema {
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)

	return resp.Schema
}

// resourceTypeName returns the type name a resource answers to, so a mover can name
// its source the way the provider registers it rather than repeating the string.
func resourceTypeName(ctx context.Context, r resource.Resource) string {
	var resp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &resp)

	return resp.TypeName
}

// movedFrom builds the mover a beta resource offers for the v6 resource it replaces,
// which is the one thing every one of them does the same way. Any rewrites are the
// attributes that resource renamed rather than restated: see attributeRewrite.
func movedFrom(
	ctx context.Context,
	source resource.Resource,
	target resource.Resource,
	rewrites ...attributeRewrite,
) []resource.StateMover {
	return []resource.StateMover{
		migrateStateMover(
			resourceTypeName(ctx, source),
			declaredResourceSchema(ctx, source),
			declaredResourceSchema(ctx, target),
			rewrites...,
		),
	}
}
