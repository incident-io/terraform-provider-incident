package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// TestScheduleV3ResourceSchema builds the schema, which resolves every
// apischema.Docstring call against the embedded OpenAPI schema and panics if a
// definition or property is missing. It's the quickest way to catch a schedule
// resource built against a stale vendored schema.
func TestScheduleV3ResourceSchema(t *testing.T) {
	ctx := context.Background()
	r := NewIncidentScheduleBetaResource()

	var metaResp resource.MetadataResponse
	r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "incident"}, &metaResp)
	if metaResp.TypeName != "incident_schedule_beta" {
		t.Fatalf("unexpected type name: %q", metaResp.TypeName)
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema build produced diagnostics: %+v", schemaResp.Diagnostics)
	}

	for _, name := range []string{"id", "name", "timezone", "team_ids", "holidays_public_config"} {
		if _, ok := schemaResp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing expected attribute %q", name)
		}
	}
}

// TestTeamIDsToState covers the null-versus-empty distinction that decides
// whether an unowned schedule shows a diff on every plan: the API always returns
// team_ids, so "no teams" has to read back as whatever the config used.
func TestTeamIDsToState(t *testing.T) {
	nullSet := types.SetNull(types.StringType)
	emptySet := types.SetValueMust(types.StringType, []attr.Value{})

	if got := teamIDsToState(nil, nullSet); !got.IsNull() {
		t.Errorf("no teams with the attribute unset should stay null, got %v", got)
	}
	if got := teamIDsToState([]string{}, nullSet); !got.IsNull() {
		t.Errorf("empty teams with the attribute unset should stay null, got %v", got)
	}
	if got := teamIDsToState([]string{}, emptySet); got.IsNull() {
		t.Error("empty teams with team_ids = [] should stay an empty set, not null")
	}

	got := teamIDsToState([]string{"01ABC"}, nullSet)
	if got.IsNull() || len(got.Elements()) != 1 {
		t.Errorf("expected a one-element set, got %v", got)
	}
}

// TestScheduleV3FromAPI checks the holidays block round-trips, and that the API
// omitting it reads back as an absent block rather than an empty one.
func TestScheduleV3FromAPI(t *testing.T) {
	withHolidays := incidentScheduleBetaFromAPI(client.ScheduleV3{
		Id:                   "01SCHED",
		Name:                 "Platform on-call",
		Timezone:             "Europe/London",
		TeamIds:              []string{},
		HolidaysPublicConfig: &client.ScheduleHolidaysPublicConfigV2{CountryCodes: []string{"GB", "FR"}},
	}, types.SetNull(types.StringType))

	if withHolidays.HolidaysPublicConfig == nil {
		t.Fatal("expected holidays config to be set")
	}
	if len(withHolidays.HolidaysPublicConfig.CountryCodes) != 2 {
		t.Errorf("expected 2 country codes, got %v", withHolidays.HolidaysPublicConfig.CountryCodes)
	}
	if withHolidays.ID.ValueString() != "01SCHED" || withHolidays.Timezone.ValueString() != "Europe/London" {
		t.Errorf("unexpected model: %+v", withHolidays)
	}

	without := incidentScheduleBetaFromAPI(client.ScheduleV3{Id: "01SCHED", TeamIds: []string{}}, types.SetNull(types.StringType))
	if without.HolidaysPublicConfig != nil {
		t.Errorf("expected no holidays config, got %+v", without.HolidaysPublicConfig)
	}
}
