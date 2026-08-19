package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// fakeAlertSourceAttributeValidateAPI stands in for the validate endpoint during a plan.
// Counting requests is what proves the plan stays quiet when there is nothing worth checking.
type fakeAlertSourceAttributeValidateAPI struct {
	// status is the response code, 204 when unset. body is served with it, so a case can
	// assert the API's own words reach the user.
	status int
	body   string

	requests int
	// received is the last request body, and sourceID the source it was sent about.
	received []byte
	sourceID string
}

func (f *fakeAlertSourceAttributeValidateAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v3/alert_sources/{id}/attributes/actions/validate", func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		f.received, _ = io.ReadAll(r.Body)
		f.sourceID = r.PathValue("id")

		status := f.status
		if status == 0 {
			status = http.StatusNoContent
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if f.body != "" {
			_, _ = w.Write([]byte(f.body))
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	api, err := client.New(t.Context(), "test-key", server.URL, "test")
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	return api
}

// alertSourceAttributePlan builds a plan the way the validate tests build a config: the two
// ids set, everything else null, then the overrides the case is about.
func alertSourceAttributePlan(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Plan {
	t.Helper()

	config := alertSourceAttributeConfig(t, overrides)

	return tfsdk.Plan(config)
}

// modifyAlertSourceAttributePlan runs ModifyPlan from no prior state, which is a create. A nil
// plan is a destroy.
func modifyAlertSourceAttributePlan(
	t *testing.T, api *fakeAlertSourceAttributeValidateAPI, plan *tfsdk.Plan,
) resource.ModifyPlanResponse {
	t.Helper()

	config, objType := alertSourceAttributeSchemaType(t)
	null := tftypes.NewValue(objType, nil)

	if plan == nil {
		plan = &tfsdk.Plan{Schema: config.Schema, Raw: null}
	}

	return modifyAlertSourceAttributePlanFrom(t, api, *plan, tfsdk.State{Schema: config.Schema, Raw: null})
}

func modifyAlertSourceAttributePlanFrom(
	t *testing.T, api *fakeAlertSourceAttributeValidateAPI, plan tfsdk.Plan, state tfsdk.State,
) resource.ModifyPlanResponse {
	t.Helper()

	r := &alertSourceAttributeBetaResource{client: api.start(t)}

	var resp resource.ModifyPlanResponse
	r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: state}, &resp)

	return resp
}

// Asserts the request too: a payload that doesn't describe the planned binding, or one asked
// about the wrong source, would validate something nobody is about to apply.
func TestAlertSourceAttributeBetaModifyPlanAcceptsValidBinding(t *testing.T) {
	api := &fakeAlertSourceAttributeValidateAPI{}
	plan := alertSourceAttributePlan(t, map[string]tftypes.Value{
		"value_literal": stringValue("critical"),
	})

	resp := modifyAlertSourceAttributePlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Fatalf("expected 1 request, got %d", api.requests)
	}
	if api.sourceID != "01ALERTSOURCE" {
		t.Errorf("expected the planned source in the path, got %q", api.sourceID)
	}

	var sent struct {
		AlertSourceAttribute struct {
			AlertAttributeID string  `json:"alert_attribute_id"`
			MergeStrategy    *string `json:"merge_strategy"`
			Value            *struct {
				Literal *string `json:"literal"`
			} `json:"value"`
		} `json:"alert_source_attribute"`
	}
	if err := json.Unmarshal(api.received, &sent); err != nil {
		t.Fatalf("decoding what we sent: %v (%s)", err, api.received)
	}

	attribute := sent.AlertSourceAttribute
	if attribute.AlertAttributeID != "01ALERTATTRIBUTE" {
		t.Errorf("expected the planned attribute, got %q", attribute.AlertAttributeID)
	}
	if attribute.Value == nil || attribute.Value.Literal == nil || *attribute.Value.Literal != "critical" {
		t.Errorf("expected the planned value to be sent, got %+v", attribute.Value)
	}
	// Unknown in the plan, so sending one would ask about a strategy the apply won't choose.
	if attribute.MergeStrategy != nil {
		t.Errorf("expected no merge strategy to be sent, got %q", *attribute.MergeStrategy)
	}
}

