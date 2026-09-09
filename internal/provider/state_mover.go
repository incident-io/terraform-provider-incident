package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// renameStateMover moves state from a resource that has been renamed, where the
// schema either side of the rename is the same one.
//
// Terraform asks the provider to move state whenever a `moved` block changes a
// resource's type, because it cannot know whether the two schemas are
// compatible - and getting that wrong loses somebody's state. A rename is the
// case where they are identical, so the move is a copy: whatever the source
// resource held is what the target should hold, and a `terraform plan` straight
// afterwards reports no changes.
//
// The source type name is the name the resource used to go by, which is what a
// practitioner writes in the `from` argument:
//
//	moved {
//	  from = incident_schedule_beta.primary
//	  to   = incident_schedule.primary
//	}
//
// A mover that does not recognise the source returns no state and no
// diagnostics, which the framework reads as "skipped": it tries the next mover
// the resource offers, and if none of them take the move it reports an error
// naming the source and target types. That is a better error than any we could
// write here, so recognising the source is all this has to do.
//
// It is deliberately strict about the schema version. Writing state we do not
// understand into the target resource is worse than refusing the move, because
// a practitioner can undo a refusal.
func renameStateMover(sourceTypeName string, sourceSchema schema.Schema) resource.StateMover {
	return resource.StateMover{
		// Setting this asks the framework to decode the source state for us, which for
		// a rename it can always do, the schema being the target's own.
		SourceSchema: &sourceSchema,
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if req.SourceTypeName != sourceTypeName {
				return // not a source we know: skip, and let the framework try the next mover
			}

			// Schemas are versioned separately from the provider, and this resource has
			// only ever had the one version, so a source at another version is state we
			// have never written and cannot vouch for.
			if req.SourceSchemaVersion != sourceSchema.Version {
				return
			}

			// The framework logs its decoding error at debug level and leaves this nil,
			// so a nil here is the only sign we get that state we were meant to be able
			// to read did not read. Refuse the move rather than write a partial value.
			if req.SourceState == nil {
				resp.Diagnostics.AddError(
					"Unable to Move Resource State",
					fmt.Sprintf(
						"The state held by %s could not be read using the schema of the resource it is moving to, "+
							"which is the same schema it was written with. This is always a problem with the "+
							"provider: please report it to us, with the output of `terraform plan` run with "+
							"TF_LOG=debug.\n\nSource resource type: %s\nSource schema version: %d",
						sourceTypeName, req.SourceTypeName, req.SourceSchemaVersion,
					),
				)
				return
			}

			resp.TargetState.Raw = req.SourceState.Raw

			// Private state belongs to the resource rather than to the name it goes by,
			// and a rename does not change what any of it means.
			if req.SourcePrivate != nil {
				resp.TargetPrivate = req.SourcePrivate
			}
		},
	}
}

// declaredResourceSchema returns the schema a resource declares, for the places that
// need it outside of the framework's own call to Schema - a state mover, which
// has to describe the state it is moving from.
func declaredResourceSchema(ctx context.Context, r resource.Resource) schema.Schema {
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)

	return resp.Schema
}
