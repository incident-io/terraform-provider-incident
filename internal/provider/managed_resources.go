package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// claimResourceOnImport claims a resource as it enters Terraform state via an
// import, unless the provider is configured not to.
//
// Terraform asks a provider to import during plan, and never during apply: the
// apply walk replays the state the plan already imported. That makes this claim
// a write during an operation people reasonably expect to be read-only, which is
// what mark_imported_resources_as_managed = false opts out of.
//
// Nothing else is needed to claim those resources eventually. Every Create and
// Update claims too, either by calling claimResource or by carrying the
// incident.io/terraform/version annotation in its payload, so an import with
// this off is claimed by the first apply that changes the resource. A
// configuration that already matches the account produces no such apply, and
// stays unclaimed until one does.
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
