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

// betaResourceMove is a migration somebody can write as a `moved` block: the v6
// resource they are on, and the beta resource that replaces it.
type betaResourceMove struct {
	source resource.Resource
	target resource.Resource
}

// betaResourceMoves is every move the provider supports. The tests below walk this
// rather than each resource on its own, so a resource that grows a mover the others
// don't have, or loses one, is a failure here rather than something to find out about
// from somebody's upgrade.
func betaResourceMoves() []betaResourceMove {
	return []betaResourceMove{
		{source: NewIncidentScheduleResource(), target: NewIncidentScheduleBetaResource()},
		{source: NewIncidentEscalationPathResource(), target: NewEscalationPathBetaResource()},
		{source: NewIncidentAlertSourceResource(), target: NewAlertSourceBetaResource()},
	}
}

func (m betaResourceMove) name(ctx context.Context) string {
	return fmt.Sprintf("%s to %s", resourceTypeName(ctx, m.source), resourceTypeName(ctx, m.target))
}

// mover returns the one mover the target resource offers, failing if it offers none:
// a `moved` block against a resource with no mover fails the plan outright, and that
// failure is only visible to somebody who has already written the block.
func (m betaResourceMove) mover(ctx context.Context, t *testing.T) resource.StateMover {
	t.Helper()

	withMove, ok := m.target.(resource.ResourceWithMoveState)
	require.True(t, ok,
		"%s does not implement MoveState, so a `moved` block onto it fails the plan",
		resourceTypeName(ctx, m.target))

	movers := withMove.MoveState(ctx)
	require.Len(t, movers, 1, "expected one mover on %s", resourceTypeName(ctx, m.target))

	return movers[0]
}

// objectWith builds a state value for the schema with every attribute null except the
// ones named, which is how these tests write a source state without filling in a
// resource's whole schema by hand.
func stateObjectWith(ctx context.Context, t *testing.T, s schema.Schema, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	objectType, ok := s.Type().TerraformType(ctx).(tftypes.Object)
	require.True(t, ok, "schema is not an object")

	return objectValueWith(t, objectType, overrides)
}

// objectValueWith is the same thing for an object type rather than a whole schema, which
// is what a test needs to write a value nested inside one - a v6 alert source's
// template, say.
func objectValueWith(t *testing.T, objectType tftypes.Object, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()

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

// nestedObjectType reads the type of an object attribute nested inside another, failing
// rather than panicking when the schema no longer holds one there.
func nestedObjectType(t *testing.T, objectType tftypes.Object, name string) tftypes.Object {
	t.Helper()

	attributeType, held := objectType.AttributeTypes[name]
	require.True(t, held, "the schema has no %s attribute", name)

	nested, isObject := attributeType.(tftypes.Object)
	require.True(t, isObject, "%s is not an object", name)

	return nested
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

// TestBetaResourceMovesCarryTheObject is the test the whole migration rests on: state
// written by the v6 resource comes out the other side naming the same object, so the
// refresh that follows the move has something to ask the API about.
func TestBetaResourceMovesCarryTheObject(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaResourceMoves() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			overrides := stringOverrides(ctx, t, sourceSchema)
			overrides["id"] = tftypes.NewValue(tftypes.String, "01ABC123DEF456GHI789JKL")

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
			assert.Equal(t,
				tftypes.NewValue(tftypes.String, "01ABC123DEF456GHI789JKL"), moved["id"],
				"the object's ID has to move, or the refresh has nothing to read")

			// Every string both schemas hold the same way moves with it. The rest is left
			// null for the refresh, which is asserted below.
			for _, name := range sharedAttributes(ctx, sourceSchema, targetSchema) {
				if override, isString := overrides[name]; isString {
					assert.Equal(t, override, moved[name], "%s did not move", name)
				}
			}
		})
	}
}

