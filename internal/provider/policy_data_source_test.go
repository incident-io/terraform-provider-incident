package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func policyFixtures() []client.PolicyV2 {
	policy := func(id, name string, policyType client.PolicyV2PolicyType) client.PolicyV2 {
		return client.PolicyV2{
			Id:          id,
			Name:        name,
			Description: lo.ToPtr(name + " description"),
			Status:      "enabled",
			PolicyType:  policyType,
		}
	}

	return []client.PolicyV2{
		policy("01FOLLOWUP", "Follow-ups completed", "follow_up"),
		policy("01POSTMORTEM", "Post-mortems within 5 days", "post_mortem"),
		policy("01READINESS", "Responders carry a phone", "on_call_readiness"),
	}
}

func TestPolicyDataSourceRead(t *testing.T) {
	api, requests := startFakePoliciesAPI(t, policyFixtures(), 50)

	t.Run("by id", func(t *testing.T) {
		model, resp := readPolicy(t, api, "01POSTMORTEM", "")
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
		if got, want := model.Name.ValueString(), "Post-mortems within 5 days"; got != want {
			t.Errorf("got name %q, want %q", got, want)
		}
		if got, want := model.PolicyType.ValueString(), "post_mortem"; got != want {
			t.Errorf("got policy_type %q, want %q", got, want)
		}
		if model.Status.ValueString() != "enabled" {
			t.Errorf("want status enabled, got %q", model.Status.ValueString())
		}
	})

	t.Run("by name", func(t *testing.T) {
		model, resp := readPolicy(t, api, "", "Responders carry a phone")
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
		if got, want := model.ID.ValueString(), "01READINESS"; got != want {
			t.Errorf("got id %q, want %q", got, want)
		}
		if got, want := model.PolicyType.ValueString(), "on_call_readiness"; got != want {
			t.Errorf("got policy_type %q, want %q", got, want)
		}
	})

	t.Run("a name that matches nothing says so", func(t *testing.T) {
		_, resp := readPolicy(t, api, "", "Debriefs held")
		assertPolicyError(t, resp, `no policy found with name "Debriefs held"`)
	})

	t.Run("an id that doesn't exist surfaces the API's error", func(t *testing.T) {
		_, resp := readPolicy(t, api, "01NONSENSE", "")
		assertPolicyError(t, resp, "not_found")
	})

	t.Run("a lookup by id makes no list call", func(t *testing.T) {
		before := requests.listCalls
		if _, resp := readPolicy(t, api, "01FOLLOWUP", ""); resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
		}
		if requests.listCalls != before {
			t.Errorf("want no list calls, got %d", requests.listCalls-before)
		}
	})
}

// TestPolicyDataSourceReadPagesToFindAName is the case the timestamp data source doesn't
// have: the policy list is paginated and carries no name filter, so a name on a later page
// is only found by walking the cursor.
func TestPolicyDataSourceReadPagesToFindAName(t *testing.T) {
	// One per page, so the third policy needs two cursor follows.
	api, requests := startFakePoliciesAPI(t, policyFixtures(), 1)

	model, resp := readPolicy(t, api, "", "Responders carry a phone")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
	}
	if got, want := model.ID.ValueString(), "01READINESS"; got != want {
		t.Errorf("got id %q, want %q", got, want)
	}
	if requests.listCalls != 3 {
		t.Errorf("want 3 list calls to reach the third page, got %d", requests.listCalls)
	}
}

// TestPolicyDataSourceReadStopsAtTheLastPage pins the loop's exit. The endpoint returns an
// after cursor only while another page exists, so a name that matches nothing has to end the
// walk rather than re-request the final page forever.
func TestPolicyDataSourceReadStopsAtTheLastPage(t *testing.T) {
	api, requests := startFakePoliciesAPI(t, policyFixtures(), 1)

	_, resp := readPolicy(t, api, "", "Never going to match")
	assertPolicyError(t, resp, "no policy found")

	if requests.listCalls != 3 {
		t.Errorf("want exactly 3 list calls for 3 pages, got %d", requests.listCalls)
	}
}

