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

// fakeIncidentFormsAPI is an in-memory stand-in for the incident forms endpoints, close
// enough to the real ones for the resource to be driven through Terraform without an
// account. Like the real API it:
//
//   - seeds a default form of every lifecycle type, which can be neither created nor deleted
//   - rejects a second form for the same form type and incident type
//   - reconciles a whole-form write onto the stored elements: an element with a natural key
//     is matched on it, a divider or text element on its id, and an unmatched stored element
//     is archived
//   - ranks elements by list position, except name, incident_type and status, which always
//     come first
//   - fills in each element's defaults (required_if never_require, config, can_select_no_value)
//     and returns condition groups in the read shape, with labels
//   - answers validate as create or update would, plus a warning when a pinned element is
//     listed out of place
//   - records managed resource claims, so a test can see a form handed back
type fakeIncidentFormsAPI struct {
	t *testing.T

	mu       sync.Mutex
	forms    []client.IncidentFormV3
	claims   map[string]map[string]string
	requests []string
	nextID   int
}

func startFakeIncidentFormsAPI(t *testing.T) (*fakeIncidentFormsAPI, string) {
	t.Helper()

	fake := &fakeIncidentFormsAPI{t: t, claims: map[string]map[string]string{}}
	for _, formType := range []string{"declare", "accept", "update", "resolve", "retrospective"} {
		form := client.IncidentFormV3{
			Id:          fake.id(),
			FormType:    client.IncidentFormV3FormType(formType),
			Expressions: []client.ExpressionV3{},
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		elements := []client.IncidentFormLifecycleElementV3{}
		for _, elementType := range fakeDefaultElements(formType) {
			elements = append(elements, fake.readElement(client.IncidentFormLifecycleElementPayloadV3{
				ElementType: client.IncidentFormLifecycleElementPayloadV3ElementType(elementType),
			}, fake.id()))
		}
		form.LifecycleElements = &elements
		fake.forms = append(fake.forms, form)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v3/incident_forms", fake.list)
	mux.HandleFunc("POST /v3/incident_forms", fake.create)
	mux.HandleFunc("POST /v3/incident_forms/actions/validate", fake.validate)
	mux.HandleFunc("GET /v3/incident_forms/{id}", fake.show)
	mux.HandleFunc("PUT /v3/incident_forms/{id}", fake.update)
	mux.HandleFunc("DELETE /v3/incident_forms/{id}", fake.destroy)
	mux.HandleFunc("POST /v2/managed_resources", fake.claim)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.requests = append(fake.requests, r.Method+" "+r.URL.Path)
		fake.mu.Unlock()

		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	return fake, server.URL
}

// fakeDefaultElements is what each seeded default form starts with: a plausible set of
// the elements the real form type allows.
func fakeDefaultElements(formType string) []string {
	switch formType {
	case "declare":
		return []string{"name", "severity", "summary"}
	case "update":
		return []string{"status", "update_message", "severity"}
	case "retrospective":
		return []string{"name", "incident_type", "severity"}
	default:
		return []string{"name", "severity"}
	}
}

// defaultForm returns the seeded default form of a type.
func (f *fakeIncidentFormsAPI) defaultForm(formType string) client.IncidentFormV3 {
	f.mu.Lock()
	defer f.mu.Unlock()

	form, _ := lo.Find(f.forms, func(form client.IncidentFormV3) bool {
		return string(form.FormType) == formType && form.IncidentTypeId == nil
	})

	return form
}

// claimedBy returns the annotations the latest claim on a form carried; an empty map is a
// form handed back to the dashboard, and ok is false when it was never claimed.
func (f *fakeIncidentFormsAPI) claimedBy(id string) (map[string]string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	annotations, ok := f.claims[id]

	return annotations, ok
}

// elementIDs returns a stored form's element IDs in order, for asserting that an element
// kept its identity across an update.
func (f *fakeIncidentFormsAPI) elementIDs(formID string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(formID)
	if idx < 0 {
		return nil
	}

	return lo.Map(lo.FromPtr(f.forms[idx].LifecycleElements), func(element client.IncidentFormLifecycleElementV3, _ int) string {
		return lo.FromPtr(element.Id)
	})
}

// id mints IDs that sort in creation order, as ULIDs do.
func (f *fakeIncidentFormsAPI) id() string {
	f.nextID++

	return fmt.Sprintf("01FAKE%020d", f.nextID)
}

func (f *fakeIncidentFormsAPI) find(id string) int {
	return slices.IndexFunc(f.forms, func(form client.IncidentFormV3) bool { return form.Id == id })
}

func (f *fakeIncidentFormsAPI) list(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	pageSize := 25
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil {
			f.writeValidationError(w, http.StatusBadRequest, "page_size", "must be an integer")
			return
		}
		pageSize = size
	}
	after := r.URL.Query().Get("after")

	forms := slices.Clone(f.forms)
	slices.SortFunc(forms, func(a, b client.IncidentFormV3) int { return strings.Compare(a.Id, b.Id) })
	if after != "" {
		forms = lo.Filter(forms, func(form client.IncidentFormV3, _ int) bool { return form.Id > after })
	}
	if len(forms) > pageSize {
		forms = forms[:pageSize]
	}

	meta := client.PaginationMetaResultV3{PageSize: int64(pageSize)}
	if len(forms) == pageSize {
		meta.After = lo.ToPtr(forms[len(forms)-1].Id)
	}

	writeJSONStatus(f.t, w, http.StatusOK, client.IncidentFormsListResultV3{IncidentForms: forms, PaginationMeta: meta})
}

func (f *fakeIncidentFormsAPI) show(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		f.writeNotFound(w)
		return
	}

	writeJSONStatus(f.t, w, http.StatusOK, client.IncidentFormsShowResultV3{IncidentForm: f.forms[idx]})
}