// An attribute the new schema says differently is the case the mover must not guess at:
// leaving it null is what lets the new resource's own Read fill it in from the API.
func TestBetaResourceMovesLeaveTheRestToTheRefresh(t *testing.T) {
	ctx := context.Background()

	// The attributes each migration cannot carry, and so must leave null: either the new
	// schema says them differently, or the v6 resource never held them. They are worth
	// naming rather than deriving, because a mover that started copying one of them would
	// be writing state the new resource has never read.
	leftToTheRefresh := map[string][]string{
		"incident_schedule_beta":        {},
		"incident_escalation_path_beta": {"start", "sequences"},
		"incident_alert_source_beta": {
			"priority", "visible_to_teams", "named_expression", "is_private", "version",
		},
	}

	for _, move := range betaResourceMoves() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			resp := moveStateResponse(ctx, t, targetSchema)
			move.mover(ctx, t).StateMover(ctx, resource.MoveStateRequest{
				SourceTypeName:      resourceTypeName(ctx, move.source),
				SourceSchemaVersion: sourceSchema.Version,
				SourceState: &tfsdk.State{
					Schema: sourceSchema,
					Raw: stateObjectWith(ctx, t, sourceSchema, map[string]tftypes.Value{
						"id": tftypes.NewValue(tftypes.String, "01ABC123DEF456GHI789JKL"),
					}),
				},
			}, &resp)

			require.Empty(t, resp.Diagnostics)

			moved := attributesOf(t, resp.TargetState.Raw)
			for _, name := range leftToTheRefresh[resourceTypeName(ctx, move.target)] {
				require.Contains(t, moved, name)
				assert.True(t, moved[name].IsNull(), "%s should be left for the refresh to read", name)
			}

			// Nothing may be unknown either: an unknown in state is a value Terraform will
			// ask the provider to fill in on apply rather than on refresh.
			for name, value := range moved {
				assert.True(t, value.IsKnown(), "%s is unknown, which state cannot hold", name)
			}
		})
	}
}

// TestBetaResourceMovesCarryWhatAReadWouldNot pins the attributes that have to move
// because the new resource keeps them from prior state rather than reading them back.
// A schema change that stopped one of these moving would leave the migration planning
// a change nobody asked for, which is exactly the thing these movers exist to avoid.
func TestBetaResourceMovesCarryWhatAReadWouldNot(t *testing.T) {
	ctx := context.Background()

	mustMove := map[string][]string{
		// incidentScheduleBetaFromAPI takes team_ids from prior state, the API not saying
		// which of a schedule's teams the author wrote.
		"incident_schedule_beta": {"team_ids"},
		// buildModel reads everything but the sequence names from the API.
		"incident_escalation_path_beta": {"team_ids", "working_hours", "repeat_config"},
		// alertSourceBetaFromAPI reconciles these against prior state to keep the
		// author's spelling of a team list or a condition.
		"incident_alert_source_beta": {"owning_team_ids", "filter_condition_groups"},
	}

	for _, move := range betaResourceMoves() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			shared := sharedAttributes(ctx, sourceSchema, targetSchema)
			for _, name := range mustMove[resourceTypeName(ctx, move.target)] {
				assert.Contains(t, shared, name,
					"%s has to move: the resource keeps it from prior state rather than reading it back", name)
			}
		})
	}
}

// TestAlertSourceMoveCarriesTheTemplatedText covers the one rewrite the provider has:
// an alert source's title and description sit under `template` on the v6 resource and at
// the top level on the beta one, in the same shape, so the move carries them out rather
// than leaving them to the refresh.
//
// Reading them back instead would look like it worked and then plan a change. The
// framework skips a type's semantic equality when the prior value is null, which is what
// a move leaves behind, so the refresh would store the API's JSON where the
// configuration has the author's - the same document, different bytes - and the plan
// straight after the move would ask to rewrite both.
func TestAlertSourceMoveCarriesTheTemplatedText(t *testing.T) {
	ctx := context.Background()

	move := betaResourceMove{
		source: NewIncidentAlertSourceResource(),
		target: NewAlertSourceBetaResource(),
	}
	sourceSchema := declaredResourceSchema(ctx, move.source)
	targetSchema := declaredResourceSchema(ctx, move.target)

	sourceType, isObject := sourceSchema.Type().TerraformType(ctx).(tftypes.Object)
	require.True(t, isObject)
	templateType := nestedObjectType(t, sourceType, "template")

	// A literal and a reference, one each, because they are the two spellings a rich
	// text value has and the mover carries the value whichever it holds.
	title := objectValueWith(t, nestedObjectType(t, templateType, "title"), map[string]tftypes.Value{
		"literal": tftypes.NewValue(tftypes.String,
			`{"content":[{"content":[{"text":"An alert","type":"text"}],"type":"paragraph"}],"type":"doc"}`),
	})
	description := objectValueWith(t, nestedObjectType(t, templateType, "description"), map[string]tftypes.Value{
		"reference": tftypes.NewValue(tftypes.String, "payload.summary"),
	})

	moveAlertSource := func(t *testing.T, template tftypes.Value) map[string]tftypes.Value {
		t.Helper()

		resp := moveStateResponse(ctx, t, targetSchema)
		move.mover(ctx, t).StateMover(ctx, resource.MoveStateRequest{
			SourceTypeName:      resourceTypeName(ctx, move.source),
			SourceSchemaVersion: sourceSchema.Version,
			SourceState: &tfsdk.State{
				Schema: sourceSchema,
				Raw: stateObjectWith(ctx, t, sourceSchema, map[string]tftypes.Value{
					"id":       tftypes.NewValue(tftypes.String, "01ABC123DEF456GHI789JKL"),
					"template": template,
				}),
			},
		}, &resp)

		require.Empty(t, resp.Diagnostics)
		require.False(t, resp.TargetState.Raw.IsNull(), "the mover skipped a source it should have taken")

		return attributesOf(t, resp.TargetState.Raw)
	}

	t.Run("a template holding both", func(t *testing.T) {
		moved := moveAlertSource(t, objectValueWith(t, templateType, map[string]tftypes.Value{
			"title":       title,
			"description": description,
		}))

		assert.Equal(t, title, moved["title"], "the title has to move, or the plan after the move rewrites it")
		assert.Equal(t, description, moved["description"])
	})

	// A heartbeat source has no template to carry anything out of, and neither has state
	// written before the attribute existed. Both leave the attributes null, which is what
	// the refresh reads onto.
	t.Run("a source with no template", func(t *testing.T) {
		moved := moveAlertSource(t, tftypes.NewValue(templateType, nil))

		assert.True(t, moved["title"].IsNull())
		assert.True(t, moved["description"].IsNull())
	})
}

