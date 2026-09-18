package provider

import (
	"context"
	"fmt"

	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// claimResourceOnImport claims a resource as it enters Terraform state via an
// import, unless the provider is configured not to.
//
// Terraform asks a provider to import during plan, and never during apply: the
// apply walk replays the state the plan already imported. That makes this claim
// a write during an operation people reasonably expect to be read-only, which is
// what mark_imported_resources_as_managed = false opts out of.
//
// Nothing else is needed to claim those resources eventually. A Create or Update
// claims too unless the resource sets unlock_in_dashboard, either by calling
// claimResource or by carrying the incident.io/terraform/version annotation in
// its payload, so an import with this off is claimed by the first apply that
// changes the resource. A configuration that already matches the account
// produces no such apply, and stays unclaimed until one does.
func claimResourceOnImport(
	ctx context.Context,
	apiClient *client.ClientWithResponses,
	resourceID string,
	diagnostics *diag.Diagnostics,
	resourceType client.ManagedResourcesCreateManagedResourcePayloadV2ResourceType,
	terraformVersion string,
	markImportedAsManaged bool,
) {
	if !markImportedAsManaged {
		return
	}

	claimResource(ctx, apiClient, resourceID, diagnostics, resourceType, terraformVersion)
}

func claimResource(
	ctx context.Context,
	apiClient *client.ClientWithResponses,
	resourceID string,
	diagnostics *diag.Diagnostics,
	resourceType client.ManagedResourcesCreateManagedResourcePayloadV2ResourceType,
	terraformVersion string,
) {
	payload := client.ManagedResourcesV2CreateManagedResourceJSONRequestBody{
		Annotations: map[string]string{
			"incident.io/terraform/version": terraformVersion,
		},
		ResourceType: resourceType,
		ResourceId:   resourceID,
	}

	_, err := apiClient.ManagedResourcesV2CreateManagedResourceWithResponse(ctx, payload)
	if err != nil {
		diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create managed resource, got error: %s", err))
		return
	}
}

// shouldClaim reports whether to claim a resource, which is everything but one asking to
// stay unlocked in the dashboard. Unset means claim, which is how the provider behaved
// before this was configurable.
func shouldClaim(unlockInDashboard types.Bool) bool {
	if unlockInDashboard.IsNull() || unlockInDashboard.IsUnknown() {
		return true
	}

	return !unlockInDashboard.ValueBool()
}

// unclaimResource hands a resource back to the dashboard, for a configuration that sets
// unlock_in_dashboard on a resource Terraform had already claimed.
//
// There is no unclaim endpoint: the claim endpoint infers who manages a resource from the
// annotations it is sent, and an empty set means the dashboard.
func unclaimResource(
	ctx context.Context,
	apiClient *client.ClientWithResponses,
	resourceID string,
	diagnostics *diag.Diagnostics,
	resourceType client.ManagedResourcesCreateManagedResourcePayloadV2ResourceType,
) {
	payload := client.ManagedResourcesV2CreateManagedResourceJSONRequestBody{
		Annotations:  map[string]string{},
		ResourceType: resourceType,
		ResourceId:   resourceID,
	}

	_, err := apiClient.ManagedResourcesV2CreateManagedResourceWithResponse(ctx, payload)
	if err != nil {
		diagnostics.AddError("Client Error", fmt.Sprintf("Unable to unclaim managed resource, got error: %s", err))
		return
	}
}

// unlockInDashboardAttribute is the opt-out every claimable resource carries. Optional and
// not Computed: a default would show up as a change to every resource already in state.
func unlockInDashboardAttribute() schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: "Whether to leave this resource unlocked in the incident.io dashboard, so " +
			"people can edit it there. Defaults to `false`: Terraform claims what it manages, and a " +
			"claimed resource cannot be edited in the dashboard. Set it to `true` to leave the resource " +
			"unclaimed — pair that with `lifecycle { ignore_changes = [...] }` naming the attributes " +
			"people edit, or the next apply reverts them. Setting it on a resource Terraform already " +
			"claimed hands that resource back, and someone disconnecting one in the dashboard shows as " +
			"no change.",
		Optional: true,
	}
}

// syncAnnotations is what claims a schedule sync rule or target: the API records one as a
// managed resource only when the write carries annotations, so sending none records
// nothing.
func syncAnnotations(unlockInDashboard types.Bool, terraformVersion string) *map[string]string {
	if !shouldClaim(unlockInDashboard) {
		return nil
	}

	return &map[string]string{
		"incident.io/terraform/version": terraformVersion,
	}
}

// unlockInDashboardDataSourceAttribute exists so a data source sharing a resource's
// model still fits its schema. The claim lives on a managed-resource record no read
// endpoint returns, so this is always null.
func unlockInDashboardDataSourceAttribute() dsschema.BoolAttribute {
	return dsschema.BoolAttribute{
		MarkdownDescription: "Not populated: whether Terraform claims a resource is configuration, and no read endpoint reports it.",
		Computed:            true,
	}
}