// fakeFormPayload is the whole-form part of a create, update or validate payload.
type fakeFormPayload struct {
	FormType          string                                          `json:"form_type"`
	IncidentTypeID    *string                                         `json:"incident_type_id,omitempty"`
	Expressions       []client.ExpressionPayloadV3                    `json:"expressions"`
	LifecycleElements *[]client.IncidentFormLifecycleElementPayloadV3 `json:"lifecycle_elements,omitempty"`
	ID                *string                                         `json:"id,omitempty"`
}

func (f *fakeIncidentFormsAPI) create(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var payload fakeFormPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.writeValidationError(w, http.StatusBadRequest, "", err.Error())
		return
	}

	form, ok := f.createForm(w, payload)
	if !ok {
		return
	}
	f.forms = append(f.forms, form)

	writeJSONStatus(f.t, w, http.StatusCreated, client.IncidentFormsCreateResultV3{IncidentForm: form})
}

// createForm builds a form from a payload, writing the error the real API would and
// returning false when it can't.
func (f *fakeIncidentFormsAPI) createForm(w http.ResponseWriter, payload fakeFormPayload) (client.IncidentFormV3, bool) {
	if payload.IncidentTypeID == nil && payload.FormType != "custom-fields" {
		f.writeValidationError(w, http.StatusUnprocessableEntity, "incident_type_id",
			"The default form of this type already exists. Set incident_type_id to create a form for one incident type, or update the default form.")
		return client.IncidentFormV3{}, false
	}
	if lo.ContainsBy(f.forms, func(form client.IncidentFormV3) bool {
		return string(form.FormType) == payload.FormType && lo.FromPtr(form.IncidentTypeId) == lo.FromPtr(payload.IncidentTypeID)
	}) {
		f.writeValidationError(w, http.StatusUnprocessableEntity, "incident_type_id", "A form of this type already exists for this incident type")
		return client.IncidentFormV3{}, false
	}

	form := client.IncidentFormV3{
		Id:                f.id(),
		FormType:          client.IncidentFormV3FormType(payload.FormType),
		IncidentTypeId:    payload.IncidentTypeID,
		LifecycleElements: &[]client.IncidentFormLifecycleElementV3{},
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if !f.sync(w, &form, payload) {
		return client.IncidentFormV3{}, false
	}

	return form, true
}

func (f *fakeIncidentFormsAPI) update(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		f.writeNotFound(w)
		return
	}

	var payload fakeFormPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.writeValidationError(w, http.StatusBadRequest, "", err.Error())
		return
	}

	form := f.forms[idx]
	if !f.replaceForm(w, &form, payload) {
		return
	}
	f.forms[idx] = form

	writeJSONStatus(f.t, w, http.StatusOK, client.IncidentFormsUpdateResultV3{IncidentForm: form})
}

