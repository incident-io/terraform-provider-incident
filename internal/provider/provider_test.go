package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var testRunID = uuid.NewString()

func StableSuffix(thing string) string {
	return fmt.Sprintf("%s (%s)", thing, testRunID)
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"incident": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv("INCIDENT_API_KEY") == "" {
		t.Skip("No INCIDENT_API_KEY environment variable set, skipping")
	}
}
