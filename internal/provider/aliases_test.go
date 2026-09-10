package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registeredResourceNames is every resource type the provider answers to.
func registeredResourceNames(ctx context.Context, t *testing.T) map[string]resource.Resource {
	t.Helper()

	p, ok := New("test")().(*IncidentProvider)
	require.True(t, ok)

	names := map[string]resource.Resource{}
	for _, newResource := range p.Resources(ctx) {
		r := newResource()
		name := resourceTypeName(ctx, r)
		require.NotContains(t, names, name, "two resources both answer to %s", name)
		names[name] = r
	}

	return names
}

// registeredDataSourceNames is every data source type the provider answers to.
func registeredDataSourceNames(ctx context.Context, t *testing.T) map[string]datasource.DataSource {
	t.Helper()

	incident, ok := New("test")().(*IncidentProvider)
	require.True(t, ok)

	names := map[string]datasource.DataSource{}
	for _, newDataSource := range incident.DataSources(ctx) {
		d := newDataSource()

		var resp datasource.MetadataResponse
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "incident"}, &resp)

		require.NotContains(t, names, resp.TypeName, "two data sources both answer to %s", resp.TypeName)
		names[resp.TypeName] = d
	}

	return names
}

// Every `_beta` name a v6 configuration could hold is still registered, because a name
// that stopped being registered is a configuration that stops planning - which is the
// breaking change the aliases exist to avoid. Losing one would otherwise only show up in
// somebody's upgrade.
func TestEveryBetaNameIsStillRegistered(t *testing.T) {
	ctx := context.Background()

	resources := registeredResourceNames(ctx, t)
	for _, name := range []string{
		"incident_alert_source_beta",
		"incident_alert_source_attribute_beta",
		"incident_escalation_path_beta",
		"incident_schedule_beta",
		"incident_schedule_rotation_beta",
	} {
		assert.Contains(t, resources, name)
	}

	dataSources := registeredDataSourceNames(ctx, t)
	for _, name := range []string{
		"incident_alert_source_attribute_beta",
		"incident_escalation_path_beta",
		"incident_schedule_beta",
		"incident_schedule_rotation_beta",
	} {
		assert.Contains(t, dataSources, name)
	}
}

// An alias warns and its resource doesn't, which is the whole difference between them at
// plan time. A missing warning leaves people on the old name with nothing telling them to
// move before v8 removes it; a warning on the real resource would fire on every plan
// anyone ever runs.
func TestOnlyTheAliasesAreDeprecated(t *testing.T) {
	ctx := context.Background()

	for name, r := range registeredResourceNames(ctx, t) {
		t.Run(name, func(t *testing.T) {
			message := declaredResourceSchema(ctx, r).DeprecationMessage

			if !strings.HasSuffix(name, "_beta") {
				assert.Empty(t, message, "a resource anyone should be using must not warn")
				return
			}

			require.NotEmpty(t, message, "a deprecated name has to say so")
			assert.Contains(t, message, strings.TrimSuffix(name, "_beta"),
				"the warning has to name the resource to move to")
			assert.Contains(t, message, "moved {", "the warning has to say how to move")
		})
	}
}

// The alias and the resource are one registration, so an alias's documentation would
// otherwise repeat a page that already exists under the new name. It says where the
// resource went instead.
func TestAliasDocumentationPointsAtTheNewName(t *testing.T) {
	ctx := context.Background()

	for name, r := range registeredResourceNames(ctx, t) {
		if !strings.HasSuffix(name, "_beta") {
			continue
		}

		t.Run(name, func(t *testing.T) {
			description := declaredResourceSchema(ctx, r).MarkdownDescription

			assert.Contains(t, description, "renamed to")
			assert.Contains(t, description, strings.TrimSuffix(name, "_beta"))
			assert.Contains(t, description, "moved {")
		})
	}
}