// A 422 fails the plan and repeats what the API said, field path included.
func TestAlertSourceAttributeBetaModifyPlanRejectsInvalidBinding(t *testing.T) {
	api := &fakeAlertSourceAttributeValidateAPI{
		status: http.StatusUnprocessableEntity,
		body:   `{"type":"validation_error","errors":[{"code":"invalid_value","message":"referenced resource not found in scope","source":{"field":"alert_source_attribute.value"}}]}`,
	}
	plan := alertSourceAttributePlan(t, map[string]tftypes.Value{
		"value_reference": stringValue("payload.nonsense"),
	})

	resp := modifyAlertSourceAttributePlan(t, api, &plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error, got %+v", resp.Diagnostics)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "not found in scope") {
		t.Errorf("expected the API's explanation to reach the user, got %q", detail)
	}
}

// Any other status means the check didn't run, over a config that may be perfectly good — a
// 404 included, since the source can be created by an apply this plan can't see.
func TestAlertSourceAttributeBetaModifyPlanWarnsWhenCheckUnavailable(t *testing.T) {
	original := alertSourceAttributeValidateTimeout
	alertSourceAttributeValidateTimeout = 50 * time.Millisecond
	t.Cleanup(func() { alertSourceAttributeValidateTimeout = original })

	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		api := &fakeAlertSourceAttributeValidateAPI{status: status}
		plan := alertSourceAttributePlan(t, map[string]tftypes.Value{
			"value_literal": stringValue("critical"),
		})

		resp := modifyAlertSourceAttributePlan(t, api, &plan)

		if resp.Diagnostics.HasError() {
			t.Errorf("status %d: expected no error, got %+v", status, resp.Diagnostics)
		}
		if resp.Diagnostics.WarningsCount() != 1 {
			t.Errorf("status %d: expected 1 warning, got %+v", status, resp.Diagnostics)
		}
	}
}

// merge_strategy is unknown on every create, the source deciding it. Treating that as
// unsettled would skip the plans worth checking.
func TestAlertSourceAttributeBetaModifyPlanValidatesWhenOnlyMergeStrategyIsUnknown(t *testing.T) {
	api := &fakeAlertSourceAttributeValidateAPI{}
	plan := alertSourceAttributePlan(t, map[string]tftypes.Value{
		"value_literal":  stringValue("critical"),
		"merge_strategy": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})

	resp := modifyAlertSourceAttributePlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Errorf("expected the binding to be checked, got %d requests", api.requests)
	}
}

// The shape of a first apply: the source is created by the same run, so there is nothing yet
// to validate against.
func TestAlertSourceAttributeBetaModifyPlanSkipsUnknownValues(t *testing.T) {
	for name, override := range map[string]map[string]tftypes.Value{
		"source not created yet": {
			"alert_source_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"value_literal":   stringValue("critical"),
		},
		"value from another resource": {
			"value_literal": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		},
		"expression from another resource": {
			"expression": tftypes.NewValue(alertSourceAttributeType(t, "expression"), tftypes.UnknownValue),
		},
	} {
		t.Run(name, func(t *testing.T) {
			api := &fakeAlertSourceAttributeValidateAPI{}
			plan := alertSourceAttributePlan(t, override)

			resp := modifyAlertSourceAttributePlan(t, api, &plan)

			if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
				t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
			}
			if api.requests != 0 {
				t.Errorf("expected no request for an unsettled plan, got %d", api.requests)
			}
		})
	}
}

// ModifyPlan runs for every resource in the plan, changed or not, so without this a source
// with a dozen attributes costs a dozen checks per plan.
func TestAlertSourceAttributeBetaModifyPlanSkipsUnchangedBinding(t *testing.T) {
	api := &fakeAlertSourceAttributeValidateAPI{}
	plan := alertSourceAttributePlan(t, map[string]tftypes.Value{
		"value_literal":  stringValue("critical"),
		"merge_strategy": stringValue("first-wins"),
	})

	resp := modifyAlertSourceAttributePlanFrom(t, api, plan, tfsdk.State(plan))

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for an unchanged binding, got %d", api.requests)
	}
}

func TestAlertSourceAttributeBetaModifyPlanSkipsDestroy(t *testing.T) {
	api := &fakeAlertSourceAttributeValidateAPI{}

	resp := modifyAlertSourceAttributePlan(t, api, nil)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for a destroy, got %d", api.requests)
	}
}

// The gate names its exemptions as strings, so a schema rename would silently start waiting on
// a value the check never sends, skipping every create.
func TestAlertSourceAttributeValidateUnsentAttributesExist(t *testing.T) {
	_, objType := alertSourceAttributeSchemaType(t)

	for _, name := range alertSourceAttributeValidateUnsentAttributes {
		if _, ok := objType.AttributeTypes[name]; !ok {
			t.Errorf("the settled gate exempts %q, which the schema no longer has", name)
		}
	}
}
