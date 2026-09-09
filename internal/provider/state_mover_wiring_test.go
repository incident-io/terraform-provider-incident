package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// registeredResources is every resource the provider offers, by the type name
// Terraform knows it as. Going through the provider rather than a list written
// out here is what makes the tests below notice a promotion that forgets its
// mover.
func registeredResources(t *testing.T) map[string]resource.Resource {
	t.Helper()

	ctx := context.Background()
	resources := map[string]resource.Resource{}

	for _, newResource := range (&IncidentProvider{}).Resources(ctx) {
		r := newResource()

		var metadata resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &metadata)
		resources[metadata.TypeName] = r
	}

	return resources
}

// TestPromotedResourcesAcceptTheirFormerName is the test the whole rename rests
// on. A `moved` block against a resource with no mover fails the plan outright,
// and the failure is only visible to someone who has already written the block -
// so it has to be caught here rather than in an upgrade.
func TestPromotedResourcesAcceptTheirFormerName(t *testing.T) {
	ctx := context.Background()
	resources := registeredResources(t)

	for former, promoted := range renamedInV7 {
		t.Run(former+" to "+promoted, func(t *testing.T) {
			r, ok := resources[promoted]
			if !ok {
				t.Fatalf("the provider registers no %s, so nothing can be moved to it", promoted)
			}

			mover, ok := r.(resource.ResourceWithMoveState)
			if !ok {
				t.Fatalf("%s does not implement MoveState, so a moved block from %s fails the plan", promoted, former)
			}

			// The state a resource of the former name held: its own schema, since a
			// rename does not change it. Null leaves are enough - the mover copies
			// the value rather than reading it.
			schema := declaredResourceSchema(ctx, r)
			state := tfsdk.State{
				Schema: schema,
				Raw:    nullFilledObject(t, schema.Type().TerraformType(ctx)),
			}

			resp := resource.MoveStateResponse{
				TargetState: tfsdk.State{
					Schema: schema,
					Raw:    tftypes.NewValue(schema.Type().TerraformType(ctx), nil),
				},
			}

			var moved bool
			for _, stateMover := range mover.MoveState(ctx) {
				stateMover.StateMover(ctx, resource.MoveStateRequest{
					SourceTypeName: former,
					SourceState:    &state,
				}, &resp)

				if resp.Diagnostics.HasError() {
					t.Fatalf("moving from %s: %+v", former, resp.Diagnostics)
				}
				if !resp.TargetState.Raw.IsNull() {
					moved = true
					break
				}
			}

			if !moved {
				t.Errorf("no mover on %s took a move from %s", promoted, former)
			}
			if !resp.TargetState.Raw.Equal(state.Raw) {
				t.Errorf("the state moved across changed\n got: %s\nwant: %s", resp.TargetState.Raw, state.Raw)
			}
		})
	}
}

// A mover that took a move from any resource type would quietly write state it
// has not read into the target. Terraform names the source type, so the guard is
// cheap; this is the test that it is there.
func TestPromotedResourcesRefuseAnUnrelatedSource(t *testing.T) {
	ctx := context.Background()
	resources := registeredResources(t)

	for _, promoted := range renamedInV7 {
		t.Run(promoted, func(t *testing.T) {
			mover, ok := resources[promoted].(resource.ResourceWithMoveState)
			if !ok {
				t.Skipf("%s has no movers", promoted)
			}

			schema := declaredResourceSchema(ctx, resources[promoted])
			state := tfsdk.State{
				Schema: schema,
				Raw:    nullFilledObject(t, schema.Type().TerraformType(ctx)),
			}

			resp := resource.MoveStateResponse{
				TargetState: tfsdk.State{
					Schema: schema,
					Raw:    tftypes.NewValue(schema.Type().TerraformType(ctx), nil),
				},
			}

			for _, stateMover := range mover.MoveState(ctx) {
				stateMover.StateMover(ctx, resource.MoveStateRequest{
					SourceTypeName: "incident_something_else",
					SourceState:    &state,
				}, &resp)
			}

			if len(resp.Diagnostics) > 0 {
				t.Errorf("expected the move to be skipped without diagnostics, got %+v", resp.Diagnostics)
			}
			if !resp.TargetState.Raw.IsNull() {
				t.Errorf("expected no state to be written, got %s", resp.TargetState.Raw)
			}
		})
	}
}

// Every name in renamedInV7 has to be one Terraform will actually see in a
// `from` argument, which means the resource that used to answer to it is gone
// and the one that took its place is registered. A name left in this table by
// mistake would have the tests above passing against nothing.
func TestRenamedInV7NamesAreGone(t *testing.T) {
	resources := registeredResources(t)

	for former, promoted := range renamedInV7 {
		if _, ok := resources[former]; ok {
			t.Errorf("the provider still registers %s, so it has not been renamed to %s", former, promoted)
		}
		if _, ok := resources[promoted]; !ok {
			t.Errorf("the provider registers no %s", promoted)
		}
	}
}

// And the reverse: a `_beta` resource or data source still registered is one
// this table has not accounted for, which would ship a beta name in a release
// that promised none.
func TestNoBetaResourcesRemain(t *testing.T) {
	ctx := context.Background()

	for name := range registeredResources(t) {
		if strings.HasSuffix(name, "_beta") {
			t.Errorf("%s is still registered as a resource", name)
		}
	}

	for _, newDataSource := range (&IncidentProvider{}).DataSources(ctx) {
		var metadata datasource.MetadataResponse
		newDataSource().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "incident"}, &metadata)

		if strings.HasSuffix(metadata.TypeName, "_beta") {
			t.Errorf("%s is still registered as a data source", metadata.TypeName)
		}
	}
}
