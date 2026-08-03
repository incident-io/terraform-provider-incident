package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/incident-io/terraform-provider-incident/internal/client"
)

var testRunID = uuid.NewString()

// testRunShortID keeps concurrent runs apart without eating the name length limits
// several resources enforce: custom fields cap at 50 characters, and a full UUID adds
// 39. Eight hex characters is 4.3 billion values, which is plenty for the handful of
// runs that overlap in practice.
var testRunShortID = testRunID[:8]

func StableSuffix(thing string) string {
	return fmt.Sprintf("%s (%s)", thing, testRunShortID)
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"incident": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv("INCIDENT_API_KEY") == "" {
		t.Skip("No INCIDENT_API_KEY environment variable set, skipping")
	} else {
		apiKey := os.Getenv("INCIDENT_API_KEY")
		endpoint := os.Getenv("INCIDENT_ENDPOINT")
		if endpoint == "" {
			endpoint = "https://api.incident.io"
		}
		var err error
		testClient, err = client.New(context.Background(), apiKey, endpoint, "test")
		if err != nil {
			t.Fatalf("Error creating client: %s", err)
		}
	}
}

var testClient *client.ClientWithResponses
