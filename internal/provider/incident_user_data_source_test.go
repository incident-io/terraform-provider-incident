package provider

import (
	"testing"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// TestSelectUser covers disambiguating an email or Slack user ID lookup that
// matches more than one user, which happens because we list with
// IncludeInactive. See RESP-18677.
func TestSelectUser(t *testing.T) {
	user := func(id string, isActive bool) client.UserWithRolesV2 {
		return client.UserWithRolesV2{Id: id, Name: id, IsActive: isActive}
	}

	cases := []struct {
		name    string
		users   []client.UserWithRolesV2
		wantID  string
		wantErr string
	}{
		{
			name:    "no users",
			users:   nil,
			wantErr: "user not found",
		},
		{
			name:   "single active user",
			users:  []client.UserWithRolesV2{user("active", true)},
			wantID: "active",
		},
		{
			// We resolve inactive users so an apply doesn't break the moment
			// someone is offboarded.
			name:   "single inactive user",
			users:  []client.UserWithRolesV2{user("inactive", false)},
			wantID: "inactive",
		},
		{
			// The RESP-18677 regression: someone signed in via Slack before SSO,
			// then again via SSO, and the first account was deactivated.
			name:   "one active and one deactivated duplicate",
			users:  []client.UserWithRolesV2{user("deactivated", false), user("active", true)},
			wantID: "active",
		},
		{
			name:   "one active among several deactivated duplicates",
			users:  []client.UserWithRolesV2{user("old", false), user("active", true), user("older", false)},
			wantID: "active",
		},
		{
			name:    "several users, none active",
			users:   []client.UserWithRolesV2{user("one", false), user("two", false)},
			wantErr: "multiple users found",
		},
		{
			name:    "several active users",
			users:   []client.UserWithRolesV2{user("one", true), user("two", true)},
			wantErr: "multiple users found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectUser(tc.users)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got user %+v", tc.wantErr, got)
				}
				if err.Error() != tc.wantErr {
					t.Fatalf("expected error %q, got %q", tc.wantErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got == nil {
				t.Fatal("expected a user, got nil")
			}
			if got.Id != tc.wantID {
				t.Fatalf("expected user %q, got %q", tc.wantID, got.Id)
			}
		})
	}
}
