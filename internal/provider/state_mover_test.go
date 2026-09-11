package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// betaRename is a rename somebody can write as a `moved` block: the `_beta` name a
// resource answered to in v6, and the name it answers to now. Both are the same resource:
// see aliases.go.
type betaRename struct {
	source resource.Resource
	target resource.Resource
}

// betaRenames is every rename the provider supports. The tests below walk this rather
// than each resource on its own, so a resource that grows a mover the others don't have,
// or loses one, is a failure here rather than something to find out about from somebody's
// upgrade.
func betaRenames() []betaRename {
	return []betaRename{
		{source: NewIncidentScheduleBetaResource(), target: NewIncidentScheduleResource()},
		{source: NewIncidentScheduleRotationBetaResource(), target: NewIncidentScheduleRotationResource()},
		{source: NewIncidentEscalationPathBetaResource(), target: NewIncidentEscalationPathResource()},
		{source: NewIncidentAlertSourceBetaResource(), target: NewIncidentAlertSourceResource()},
		{source: NewIncidentAlertSourceAttributeBetaResource(), target: NewIncidentAlertSourceAttributeResource()},
	}
}

func (m betaRename) name(ctx context.Context) string {
	return fmt.Sprintf("%s to %s", resourceTypeName(ctx, m.source), resourceTypeName(ctx, m.target))
}

// mover returns the one mover the target resource offers, failing if it offers none:
// a `moved` block against a resource with no mover fails the plan outright, and that
// failure is only visible to somebody who has already written the block.
func (m betaRename) mover(ctx context.Context, t *testing.T) resource.StateMover {
	t.Helper()

	withMove, ok := m.target.(resource.ResourceWithMoveState)
	require.True(t, ok,
		"%s does not implement MoveState, so a `moved` block onto it fails the plan",
		resourceTypeName(ctx, m.target))

	movers := withMove.MoveState(ctx)
	require.Len(t, movers, 1, "expected one mover on %s", resourceTypeName(ctx, m.target))

	return movers[0]
}

// stateObjectWith builds a state value for the schema with every attribute null except
// the ones named, which is how these tests write a source state without filling in a
// resource's whole schema by hand.
func stateObjectWith(ctx context.Context, t *testing.T, s schema.Schema, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	objectType, ok := s.Type().TerraformType(ctx).(tftypes.Object)
	require.True(t, ok, "schema is not an object")

	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		if override, given := overrides[name]; given {
			values[name] = override
			continue
		}
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	for name := range overrides {
		require.Contains(t, objectType.AttributeTypes, name, "override names an attribute the object doesn't have")
	}

	return tftypes.NewValue(objectType, values)
}

// stringOverrides gives every string attribute in the schema a value of its own, so a
// test can tell an attribute that was carried across from one that happens to match.
func stringOverrides(ctx context.Context, t *testing.T, s schema.Schema) map[string]tftypes.Value {
	t.Helper()

	objectType, ok := s.Type().TerraformType(ctx).(tftypes.Object)
	require.True(t, ok, "schema is not an object")

	overrides := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		if attributeType.Is(tftypes.String) {
			overrides[name] = tftypes.NewValue(tftypes.String, "value of "+name)
		}
	}

	return overrides
}

// moveStateResponse is the response the framework hands a mover: the target resource's
// schema, holding a null value until a mover sets one. The framework reads a response
// still holding that null as the mover having skipped the move, so these tests have to
// start from it to tell skipping apart from moving.
func moveStateResponse(ctx context.Context, t *testing.T, targetSchema schema.Schema) resource.MoveStateResponse {
	t.Helper()

	return resource.MoveStateResponse{
		TargetState: tfsdk.State{
			Schema: targetSchema,
			Raw:    tftypes.NewValue(targetSchema.Type().TerraformType(ctx), nil),
		},
	}
}

// attributesOf reads a state value back as its attributes, for asserting on what a
// mover wrote.
func attributesOf(t *testing.T, value tftypes.Value) map[string]tftypes.Value {
	t.Helper()

	var attributes map[string]tftypes.Value
	require.NoError(t, value.As(&attributes))

	return attributes
}

// TestBetaRenamesCarryTheObject is the test the whole rename rests on: state written
// under the old name comes out the other side naming the same object, so the resource
// carries on managing what it was managing.
func TestBetaRenamesCarryTheObject(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaRenames() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			// Every string gets a value of its own, which covers whichever of them names
			// the object: `id` for most of these, and the pair of IDs an alert source
			// attribute binding is keyed by.
			overrides := stringOverrides(ctx, t, sourceSchema)

			resp := moveStateResponse(ctx, t, targetSchema)
			move.mover(ctx, t).StateMover(ctx, resource.MoveStateRequest{
				SourceTypeName:      resourceTypeName(ctx, move.source),
				SourceSchemaVersion: sourceSchema.Version,
				SourceState: &tfsdk.State{
					Schema: sourceSchema,
					Raw:    stateObjectWith(ctx, t, sourceSchema, overrides),
				},
			}, &resp)

			require.Empty(t, resp.Diagnostics)
			require.False(t, resp.TargetState.Raw.IsNull(), "the mover skipped a source it should have taken")

			// Private state belongs to the resource that wrote it rather than to the
			// object, and the resource being moved to has never read the old one's.
			assert.Nil(t, resp.TargetPrivate)

			moved := attributesOf(t, resp.TargetState.Raw)
			for _, name := range identityAttributes(targetSchema) {
				assert.Equal(t, overrides[name], moved[name],
					"%s names the object, so it has to move or the resource stops managing it", name)
			}

			for name, override := range overrides {
				assert.Equal(t, override, moved[name], "%s did not move", name)
			}

			// Nothing may be unknown: an unknown in state is a value Terraform will ask
			// the provider to fill in on apply rather than on refresh.
			for name, value := range moved {
				assert.True(t, value.IsKnown(), "%s is unknown, which state cannot hold", name)
			}
		})
	}
}

