package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeEscalationPathsAPI is an in-memory stand-in for the V2 and V3 escalation path
// endpoints, enforcing the rules that make the provider pick between them:
//
//   - V2 refuses a write branching on escalation.urgency, and V3 one branching on
//     escalation.priority, with a 422;
//   - V2 refuses to show or update a path stored with urgency conditions, with a 409;
//   - V3 shows a path stored with priority conditions as urgency ones, as the real API
//     converts each priority to its default urgency.
//
// Paths are stored as the JSON payload they were written with, with each target's high/low
// setting under a version-neutral key, and rendered in the shape of whichever version
// reads them.
type fakeEscalationPathsAPI struct {
	t *testing.T

	mu     sync.Mutex
	paths  map[string]map[string]any
	nextID int
	calls  []string
}

// fakePriorityUrgencies is the default urgency of each priority the tests branch on.
var fakePriorityUrgencies = map[string]string{"01P1": "01HIGH", "01P2": "01LOW"}

func newFakeEscalationPathsAPI(t *testing.T) *fakeEscalationPathsAPI {
	fake := &fakeEscalationPathsAPI{t: t, paths: map[string]map[string]any{}}

	mux := http.NewServeMux()
	for _, version := range []string{"v2", "v3"} {
		mux.HandleFunc("POST /"+version+"/escalation_paths", fake.handle(version, fake.create))
		mux.HandleFunc("GET /"+version+"/escalation_paths/{id}", fake.handle(version, fake.show))
		mux.HandleFunc("PUT /"+version+"/escalation_paths/{id}", fake.handle(version, fake.update))
		mux.HandleFunc("DELETE /"+version+"/escalation_paths/{id}", fake.handle(version, fake.destroy))
		mux.HandleFunc("POST /"+version+"/escalation_paths/actions/validate", fake.handle(version, fake.validate))
	}
	mux.HandleFunc("POST /v2/managed_resources", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	t.Setenv("INCIDENT_ENDPOINT", server.URL)
	t.Setenv("INCIDENT_API_KEY", "test-key")

	return fake
}

type fakeEscalationPathHandler func(w http.ResponseWriter, r *http.Request, version string)

func (f *fakeEscalationPathsAPI) handle(version string, handler fakeEscalationPathHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		call := r.Method + " /" + version + "/escalation_paths"
		switch {
		case strings.HasSuffix(r.URL.Path, "/actions/validate"):
			call += "/actions/validate"
		case r.PathValue("id") != "":
			call += "/{id}"
		}
		f.calls = append(f.calls, call)

		handler(w, r, version)
	}
}

// takeCalls returns the calls so far, and forgets them.
func (f *fakeEscalationPathsAPI) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	calls := f.calls
	f.calls = nil
	return calls
}

// setConditionSubject rewrites every branch of a stored path to test subject against ids,
// as saving the path in the dashboard would.
func (f *fakeEscalationPathsAPI) setConditionSubject(id, subject string, ids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	walkFakeNodes(f.paths[id]["path"], func(node map[string]any) {
		ifElse, ok := node["if_else"].(map[string]any)
		if !ok {
			return
		}
		values := []any{}
		for _, id := range ids {
			values = append(values, map[string]any{"literal": id})
		}
		ifElse["conditions"] = []any{map[string]any{
			"subject":        subject,
			"operation":      "one_of",
			"param_bindings": []any{map[string]any{"array_value": values}},
		}}
	})
}

func (f *fakeEscalationPathsAPI) create(w http.ResponseWriter, r *http.Request, version string) {
	body, ok := f.readPath(w, r, version)
	if !ok {
		return
	}

	f.nextID++
	id := fmt.Sprintf("01FAKEPATH%016d", f.nextID)
	body["id"] = id
	f.paths[id] = body

	f.writePath(w, http.StatusCreated, version, body)
}

func (f *fakeEscalationPathsAPI) show(w http.ResponseWriter, r *http.Request, version string) {
	stored, ok := f.paths[r.PathValue("id")]
	if !ok {
		writeFakeError(w, http.StatusNotFound, "not_found", "escalation path not found")
		return
	}
	if version == "v2" && fakeConditionsTest(stored, escalationUrgencySubject) {
		writeFakeError(w, http.StatusConflict, "conflict", "this escalation path branches on urgency: read it through V3")
		return
	}

	f.writePath(w, http.StatusOK, version, stored)
}

