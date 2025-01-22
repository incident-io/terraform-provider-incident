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
	resourceType client.ManagedResourceV2ResourceType,
	terraformVersion string,
) {
	payload := client.CreateManagedResourceRequestBody{
		Annotations: map[string]string{
			"incident.io/terraform/version": terraformVersion,
		},
		ResourceType: client.CreateManagedResourceRequestBodyResourceType(resourceType),
		ResourceId:   resourceID,
	}

	result, err := apiClient.ManagedResourcesV2CreateManagedResourceWithResponse(ctx, payload)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create managed resource, got error: %s", err))
		return
	}
}