func (f *fakeIncidentFormsAPI) replaceForm(w http.ResponseWriter, form *client.IncidentFormV3, payload fakeFormPayload) bool {
	if payload.FormType != string(form.FormType) {
		f.writeValidationError(w, http.StatusUnprocessableEntity, "form_type", "A form's type cannot be changed")
		return false
	}
	if lo.FromPtr(payload.IncidentTypeID) != lo.FromPtr(form.IncidentTypeId) {
		f.writeValidationError(w, http.StatusUnprocessableEntity, "incident_type_id", "A form cannot be moved to another incident type")
		return false
	}

	return f.sync(w, form, payload)
}

func (f *fakeIncidentFormsAPI) validate(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var payload fakeFormPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.writeValidationError(w, http.StatusBadRequest, "", err.Error())
		return
	}

	// Work on a copy, so nothing this validates is kept.
	var form client.IncidentFormV3
	if payload.ID != nil {
		idx := f.find(*payload.ID)
		if idx < 0 {
			f.writeNotFound(w)
			return
		}
		form = f.forms[idx]
		elements := slices.Clone(lo.FromPtr(form.LifecycleElements))
		form.LifecycleElements = &elements
		if !f.replaceForm(w, &form, payload) {
			return
		}
	} else {
		var ok bool
		form, ok = f.createForm(w, payload)
		if !ok {
			return
		}
	}

	warnings := []client.IncidentFormValidateWarningV3{}
	savedTypes := lo.Map(lo.FromPtr(form.LifecycleElements), func(element client.IncidentFormLifecycleElementV3, _ int) string {
		return string(element.ElementType)
	})
	requestedTypes := lo.Map(lo.FromPtr(payload.LifecycleElements), func(element client.IncidentFormLifecycleElementPayloadV3, _ int) string {
		return string(element.ElementType)
	})
	if !slices.Equal(savedTypes, requestedTypes) {
		warnings = append(warnings, client.IncidentFormValidateWarningV3{
			Summary: "The saved form will list its elements in a different order",
			Detail:  "The saved form always starts with the pinned elements, wherever this config lists them.",
		})
	}

	writeJSONStatus(f.t, w, http.StatusOK, client.IncidentFormsValidateResultV3{Warnings: warnings})
}

func (f *fakeIncidentFormsAPI) destroy(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.find(r.PathValue("id"))
	if idx < 0 {
		f.writeNotFound(w)
		return
	}

	form := f.forms[idx]
	if form.IncidentTypeId == nil && form.FormType != "custom-fields" {
		f.writeValidationError(w, http.StatusUnprocessableEntity, "id", "The default form of a type cannot be archived")
		return
	}

	f.forms = slices.Delete(f.forms, idx, idx+1)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeIncidentFormsAPI) claim(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var payload client.ManagedResourcesV2CreateManagedResourceJSONRequestBody
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		f.writeValidationError(w, http.StatusBadRequest, "", err.Error())
		return
	}
	if payload.ResourceType != client.ManagedResourcesCreateManagedResourcePayloadV2ResourceTypeIncidentForm {
		f.writeValidationError(w, http.StatusUnprocessableEntity, "resource_type", "not an incident form")
		return
	}
	f.claims[payload.ResourceId] = payload.Annotations

	w.WriteHeader(http.StatusOK)
}

