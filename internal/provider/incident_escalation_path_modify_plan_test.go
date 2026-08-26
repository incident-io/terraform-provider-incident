package provider

import (
	"encoding/json"
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

// fakeEscalationPathValidateAPI stands in for the validate endpoint during a plan.
// Counting requests is what proves the plan stays quiet when there is nothing it could
// usefully check.
type fakeEscalationPathValidateAPI struct {
	// status is the response code, 200 when unset. body is served with it, so a case can
	// assert the API's own words reach the user.
	status int
	body   string

	requests int
	// lastPayload is what the provider sent, for cases that care about the request rather
	// than the response.
	lastPayload client.EscalationsValidatePathPayloadV2
}

func (f *fakeEscalationPathValidateAPI) start(t *testing.T) *client.ClientWithResponses {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/escalation_paths/actions/validate", func(w http.ResponseWriter, r *http.Request) {
		f.requests++

		f.lastPayload = client.EscalationsValidatePathPayloadV2{}
		if err := json.NewDecoder(r.Body).Decode(&f.lastPayload); err != nil {
			t.Errorf("decoding the validate payload: %v", err)
		}

		status := f.status
		if status == 0 {
			status = http.StatusOK
		}

		body := f.body
		if body == "" && status == http.StatusOK {
			body = `{"warnings":[]}`
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
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

func escalationPathSchemaType(t *testing.T) tftypes.Object {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentEscalationPathResource().Schema(t.Context(), resource.SchemaRequest{}, &schemaResp)

	objType, ok := schemaResp.Schema.Type().TerraformType(t.Context()).(tftypes.Object)
	if !ok {
		t.Fatal("expected the escalation path schema to be an object")
	}

	return objType
}

// escalationPathTargetValue builds a single level target, matching the schema's target
// object type. scheduleModeUnknown simulates the plan for a target whose config doesn't
// set schedule_mode, which is Optional+Computed with no static default.
func escalationPathTargetValue(t *testing.T, targetType tftypes.Object, scheduleModeUnknown bool) tftypes.Value {
	t.Helper()

	attributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, targetType).As(&attributes); err != nil {
		t.Fatalf("reading the target back: %v", err)
	}
	attributes["id"] = tftypes.NewValue(tftypes.String, "01SCHEDULE")
	attributes["type"] = tftypes.NewValue(tftypes.String, "schedule")
	attributes["urgency"] = tftypes.NewValue(tftypes.String, "high")
	if scheduleModeUnknown {
		attributes["schedule_mode"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}

	return tftypes.NewValue(targetType, attributes)
}

// escalationPathPlan builds a plan for one "level" node with one target, which is all
// validation needs. Filling from the schema's own type keeps this working as the schema
// grows.
func escalationPathPlan(t *testing.T, scheduleModeUnknown bool) tftypes.Value {
	t.Helper()

	objType := escalationPathSchemaType(t)

	pathType, ok := objType.AttributeTypes["path"].(tftypes.List)
	if !ok {
		t.Fatal("expected path to be a list")
	}
	nodeType, ok := pathType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("expected path elements to be objects")
	}
	levelType, ok := nodeType.AttributeTypes["level"].(tftypes.Object)
	if !ok {
		t.Fatal("expected level to be an object")
	}
	targetsType, ok := levelType.AttributeTypes["targets"].(tftypes.List)
	if !ok {
		t.Fatal("expected level.targets to be a list")
	}
	targetType, ok := targetsType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("expected level.targets elements to be objects")
	}

	target := escalationPathTargetValue(t, targetType, scheduleModeUnknown)

	levelAttributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, levelType).As(&levelAttributes); err != nil {
		t.Fatalf("reading level back: %v", err)
	}
	levelAttributes["targets"] = tftypes.NewValue(targetsType, []tftypes.Value{target})

	nodeAttributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, nodeType).As(&nodeAttributes); err != nil {
		t.Fatalf("reading node back: %v", err)
	}
	nodeAttributes["type"] = tftypes.NewValue(tftypes.String, "level")
	nodeAttributes["level"] = tftypes.NewValue(levelType, levelAttributes)
	node := tftypes.NewValue(nodeType, nodeAttributes)

	attributes := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attrType, nil)
	}
	attributes["name"] = tftypes.NewValue(tftypes.String, "Test Path")
	attributes["path"] = tftypes.NewValue(pathType, []tftypes.Value{node})

	return tftypes.NewValue(objType, attributes)
}

