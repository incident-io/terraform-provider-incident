package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestFormFieldIDForKey covers correlating a form field's computed id with the
// prior state by key. The positional correlation the stock UseStateForUnknown
// does is wrong here: form_fields is an ordered list, so deleting or inserting
// anywhere but the end shifts every later field onto its neighbour's id.
func TestFormFieldIDForKey(t *testing.T) {
	field := func(id, key string) IncidentWorkflowFormField {
		return IncidentWorkflowFormField{
			ID:  types.StringValue(id),
			Key: types.StringValue(key),
		}
	}

	prior := []IncidentWorkflowFormField{
		field("01A", "reason"),
		field("01B", "responders"),
		field("01C", "severity"),
	}

	cases := []struct {
		name      string
		prior     []IncidentWorkflowFormField
		key       types.String
		wantID    string
		wantFound bool
	}{
		{
			// The bug: with `reason` deleted, `responders` moves to index 0. Position
			// would hand it 01A (reason's id), rewriting reason into responders.
			name:  "key keeps its id after an earlier field is deleted",
			prior: prior, key: types.StringValue("responders"),
			wantID: "01B", wantFound: true,
		},
		{
			// Likewise for an insertion at the front, which shifts everything down.
			name:  "last key keeps its id regardless of position",
			prior: prior, key: types.StringValue("severity"),
			wantID: "01C", wantFound: true,
		},
		{
			name:  "first key still resolves",
			prior: prior, key: types.StringValue("reason"),
			wantID: "01A", wantFound: true,
		},
		{
			// A genuinely new field must be left for the API to assign an id to,
			// rather than inheriting whatever sat at its index before.
			name:  "unknown key is not found",
			prior: prior, key: types.StringValue("impact"),
			wantFound: false,
		},
		{
			name:  "no prior fields",
			prior: nil, key: types.StringValue("reason"),
			wantFound: false,
		},
		{
			name:  "null key cannot correlate",
			prior: prior, key: types.StringNull(),
			wantFound: false,
		},
		{
			name:  "unknown key cannot correlate",
			prior: prior, key: types.StringUnknown(),
			wantFound: false,
		},
		{
			// Shouldn't happen once applied, but must not plan a null id.
			name:      "matched field with no id is not found",
			prior:     []IncidentWorkflowFormField{{ID: types.StringNull(), Key: types.StringValue("reason")}},
			key:       types.StringValue("reason"),
			wantFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, found := formFieldIDForKey(tc.prior, tc.key)
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if !tc.wantFound {
				return
			}
			if id.ValueString() != tc.wantID {
				t.Errorf("id = %q, want %q", id.ValueString(), tc.wantID)
			}
		})
	}
}
