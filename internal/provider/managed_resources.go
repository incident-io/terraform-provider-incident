package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

func claimResource(ctx context.Context, apiClient *client.ClientWithResponses, req resource.ImportStateRequest, resp *resource.ImportStateResponse, resourceType client.ManagedResourceV2ResourceType, terraformVersion string) {
	payload := client.CreateManagedResourceRequestBody{
		Annotations: map[string]string{
			"incident.io/terraform/version": terraformVersion,
		},
		ResourceType: client.CreateManagedResourceRequestBodyResourceType(resourceType),
		ResourceId:   req.ID,
	}

	result, err := apiClient.ManagedResourcesV2CreateManagedResourceWithResponse(ctx, payload)
	if err == nil && result.StatusCode() >= 400 {
		err = fmt.Errorf(string(result.Body))
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create managed resource, got error: %s", err))
		return
	}
}
