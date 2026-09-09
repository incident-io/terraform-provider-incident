package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The schema either side of a rename is the same one, so these tests only need
// a schema, not the resource it belongs to.
func testRenameSchema() schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true},
			"name": schema.StringAttribute{Required: true},
		},
	}
}

// testRenameState is the state a resource holds before the move.
func testRenameState(ctx context.Context, t *testing.T, sourceSchema schema.Schema) tfsdk.State {
	t.Helper()

	return tfsdk.State{
		Schema: sourceSchema,
		Raw: tftypes.NewValue(sourceSchema.Type().TerraformType(ctx), map[string]tftypes.Value{
			"id":   tftypes.NewValue(tftypes.String, "01SCHED"),
			"name": tftypes.NewValue(tftypes.String, "Platform on-call"),
		}),
	}
}

// testMoveStateResponse is the response the framework hands a mover: the target
// resource's schema, holding a null value until a mover sets one. The framework
// reads a response still holding that null as the mover having skipped the
// move, so these tests have to start from it to tell skipping apart from
// moving.
func testMoveStateResponse(ctx context.Context, targetSchema schema.Schema) resource.MoveStateResponse {
	return resource.MoveStateResponse{
		TargetState: tfsdk.State{
			Schema: targetSchema,
			Raw:    tftypes.NewValue(targetSchema.Type().TerraformType(ctx), nil),
		},
	}
}

// renamedInV7 is every rename v7 makes, which is what this mover exists for.
// Each promotion wires up its own, so listing them here checks the mover
// accepts each name before the resource that will claim it does.
var renamedInV7 = map[string]string{
	"incident_schedule_beta":               "incident_schedule",
	"incident_schedule_rotation_beta":      "incident_schedule_rotation",
	"incident_escalation_path_beta":        "incident_escalation_path",
	"incident_alert_source_beta":           "incident_alert_source",
	"incident_alert_source_attribute_beta": "incident_alert_source_attribute",
}

func TestRenameStateMover(t *testing.T) {
	ctx := context.Background()
	sourceSchema := testRenameSchema()
	state := testRenameState(ctx, t, sourceSchema)

	t.Run("moves the state of the name the resource used to go by", func(t *testing.T) {
		for former, promoted := range renamedInV7 {
			t.Run(former+" to "+promoted, func(t *testing.T) {
				mover := renameStateMover(former, sourceSchema)
				resp := testMoveStateResponse(ctx, sourceSchema)

				mover.StateMover(ctx, resource.MoveStateRequest{
					SourceTypeName: former,
					SourceState:    &state,
				}, &resp)

				if resp.Diagnostics.HasError() {
					t.Fatalf("expected the move to be accepted, got %+v", resp.Diagnostics)
				}
				if !resp.TargetState.Raw.Equal(state.Raw) {
					t.Errorf("expected the target to hold the source state\n got: %s\nwant: %s", resp.TargetState.Raw, state.Raw)
				}
			})
		}
	})

	// A resource can offer several movers and the framework tries them in turn,
	// so a mover that doesn't recognise the source has to leave the response
	// exactly as it found it. Returning an error here would fail a move another
	// mover was going to take.
	t.Run("skips a source resource type it does not know", func(t *testing.T) {
		mover := renameStateMover("incident_schedule_beta", sourceSchema)
		resp := testMoveStateResponse(ctx, sourceSchema)

		mover.StateMover(ctx, resource.MoveStateRequest{
			SourceTypeName: "incident_escalation_path_beta",
			SourceState:    &state,
		}, &resp)

		if len(resp.Diagnostics) > 0 {
			t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
		}
		if !resp.TargetState.Raw.IsNull() {
			t.Errorf("expected the target state to be left null, got %s", resp.TargetState.Raw)
		}
	})

	t.Run("skips a source at a schema version it has never written", func(t *testing.T) {
		mover := renameStateMover("incident_schedule_beta", sourceSchema)
		resp := testMoveStateResponse(ctx, sourceSchema)

		mover.StateMover(ctx, resource.MoveStateRequest{
			SourceTypeName:      "incident_schedule_beta",
			SourceSchemaVersion: 1, // the schema under test is version 0
			SourceState:         &state,
		}, &resp)

		if len(resp.Diagnostics) > 0 {
			t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
		}
		if !resp.TargetState.Raw.IsNull() {
			t.Errorf("expected the target state to be left null, got %s", resp.TargetState.Raw)
		}
	})

	// The framework leaves SourceState nil when the state didn't decode against
	// the schema we gave it, which for a rename should never happen: this is the
	// case where it has, and the move must fail rather than half-happen.
	t.Run("refuses a source whose state did not decode", func(t *testing.T) {
		mover := renameStateMover("incident_schedule_beta", sourceSchema)
		resp := testMoveStateResponse(ctx, sourceSchema)

		mover.StateMover(ctx, resource.MoveStateRequest{
			SourceTypeName: "incident_schedule_beta",
			SourceState:    nil,
		}, &resp)

		if !resp.Diagnostics.HasError() {
			t.Fatal("expected the move to be refused")
		}
		if !resp.TargetState.Raw.IsNull() {
			t.Errorf("expected the target state to be left null, got %s", resp.TargetState.Raw)
		}
	})
}

// The mover describes the state it moves from with the schema it was given, so
// that the framework decodes the source for it.
func TestRenameStateMoverSourceSchema(t *testing.T) {
	sourceSchema := testRenameSchema()
	mover := renameStateMover("incident_schedule_beta", sourceSchema)

	if mover.SourceSchema == nil {
		t.Fatal("expected a source schema, so the framework populates SourceState")
	}
	if len(mover.SourceSchema.Attributes) != len(sourceSchema.Attributes) {
		t.Errorf("expected the schema it was given, got %+v", mover.SourceSchema.Attributes)
	}
}

// declaredResourceSchema is how a mover gets hold of the schema to describe itself
// with, so it has to return the schema the resource actually declares.
func TestDeclaredResourceSchema(t *testing.T) {
	ctx := context.Background()

	declared := declaredResourceSchema(ctx, NewIncidentScheduleBetaResource())

	var direct resource.SchemaResponse
	NewIncidentScheduleBetaResource().Schema(ctx, resource.SchemaRequest{}, &direct)

	if len(declared.Attributes) == 0 {
		t.Fatal("expected the resource's attributes")
	}
	if len(declared.Attributes) != len(direct.Schema.Attributes) {
		t.Errorf("expected %d attributes, got %d", len(direct.Schema.Attributes), len(declared.Attributes))
	}
	if declared.Version != direct.Schema.Version {
		t.Errorf("expected schema version %d, got %d", direct.Schema.Version, declared.Version)
	}
}
