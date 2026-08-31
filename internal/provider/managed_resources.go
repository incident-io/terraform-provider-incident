package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
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
	if err == nil {
		return
	}

	// The managed-resources endpoint checks escalation_paths.update without
	// taking the escalation path's teams into account. A team-scoped key can
	// therefore create or update an escalation path successfully, then receive
	// this 403 while adding the secondary "managed by Terraform" metadata.
	//
	// Do not fail the Terraform operation after the escalation path itself has
	// already been created, updated, or imported. Other claim failures remain
	// errors so they are not hidden.
	var httpErr client.HTTPError
	if resourceType == client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeEscalationPath &&
		errors.As(err, &httpErr) &&
		httpErr.StatusCode == 403 &&
		bytes.Contains(httpErr.Body, []byte("missing_required_scope")) {
		diagnostics.AddWarning(
			"Unable to mark escalation path as managed by Terraform",
			fmt.Sprintf("The escalation path operation succeeded, but incident.io could not mark it as managed by Terraform because the API key's escalation_paths.update scope may be team-scoped. The resource will remain editable in the dashboard. Got error: %s", err),
		)
		return
	}

	diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create managed resource, got error: %s", err))
}
