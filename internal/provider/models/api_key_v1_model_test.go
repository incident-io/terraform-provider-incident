package models

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

// apiKeyFixture is a key as the API returns one, with both kinds of role.
func apiKeyFixture() client.APIKeyV1 {
	lastUsedAt := time.Date(2021, 9, 1, 9, 0, 0, 0, time.UTC)

	return client.APIKeyV1{
		Id:       "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:     "CI deploy key",
		Comments: lo.ToPtr("Requested in #ask-infra"),
		Roles: []client.APIKeyRoleV1{
			{Name: "viewer", Description: "can view data"},
			{Name: "catalog_viewer", Description: "can view the catalog"},
		},
		TeamIds: []string{"01G0J1EXE7AXZ2C93K61WBPYEH"},
		TeamRoles: []client.APIKeyTeamRoleV1{
			{Name: "schedules_editor", Description: "can edit schedules"},
		},
		CreatedAt:         time.Date(2021, 8, 17, 13, 28, 57, 0, time.UTC),
		TokenLastIssuedAt: time.Date(2021, 8, 20, 10, 0, 0, 0, time.UTC),
		LastUsedAt:        &lastUsedAt,
	}
}

func TestAPIKeyModelFromAPI(t *testing.T) {
	t.Parallel()

	model := APIKeyModel{}.FromAPI(apiKeyFixture(), APIKeyLocals{
		Token:                      types.StringValue("inc_abc123"),
		TokenVersion:               types.Int64Value(2),
		RotationGracePeriodMinutes: types.Int64Value(30),
	})

	assert.Equal(t, "01FCNDV6P870EA6S7TK1DSYDG0", model.ID.ValueString())
	assert.Equal(t, "CI deploy key", model.Name.ValueString())
	assert.Equal(t, "Requested in #ask-infra", model.Comments.ValueString())

	// The roles arrive as objects carrying a description as well; only the names, which
	// are what a configuration sets, are kept.
	assert.ElementsMatch(t, []string{"viewer", "catalog_viewer"}, model.RoleNamesSlice())
	assert.Equal(t, []string{"01G0J1EXE7AXZ2C93K61WBPYEH"}, model.TeamIDsSlice())
	assert.Equal(t, []string{"schedules_editor"}, model.TeamRoleNamesSlice())

	// None of these three come from the API, so they must survive untouched.
	assert.Equal(t, "inc_abc123", model.Token.ValueString())
	assert.Equal(t, int64(2), model.TokenVersion.ValueInt64())
	assert.Equal(t, int64(30), model.RotationGracePeriodMinutes.ValueInt64())

	createdAt, diags := model.CreatedAt.ValueRFC3339Time()
	require.False(t, diags.HasError())
	assert.Equal(t, time.Date(2021, 8, 17, 13, 28, 57, 0, time.UTC), createdAt)

	issuedAt, diags := model.TokenLastIssuedAt.ValueRFC3339Time()
	require.False(t, diags.HasError())
	assert.Equal(t, time.Date(2021, 8, 20, 10, 0, 0, 0, time.UTC), issuedAt)

	usedAt, diags := model.LastUsedAt.ValueRFC3339Time()
	require.False(t, diags.HasError())
	assert.Equal(t, time.Date(2021, 9, 1, 9, 0, 0, 0, time.UTC), usedAt)
}

// TestAPIKeyModelFromAPIAbsentValues covers a key the API describes as sparsely as it can:
// no comments, no roles, no teams, never used.
func TestAPIKeyModelFromAPIAbsentValues(t *testing.T) {
	t.Parallel()

	key := client.APIKeyV1{
		Id:        "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:      "Unused key",
		Roles:     []client.APIKeyRoleV1{},
		TeamIds:   []string{},
		TeamRoles: []client.APIKeyTeamRoleV1{},
	}

	model := APIKeyModel{}.FromAPI(key, APIKeyLocals{
		Token:                      types.StringNull(),
		TokenVersion:               types.Int64Null(),
		RotationGracePeriodMinutes: types.Int64Value(30),
	})

	assert.True(t, model.Comments.IsNull())

	// The collections are empty rather than null, matching what the schema's defaults plan
	// for an attribute the configuration leaves out: a null here would read back as a
	// change on every plan.
	for name, set := range map[string]types.Set{
		"role_names":      model.RoleNames,
		"team_ids":        model.TeamIDs,
		"team_role_names": model.TeamRoleNames,
	} {
		assert.False(t, set.IsNull(), "%s should be empty, not null", name)
		assert.Empty(t, set.Elements(), "%s should be empty", name)
	}

	// A key nothing has authenticated with has no last_used_at, and null is how that reads.
	assert.True(t, model.LastUsedAt.IsNull())

	// An imported key has no token, and no version to rotate from.
	assert.True(t, model.Token.IsNull())
	assert.True(t, model.TokenVersion.IsNull())
}

