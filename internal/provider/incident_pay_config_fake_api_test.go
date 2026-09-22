package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// fakePayConfigsAPI is an in-memory stand-in for the pay configs endpoints, close enough
// to the real ones for the resource to be driven through Terraform without an account:
// it keeps rules in the order they were added, appends a created rule, rejects a one-off
// rule that overlaps another on every write, reports times in UTC, and pages the list.
//
// It records every request, so a test can say which writes a change should and should
// not have made: the whole point of reconciling rule by rule is that a change to one rule
// touches nothing else.
type fakePayConfigsAPI struct {
	t *testing.T

	mu       sync.Mutex
	configs  []client.PayConfigV2
	requests []string
	nextID   int

	// pageSize caps the page the list endpoint serves, whatever page_size asks for, so a
	// test can make the data source walk the cursor with a handful of configs.
	pageSize int
}

func startFakePayConfigsAPI(t *testing.T) (*fakePayConfigsAPI, string) {
	t.Helper()

	fake := &fakePayConfigsAPI{t: t, pageSize: 250}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/pay_configs", fake.list)
	mux.HandleFunc("POST /v2/pay_configs", fake.create)
	mux.HandleFunc("GET /v2/pay_configs/{id}", fake.show)
	mux.HandleFunc("PUT /v2/pay_configs/{id}", fake.update)
	mux.HandleFunc("DELETE /v2/pay_configs/{id}", fake.destroy)
	mux.HandleFunc("POST /v2/pay_configs/{id}/weekly_rules", fake.createWeeklyRule)
	mux.HandleFunc("PUT /v2/pay_configs/{id}/weekly_rules/{rule}", fake.updateWeeklyRule)
	mux.HandleFunc("DELETE /v2/pay_configs/{id}/weekly_rules/{rule}", fake.destroyWeeklyRule)
	mux.HandleFunc("POST /v2/pay_configs/{id}/one_off_rules", fake.createOneOffRule)
	mux.HandleFunc("PUT /v2/pay_configs/{id}/one_off_rules/{rule}", fake.updateOneOffRule)
	mux.HandleFunc("DELETE /v2/pay_configs/{id}/one_off_rules/{rule}", fake.destroyOneOffRule)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.requests = append(fake.requests, r.Method+" "+r.URL.Path)
		fake.mu.Unlock()

		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	return fake, server.URL
}

// seed adds a config directly, for a data source test that wants something to find.
func (f *fakePayConfigsAPI) seed(name string) client.PayConfigV2 {
	f.mu.Lock()
	defer f.mu.Unlock()

	config := client.PayConfigV2{
		Id:            f.id(),
		Name:          name,
		Timezone:      "Europe/London",
		Currency:      "GBP",
		BaseRateCents: 100,
		RateTimeUnit:  "hour",
		WeeklyRules:   []client.PayConfigWeeklyRuleV2{},
		OneOffRules:   []client.PayConfigOneOffRuleV2{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	f.configs = append(f.configs, config)

	return config
}

// writes returns the requests that changed something since the last call, and forgets
// them. Reads are left out: how many times Terraform refreshes is its business.
func (f *fakePayConfigsAPI) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	writes := lo.Filter(f.requests, func(request string, _ int) bool {
		return !strings.HasPrefix(request, "GET ")
	})
	f.requests = nil

	return writes
}

// listCalls counts the list requests so far, and forgets every request.
func (f *fakePayConfigsAPI) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	count := lo.CountBy(f.requests, func(request string) bool {
		return request == "GET /v2/pay_configs"
	})
	f.requests = nil

	return count
}

// config returns a copy of a config by ID, for a test to inspect what was stored.
func (f *fakePayConfigsAPI) config(id string) (client.PayConfigV2, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(id)
	if idx < 0 {
		return client.PayConfigV2{}, false
	}

	return f.configs[idx], true
}

// id mints IDs that sort in creation order, as ULIDs do.
func (f *fakePayConfigsAPI) id() string {
	f.nextID++

	return fmt.Sprintf("01FAKE%020d", f.nextID)
}

func (f *fakePayConfigsAPI) find(id string) int {
	return lo.IndexOf(lo.Map(f.configs, func(config client.PayConfigV2, _ int) string { return config.Id }), id)
}

func (f *fakePayConfigsAPI) list(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	pageSize := f.pageSize
	if requested, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && requested < pageSize {
		pageSize = requested
	}

	// Newest first, as the real endpoint orders by ID descending.
	ordered := append([]client.PayConfigV2{}, f.configs...)
	slices.Reverse(ordered)
	if after := r.URL.Query().Get("after"); after != "" {
		ordered = lo.Filter(ordered, func(config client.PayConfigV2, _ int) bool {
			return config.Id < after
		})
	}

	page := ordered[:min(pageSize, len(ordered))]
	meta := client.PaginationMetaResultV2{PageSize: int64(pageSize)}
	// The real endpoint offers a cursor whenever the page filled, whether or not another
	// page exists.
	if len(page) == pageSize && len(page) > 0 {
		meta.After = lo.ToPtr(page[len(page)-1].Id)
	}

	writeJSON(f.t, w, client.PayConfigsListResultV2{PayConfigs: page, PaginationMeta: meta})
}

