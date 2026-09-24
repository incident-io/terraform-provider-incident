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

// fakeAnnouncementsAPI is an in-memory stand-in for the announcement rules and templates
// endpoints, close enough to the real ones for both resources to be driven through
// Terraform without an account. Like the real API it:
//
//   - seeds a default template, which rules use when they don't name one, and which can't
//     be deleted
//   - fills in a rule's defaults (private_incident_scope none, leave_in_place)
//   - keeps a rule's optional fields when an update omits them
//   - returns condition groups in the read shape, with labels
//   - stores template fields in the order written, reads an empty emoji back as absent,
//     and derives each field's title from its type
//   - refuses to delete a template a rule still uses
type fakeAnnouncementsAPI struct {
	t *testing.T

	mu        sync.Mutex
	rules     []client.AnnouncementRuleV2
	templates []client.AnnouncementTemplateV2
	requests  []string
	nextID    int

	// pageSize caps the page the rules list serves, so a test can make the data source walk
	// the cursor with a handful of rules.
	pageSize int
}

func startFakeAnnouncementsAPI(t *testing.T) (*fakeAnnouncementsAPI, string) {
	t.Helper()

	fake := &fakeAnnouncementsAPI{t: t, pageSize: 250}
	fake.templates = append(fake.templates, client.AnnouncementTemplateV2{
		Id:            fake.id(),
		Name:          "Default",
		IsDefault:     true,
		Fields:        []client.AnnouncementTemplateFieldV2{},
		Actions:       []client.AnnouncementTemplateActionV2{},
		OwningTeamIds: []string{},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/announcement_rules", fake.listRules)
	mux.HandleFunc("POST /v2/announcement_rules", fake.createRule)
	mux.HandleFunc("GET /v2/announcement_rules/{id}", fake.showRule)
	mux.HandleFunc("PUT /v2/announcement_rules/{id}", fake.updateRule)
	mux.HandleFunc("DELETE /v2/announcement_rules/{id}", fake.destroyRule)
	mux.HandleFunc("GET /v2/announcement_templates", fake.listTemplates)
	mux.HandleFunc("POST /v2/announcement_templates", fake.createTemplate)
	mux.HandleFunc("GET /v2/announcement_templates/{id}", fake.showTemplate)
	mux.HandleFunc("PUT /v2/announcement_templates/{id}", fake.updateTemplate)
	mux.HandleFunc("DELETE /v2/announcement_templates/{id}", fake.destroyTemplate)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.requests = append(fake.requests, r.Method+" "+r.URL.Path)
		fake.mu.Unlock()

		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	return fake, server.URL
}

// defaultTemplate returns the seeded default template.
func (f *fakeAnnouncementsAPI) defaultTemplate() client.AnnouncementTemplateV2 {
	f.mu.Lock()
	defer f.mu.Unlock()

	template, _ := lo.Find(f.templates, func(template client.AnnouncementTemplateV2) bool { return template.IsDefault })

	return template
}

// seedRule adds a rule directly, for a data source test that wants something to find.
func (f *fakeAnnouncementsAPI) seedRule(name string) client.AnnouncementRuleV2 {
	f.mu.Lock()
	defer f.mu.Unlock()

	rule := client.AnnouncementRuleV2{
		Id:                               f.id(),
		Name:                             name,
		ConditionGroups:                  []client.ConditionGroupV2{},
		SlackChannelIds:                  []string{"C0SEEDED"},
		MicrosoftTeamsChannelIds:         []string{},
		UpdateSharingMode:                "thread",
		Mode:                             "include_triage",
		PrivateIncidentScope:             lo.ToPtr(client.AnnouncementRuleV2PrivateIncidentScope("none")),
		ConditionsNoLongerApplyBehaviour: "leave_in_place",
		TemplateId:                       lo.ToPtr(f.templates[0].Id),
		OwningTeamIds:                    []string{},
		CreatedAt:                        time.Now().UTC(),
		UpdatedAt:                        time.Now().UTC(),
	}
	f.rules = append(f.rules, rule)

	return rule
}

// listCalls counts the rule list requests so far, and forgets every request.
func (f *fakeAnnouncementsAPI) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	count := lo.CountBy(f.requests, func(request string) bool { return request == "GET /v2/announcement_rules" })
	f.requests = nil

	return count
}

// id mints IDs that sort in creation order, as ULIDs do.
func (f *fakeAnnouncementsAPI) id() string {
	f.nextID++

	return fmt.Sprintf("01FAKE%020d", f.nextID)
}

func (f *fakeAnnouncementsAPI) findRule(id string) int {
	return slices.IndexFunc(f.rules, func(rule client.AnnouncementRuleV2) bool { return rule.Id == id })
}

func (f *fakeAnnouncementsAPI) findTemplate(id string) int {
	return slices.IndexFunc(f.templates, func(template client.AnnouncementTemplateV2) bool { return template.Id == id })
}

func (f *fakeAnnouncementsAPI) listRules(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	pageSize := f.pageSize
	if requested, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && requested < pageSize {
		pageSize = requested
	}

	// Newest first, as the real endpoint orders by creation time descending.
	ordered := slices.Clone(f.rules)
	slices.Reverse(ordered)
	if after := r.URL.Query().Get("after"); after != "" {
		ordered = lo.Filter(ordered, func(rule client.AnnouncementRuleV2, _ int) bool { return rule.Id < after })
	}

	page := ordered[:min(pageSize, len(ordered))]
	meta := client.PaginationMetaResultV2{PageSize: int64(pageSize)}
	// The real endpoint offers a cursor whenever the page filled, whether or not another
	// page exists.
	if len(page) == pageSize && len(page) > 0 {
		meta.After = lo.ToPtr(page[len(page)-1].Id)
	}

	writeJSON(f.t, w, client.AnnouncementRulesListResultV2{AnnouncementRules: page, PaginationMeta: meta})
}

func (f *fakeAnnouncementsAPI) createRule(w http.ResponseWriter, r *http.Request) {
	var payload client.AnnouncementRulesCreatePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	templateID := lo.FromPtr(payload.TemplateId)
	if templateID == "" {
		templateID = f.templates[0].Id
	} else if f.findTemplate(templateID) < 0 {
		f.writeValidationError(w, "template_id", "Template not found")
		return
	}

	rule := client.AnnouncementRuleV2{
		Id:                               f.id(),
		Name:                             payload.Name,
		ConditionGroups:                  readConditionGroups(f.t, payload.ConditionGroups),
		SlackChannelIds:                  lo.CoalesceSliceOrEmpty(lo.FromPtr(payload.SlackChannelIds)),
		MicrosoftTeamsChannelIds:         lo.CoalesceSliceOrEmpty(lo.FromPtr(payload.MicrosoftTeamsChannelIds)),
		UpdateSharingMode:                client.AnnouncementRuleV2UpdateSharingMode(payload.UpdateSharingMode),
		Mode:                             client.AnnouncementRuleV2Mode(payload.Mode),
		PrivateIncidentScope:             lo.ToPtr(client.AnnouncementRuleV2PrivateIncidentScope(lo.CoalesceOrEmpty(string(lo.FromPtr(payload.PrivateIncidentScope)), "none"))),
		ConditionsNoLongerApplyBehaviour: client.AnnouncementRuleV2ConditionsNoLongerApplyBehaviour(lo.CoalesceOrEmpty(string(lo.FromPtr(payload.ConditionsNoLongerApplyBehaviour)), "leave_in_place")),
		TemplateId:                       lo.ToPtr(templateID),
		OwningTeamIds:                    lo.CoalesceSliceOrEmpty(lo.FromPtr(payload.OwningTeamIds)),
		CreatedAt:                        time.Now().UTC(),
		UpdatedAt:                        time.Now().UTC(),
	}
	f.rules = append(f.rules, rule)

	writeJSONStatus(f.t, w, http.StatusCreated, client.AnnouncementRulesCreateResultV2{AnnouncementRule: rule})
}

func (f *fakeAnnouncementsAPI) showRule(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	index := f.findRule(r.PathValue("id"))
	if index < 0 {
		writeNotFound(w, "announcement rule")
		return
	}

	writeJSON(f.t, w, client.AnnouncementRulesShowResultV2{AnnouncementRule: f.rules[index]})
}

func (f *fakeAnnouncementsAPI) updateRule(w http.ResponseWriter, r *http.Request) {
	var payload client.AnnouncementRulesUpdatePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	index := f.findRule(r.PathValue("id"))
	if index < 0 {
		writeNotFound(w, "announcement rule")
		return
	}

	rule := &f.rules[index]
	rule.Name = payload.Name
	rule.ConditionGroups = readConditionGroups(f.t, payload.ConditionGroups)
	rule.UpdateSharingMode = client.AnnouncementRuleV2UpdateSharingMode(payload.UpdateSharingMode)
	rule.Mode = client.AnnouncementRuleV2Mode(payload.Mode)
	// An omitted optional field is left as it was.
	if payload.SlackChannelIds != nil {
		rule.SlackChannelIds = lo.CoalesceSliceOrEmpty(*payload.SlackChannelIds)
	}
	if payload.MicrosoftTeamsChannelIds != nil {
		rule.MicrosoftTeamsChannelIds = lo.CoalesceSliceOrEmpty(*payload.MicrosoftTeamsChannelIds)
	}
	if payload.PrivateIncidentScope != nil {
		rule.PrivateIncidentScope = lo.ToPtr(client.AnnouncementRuleV2PrivateIncidentScope(*payload.PrivateIncidentScope))
	}
	if payload.ConditionsNoLongerApplyBehaviour != nil {
		rule.ConditionsNoLongerApplyBehaviour = client.AnnouncementRuleV2ConditionsNoLongerApplyBehaviour(*payload.ConditionsNoLongerApplyBehaviour)
	}
	if payload.TemplateId != nil {
		if f.findTemplate(*payload.TemplateId) < 0 {
			f.writeValidationError(w, "template_id", "Template not found")
			return
		}
		rule.TemplateId = payload.TemplateId
	}
	if payload.OwningTeamIds != nil {
		rule.OwningTeamIds = lo.CoalesceSliceOrEmpty(*payload.OwningTeamIds)
	}
	rule.UpdatedAt = time.Now().UTC()

	writeJSON(f.t, w, client.AnnouncementRulesUpdateResultV2{AnnouncementRule: *rule})
}

func (f *fakeAnnouncementsAPI) destroyRule(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	index := f.findRule(r.PathValue("id"))
	if index < 0 {
		writeNotFound(w, "announcement rule")
		return
	}
	f.rules = slices.Delete(f.rules, index, index+1)

	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeAnnouncementsAPI) listTemplates(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Ordered by name, as the real endpoint does.
	ordered := slices.Clone(f.templates)
	slices.SortFunc(ordered, func(a, b client.AnnouncementTemplateV2) int { return strings.Compare(a.Name, b.Name) })

	writeJSON(f.t, w, client.AnnouncementTemplatesListResultV2{AnnouncementTemplates: ordered})
}

func (f *fakeAnnouncementsAPI) createTemplate(w http.ResponseWriter, r *http.Request) {
	var payload client.AnnouncementTemplatesCreatePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.nameTaken(payload.Name, "") {
		f.writeValidationError(w, "name", "an announcement template with this name already exists")
		return
	}

	template := client.AnnouncementTemplateV2{
		Id:            f.id(),
		Name:          payload.Name,
		Fields:        readTemplateFields(lo.FromPtr(payload.Fields)),
		Actions:       readTemplateActions(lo.FromPtr(payload.Actions)),
		OwningTeamIds: lo.CoalesceSliceOrEmpty(lo.FromPtr(payload.OwningTeamIds)),
	}
	f.templates = append(f.templates, template)

	writeJSONStatus(f.t, w, http.StatusCreated, client.AnnouncementTemplatesCreateResultV2{AnnouncementTemplate: template})
}

func (f *fakeAnnouncementsAPI) showTemplate(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	index := f.findTemplate(r.PathValue("id"))
	if index < 0 {
		writeNotFound(w, "announcement template")
		return
	}

	writeJSON(f.t, w, client.AnnouncementTemplatesShowResultV2{AnnouncementTemplate: f.templates[index]})
}

func (f *fakeAnnouncementsAPI) updateTemplate(w http.ResponseWriter, r *http.Request) {
	var payload client.AnnouncementTemplatesUpdatePayloadV2
	if !decodeJSON(w, r, &payload) {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	index := f.findTemplate(r.PathValue("id"))
	if index < 0 {
		writeNotFound(w, "announcement template")
		return
	}
	if f.nameTaken(payload.Name, f.templates[index].Id) {
		f.writeValidationError(w, "name", "an announcement template with this name already exists")
		return
	}

	template := &f.templates[index]
	template.Name = payload.Name
	template.Fields = readTemplateFields(payload.Fields)
	template.Actions = readTemplateActions(payload.Actions)
	if payload.OwningTeamIds != nil {
		template.OwningTeamIds = lo.CoalesceSliceOrEmpty(*payload.OwningTeamIds)
	}

	writeJSON(f.t, w, client.AnnouncementTemplatesUpdateResultV2{AnnouncementTemplate: *template})
}

func (f *fakeAnnouncementsAPI) destroyTemplate(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := r.PathValue("id")
	index := f.findTemplate(id)
	if index < 0 {
		writeNotFound(w, "announcement template")
		return
	}
	if f.templates[index].IsDefault {
		f.writeValidationError(w, "id", "the default template cannot be deleted")
		return
	}
	if lo.ContainsBy(f.rules, func(rule client.AnnouncementRuleV2) bool { return lo.FromPtr(rule.TemplateId) == id }) {
		f.writeValidationError(w, "id", "Cannot delete this template as it is used by an announcement rule")
		return
	}
	f.templates = slices.Delete(f.templates, index, index+1)

	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeAnnouncementsAPI) nameTaken(name, excludeID string) bool {
	return lo.ContainsBy(f.templates, func(template client.AnnouncementTemplateV2) bool {
		return template.Name == name && template.Id != excludeID
	})
}

// readConditionGroups converts written condition groups into the read shape. The binding
// values are the same JSON with a label added, so a round trip through JSON does it.
func readConditionGroups(t *testing.T, groups []client.ConditionGroupPayloadV2) []client.ConditionGroupV2 {
	return lo.Map(groups, func(group client.ConditionGroupPayloadV2, _ int) client.ConditionGroupV2 {
		return client.ConditionGroupV2{
			Conditions: lo.Map(group.Conditions, func(condition client.ConditionPayloadV2, _ int) client.ConditionV2 {
				raw, err := json.Marshal(condition.ParamBindings)
				if err != nil {
					t.Fatalf("encoding param bindings: %v", err)
				}
				var bindings []client.EngineParamBindingV2
				if err := json.Unmarshal(raw, &bindings); err != nil {
					t.Fatalf("decoding param bindings: %v", err)
				}

				return client.ConditionV2{
					Subject:       client.ConditionSubjectV2{Label: condition.Subject, Reference: condition.Subject},
					Operation:     client.ConditionOperationV2{Label: condition.Operation, Value: condition.Operation},
					ParamBindings: lo.CoalesceSliceOrEmpty(bindings),
				}
			}),
		}
	})
}

func readTemplateFields(fields []client.AnnouncementTemplateFieldPayloadV2) []client.AnnouncementTemplateFieldV2 {
	return lo.Map(fields, func(field client.AnnouncementTemplateFieldPayloadV2, _ int) client.AnnouncementTemplateFieldV2 {
		return client.AnnouncementTemplateFieldV2{
			FieldType:           client.AnnouncementTemplateFieldV2FieldType(field.FieldType),
			Rank:                field.Rank,
			Title:               strings.TrimPrefix(string(field.FieldType), "announcement_post_fields_"),
			Emoji:               lo.EmptyableToPtr(lo.FromPtr(field.Emoji)),
			CustomFieldId:       field.CustomFieldId,
			IncidentRoleId:      field.IncidentRoleId,
			IncidentTimestampId: field.IncidentTimestampId,
			RichText:            field.RichText,
		}
	})
}

func readTemplateActions(actions []client.AnnouncementTemplateActionPayloadV2) []client.AnnouncementTemplateActionV2 {
	return lo.Map(actions, func(action client.AnnouncementTemplateActionPayloadV2, _ int) client.AnnouncementTemplateActionV2 {
		return client.AnnouncementTemplateActionV2{
			ActionType: client.AnnouncementTemplateActionV2ActionType(action.ActionType),
			Rank:       action.Rank,
			Title:      strings.TrimPrefix(string(action.ActionType), "announcement_post_actions_"),
			Emoji:      lo.EmptyableToPtr(lo.FromPtr(action.Emoji)),
		}
	})
}

// writeValidationError answers as the real API does for a payload that fails validation.
func (f *fakeAnnouncementsAPI) writeValidationError(w http.ResponseWriter, field, message string) {
	writeJSONStatus(f.t, w, http.StatusUnprocessableEntity, map[string]any{
		"type":   "validation_error",
		"status": http.StatusUnprocessableEntity,
		"errors": []map[string]any{{
			"code":    "invalid_value",
			"message": message,
			"source":  map[string]string{"field": field},
		}},
	})
}