// pinnedElementTypes always sort first, in this order, whatever the payload says.
var pinnedElementTypes = []string{"name", "incident_type", "status"}

// sync writes the payload's expressions and elements onto the form, as the real
// SyncLifecycleForm does: match, update or create, archive the rest, then rank.
func (f *fakeIncidentFormsAPI) sync(w http.ResponseWriter, form *client.IncidentFormV3, payload fakeFormPayload) bool {
	form.Expressions = lo.Map(payload.Expressions, func(expression client.ExpressionPayloadV3, _ int) client.ExpressionV3 {
		return roundTrip[client.ExpressionV3](f.t, expression)
	})

	existing := lo.FromPtr(form.LifecycleElements)
	kept := map[string]bool{}
	seenKeys := map[string]bool{}
	synced := []client.IncidentFormLifecycleElementV3{}

	for idx, element := range lo.FromPtr(payload.LifecycleElements) {
		field := fmt.Sprintf("lifecycle_elements.%d", idx)
		key := elementKey(string(element.ElementType), element.CustomFieldId, element.IncidentRoleId, element.IncidentTimestampId)
		hasNaturalKey := element.ElementType != "divider" && element.ElementType != "text"

		var current *client.IncidentFormLifecycleElementV3
		if element.Id != nil {
			found, ok := lo.Find(existing, func(candidate client.IncidentFormLifecycleElementV3) bool {
				return lo.FromPtr(candidate.Id) == *element.Id
			})
			if !ok {
				f.writeValidationError(w, http.StatusUnprocessableEntity, field+".id", "No element with this ID exists on this form")
				return false
			}
			if elementKey(string(found.ElementType), found.CustomFieldId, found.IncidentRoleId, found.IncidentTimestampId) != key {
				f.writeValidationError(w, http.StatusUnprocessableEntity, field+".id", "This ID belongs to a different element")
				return false
			}
			current = &found
		} else if hasNaturalKey {
			if found, ok := lo.Find(existing, func(candidate client.IncidentFormLifecycleElementV3) bool {
				return elementKey(string(candidate.ElementType), candidate.CustomFieldId, candidate.IncidentRoleId, candidate.IncidentTimestampId) == key
			}); ok {
				current = &found
			}
		}

		if hasNaturalKey {
			if seenKeys[key] {
				f.writeValidationError(w, http.StatusUnprocessableEntity, field, "This element appears more than once on the form")
				return false
			}
			seenKeys[key] = true
		}

		id := f.id()
		if current != nil {
			if kept[*current.Id] {
				f.writeValidationError(w, http.StatusUnprocessableEntity, field+".id", "This element appears more than once on the form")
				return false
			}
			kept[*current.Id] = true
			id = *current.Id
		}

		synced = append(synced, f.readElement(element, id))
	}

	// Pinned elements first, in their fixed order, then everything else by list position.
	slices.SortStableFunc(synced, func(a, b client.IncidentFormLifecycleElementV3) int {
		return pinnedRank(string(a.ElementType)) - pinnedRank(string(b.ElementType))
	})

	form.LifecycleElements = &synced
	form.UpdatedAt = time.Now().UTC()

	return true
}

// pinnedRank is negative for pinned types, so they sort first, and zero for the rest.
func pinnedRank(elementType string) int {
	idx := slices.Index(pinnedElementTypes, elementType)
	if idx < 0 {
		return 0
	}

	return idx - len(pinnedElementTypes)
}

func elementKey(elementType string, customFieldID, incidentRoleID, incidentTimestampID *string) string {
	return strings.Join([]string{elementType, lo.FromPtr(customFieldID), lo.FromPtr(incidentRoleID), lo.FromPtr(incidentTimestampID)}, ":")
}

