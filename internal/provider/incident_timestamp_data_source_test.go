package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// accIncidentTimestampDataSource looks a timestamp up both ways round.
//
// Timestamps can't be created from Terraform, so rather than hardcode one of the
// defaults the test asks the account what it has, the way the workflow examples
// do for Slack channels and users.
func accIncidentTimestampDataSource(t *testing.T) {
	// The lookup below runs before resource.Test, so honour TF_ACC ourselves rather
	// than calling the API during a unit test run, then initialise testClient.
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set, skipping acceptance test")
	}
	testAccPreCheck(t)

	timestamp := testAccIncidentTimestamp(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testRunTemplate("incident_timestamp_data_source", `
data "incident_incident_timestamp" "by_id" {
  id = {{ quote .Id }}
}
data "incident_incident_timestamp" "by_name" {
  name = {{ quote .Name }}
}
`, timestamp),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.incident_incident_timestamp.by_id", "name", timestamp.Name),
					resource.TestCheckResourceAttr(
						"data.incident_incident_timestamp.by_id", "rank", fmt.Sprintf("%d", timestamp.Rank)),
					resource.TestCheckResourceAttr(
						"data.incident_incident_timestamp.by_name", "id", timestamp.Id),
					resource.TestCheckResourceAttrPair(
						"data.incident_incident_timestamp.by_name", "rank",
						"data.incident_incident_timestamp.by_id", "rank"),
				),
			},
		},
	})
}

// Both lookup attributes are Optional and Computed, so setting both has to be
// rejected explicitly rather than by the schema.
func accIncidentTimestampDataSourceAmbiguousLookup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "incident_incident_timestamp" "both" {
  id   = "01FCNDV6P870EA6S7TK1DSYD5H"
  name = "Closed"
}
`,
				ExpectError: regexp.MustCompile("Ambiguous lookup"),
			},
		},
	})
}

// testAccIncidentTimestamp returns a timestamp with a unique name from the test
// account, which every account has: incident.io sets several on each incident.
func testAccIncidentTimestamp(t *testing.T) client.IncidentTimestampV2 {
	t.Helper()

	result, err := testClient.IncidentTimestampsV2ListWithResponse(t.Context())
	if err != nil {
		t.Fatalf("listing incident timestamps: %s", err)
	}
	if result.JSON200 == nil {
		t.Fatalf("listing incident timestamps: %s", string(result.Body))
	}

	count := map[string]int{}
	for _, timestamp := range result.JSON200.IncidentTimestamps {
		count[timestamp.Name]++
	}
	for _, timestamp := range result.JSON200.IncidentTimestamps {
		if count[timestamp.Name] == 1 {
			return timestamp
		}
	}

	t.Skip("no incident timestamp with a unique name in the test account")
	return client.IncidentTimestampV2{}
}

// TestIncidentTimestampDataSourceRead covers the lookups against a fake API, so
// the error messages someone hits for a missing or duplicated name are checked
// without needing an account.
func TestIncidentTimestampDataSourceRead(t *testing.T) {
	timestamps := []client.IncidentTimestampV2{
		{Id: "01REPORTED", Name: "Reported", Rank: 1},
		{Id: "01CLOSED", Name: "Closed", Rank: 2},
		{Id: "01DUPLICATE", Name: "Impact started", Rank: 3},
		{Id: "01DUPLICATE2", Name: "Impact started", Rank: 4},
	}
	api := startFakeIncidentTimestampsAPI(t, timestamps)

	t.Run("by id", func(t *testing.T) {
		model, resp := readIncidentTimestamp(t, api, "01CLOSED", "")
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
		if got, want := model.Name.ValueString(), "Closed"; got != want {
			t.Errorf("got name %q, want %q", got, want)
		}
		if got, want := model.Rank.ValueInt64(), int64(2); got != want {
			t.Errorf("got rank %d, want %d", got, want)
		}
	})

	t.Run("by name", func(t *testing.T) {
		model, resp := readIncidentTimestamp(t, api, "", "Reported")
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
		if got, want := model.ID.ValueString(), "01REPORTED"; got != want {
			t.Errorf("got id %q, want %q", got, want)
		}
		if got, want := model.Rank.ValueInt64(), int64(1); got != want {
			t.Errorf("got rank %d, want %d", got, want)
		}
	})

	t.Run("a name that matches nothing says so", func(t *testing.T) {
		_, resp := readIncidentTimestamp(t, api, "", "Mitigated")
		assertIncidentTimestampError(t, resp, `no incident timestamp found with name "Mitigated"`)
	})

	t.Run("a name matching more than one points at the id", func(t *testing.T) {
		_, resp := readIncidentTimestamp(t, api, "", "Impact started")
		assertIncidentTimestampError(t, resp, "look it up by id instead")
	})

	t.Run("an id that doesn't exist surfaces the API's error", func(t *testing.T) {
		_, resp := readIncidentTimestamp(t, api, "01NONSENSE", "")
		assertIncidentTimestampError(t, resp, "not_found")
	})
}

// TestIncidentTimestampDataSourceValidateConfig covers rejecting a lookup that
// names both attributes, or neither.
func TestIncidentTimestampDataSourceValidateConfig(t *testing.T) {
	for _, tc := range []struct {
		name    string
		id      string
		lookup  string
		wantErr string
	}{
		{name: "id only", id: "01CLOSED"},
		{name: "name only", lookup: "Closed"},
		{name: "both", id: "01CLOSED", lookup: "Closed", wantErr: "Ambiguous lookup"},
		{name: "neither", wantErr: "Missing lookup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			d := &IncidentTimestampDataSource{}

			schemaResp := incidentTimestampSchema(t, d)
			resp := &datasource.ValidateConfigResponse{}
			d.ValidateConfig(ctx, datasource.ValidateConfigRequest{
				Config: tfsdk.Config{
					Schema: schemaResp.Schema,
					Raw:    incidentTimestampConfig(ctx, schemaResp, tc.id, tc.lookup),
				},
			}, resp)

			if tc.wantErr == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
				}
				return
			}

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected an error diagnostic")
			}
			if summary := resp.Diagnostics.Errors()[0].Summary(); summary != tc.wantErr {
				t.Errorf("got summary %q, want %q", summary, tc.wantErr)
			}
		})
	}
}

// startFakeIncidentTimestampsAPI serves the list and show endpoints for a fixed
// set of timestamps.
func startFakeIncidentTimestampsAPI(t *testing.T, timestamps []client.IncidentTimestampV2) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/incident_timestamps", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, client.IncidentTimestampsListResultV2{IncidentTimestamps: timestamps})
	})
	mux.HandleFunc("/v2/incident_timestamps/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v2/incident_timestamps/")
		for _, timestamp := range timestamps {
			if timestamp.Id == id {
				writeJSON(t, w, client.IncidentTimestampsShowResultV2{IncidentTimestamp: timestamp})
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"not_found","status":404,"detail":"incident timestamp not found"}`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

