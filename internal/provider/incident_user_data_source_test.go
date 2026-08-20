package provider

import (
	"testing"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// TestSelectUserByEmail covers picking a user when an email lookup matches more
// than one row, which happens because we list with include_inactive.
func TestSelectUserByEmail(t *testing.T) {
	user := func(id string, isActive bool) client.UserWithRolesV2 {
		return client.UserWithRolesV2{Id: id, Name: id, IsActive: isActive}
	}

	cases := []struct {
		name    string
		users   []client.UserWithRolesV2
		wantID  string
		wantErr bool
	}{
		{
			name:    "no matches errors",
			users:   nil,
			wantErr: true,
		},
		{
			name:   "single active match",
			users:  []client.UserWithRolesV2{user("active", true)},
			wantID: "active",
		},
		{
			// A user who has since been offboarded must still resolve, so an
			// apply doesn't break the moment they're deactivated.
			name:   "single inactive match still resolves",
			users:  []client.UserWithRolesV2{user("inactive", false)},
			wantID: "inactive",
		},
		{
			// The reported bug (PR-497): a deactivated duplicate left over from
			// a merge broke the lookup for the live SSO account.
			name:   "one active alongside deactivated duplicates",
			users:  []client.UserWithRolesV2{user("deactivated-slack", false), user("live", true), user("deactivated-saml", false)},
			wantID: "live",
		},
		{
			name:    "several active matches is genuinely ambiguous",
			users:   []client.UserWithRolesV2{user("one", true), user("two", true)},
			wantErr: true,
		},
		{
			name:    "several matches with none active is ambiguous",
			users:   []client.UserWithRolesV2{user("one", false), user("two", false)},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectUserByEmail(tc.users)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got user %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got.Id != tc.wantID {
				t.Errorf("got user %q, want %q", got.Id, tc.wantID)
			}
		})
	}
}
