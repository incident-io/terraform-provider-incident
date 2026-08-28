// Package shim exposes the provider to consumers outside this module.
//
// The implementation lives under internal/, which Go forbids other modules from
// importing. The Pulumi bridge (github.com/incident-io/pulumi-incident) needs a
// provider.Provider value to wrap, so it depends on this package instead.
package shim

import (
	"github.com/hashicorp/terraform-plugin-framework/provider"

	incident "github.com/incident-io/terraform-provider-incident/v6/internal/provider"
)

// NewProvider returns the incident.io Terraform provider.
//
// version is reported as Terraform provider metadata. The bridge passes its own
// Pulumi package version, so the two stay in step.
func NewProvider(version string) provider.Provider {
	return incident.New(version)()
}