// modifyEscalationPathPlan runs ModifyPlan against the fake API, as a create: no prior
// state. A nil plan is a destroy.
func modifyEscalationPathPlan(t *testing.T, api *fakeEscalationPathValidateAPI, plan *tftypes.Value) resource.ModifyPlanResponse {
	t.Helper()

	return modifyEscalationPathPlanWithState(t, api, plan, tftypes.NewValue(escalationPathSchemaType(t), nil))
}

// modifyEscalationPathPlanWithState runs ModifyPlan against a resource that already
// exists, for the cases that turn on how the plan differs from its state.
func modifyEscalationPathPlanWithState(t *testing.T, api *fakeEscalationPathValidateAPI, plan *tftypes.Value, state tftypes.Value) resource.ModifyPlanResponse {
	t.Helper()

	var schemaResp resource.SchemaResponse
	NewIncidentEscalationPathResource().Schema(t.Context(), resource.SchemaRequest{}, &schemaResp)

	planRaw := tftypes.NewValue(escalationPathSchemaType(t), nil)
	if plan != nil {
		planRaw = *plan
	}

	r := &IncidentEscalationPathResource{client: api.start(t)}

	var resp resource.ModifyPlanResponse
	r.ModifyPlan(t.Context(), resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw},
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: state},
	}, &resp)

	return resp
}

func TestEscalationPathModifyPlanAcceptsValidPath(t *testing.T) {
	api := &fakeEscalationPathValidateAPI{}
	plan := escalationPathPlan(t, false)

	resp := modifyEscalationPathPlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Errorf("expected 1 request, got %d", api.requests)
	}
}

// A 422 is the API rejecting this config, which is the whole point of the check, so it
// fails the plan and repeats what the API said.
func TestEscalationPathModifyPlanRejectsInvalidPath(t *testing.T) {
	api := &fakeEscalationPathValidateAPI{
		status: http.StatusUnprocessableEntity,
		body:   `{"type":"validation_error","errors":[{"code":"invalid_value","message":"selected_rota_id is required","source":{"field":"path.0.level.targets.0.selected_rota_id"}}]}`,
	}
	plan := escalationPathPlan(t, false)

	resp := modifyEscalationPathPlan(t, api, &plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error, got %+v", resp.Diagnostics)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	if want := "path.0.level.targets.0.selected_rota_id"; !strings.Contains(detail, want) {
		t.Errorf("expected the field path %q in the diagnostic, got %q", want, detail)
	}
}

// Any other status means the check didn't run: the endpoint isn't deployed yet, the API is
// down. The config may be perfectly good, so this warns rather than failing the plan.
// The 500 also covers the timeout: the shared client would retry it for minutes, so this
// passing quickly is the bound doing its job.
func TestEscalationPathModifyPlanWarnsWhenCheckUnavailable(t *testing.T) {
	original := escalationPathValidateTimeout
	escalationPathValidateTimeout = 50 * time.Millisecond
	t.Cleanup(func() { escalationPathValidateTimeout = original })

	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		api := &fakeEscalationPathValidateAPI{status: status}
		plan := escalationPathPlan(t, false)

		resp := modifyEscalationPathPlan(t, api, &plan)

		if resp.Diagnostics.HasError() {
			t.Errorf("status %d: expected no error, got %+v", status, resp.Diagnostics)
		}
		if resp.Diagnostics.WarningsCount() != 1 {
			t.Errorf("status %d: expected 1 warning, got %+v", status, resp.Diagnostics)
		}
	}
}

