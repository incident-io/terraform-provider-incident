package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// migrateStateMover moves state from another name for the same resource.
//
// Terraform asks the provider to move state whenever a `moved` block changes a
// resource's type, because it cannot know whether the two schemas are
// compatible - and getting that wrong loses somebody's state. The renames in
// v7 are the case where they are not merely compatible but identical, because
// both names are one registration of one resource:
//
//	moved {
//	  from = incident_schedule_beta.primary
//	  to   = incident_schedule.primary
//	}
//
// which is the difference between a rename somebody can review in a pull
// request and one they run by hand against production state with
// `terraform state rm` and an import. See aliases.go.
//
// What it writes is decided by comparing the two schemas rather than assumed.
// Attributes both hold under the same name with the same type are copied
// across, and any other attribute is left null, because Terraform refreshes a
// moved resource before it plans it: whatever the mover leaves null is read
// back from the API by the new resource's own Read. For a rename that is
// nothing - every attribute is shared - and TestEveryAttributeMovesAcross
// fails if that ever stops being true, which it would if an alias drifted from
// the resource it aliases.
//
// A mover that does not recognise its source returns no state and no
// diagnostics, which the framework reads as "skipped": it tries the next mover
// the resource offers, and if none of them take the move Terraform reports an
// error naming the source and target types. That is a better error than any we
// could write here, so recognising the source is all this has to do.
func migrateStateMover(
	sourceTypeName string,
	sourceSchema, targetSchema schema.Schema,
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

			values := make(map[string]tftypes.Value, len(targetType.AttributeTypes))
			for name, attributeType := range targetType.AttributeTypes {
				if carried[name] {
					values[name] = sourceAttributes[name]
					continue
				}

				// Null rather than unknown: state holds no unknowns, and null is what the
				// new resource's Read will overwrite on the refresh that follows.
				values[name] = tftypes.NewValue(attributeType, nil)
			}

			// Without something naming the object, the moved resource manages nothing and a
			// plan reads it as a resource to create. Better to refuse a move somebody can
			// undo than to write that.
			for _, name := range identityAttributes(targetSchema) {
				if value, held := values[name]; !held || value.IsNull() {
					resp.Diagnostics.AddError(
						"Unable to Move Resource State",
						fmt.Sprintf(
							"The state held by %s has no %s to move, so the resource it is moving to "+
								"would not know which object it manages.\n\nIf the resource was never "+
								"applied there is nothing to move: remove the `moved` block and apply the "+
								"new resource instead.",
							sourceTypeName, name,
						),
					)
					return
				}
			}

			resp.TargetState.Raw = tftypes.NewValue(targetType, values)

			// Private state is not carried across. It belongs to the resource that wrote
			// it rather than to the object, and the resource being moved to has never read
			// the old one's, so anything kept there would be a value it cannot interpret.
		},
	}
}

// identityAttributes returns the attributes a moved resource needs filled in to name the
// object it manages.
//
// Most resources name it with `id`. One doesn't: an alert source attribute binding is
// keyed by the source and the attribute it binds, and has no `id` at all, so a mover that
// only ever checked `id` would refuse every move of one. Where there is no `id`, the
// attributes the schema makes Required are what the resource is addressed by.
func identityAttributes(target schema.Schema) []string {
	if _, hasID := target.Attributes["id"]; hasID {
		return []string{"id"}
	}

	required := []string{}
	for name, attribute := range target.Attributes {
		if attribute.IsRequired() {
			required = append(required, name)
		}
	}
	sort.Strings(required)

	return required
}

// sharedAttributes returns the attributes the two schemas hold in common: the same
// name, and the same underlying type. Those are the ones a move can copy without
// translating anything, so they are the ones migrateStateMover carries across.
//
// Comparing types rather than trusting the name is what makes the copy safe. Two
// schemas can call something the same thing and hold it differently, and copying
// between those would write a value the new resource cannot read. Anything left out
// here is filled in by the refresh instead.
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

// movedFrom builds the mover a renamed resource offers for the name it used to answer
// to, which is the one thing every one of them does the same way. The source is that
// resource constructed under its `_beta` alias: see aliases.go.
func movedFrom(
	ctx context.Context,
	source resource.Resource,
	target resource.Resource,
) []resource.StateMover {
	return []resource.StateMover{
		migrateStateMover(
			resourceTypeName(ctx, source),
			declaredResourceSchema(ctx, source),
			declaredResourceSchema(ctx, target),
		),
	}
}