func TestAPIKeyModelToCreatePayload(t *testing.T) {
	t.Parallel()

	model := APIKeyModel{
		Name:          types.StringValue("CI deploy key"),
		Comments:      types.StringValue("Requested in #ask-infra"),
		RoleNames:     stringSet(t, "viewer"),
		TeamIDs:       stringSet(t, "01G0J1EXE7AXZ2C93K61WBPYEH"),
		TeamRoleNames: stringSet(t, "schedules_editor"),
	}

	payload := model.ToCreatePayload()

	assert.Equal(t, "CI deploy key", payload.Name)
	require.NotNil(t, payload.Comments)
	assert.Equal(t, "Requested in #ask-infra", *payload.Comments)
	assert.Equal(t, []client.APIKeysCreatePayloadV1RoleNames{"viewer"}, payload.RoleNames)
	assert.Equal(t, []string{"01G0J1EXE7AXZ2C93K61WBPYEH"}, payload.TeamIds)
	assert.Equal(t, []client.APIKeysCreatePayloadV1TeamRoleNames{"schedules_editor"}, payload.TeamRoleNames)
}

// TestAPIKeyModelToCreatePayloadOmitsAbsentComments covers a brand new key with nothing to
// clear: absent comments are left out rather than sent as an empty string.
func TestAPIKeyModelToCreatePayloadOmitsAbsentComments(t *testing.T) {
	t.Parallel()

	model := APIKeyModel{
		Name:          types.StringValue("CI deploy key"),
		Comments:      types.StringNull(),
		RoleNames:     stringSet(t, "viewer"),
		TeamIDs:       stringSet(t),
		TeamRoleNames: stringSet(t),
	}

	payload := model.ToCreatePayload()

	assert.Nil(t, payload.Comments)

	// Empty rather than nil: a nil slice serialises as an omitted field, which the API reads
	// as "leave unchanged" rather than "none of these".
	assert.NotNil(t, payload.TeamIds)
	assert.Empty(t, payload.TeamIds)
	assert.NotNil(t, payload.TeamRoleNames)
	assert.Empty(t, payload.TeamRoleNames)
}

// TestAPIKeyModelToUpdatePayloadAlwaysSendsEverything covers clearing a key's roles and
// comments. The API reads an omitted field as "leave unchanged", so a config that drops a
// role has to send the shorter list rather than nothing at all.
func TestAPIKeyModelToUpdatePayloadAlwaysSendsEverything(t *testing.T) {
	t.Parallel()

	model := APIKeyModel{
		Name:          types.StringValue("CI deploy key"),
		Comments:      types.StringNull(),
		RoleNames:     stringSet(t),
		TeamIDs:       stringSet(t),
		TeamRoleNames: stringSet(t),
	}

	payload := model.ToUpdatePayload()

	require.NotNil(t, payload.Comments)
	assert.Equal(t, "", *payload.Comments)

	assert.NotNil(t, payload.RoleNames)
	assert.Empty(t, payload.RoleNames)
	assert.NotNil(t, payload.TeamIds)
	assert.Empty(t, payload.TeamIds)
	assert.NotNil(t, payload.TeamRoleNames)
	assert.Empty(t, payload.TeamRoleNames)
}

// TestAPIKeyModelSlicesFromUnsetSets covers a model whose sets are null or unknown, which
// is what an attribute waiting on another resource looks like. The payload has to carry an
// empty array rather than a nil the API would read as "leave unchanged".
func TestAPIKeyModelSlicesFromUnsetSets(t *testing.T) {
	t.Parallel()

	for name, set := range map[string]types.Set{
		"null":    types.SetNull(types.StringType),
		"unknown": types.SetUnknown(types.StringType),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			model := APIKeyModel{RoleNames: set, TeamIDs: set, TeamRoleNames: set}

			for label, got := range map[string][]string{
				"role_names":      model.RoleNamesSlice(),
				"team_ids":        model.TeamIDsSlice(),
				"team_role_names": model.TeamRoleNamesSlice(),
			} {
				assert.NotNil(t, got, "%s should be empty, not nil", label)
				assert.Empty(t, got, "%s should be empty", label)
			}
		})
	}
}

func TestAPIKeyDataSourceModelFromAPI(t *testing.T) {
	t.Parallel()

	model := APIKeyDataSourceModel{}.FromAPI(apiKeyFixture())

	assert.Equal(t, "01FCNDV6P870EA6S7TK1DSYDG0", model.ID.ValueString())
	assert.Equal(t, "CI deploy key", model.Name.ValueString())
	assert.Equal(t, "Requested in #ask-infra", model.Comments.ValueString())
	assert.ElementsMatch(t, []string{"viewer", "catalog_viewer"}, apiKeyStringSlice(model.RoleNames))
	assert.Equal(t, []string{"01G0J1EXE7AXZ2C93K61WBPYEH"}, apiKeyStringSlice(model.TeamIDs))
	assert.Equal(t, []string{"schedules_editor"}, apiKeyStringSlice(model.TeamRoleNames))
}

// stringSet builds a set of strings for a model under test.
func stringSet(t *testing.T, values ...string) types.Set {
	t.Helper()

	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}

	set, diags := types.SetValue(types.StringType, elements)
	require.False(t, diags.HasError(), "%+v", diags)

	return set
}
