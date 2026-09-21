package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
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

// Every lookup attribute is Optional *and* Computed, because a config only sets
// the one it looks the user up by and reads the rest back. Optional-only leaves
// the others null in a plan that defers the read (the email comes from another
// resource), so the apply's real values read as an inconsistent result; and
// `terraform test`'s override_data only synthesises values for Computed
// attributes, so mocking a user's id silently left it null (issue #600).
func TestUserDataSourceLookupAttributesAreComputed(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&IncidentUserDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), "schema: %s", resp.Diagnostics)

	for _, name := range []string{"email", "id", "slack_user_id"} {
		attribute, ok := resp.Schema.Attributes[name]
		require.True(t, ok, "%s is missing from the schema", name)

		assert.True(t, attribute.IsOptional(), "%s should be optional: it's one of the ways to look a user up", name)
		assert.True(t, attribute.IsComputed(), "%s should be computed: it's read back when the lookup uses another attribute", name)
	}
}

// The state the data source writes must fit the schema: every attribute the
// model populates from the API needs somewhere to go.
func TestUserDataSourceSchemaMatchesModel(t *testing.T) {
	ctx := context.Background()

	resp := &datasource.SchemaResponse{}
	(&IncidentUserDataSource{}).Schema(ctx, datasource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), "schema: %s", resp.Diagnostics)

	model := (&IncidentUserDataSource{}).buildModel(client.UserWithRolesV2{
		Id:          "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:        "Alice Bobson",
		Email:       lo.ToPtr("alice@example.com"),
		SlackUserId: lo.ToPtr("U0123456789"),
		IsActive:    true,
	})

	state := tfsdk.State{
		Schema: resp.Schema,
		Raw:    tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil),
	}
	assert.False(t, state.Set(ctx, model).HasError(), "setting state from the API model")
}
