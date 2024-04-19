package provider

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
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

func overrideAndMarshalModel[T any](base *T, override *T) string {
	// Merge any non-zero fields in override into the base model.
	if override != nil {
		for idx := 0; idx < reflect.TypeOf(*override).NumField(); idx++ {
			field := reflect.ValueOf(*override).Field(idx)
			if !field.IsZero() {
				reflect.ValueOf(&base).Elem().Field(idx).Set(field)
			}
		}
	}

	var buf bytes.Buffer
	if err := incidentWorkflowTemplate.Execute(&buf, base); err != nil {
		panic(err)
	}

	return buf.String()
}
