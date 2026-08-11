package provider

import (
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

// fakeValidateAPI stands in for the validate endpoint during a plan. Counting requests is
// what proves the plan stays quiet when there is nothing it could usefully check.
type fakeValidateAPI struct {
	// status is the response code, 204 when unset. body is served with it, so a case can
	// assert the API's own words reach the user.
	status int
	body   string

	requests int
}

func (f *fakeValidateAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/alert_sources/actions/validate", func(w http.ResponseWriter, r *http.Request) {
		f.requests++

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

// alertSourcePlan builds a plan with every attribute null but source_type and template,
// which is all validation reads. Filling from the schema's own type keeps these cases
// working as the schema grows.
func alertSourcePlan(t *testing.T, templateUnknown bool) tftypes.Value {
	t.Helper()

	objType := alertSourceSchemaType(t)

	attributes := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attrType, nil)
	}
	attributes["source_type"] = tftypes.NewValue(tftypes.String, "http")

	templateType := objType.AttributeTypes["template"]
	if templateUnknown {
		attributes["template"] = tftypes.NewValue(templateType, tftypes.UnknownValue)
	} else {
		attributes["template"] = nullFilledObject(t, templateType)
	}

	return tftypes.NewValue(objType, attributes)
}

// alertSourcePlanWithBinding builds a plan carrying one attribute binding whose
// merge_strategy is unknown, which is how Terraform plans a binding the config doesn't set
// one on.
func alertSourcePlanWithBinding(t *testing.T) tftypes.Value {
	t.Helper()

	objType := alertSourceSchemaType(t)
	templateType, ok := objType.AttributeTypes["template"].(tftypes.Object)
	if !ok {
		t.Fatal("expected template to be an object")
	}

	attributesType, ok := templateType.AttributeTypes["attributes"].(tftypes.Set)
	if !ok {
		t.Fatalf("expected template.attributes to be a set, got %s", templateType.AttributeTypes["attributes"])
	}

	bindingType, ok := attributesType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("expected template.attributes elements to be objects")
	}

	binding := nullFilledObject(t, bindingType)
	bindingAttributes := map[string]tftypes.Value{}
	if err := binding.As(&bindingAttributes); err != nil {
		t.Fatalf("reading the binding back: %v", err)
	}
	bindingAttributes["alert_attribute_id"] = tftypes.NewValue(tftypes.String, "01ATTRIBUTE")

	// The binding object holds merge_strategy, and Terraform leaves it unknown.
	inner, ok := bindingType.AttributeTypes["binding"].(tftypes.Object)
	if !ok {
		t.Fatal("expected the binding attribute to be an object")
	}
	innerAttributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, inner).As(&innerAttributes); err != nil {
		t.Fatalf("reading the inner binding back: %v", err)
	}
	innerAttributes["merge_strategy"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	bindingAttributes["binding"] = tftypes.NewValue(inner, innerAttributes)

	template := map[string]tftypes.Value{}
	if err := nullFilledObject(t, templateType).As(&template); err != nil {
		t.Fatalf("reading the template back: %v", err)
	}
	template["attributes"] = tftypes.NewValue(attributesType, []tftypes.Value{
		tftypes.NewValue(bindingType, bindingAttributes),
	})

	attributes := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attrType, nil)
	}
	attributes["source_type"] = tftypes.NewValue(tftypes.String, "http")
	attributes["template"] = tftypes.NewValue(templateType, template)

	return tftypes.NewValue(objType, attributes)
}

func alertSourceSchemaType(t *testing.T) tftypes.Object {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentAlertSourceResource().Schema(t.Context(), resource.SchemaRequest{}, &schemaResp)

	objType, ok := schemaResp.Schema.Type().TerraformType(t.Context()).(tftypes.Object)
	if !ok {
		t.Fatal("expected the alert source schema to be an object")
	}

	return objType
}

// nullFilledObject builds an object whose leaves are all null. Nested objects are built
// rather than nulled, because the model's param-binding types don't accept a null.
func nullFilledObject(t *testing.T, attrType tftypes.Type) tftypes.Value {
	t.Helper()

	objType, ok := attrType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected an object type, got %s", attrType)
	}

	attributes := map[string]tftypes.Value{}
	for name, nested := range objType.AttributeTypes {
		if _, nestedIsObject := nested.(tftypes.Object); nestedIsObject {
			attributes[name] = nullFilledObject(t, nested)
			continue
		}

		attributes[name] = tftypes.NewValue(nested, nil)
	}

	return tftypes.NewValue(objType, attributes)
}