// A successful response can still carry warnings about a config that's valid but has no
// practical effect, like an if_else with an empty branch. Those should reach the user
// without failing the plan.
func TestEscalationPathModifyPlanSurfacesWarnings(t *testing.T) {
	api := &fakeEscalationPathValidateAPI{
		body: `{"warnings":[{"path":"path.0.if_else.then_path","summary":"if_else has an empty \"then\" branch","detail":"When the condition matches, escalation stops here instead of continuing."}]}`,
	}
	plan := escalationPathPlan(t, false)

	resp := modifyEscalationPathPlan(t, api, &plan)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no error, got %+v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 {
		t.Fatalf("expected 1 warning, got %+v", resp.Diagnostics)
	}
	if got := resp.Diagnostics.Warnings()[0].Summary(); got != `if_else has an empty "then" branch` {
		t.Errorf("expected the API's warning summary, got %q", got)
	}
}

// schedule_mode is Optional+Computed, so it is unknown in the plan for every target the
// config doesn't set it on. Treating that as unsettled would skip the check on exactly the
// configs worth checking, so this guards the gate against tightening back to "everything
// known".
func TestEscalationPathModifyPlanValidatesWhenOnlyComputedValuesAreUnknown(t *testing.T) {
	api := &fakeEscalationPathValidateAPI{}
	plan := escalationPathPlan(t, true)

	resp := modifyEscalationPathPlan(t, api, &plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Errorf("expected the path to be checked, got %d requests", api.requests)
	}
}

// A plan that matches state changes nothing, so there is no apply to fail - and checking
// anyway would cost a request per escalation path on every plan.
func TestEscalationPathModifyPlanSkipsUnchangedPath(t *testing.T) {
	api := &fakeEscalationPathValidateAPI{}
	plan := escalationPathPlan(t, false)

	resp := modifyEscalationPathPlanWithState(t, api, &plan, plan)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for an unchanged path, got %d", api.requests)
	}
}

// A target pointing at a rota this same apply creates is unknown until it exists.
// Validating around the gap would report errors the apply won't hit, so we don't ask.
func TestEscalationPathModifyPlanSkipsUnknownTarget(t *testing.T) {
	api := &fakeEscalationPathValidateAPI{}
	plan := escalationPathPlan(t, false)

	objType := escalationPathSchemaType(t)
	pathType, ok := objType.AttributeTypes["path"].(tftypes.List)
	if !ok {
		t.Fatal("expected path to be a list")
	}
	nodeType, ok := pathType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("expected path elements to be objects")
	}
	levelType, ok := nodeType.AttributeTypes["level"].(tftypes.Object)
	if !ok {
		t.Fatal("expected level to be an object")
	}
	targetsType, ok := levelType.AttributeTypes["targets"].(tftypes.List)
	if !ok {
		t.Fatal("expected level.targets to be a list")
	}
	targetType, ok := targetsType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("expected level.targets elements to be objects")
	}

	attributes := map[string]tftypes.Value{}
	if err := plan.As(&attributes); err != nil {
		t.Fatalf("reading plan back: %v", err)
	}
	pathElements := []tftypes.Value{}
	if err := attributes["path"].As(&pathElements); err != nil {
		t.Fatalf("reading path back: %v", err)
	}
	nodeAttributes := map[string]tftypes.Value{}
	if err := pathElements[0].As(&nodeAttributes); err != nil {
		t.Fatalf("reading node back: %v", err)
	}
	levelAttributes := map[string]tftypes.Value{}
	if err := nodeAttributes["level"].As(&levelAttributes); err != nil {
		t.Fatalf("reading level back: %v", err)
	}
	targetAttributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, targetType).As(&targetAttributes); err != nil {
		t.Fatalf("reading target back: %v", err)
	}
	// selected_rota_id referencing a rota created in the same apply: known type, unknown
	// value.
	targetAttributes["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	levelAttributes["targets"] = tftypes.NewValue(targetsType, []tftypes.Value{tftypes.NewValue(targetType, targetAttributes)})
	nodeAttributes["level"] = tftypes.NewValue(levelType, levelAttributes)
	attributes["path"] = tftypes.NewValue(pathType, []tftypes.Value{tftypes.NewValue(nodeType, nodeAttributes)})
	unsettled := tftypes.NewValue(objType, attributes)

	resp := modifyEscalationPathPlan(t, api, &unsettled)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for an unsettled target, got %d", api.requests)
	}
}

