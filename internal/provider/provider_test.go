package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
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

// testTemplateFuncs is sprig plus stableSuffix, which names a resource uniquely per
// test run so runs against the shared test org don't collide:
// {{ stableSuffix "My thing" | quote }}.
func testTemplateFuncs() template.FuncMap {
	funcs := sprig.TxtFuncMap()
	funcs["stableSuffix"] = StableSuffix

	return funcs
}

func testRunTemplate(tmplName, source string, args any) string {
	tmpl := template.Must(template.New(tmplName).Funcs(testTemplateFuncs()).Parse(source))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, args)
	if err != nil {
		panic(err)
	}

	// A Sprintf verb in a Go template is emitted verbatim rather than substituted, so a
	// resource silently gets a fixed name that collides with every other run. Fail here
	// instead, where the cause is obvious.
	out := buf.String()
	if strings.Contains(out, "%[") {
		panic(fmt.Sprintf("%s: rendered config contains an unsubstituted format verb, use {{ }} instead", tmplName))
	}

	return out
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"incident": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck skips the test when there are no API credentials, and otherwise makes
// testClient ready for the checks and cleanup that talk to the API directly.
//
// Every acceptance test runs this from its PreCheck, so the client is built exactly once
// rather than reassigned each time: tests run in parallel, and a plain assignment here
// would be a write race between them. Reads of testClient after this returns see the
// built client, because sync.Once establishes that ordering.
func testAccPreCheck(t *testing.T) {
	apiKey := os.Getenv("INCIDENT_API_KEY")
	if apiKey == "" {
		t.Skip("No INCIDENT_API_KEY environment variable set, skipping")
	}

	testClientOnce.Do(func() {
		endpoint := os.Getenv("INCIDENT_ENDPOINT")
		if endpoint == "" {
			endpoint = "https://api.incident.io"
		}

		testClient, testClientErr = client.New(context.Background(), apiKey, endpoint, "test")
	})

	if testClientErr != nil {
		t.Fatalf("Error creating client: %s", testClientErr)
	}
}

var (
	testClientOnce sync.Once
	testClient     *client.ClientWithResponses
	testClientErr  error
)
