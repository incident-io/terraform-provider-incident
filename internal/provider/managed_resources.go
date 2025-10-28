package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func claimResource(
	ctx context.Context,
	apiClient *client.ClientWithResponses,
	resourceID string,
	diagnostics diag.Diagnostics,
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
