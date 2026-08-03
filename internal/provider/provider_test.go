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

// testRunShortID keeps concurrent runs apart while costing as few characters as
// possible. Custom fields cap names at 50, and several test names compose: a route
// name carries the suffix and is then interpolated into a custom field name, so every
// character here is paid for more than once. Four hex characters is 65,536 values,
// ample for the handful of runs that ever overlap.
var testRunShortID = testRunID[:4]

// StableSuffix makes a name unique to this test run. The result is not meant to be
// readable, only unique and short: see testRunShortID.
func StableSuffix(thing string) string {
	return fmt.Sprintf("%s-%s", thing, testRunShortID)
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