func TestPolicyDataSourceValidateConfig(t *testing.T) {
	for _, tc := range []struct {
		name    string
		id      string
		lookup  string
		wantErr string
	}{
		{name: "id only", id: "01POSTMORTEM"},
		{name: "name only", lookup: "Post-mortems within 5 days"},
		{name: "both", id: "01POSTMORTEM", lookup: "Post-mortems within 5 days", wantErr: "Ambiguous lookup"},
		{name: "neither", wantErr: "Missing lookup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			d := &IncidentPolicyDataSource{}
			schemaResp := policyDataSourceSchema(t, d)

			resp := &datasource.ValidateConfigResponse{}
			d.ValidateConfig(ctx, datasource.ValidateConfigRequest{
				Config: tfsdk.Config{
					Schema: schemaResp.Schema,
					Raw:    policyDataSourceConfig(ctx, schemaResp, tc.id, tc.lookup),
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

type policyAPIRequests struct {
	listCalls int
}

// startFakePoliciesAPI serves the list and show endpoints, paginating the list at pageSize
// so the cursor walk can be exercised. It also counts list calls, which is how the tests
// tell paging from a single fetch.
func startFakePoliciesAPI(t *testing.T, policies []client.PolicyV2, pageSize int) (*client.ClientWithResponses, *policyAPIRequests) {
	t.Helper()

	requests := &policyAPIRequests{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/policies", func(w http.ResponseWriter, r *http.Request) {
		requests.listCalls++

		start := 0
		if after := r.URL.Query().Get("after"); after != "" {
			for idx, policy := range policies {
				if policy.Id == after {
					start = idx + 1
					break
				}
			}
		}

		end := min(start+pageSize, len(policies))
		page := policies[start:end]

		meta := client.PaginationMetaResultV2{PageSize: int64(pageSize)}
		// Only advertise a cursor while another page exists, matching the real endpoint.
		if end < len(policies) && len(page) > 0 {
			meta.After = lo.ToPtr(page[len(page)-1].Id)
		}

		writeJSON(t, w, client.PoliciesListResultV2{Policies: page, PaginationMeta: meta})
	})
	mux.HandleFunc("/v2/policies/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v2/policies/")
		for _, policy := range policies {
			if policy.Id == id {
				writeJSON(t, w, client.PoliciesShowResultV2{Policy: policy})
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"not_found","status":404,"detail":"policy not found"}`))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api, requests
}

// readPolicy runs Read with id and name set in config, as Terraform would. An empty string
// stands for an attribute the config leaves unset.
func readPolicy(t *testing.T, api *client.ClientWithResponses, id, name string) (IncidentPolicyDataSourceModel, *datasource.ReadResponse) {
	t.Helper()

	ctx := t.Context()
	d := &IncidentPolicyDataSource{dataSourceConfigurer: withClientDataSource(api)}

	schemaResp := policyDataSourceSchema(t, d)
	resp := &datasource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    policyDataSourceConfig(ctx, schemaResp, id, name),
		},
	}, resp)

	var model IncidentPolicyDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &model)...)
	}

	return model, resp
}

func policyDataSourceSchema(t *testing.T, d *IncidentPolicyDataSource) datasource.SchemaResponse {
	t.Helper()

	var schemaResp datasource.SchemaResponse
	d.Schema(t.Context(), datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building the schema: %v", schemaResp.Diagnostics.Errors())
	}

	return schemaResp
}

func policyDataSourceConfig(ctx context.Context, schemaResp datasource.SchemaResponse, id, name string) tftypes.Value {
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
		"description": tftypes.NewValue(tftypes.String, nil),
		"status":      tftypes.NewValue(tftypes.String, nil),
		"policy_type": tftypes.NewValue(tftypes.String, nil),
	})
}

func assertPolicyError(t *testing.T, resp *datasource.ReadResponse, want string) {
	t.Helper()

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic")
	}

	err := resp.Diagnostics.Errors()[0]
	joined := fmt.Sprintf("%s | %s", err.Summary(), err.Detail())
	if !strings.Contains(joined, want) {
		t.Errorf("diagnostic %q does not mention %q", joined, want)
	}
}

// TestAccIncidentPolicyDataSource reads a policy this test creates, by id and by name, so
// the round trip through the real API is covered rather than just the fake one above.
func TestAccIncidentPolicyDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testRunTemplate("policy_data_source", `
resource "incident_policy" "subject" {
  name        = {{ quote .Name }}
  description = "Created by the incident_policy data source acceptance test."

  condition_groups = []

  on_call_readiness = {
    high_urgency = [
      {
        method_types      = ["email"]
        max_delay_seconds = 300
      }
    ]
  }
}

data "incident_policy" "by_id" {
  id = incident_policy.subject.id
}

data "incident_policy" "by_name" {
  name = incident_policy.subject.name
}
`, struct{ Name string }{Name: StableSuffix("Data source lookup (acceptance)")}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.incident_policy.by_id", "name", "incident_policy.subject", "name"),
					resource.TestCheckResourceAttrPair(
						"data.incident_policy.by_name", "id", "incident_policy.subject", "id"),
					resource.TestCheckResourceAttr(
						"data.incident_policy.by_id", "policy_type", "on_call_readiness"),
					resource.TestCheckResourceAttr(
						"data.incident_policy.by_id", "status", "enabled"),
					// The name lookup pages the list, so this also proves the cursor walk
					// works against the real endpoint and not just the fake one.
					resource.TestCheckResourceAttrPair(
						"data.incident_policy.by_name", "policy_type",
						"data.incident_policy.by_id", "policy_type"),
				),
			},
		},
	})
}
