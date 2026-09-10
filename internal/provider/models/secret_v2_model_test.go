package models

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func TestSecretModelFromAPI(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2021, 8, 17, 13, 28, 57, 0, time.UTC)
	updatedAt := time.Date(2021, 9, 1, 9, 0, 0, 0, time.UTC)

	secret := client.SecretV2{
		Id:            "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:          "PagerDuty webhook token",
		Description:   lo.ToPtr("Auth token for the PagerDuty outgoing webhook"),
		OwningTeamIds: []string{"01G0J1EXE7AXZ2C93K61WBPYEH"},
		LastFourChars: lo.ToPtr("c123"),
		Version:       3,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}

	model := SecretModel{}.FromAPI(secret, types.Int64Value(2))

	assert.Equal(t, "01FCNDV6P870EA6S7TK1DSYDG0", model.ID.ValueString())
	assert.Equal(t, "PagerDuty webhook token", model.Name.ValueString())
	assert.Equal(t, "Auth token for the PagerDuty outgoing webhook", model.Description.ValueString())
	assert.Equal(t, []string{"01G0J1EXE7AXZ2C93K61WBPYEH"}, model.OwningTeamIDsSlice())
	assert.Equal(t, int64(3), model.Version.ValueInt64())
	assert.Equal(t, "c123", model.LastFourChars.ValueString())

	// The value is write-only, so it must never reach state, and the version it was set
	// under is the practitioner's own counter rather than anything the API reported.
	assert.True(t, model.ValueWO.IsNull())
	assert.Equal(t, int64(2), model.ValueWOVersion.ValueInt64())
}

func TestSecretModelFromAPIOmittedFields(t *testing.T) {
	t.Parallel()

	// A secret with no description, no owning teams, and a value too short to have a
	// masked last-four: every optional field absent.
	secret := client.SecretV2{
		Id:            "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:          "Short",
		OwningTeamIds: []string{},
		Version:       1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	model := SecretModel{}.FromAPI(secret, types.Int64Null())

	assert.True(t, model.Description.IsNull())
	assert.True(t, model.LastFourChars.IsNull())
	assert.True(t, model.ValueWOVersion.IsNull())
	assert.Empty(t, model.OwningTeamIDsSlice())
	assert.False(t, model.OwningTeamIDs.IsNull(), "an unset owning_team_ids is an empty set, not null")
}

func TestSecretModelToCreatePayload(t *testing.T) {
	t.Parallel()

	model := SecretModel{
		Name:          types.StringValue("PagerDuty webhook token"),
		Description:   types.StringValue("Auth token for the PagerDuty outgoing webhook"),
		OwningTeamIDs: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("team_1")}),
	}

	payload := model.ToCreatePayload("sk_live_abc123")

	assert.Equal(t, "PagerDuty webhook token", payload.Name)
	assert.Equal(t, "sk_live_abc123", payload.Value)
	require.NotNil(t, payload.Description)
	assert.Equal(t, "Auth token for the PagerDuty outgoing webhook", *payload.Description)
	require.NotNil(t, payload.OwningTeamIds)
	assert.Equal(t, []string{"team_1"}, *payload.OwningTeamIds)
}

func TestSecretModelToCreatePayloadWithoutDescription(t *testing.T) {
	t.Parallel()

	model := SecretModel{
		Name:        types.StringValue("Short"),
		Description: types.StringNull(),
	}

	payload := model.ToCreatePayload("sk_live_abc123")

	// There's nothing to clear on a new secret, so an absent description is omitted.
	assert.Nil(t, payload.Description)
	require.NotNil(t, payload.OwningTeamIds)
	assert.Empty(t, *payload.OwningTeamIds)
}

func TestSecretModelToUpdatePayload(t *testing.T) {
	t.Parallel()

	model := SecretModel{
		Name:          types.StringValue("Renamed"),
		Description:   types.StringValue("Still in use"),
		OwningTeamIDs: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("team_1")}),
	}

	payload := model.ToUpdatePayload()

	assert.Equal(t, "Renamed", payload.Name)
	require.NotNil(t, payload.Description)
	assert.Equal(t, "Still in use", *payload.Description)
	require.NotNil(t, payload.OwningTeamIds)
	assert.Equal(t, []string{"team_1"}, *payload.OwningTeamIds)
}

func TestSecretModelToUpdatePayloadClearsRemovedFields(t *testing.T) {
	t.Parallel()

	// The API reads an omitted description or owning_team_ids as "leave unchanged", so a
	// config that dropped either has to send the empty value to actually clear it.
	model := SecretModel{
		Name:          types.StringValue("Renamed"),
		Description:   types.StringNull(),
		OwningTeamIDs: types.SetNull(types.StringType),
	}

	payload := model.ToUpdatePayload()

	require.NotNil(t, payload.Description)
	assert.Equal(t, "", *payload.Description)
	require.NotNil(t, payload.OwningTeamIds)
	assert.Empty(t, *payload.OwningTeamIds)
}

func TestSecretDataSourceModelFromAPI(t *testing.T) {
	t.Parallel()

	secret := client.SecretV2{
		Id:            "01FCNDV6P870EA6S7TK1DSYDG0",
		Name:          "PagerDuty webhook token",
		OwningTeamIds: []string{"team_1", "team_2"},
		LastFourChars: lo.ToPtr("c123"),
		Version:       3,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	model := SecretDataSourceModel{}.FromAPI(secret)

	assert.Equal(t, "01FCNDV6P870EA6S7TK1DSYDG0", model.ID.ValueString())
	assert.Equal(t, "PagerDuty webhook token", model.Name.ValueString())
	assert.Equal(t, int64(3), model.Version.ValueInt64())
	assert.Equal(t, "c123", model.LastFourChars.ValueString())
	assert.Len(t, model.OwningTeamIDs.Elements(), 2)
	assert.True(t, model.Description.IsNull())
}