// readIncidentTimestamp runs Read with id and name set in config, as Terraform
// would. An empty string stands for an attribute the config leaves unset.
func readIncidentTimestamp(t *testing.T, api *client.ClientWithResponses, id, name string) (IncidentTimestampDataSourceModel, *datasource.ReadResponse) {
	t.Helper()

	ctx := t.Context()
	d := &IncidentTimestampDataSource{dataSourceConfigurer: withClientDataSource(api)}

	schemaResp := incidentTimestampSchema(t, d)
	resp := &datasource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    incidentTimestampConfig(ctx, schemaResp, id, name),
		},
	}, resp)

	var model IncidentTimestampDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &model)...)
	}

	return model, resp
}

func incidentTimestampSchema(t *testing.T, d *IncidentTimestampDataSource) datasource.SchemaResponse {
	t.Helper()

	var schemaResp datasource.SchemaResponse
	d.Schema(t.Context(), datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building the schema: %v", schemaResp.Diagnostics.Errors())
	}

	return schemaResp
}

func incidentTimestampConfig(ctx context.Context, schemaResp datasource.SchemaResponse, id, name string) tftypes.Value {
	attribute := func(value string) tftypes.Value {
		if value == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}

		return tftypes.NewValue(tftypes.String, value)
	}

	return tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id":   attribute(id),
		"name": attribute(name),
		// Computed, so unset in config.
		"rank": tftypes.NewValue(tftypes.Number, nil),
	})
}

func assertIncidentTimestampError(t *testing.T, resp *datasource.ReadResponse, want string) {
	t.Helper()

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}

	err := resp.Diagnostics.Errors()[0]
	joined := err.Summary() + " | " + err.Detail()
	if !strings.Contains(joined, want) {
		t.Errorf("diagnostic %q does not mention %q", joined, want)
	}
}