// readElement renders a written element the way the API reads it back: defaults filled in,
// conditions in the read shape, and an empty description absent.
func (f *fakeIncidentFormsAPI) readElement(element client.IncidentFormLifecycleElementPayloadV3, id string) client.IncidentFormLifecycleElementV3 {
	requiredIf := client.IncidentFormLifecycleElementV3RequiredIfNeverRequire
	if element.RequiredIf != nil {
		requiredIf = client.IncidentFormLifecycleElementV3RequiredIf(*element.RequiredIf)
	}

	config := &client.IncidentFormLifecycleElementConfigV3{RequireComment: lo.ToPtr(false)}
	if element.Config != nil && element.Config.RequireComment != nil {
		config.RequireComment = element.Config.RequireComment
	}

	var defaultValue *client.EngineParamBindingV3
	if element.DefaultValue != nil {
		defaultValue = lo.ToPtr(roundTrip[client.EngineParamBindingV3](f.t, *element.DefaultValue))
	}

	description := element.Description
	if description != nil && strings.TrimSpace(*description) == "" {
		description = nil
	}

	return client.IncidentFormLifecycleElementV3{
		Id:                        lo.ToPtr(id),
		ElementType:               client.IncidentFormLifecycleElementV3ElementType(element.ElementType),
		CustomFieldId:             element.CustomFieldId,
		IncidentRoleId:            element.IncidentRoleId,
		IncidentTimestampId:       element.IncidentTimestampId,
		ShowIfConditionGroups:     lo.ToPtr(readConditionGroupsV3(f.t, lo.FromPtr(element.ShowIfConditionGroups))),
		RequiredIf:                &requiredIf,
		RequiredIfConditionGroups: lo.ToPtr(readConditionGroupsV3(f.t, lo.FromPtr(element.RequiredIfConditionGroups))),
		DefaultValue:              defaultValue,
		Placeholder:               element.Placeholder,
		Description:               description,
		CanSelectNoValue:          lo.ToPtr(lo.FromPtr(element.CanSelectNoValue)),
		Config:                    config,
	}
}

// readConditionGroupsV3 converts written condition groups into the read shape, which
// decorates the subject and operation with labels.
func readConditionGroupsV3(t *testing.T, groups []client.ConditionGroupPayloadV3) []client.ConditionGroupV3 {
	return lo.Map(groups, func(group client.ConditionGroupPayloadV3, _ int) client.ConditionGroupV3 {
		return client.ConditionGroupV3{
			Conditions: lo.Map(group.Conditions, func(condition client.ConditionPayloadV3, _ int) client.ConditionV3 {
				return client.ConditionV3{
					Subject:       client.ConditionSubjectV3{Label: condition.Subject, Reference: condition.Subject},
					Operation:     client.ConditionOperationV3{Label: condition.Operation, Value: condition.Operation},
					ParamBindings: lo.CoalesceSliceOrEmpty(roundTrip[[]client.EngineParamBindingV3](t, condition.ParamBindings)),
				}
			}),
		}
	})
}

// roundTrip converts between a payload type and its read type through JSON, which works
// because the two share field names.
func roundTrip[Out any](t *testing.T, in any) Out {
	t.Helper()

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("encoding %T: %v", in, err)
	}
	var out Out
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decoding into %T: %v", out, err)
	}

	return out
}

func (f *fakeIncidentFormsAPI) writeNotFound(w http.ResponseWriter) {
	writeJSONStatus(f.t, w, http.StatusNotFound, map[string]any{
		"type":   "not_found",
		"status": http.StatusNotFound,
		"errors": []map[string]any{{"code": "not_found", "message": "Incident form not found"}},
	})
}

// writeValidationError answers as the real API does for a payload it rejects.
func (f *fakeIncidentFormsAPI) writeValidationError(w http.ResponseWriter, status int, field, message string) {
	writeJSONStatus(f.t, w, status, map[string]any{
		"type":   "validation_error",
		"status": status,
		"errors": []map[string]any{{
			"code":    "invalid_value",
			"message": message,
			"source":  map[string]string{"field": field},
		}},
	})
}