func (f *fakePayConfigsAPI) create(w http.ResponseWriter, r *http.Request) {
	var payload client.PayConfigsCreatePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	config := client.PayConfigV2{
		Id:            f.id(),
		Name:          payload.Name,
		Timezone:      payload.Timezone,
		Currency:      payload.Currency,
		BaseRateCents: payload.BaseRateCents,
		RateTimeUnit:  client.PayConfigV2RateTimeUnit(payload.RateTimeUnit),
		WeeklyRules:   []client.PayConfigWeeklyRuleV2{},
		OneOffRules:   []client.PayConfigOneOffRuleV2{},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if payload.WeeklyRules != nil {
		for _, rule := range *payload.WeeklyRules {
			config.WeeklyRules = append(config.WeeklyRules, client.PayConfigWeeklyRuleV2{
				Id: f.id(),
				Weekdays: lo.Map(rule.Weekdays, func(day client.PayConfigWeeklyRulePayloadV2Weekdays, _ int) client.PayConfigWeeklyRuleV2Weekdays {
					return client.PayConfigWeeklyRuleV2Weekdays(day)
				}),
				StartTime: rule.StartTime,
				EndTime:   rule.EndTime,
				RateCents: rule.RateCents,
			})
		}
	}
	if payload.OneOffRules != nil {
		for _, rule := range *payload.OneOffRules {
			config.OneOffRules = append(config.OneOffRules, client.PayConfigOneOffRuleV2{
				Id:        f.id(),
				Name:      rule.Name,
				StartAt:   rule.StartAt.UTC(),
				EndAt:     rule.EndAt.UTC(),
				RateCents: rule.RateCents,
			})
		}
	}

	if !f.validOneOffRules(w, config.OneOffRules) {
		return
	}

	f.configs = append(f.configs, config)

	writeJSONStatus(f.t, w, http.StatusCreated, client.PayConfigsCreateResultV2{PayConfig: config})
}

func (f *fakePayConfigsAPI) show(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	writeJSON(f.t, w, client.PayConfigsShowResultV2{PayConfig: f.configs[idx]})
}

func (f *fakePayConfigsAPI) update(w http.ResponseWriter, r *http.Request) {
	var payload client.PayConfigsUpdatePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	config := &f.configs[idx]
	config.Name = payload.Name
	config.Timezone = payload.Timezone
	config.Currency = payload.Currency
	config.BaseRateCents = payload.BaseRateCents
	config.RateTimeUnit = client.PayConfigV2RateTimeUnit(payload.RateTimeUnit)
	config.UpdatedAt = time.Now().UTC()

	writeJSON(f.t, w, client.PayConfigsUpdateResultV2{PayConfig: *config})
}

func (f *fakePayConfigsAPI) destroy(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	f.configs = append(f.configs[:idx], f.configs[idx+1:]...)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakePayConfigsAPI) createWeeklyRule(w http.ResponseWriter, r *http.Request) {
	var payload client.PayConfigsCreateWeeklyRulePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	rule := client.PayConfigWeeklyRuleV2{
		Id: f.id(),
		Weekdays: lo.Map(payload.Weekdays, func(day client.PayConfigsCreateWeeklyRulePayloadV2Weekdays, _ int) client.PayConfigWeeklyRuleV2Weekdays {
			return client.PayConfigWeeklyRuleV2Weekdays(day)
		}),
		StartTime: payload.StartTime,
		EndTime:   payload.EndTime,
		RateCents: payload.RateCents,
	}
	f.configs[idx].WeeklyRules = append(f.configs[idx].WeeklyRules, rule)
	f.configs[idx].UpdatedAt = time.Now().UTC()

	writeJSONStatus(f.t, w, http.StatusCreated, client.PayConfigsCreateWeeklyRuleResultV2{WeeklyRule: rule})
}

func (f *fakePayConfigsAPI) updateWeeklyRule(w http.ResponseWriter, r *http.Request) {
	var payload client.PayConfigsUpdateWeeklyRulePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	rules := f.configs[idx].WeeklyRules
	ruleIdx := lo.IndexOf(lo.Map(rules, func(rule client.PayConfigWeeklyRuleV2, _ int) string { return rule.Id }), r.PathValue("rule"))
	if ruleIdx < 0 {
		writeNotFound(w, "weekly rule")
		return
	}

	rule := &rules[ruleIdx]
	rule.Weekdays = lo.Map(payload.Weekdays, func(day client.PayConfigsUpdateWeeklyRulePayloadV2Weekdays, _ int) client.PayConfigWeeklyRuleV2Weekdays {
		return client.PayConfigWeeklyRuleV2Weekdays(day)
	})
	rule.StartTime = payload.StartTime
	rule.EndTime = payload.EndTime
	rule.RateCents = payload.RateCents
	f.configs[idx].UpdatedAt = time.Now().UTC()

	writeJSON(f.t, w, client.PayConfigsUpdateWeeklyRuleResultV2{WeeklyRule: *rule})
}

func (f *fakePayConfigsAPI) destroyWeeklyRule(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	before := len(f.configs[idx].WeeklyRules)
	f.configs[idx].WeeklyRules = lo.Filter(f.configs[idx].WeeklyRules, func(rule client.PayConfigWeeklyRuleV2, _ int) bool {
		return rule.Id != r.PathValue("rule")
	})
	if len(f.configs[idx].WeeklyRules) == before {
		writeNotFound(w, "weekly rule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (f *fakePayConfigsAPI) createOneOffRule(w http.ResponseWriter, r *http.Request) {
	var payload client.PayConfigsCreateOneOffRulePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	rule := client.PayConfigOneOffRuleV2{
		Id:        f.id(),
		Name:      payload.Name,
		StartAt:   payload.StartAt.UTC(),
		EndAt:     payload.EndAt.UTC(),
		RateCents: payload.RateCents,
	}
	rules := append(append([]client.PayConfigOneOffRuleV2{}, f.configs[idx].OneOffRules...), rule)
	if !f.validOneOffRules(w, rules) {
		return
	}

	f.configs[idx].OneOffRules = rules
	f.configs[idx].UpdatedAt = time.Now().UTC()

	writeJSONStatus(f.t, w, http.StatusCreated, client.PayConfigsCreateOneOffRuleResultV2{OneOffRule: rule})
}

func (f *fakePayConfigsAPI) updateOneOffRule(w http.ResponseWriter, r *http.Request) {
	var payload client.PayConfigsUpdateOneOffRulePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	rules := append([]client.PayConfigOneOffRuleV2{}, f.configs[idx].OneOffRules...)
	ruleIdx := lo.IndexOf(lo.Map(rules, func(rule client.PayConfigOneOffRuleV2, _ int) string { return rule.Id }), r.PathValue("rule"))
	if ruleIdx < 0 {
		writeNotFound(w, "one-off rule")
		return
	}

	rules[ruleIdx].Name = payload.Name
	rules[ruleIdx].StartAt = payload.StartAt.UTC()
	rules[ruleIdx].EndAt = payload.EndAt.UTC()
	rules[ruleIdx].RateCents = payload.RateCents
	if !f.validOneOffRules(w, rules) {
		return
	}

	f.configs[idx].OneOffRules = rules
	f.configs[idx].UpdatedAt = time.Now().UTC()

	writeJSON(f.t, w, client.PayConfigsUpdateOneOffRuleResultV2{OneOffRule: rules[ruleIdx]})
}

func (f *fakePayConfigsAPI) destroyOneOffRule(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		writeNotFound(w, "pay config")
		return
	}

	before := len(f.configs[idx].OneOffRules)
	f.configs[idx].OneOffRules = lo.Filter(f.configs[idx].OneOffRules, func(rule client.PayConfigOneOffRuleV2, _ int) bool {
		return rule.Id != r.PathValue("rule")
	})
	if len(f.configs[idx].OneOffRules) == before {
		writeNotFound(w, "one-off rule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// validOneOffRules applies the real API's overlap check, which runs on every write to a
// config: a rule overlaps another when it strictly contains either end of it.
func (f *fakePayConfigsAPI) validOneOffRules(w http.ResponseWriter, rules []client.PayConfigOneOffRuleV2) bool {
	for i, this := range rules {
		for j, that := range rules {
			if i == j {
				continue
			}

			containsStart := this.StartAt.Before(that.StartAt) && this.EndAt.After(that.StartAt)
			containsEnd := this.StartAt.Before(that.EndAt) && this.EndAt.After(that.EndAt)
			if containsStart || containsEnd {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = fmt.Fprintf(w, `{"type":"validation_error","status":422,"detail":"rule %q overlaps with %q"}`, this.Name, that.Name)

				return false
			}
		}
	}

	return true
}

// writeJSONStatus is writeJSON with a status other than 200. The content type has to be
// set before the status line goes out, or the client sees text/plain and refuses it.
func writeJSONStatus(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"type":"bad_request","status":400,"detail":%q}`, err.Error())

		return false
	}

	return true
}

func writeNotFound(w http.ResponseWriter, what string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = fmt.Fprintf(w, `{"type":"not_found","status":404,"detail":"could not find %s"}`, what)
}