func (f *fakeEscalationPathsAPI) update(w http.ResponseWriter, r *http.Request, version string) {
	id := r.PathValue("id")
	stored, ok := f.paths[id]
	if !ok {
		writeFakeError(w, http.StatusNotFound, "not_found", "escalation path not found")
		return
	}
	if version == "v2" && fakeConditionsTest(stored, escalationUrgencySubject) {
		writeFakeError(w, http.StatusConflict, "conflict", "this escalation path branches on urgency: update it through V3")
		return
	}

	body, ok := f.readPath(w, r, version)
	if !ok {
		return
	}
	body["id"] = id
	f.paths[id] = body

	f.writePath(w, http.StatusOK, version, body)
}

func (f *fakeEscalationPathsAPI) destroy(w http.ResponseWriter, r *http.Request, _ string) {
	delete(f.paths, r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeEscalationPathsAPI) validate(w http.ResponseWriter, r *http.Request, version string) {
	if _, ok := f.readPath(w, r, version); !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"warnings":[]}`))
}

// readPath decodes a write, refusing what the version refuses, and stores each target's
// high/low setting under a key neither version uses.
func (f *fakeEscalationPathsAPI) readPath(w http.ResponseWriter, r *http.Request, version string) (map[string]any, bool) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Errorf("decoding %s %s: %v", r.Method, r.URL.Path, err)
		writeFakeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return nil, false
	}

	refused, settingKey := escalationPrioritySubject, "interruption_level"
	if version == "v2" {
		refused, settingKey = escalationUrgencySubject, "urgency"
	}
	if fakeConditionsTest(body, refused) {
		writeFakeError(w, http.StatusUnprocessableEntity, "invalid_value", fmt.Sprintf("%s can't branch on %s", version, refused))
		return nil, false
	}

	var missing bool
	walkFakeTargets(body["path"], func(target map[string]any) {
		setting, ok := target[settingKey]
		if !ok || setting == "" {
			missing = true
		}
		delete(target, "urgency")
		delete(target, "interruption_level")
		target["setting"] = setting
	})
	if missing {
		writeFakeError(w, http.StatusUnprocessableEntity, "invalid_value", fmt.Sprintf("every target needs %s", settingKey))
		return nil, false
	}

	return body, true
}

// writePath renders a stored path in the shape the version reads it in.
func (f *fakeEscalationPathsAPI) writePath(w http.ResponseWriter, status int, version string, stored map[string]any) {
	path := deepCopyFake(stored)

	settingKey := "interruption_level"
	if version == "v2" {
		settingKey = "urgency"
	}
	walkFakeTargets(path["path"], func(target map[string]any) {
		target[settingKey] = target["setting"]
		delete(target, "setting")
	})

	walkFakeNodes(path["path"], func(node map[string]any) {
		ifElse, ok := node["if_else"].(map[string]any)
		if !ok {
			return
		}
		conditions := fakeObjects(ifElse["conditions"])
		rendered := make([]any, 0, len(conditions))
		for _, condition := range conditions {
			subject, _ := condition["subject"].(string)
			if version == "v3" && subject == escalationPrioritySubject {
				subject = escalationUrgencySubject
				walkFakeLiterals(condition["param_bindings"], func(literal string) string {
					return fakePriorityUrgencies[literal]
				})
			}
			rendered = append(rendered, map[string]any{
				"subject":        map[string]any{"reference": subject, "label": subject},
				"operation":      map[string]any{"value": condition["operation"], "label": condition["operation"]},
				"param_bindings": condition["param_bindings"],
			})
		}
		ifElse["conditions"] = rendered
		if _, ok := ifElse["else_path"]; !ok {
			ifElse["else_path"] = []any{}
		}
	})

	if _, ok := path["kind"]; !ok {
		path["kind"] = "standalone"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"escalation_path": path})
}

func writeFakeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   code,
		"status": status,
		"errors": []any{map[string]any{"code": code, "message": message}},
	})
}

// fakeConditionsTest reports whether any branch of a path tests subject.
func fakeConditionsTest(path map[string]any, subject string) bool {
	var found bool
	walkFakeNodes(path["path"], func(node map[string]any) {
		ifElse, ok := node["if_else"].(map[string]any)
		if !ok {
			return
		}
		for _, condition := range fakeObjects(ifElse["conditions"]) {
			if condition["subject"] == subject {
				found = true
			}
		}
	})
	return found
}

// fakeObjects returns the JSON objects in a decoded array, skipping anything else.
func fakeObjects(value any) []map[string]any {
	list, _ := value.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, raw := range list {
		if object, ok := raw.(map[string]any); ok {
			out = append(out, object)
		}
	}
	return out
}

func walkFakeNodes(nodes any, visit func(map[string]any)) {
	for _, node := range fakeObjects(nodes) {
		visit(node)
		if ifElse, ok := node["if_else"].(map[string]any); ok {
			walkFakeNodes(ifElse["then_path"], visit)
			walkFakeNodes(ifElse["else_path"], visit)
		}
	}
}

func walkFakeTargets(nodes any, visit func(map[string]any)) {
	walkFakeNodes(nodes, func(node map[string]any) {
		for _, block := range []string{"level", "notify_channel"} {
			holder, ok := node[block].(map[string]any)
			if !ok {
				continue
			}
			for _, target := range fakeObjects(holder["targets"]) {
				visit(target)
			}
		}
	})
}

func walkFakeLiterals(bindings any, rewrite func(string) string) {
	for _, binding := range fakeObjects(bindings) {
		for _, entry := range fakeObjects(binding["array_value"]) {
			if literal, ok := entry["literal"].(string); ok {
				entry["literal"] = rewrite(literal)
			}
		}
	}
}

func deepCopyFake(in map[string]any) map[string]any {
	raw, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

// escalationPathVersionConfig is a path that pages a schedule, branches, and pages it
// again down the branch. targetSetting and branchIf are the version-specific lines.
func escalationPathVersionConfig(targetSetting, branchIf string) string {
	return fmt.Sprintf(`
resource "incident_escalation_path" "test" {
  name  = "Platform"
  start = "main"

  sequences = {
    main = {
      nodes = [
        { level = { targets = [{ id = "01SCHEDULE", type = "schedule", %[1]s }], time_to_ack_seconds = 300 } },
        { branch = { if = { %[2]s }, then = "urgent" } },
      ]
    }
    urgent = {
      nodes = [
        { level = { targets = [{ id = "01SCHEDULE", type = "schedule", %[1]s }], time_to_ack_seconds = 300 } },
      ]
    }
  }
}
`, targetSetting, branchIf)
}

var (
	escalationPathV2Config = escalationPathVersionConfig(`urgency = "high"`, `priority_one_of = ["01P1"]`)
	escalationPathV3Config = escalationPathVersionConfig(`interruption_level = "high"`, `urgency_one_of = ["01HIGH"]`)
)

// checkCalledOnly checks every call since the last check went to one version.
func checkCalledOnly(fake *fakeEscalationPathsAPI, version string, want ...string) resource.TestCheckFunc {
	return checkCalls(fake, version, false, want...)
}

// checkWroteOnly checks every write since the last check went to one version. The refresh
// before a migration reads through the version the state was written with, so reads
// aren't checked.
func checkWroteOnly(fake *fakeEscalationPathsAPI, version string, want ...string) resource.TestCheckFunc {
	return checkCalls(fake, version, true, want...)
}

func checkCalls(fake *fakeEscalationPathsAPI, version string, writesOnly bool, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		calls := fake.takeCalls()
		for _, call := range calls {
			if writesOnly && strings.HasPrefix(call, "GET ") {
				continue
			}
			if !strings.Contains(call, "/"+version+"/") {
				return fmt.Errorf("got call %q, want only %s calls: %v", call, version, calls)
			}
		}
		for _, call := range want {
			found := false
			for _, got := range calls {
				if got == call {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("no call %q in %v", call, calls)
			}
		}
		return nil
	}
}

func TestEscalationPathAPIVersionFakeAPI(t *testing.T) {
	const address = "incident_escalation_path.test"
	firstTarget := `sequences.main.nodes.0.level.targets.0`

	t.Run("a V2 config stays on V2, and moves to V3 in place", func(t *testing.T) {
		fake := newFakeEscalationPathsAPI(t)

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: escalationPathV2Config,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr(address, firstTarget+".urgency", "high"),
						resource.TestCheckNoResourceAttr(address, firstTarget+".interruption_level"),
						resource.TestCheckResourceAttr(address, "sequences.main.nodes.1.branch.if.priority_one_of.0", "01P1"),
						checkCalledOnly(fake, "v2", "POST /v2/escalation_paths", "POST /v2/escalation_paths/actions/validate"),
					),
				},
				{
					// The config change is the migration, applied as an update through V3.
					Config: escalationPathV3Config,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr(address, firstTarget+".interruption_level", "high"),
						resource.TestCheckNoResourceAttr(address, firstTarget+".urgency"),
						resource.TestCheckResourceAttr(address, "sequences.main.nodes.1.branch.if.urgency_one_of.0", "01HIGH"),
						checkWroteOnly(fake, "v3", "PUT /v3/escalation_paths/{id}", "POST /v3/escalation_paths/actions/validate"),
					),
				},
			},
		})
	})

	t.Run("a config without V2 fields goes through V3", func(t *testing.T) {
		fake := newFakeEscalationPathsAPI(t)

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config: escalationPathV3Config,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr(address, firstTarget+".interruption_level", "high"),
						checkCalledOnly(fake, "v3", "POST /v3/escalation_paths", "POST /v3/escalation_paths/actions/validate"),
					),
				},
			},
		})
	})

	t.Run("an import reads through V3", func(t *testing.T) {
		fake := newFakeEscalationPathsAPI(t)

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{Config: escalationPathV2Config},
				{
					// The path was written with priority conditions, which V3 shows as their
					// default urgencies: a V2 config then plans a one-time update.
					Config:        escalationPathV2Config,
					ResourceName:  address,
					ImportState:   true,
					ImportStateId: "01FAKEPATH0000000000000001",
					ImportStateCheck: func(states []*terraform.InstanceState) error {
						if len(states) != 1 {
							return fmt.Errorf("got %d states, want 1", len(states))
						}
						attrs := states[0].Attributes
						if got := attrs[firstTarget+".interruption_level"]; got != "high" {
							return fmt.Errorf("got interruption_level %q, want high", got)
						}
						if got := attrs["sequences.main.nodes.1.branch.if.urgency_one_of.0"]; got != "01HIGH" {
							return fmt.Errorf("got urgency_one_of %q, want 01HIGH", got)
						}
						for _, call := range fake.takeCalls() {
							if call == "GET /v3/escalation_paths/{id}" {
								return nil
							}
						}
						return fmt.Errorf("the import never read through V3")
					},
				},
			},
		})
	})

	t.Run("a V2 path saved with urgency conditions is read through V3", func(t *testing.T) {
		fake := newFakeEscalationPathsAPI(t)

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{Config: escalationPathV2Config},
				{
					// Someone saves the path in the dashboard, which writes urgency conditions
					// V2 can no longer show. The refresh still succeeds, and the V2 config
					// plans the change back.
					PreConfig: func() {
						fake.setConditionSubject("01FAKEPATH0000000000000001", escalationUrgencySubject, "01HIGH")
					},
					Config:             escalationPathV2Config,
					PlanOnly:           true,
					ExpectNonEmptyPlan: true,
				},
				{
					// V2 can't write it back, and the error says how to move to V3.
					Config:      escalationPathV2Config,
					ExpectError: regexp.MustCompile(`Only the V3 API can update it`),
				},
				{
					Config: escalationPathV3Config,
					Check:  resource.TestCheckResourceAttr(address, "sequences.main.nodes.1.branch.if.urgency_one_of.0", "01HIGH"),
				},
			},
		})
	})

	t.Run("a config mixing V2 and V3 fields is refused", func(t *testing.T) {
		newFakeEscalationPathsAPI(t)

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{{
				Config:      escalationPathVersionConfig(`interruption_level = "high"`, `priority_one_of = ["01P1"]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Escalation path mixes V2 and V3 fields`),
			}},
		})
	})

	t.Run("the data source reads a path branching on urgency", func(t *testing.T) {
		newFakeEscalationPathsAPI(t)

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{Config: escalationPathV3Config},
				{
					Config: escalationPathV3Config + `
data "incident_escalation_path" "test" {
  id = incident_escalation_path.test.id
}
`,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("data.incident_escalation_path.test", firstTarget+".interruption_level", "high"),
						resource.TestCheckResourceAttr("data.incident_escalation_path.test", "sequences.main.nodes.1.branch.if.urgency_one_of.0", "01HIGH"),
					),
				},
			},
		})
	})

	t.Run("the data source reads a path branching on priority as before", func(t *testing.T) {
		newFakeEscalationPathsAPI(t)

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{Config: escalationPathV2Config},
				{
					Config: escalationPathV2Config + `
data "incident_escalation_path" "test" {
  id = incident_escalation_path.test.id
}
`,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttr("data.incident_escalation_path.test", firstTarget+".urgency", "high"),
						resource.TestCheckResourceAttr("data.incident_escalation_path.test", "sequences.main.nodes.1.branch.if.priority_one_of.0", "01P1"),
					),
				},
			},
		})
	})
}