// modifyAlertSourcePlan runs ModifyPlan against the fake API. A nil plan is a destroy.
func modifyAlertSourcePlan(t *testing.T, api *fakeValidateAPI, plan *tftypes.Value) resource.ModifyPlanResponse {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentAlertSourceResource().Schema(t.Context(), resource.SchemaRequest{}, &schemaResp)

	planRaw := tftypes.NewValue(alertSourceSchemaType(t), nil)
	if plan != nil {
		planRaw = *plan
	}

	r := &IncidentAlertSourceResource{client: api.start(t)}

	var resp resource.ModifyPlanResponse
	r.ModifyPlan(t.Context(), resource.ModifyPlanRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
	}, &resp)

	return resp
}

func TestAlertSourceModifyPlanAcceptsValidTemplate(t *testing.T) {
	api := &fakeValidateAPI{}
	plan := alertSourcePlan(t, false)

	resp := modifyAlertSourcePlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Errorf("expected 1 request, got %d", api.requests)
	}
}

// A 422 is the API rejecting this template, which is the whole point of the check, so it
// fails the plan and repeats what the API said.
func TestAlertSourceModifyPlanRejectsInvalidTemplate(t *testing.T) {
	api := &fakeValidateAPI{
		status: http.StatusUnprocessableEntity,
		body:   `{"type":"validation_error","errors":[{"code":"invalid_value","message":"referenced resource not found in scope","source":{"field":"template.attributes.01ABC.binding"}}]}`,
	}
	plan := alertSourcePlan(t, false)

	resp := modifyAlertSourcePlan(t, api, &plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error, got %+v", resp.Diagnostics)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "template.attributes.01ABC.binding") {
		t.Errorf("expected the field path in the diagnostic, got %q", detail)
	}
}

// Any other status means the check didn't run: the endpoint isn't deployed yet, the API is
// down. The config may be perfectly good, so this warns rather than failing the plan.
// The 500 also covers the timeout: the shared client would retry it for minutes, so this
// passing quickly is the bound doing its job.
func TestAlertSourceModifyPlanWarnsWhenCheckUnavailable(t *testing.T) {
	original := alertSourceValidateTimeout
	alertSourceValidateTimeout = 50 * time.Millisecond
	t.Cleanup(func() { alertSourceValidateTimeout = original })

	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		api := &fakeValidateAPI{status: status}
		plan := alertSourcePlan(t, false)

		resp := modifyAlertSourcePlan(t, api, &plan)

		if resp.Diagnostics.HasError() {
			t.Errorf("status %d: expected no error, got %+v", status, resp.Diagnostics)
		}
		if resp.Diagnostics.WarningsCount() != 1 {
			t.Errorf("status %d: expected 1 warning, got %+v", status, resp.Diagnostics)
		}
	}
}

// merge_strategy is Optional+Computed, so it is unknown in the plan for every binding the
// config doesn't set it on — which is most of them. Treating that as unsettled skipped the
// check on exactly the configs worth checking, so this guards the gate against tightening
// back to "everything known".
func TestAlertSourceModifyPlanValidatesWhenOnlyComputedValuesAreUnknown(t *testing.T) {
	api := &fakeValidateAPI{}
	plan := alertSourcePlanWithBinding(t)

	resp := modifyAlertSourcePlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Errorf("expected the template to be checked, got %d requests", api.requests)
	}
}

// A template Terraform hasn't settled yet would validate against values that aren't there,
// reporting errors the apply won't hit. So we don't ask.
func TestAlertSourceModifyPlanSkipsUnknownTemplate(t *testing.T) {
	api := &fakeValidateAPI{}
	plan := alertSourcePlan(t, true)

	resp := modifyAlertSourcePlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for an unsettled template, got %d", api.requests)
	}
}

func TestAlertSourceModifyPlanSkipsDestroy(t *testing.T) {
	api := &fakeValidateAPI{}

	resp := modifyAlertSourcePlan(t, api, nil)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for a destroy, got %d", api.requests)
	}
}