// A rename is only empty-plan if the move is lossless, and it is lossless because both
// names are one resource with one schema. This is what says so: every attribute the
// target holds is one the mover carries, with nothing left for a refresh to fill in.
//
// It is also the test that catches an alias drifting from the resource it aliases. An
// alias that declared its attributes differently would show up here as an attribute the
// move can't carry, rather than as somebody's plan asking to rebuild their escalation
// path.
func TestEveryAttributeMovesAcross(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaRenames() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			targetType, ok := targetSchema.Type().TerraformType(ctx).(tftypes.Object)
			require.True(t, ok, "schema is not an object")

			shared := sharedAttributes(ctx, sourceSchema, targetSchema)
			for name := range targetType.AttributeTypes {
				assert.Contains(t, shared, name,
					"%s is not carried by the rename, so the plan after it would not be empty", name)
			}
			assert.Len(t, shared, len(targetType.AttributeTypes),
				"the rename carries an attribute the resource no longer has")
		})
	}
}

// Movers are consulted in turn until one takes the move, so a mover that doesn't
// recognise the source has to leave the response exactly as it found it. Returning an
// error here would fail a move another mover was going to take.
func TestBetaRenamesSkipSourcesTheyDoNotKnow(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaRenames() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			sourceState := &tfsdk.State{
				Schema: sourceSchema,
				Raw:    stateObjectWith(ctx, t, sourceSchema, stringOverrides(ctx, t, sourceSchema)),
			}

			t.Run("a resource type it has never read", func(t *testing.T) {
				resp := moveStateResponse(ctx, t, targetSchema)
				move.mover(ctx, t).StateMover(ctx, resource.MoveStateRequest{
					SourceTypeName:      "incident_catalog_type",
					SourceSchemaVersion: sourceSchema.Version,
					SourceState:         sourceState,
				}, &resp)

				assert.Empty(t, resp.Diagnostics, "a skipped move must not report anything")
				assert.True(t, resp.TargetState.Raw.IsNull(), "a skipped move must not write state")
			})

			// Schemas are versioned separately from the provider, so a source at a version
			// we have never written is state we cannot vouch for.
			t.Run("a schema version it has never written", func(t *testing.T) {
				resp := moveStateResponse(ctx, t, targetSchema)
				move.mover(ctx, t).StateMover(ctx, resource.MoveStateRequest{
					SourceTypeName:      resourceTypeName(ctx, move.source),
					SourceSchemaVersion: sourceSchema.Version + 1,
					SourceState:         sourceState,
				}, &resp)

				assert.Empty(t, resp.Diagnostics)
				assert.True(t, resp.TargetState.Raw.IsNull())
			})
		})
	}
}

// State with no ID names no object, so the refresh that would fill the rest in has
// nothing to ask about. Refusing is better than writing state that reads as a resource
// to create, which is what a plan would make of it.
func TestBetaRenamesRefuseStateThatNamesNoObject(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaRenames() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			resp := moveStateResponse(ctx, t, targetSchema)
			move.mover(ctx, t).StateMover(ctx, resource.MoveStateRequest{
				SourceTypeName:      resourceTypeName(ctx, move.source),
				SourceSchemaVersion: sourceSchema.Version,
				SourceState: &tfsdk.State{
					Schema: sourceSchema,
					Raw:    stateObjectWith(ctx, t, sourceSchema, nil),
				},
			}, &resp)

			assert.True(t, resp.Diagnostics.HasError(), "expected the move to be refused")
			assert.True(t, resp.TargetState.Raw.IsNull(), "a refused move must not write state")
		})
	}
}

// The framework leaves SourceState nil when the state didn't decode against the schema
// we gave it, which for a resource's own schema should never happen: this is the case
// where it has, and the move must fail rather than half-happen.
func TestBetaRenamesRefuseStateThatDidNotDecode(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaRenames() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			resp := moveStateResponse(ctx, t, targetSchema)
			move.mover(ctx, t).StateMover(ctx, resource.MoveStateRequest{
				SourceTypeName:      resourceTypeName(ctx, move.source),
				SourceSchemaVersion: sourceSchema.Version,
				SourceState:         nil,
			}, &resp)

			assert.True(t, resp.Diagnostics.HasError())
			assert.True(t, resp.TargetState.Raw.IsNull())
		})
	}
}

// The mover describes the state it moves from with the source resource's own schema, so
// that the framework decodes the source for it rather than handing it raw JSON.
func TestBetaRenameMoversDescribeTheirSource(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaRenames() {
		t.Run(move.name(ctx), func(t *testing.T) {
			mover := move.mover(ctx, t)
			require.NotNil(t, mover.SourceSchema, "without a source schema the framework hands the mover nothing to read")

			// Compared by type rather than by schema: a schema holds validators and plan
			// modifiers, which are functions and so never compare equal. The type is what
			// the framework decodes the source state with, which is what matters here.
			assert.Equal(t,
				declaredResourceSchema(ctx, move.source).Type().TerraformType(ctx),
				mover.SourceSchema.Type().TerraformType(ctx))
		})
	}
}