func TestEscalationPathModifyPlanSkipsDestroy(t *testing.T) {
	api := &fakeEscalationPathValidateAPI{}

	resp := modifyEscalationPathPlan(t, api, nil)

	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() > 0 {
		t.Errorf("expected no diagnostics, got %+v", resp.Diagnostics)
	}
	if api.requests != 0 {
		t.Errorf("expected no request for a destroy, got %d", api.requests)
	}
}

// escalationPathReassignmentPlan builds a plan for one escalation_path node, so a test can
// watch what the provider sends the validate endpoint for a reassignment. It's the closest
// we get to exercising the node without a stack that has the feature enabled: the fake API
// accepts what a flagged-off one would reject, leaving the provider's own conversion as
// the only thing under test.
func escalationPathReassignmentPlan(t *testing.T, targetPathID string) tftypes.Value {
	t.Helper()

	objType := escalationPathSchemaType(t)

	pathType, ok := objType.AttributeTypes["path"].(tftypes.List)
	if !ok {
		t.Fatal("expected path to be a list")
	}
	nodeType, ok := pathType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("expected path elements to be objects")
	}
	blockType, ok := nodeType.AttributeTypes["escalation_path"].(tftypes.Object)
	if !ok {
		t.Fatal("expected escalation_path to be an object")
	}

	blockAttributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, blockType).As(&blockAttributes); err != nil {
		t.Fatalf("reading escalation_path back: %v", err)
	}
	blockAttributes["escalation_path_id"] = tftypes.NewValue(tftypes.String, targetPathID)

	nodeAttributes := map[string]tftypes.Value{}
	if err := nullFilledObject(t, nodeType).As(&nodeAttributes); err != nil {
		t.Fatalf("reading node back: %v", err)
	}
	nodeAttributes["type"] = tftypes.NewValue(tftypes.String, "escalation_path")
	nodeAttributes["escalation_path"] = tftypes.NewValue(blockType, blockAttributes)
	node := tftypes.NewValue(nodeType, nodeAttributes)

	attributes := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attrType, nil)
	}
	attributes["name"] = tftypes.NewValue(tftypes.String, "Reassigning path")
	attributes["path"] = tftypes.NewValue(pathType, []tftypes.Value{node})

	return tftypes.NewValue(objType, attributes)
}

// TestEscalationPathModifyPlanSendsReassignment reads the payload a plan puts on the wire,
// which is the half of the round trip the model tests can't see: the schema, the object
// decode and toPathPayload all have to agree before the block reaches the API at all. A
// reassignment arriving with a nil block is the failure this node type exists to fix, and
// from outside the provider it looks identical to a clean plan.
func TestEscalationPathModifyPlanSendsReassignment(t *testing.T) {
	api := &fakeEscalationPathValidateAPI{}
	plan := escalationPathReassignmentPlan(t, "01TARGET")

	resp := modifyEscalationPathPlan(t, api, &plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %+v", resp.Diagnostics)
	}
	if api.requests != 1 {
		t.Fatalf("expected 1 validate request, got %d", api.requests)
	}
	if len(api.lastPayload.Path) != 1 {
		t.Fatalf("expected 1 node in the payload, got %d", len(api.lastPayload.Path))
	}

	node := api.lastPayload.Path[0]
	if node.Type != client.EscalationPathNodePayloadV2TypeEscalationPath {
		t.Errorf("got type %q, want escalation_path", node.Type)
	}
	if node.EscalationPath == nil {
		t.Fatal("the node reached the API without its escalation_path block")
	}
	if got := node.EscalationPath.EscalationPathId; got != "01TARGET" {
		t.Errorf("got escalation_path_id %q, want 01TARGET", got)
	}
}