// A resource can offer several movers and the framework tries them in turn, so a mover
// that doesn't recognise the source has to leave the response exactly as it found it.
// Returning an error here would fail a move another mover was going to take.
func TestBetaResourceMovesSkipSourcesTheyDoNotKnow(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaResourceMoves() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			sourceState := &tfsdk.State{
				Schema: sourceSchema,
				Raw: stateObjectWith(ctx, t, sourceSchema, map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "01ABC123DEF456GHI789JKL"),
				}),
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
func TestBetaResourceMovesRefuseStateWithoutAnID(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaResourceMoves() {
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
func TestBetaResourceMovesRefuseStateThatDidNotDecode(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaResourceMoves() {
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
func TestBetaResourceMoversDescribeTheirSource(t *testing.T) {
	ctx := context.Background()

	for _, move := range betaResourceMoves() {
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

// sharedAttributes is what decides the whole shape of a move, so the mapping is asserted
// rather than left to whatever the two schemas happen to have in common. A schema change
// that adds an attribute to both sides fails here, which is the review this deserves.
func TestSharedAttributesOfEachMove(t *testing.T) {
	ctx := context.Background()

	expected := map[string][]string{
		"incident_schedule_beta": {
			"holidays_public_config", "id", "name", "team_ids", "timezone",
		},
		"incident_escalation_path_beta": {
			"id", "name", "repeat_config", "team_ids", "working_hours",
		},
		"incident_alert_source_beta": {
			"alert_events_url", "auto_resolve_incident_alerts", "auto_resolve_timeout_minutes",
			"disabled", "email_address", "email_options", "filter_condition_groups",
			"fixed_team_id", "heartbeat_options", "http_custom_options", "id", "jira_options",
			"name", "owning_team_ids", "rate_limit_sharding", "secret_token", "source_type",
		},
	}

	for _, move := range betaResourceMoves() {
		t.Run(move.name(ctx), func(t *testing.T) {
			target := resourceTypeName(ctx, move.target)
			require.Contains(t, expected, target, "no expected attributes written down for this move")

			assert.Equal(t, expected[target], sharedAttributes(ctx,
				declaredResourceSchema(ctx, move.source),
				declaredResourceSchema(ctx, move.target),
			))
		})
	}
}

// The attributes a rewrite drops have no equivalent to move to, so they must not appear
// on the target at all: an attribute both schemas hold would be copied, and copying one
// of these would write the old shape into the new resource.
func TestRewrittenAttributesAreNotSharedByAccident(t *testing.T) {
	ctx := context.Background()

	dropped := map[string]string{
		"incident_schedule_beta":        "rotations",
		"incident_escalation_path_beta": "path",
		"incident_alert_source_beta":    "template",
	}

	for _, move := range betaResourceMoves() {
		t.Run(move.name(ctx), func(t *testing.T) {
			sourceSchema := declaredResourceSchema(ctx, move.source)
			targetSchema := declaredResourceSchema(ctx, move.target)

			name := dropped[resourceTypeName(ctx, move.target)]
			require.Contains(t, sourceSchema.Attributes, name, "the v6 resource should still hold %s", name)
			assert.NotContains(t, sharedAttributes(ctx, sourceSchema, targetSchema), name)
		})
	}
}
