package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func TestEntryUnchanged(t *testing.T) {
	model := &IncidentCatalogEntriesResourceModel{
		ManagedAttributes: types.SetValueMust(types.StringType, []attr.Value{
			types.StringValue("owner"), types.StringValue("regions"),
		}),
	}
	desired := func() client.CatalogCreateEntryPayloadV3 {
		return client.CatalogCreateEntryPayloadV3{
			Name:    "api",
			Aliases: &[]string{"api", "prod/api"},
			AttributeValues: map[string]client.CatalogEngineParamBindingPayloadV3{
				"owner":   {Value: &client.CatalogEngineParamBindingValuePayloadV3{Literal: lo.ToPtr("team-a")}},
				"regions": {ArrayValue: &[]client.CatalogEngineParamBindingValuePayloadV3{}},
			},
		}
	}
	current := func() client.CatalogEntryV3 {
		return client.CatalogEntryV3{
			Name:    "api",
			Aliases: []string{"api", "prod/api"},
			AttributeValues: map[string]client.CatalogEntryEngineParamBindingV3{
				"owner": {Value: &client.CatalogEntryEngineParamBindingValueV3{Literal: lo.ToPtr("team-a")}},
				// The API omits an empty array, while the payload sends [].
				"regions":   {},
				"unmanaged": {Value: &client.CatalogEntryEngineParamBindingValueV3{Literal: lo.ToPtr("anything")}},
			},
		}
	}

	t.Run("matching entry", func(t *testing.T) {
		assert.True(t, model.entryUnchanged(desired(), current()))
	})

	// Aliases are a list in state, which takes the API's order, so a reorder must be sent.
	t.Run("aliases in another order", func(t *testing.T) {
		entry := current()
		entry.Aliases = []string{"prod/api", "api"}
		assert.False(t, model.entryUnchanged(desired(), entry))
	})

	t.Run("duplicate alias", func(t *testing.T) {
		entry := current()
		entry.Aliases = []string{"api", "prod/api", "api"}
		assert.False(t, model.entryUnchanged(desired(), entry))
	})

	t.Run("no aliases on either side", func(t *testing.T) {
		payload := desired()
		payload.Aliases = &[]string{}
		entry := current()
		entry.Aliases = nil
		assert.True(t, model.entryUnchanged(payload, entry))
	})

	// Only empty arrays are normalised: an empty literal is still a value.
	t.Run("empty literal against no value", func(t *testing.T) {
		payload := desired()
		payload.AttributeValues["owner"] = client.CatalogEngineParamBindingPayloadV3{
			Value: &client.CatalogEngineParamBindingValuePayloadV3{Literal: lo.ToPtr("")},
		}
		entry := current()
		entry.AttributeValues["owner"] = client.CatalogEntryEngineParamBindingV3{}
		assert.False(t, model.entryUnchanged(payload, entry))
	})

	t.Run("changed alias", func(t *testing.T) {
		entry := current()
		entry.Aliases = []string{"api", "staging/api"}
		assert.False(t, model.entryUnchanged(desired(), entry))
	})

	t.Run("changed name", func(t *testing.T) {
		entry := current()
		entry.Name = "api (legacy)"
		assert.False(t, model.entryUnchanged(desired(), entry))
	})

	t.Run("changed managed attribute", func(t *testing.T) {
		entry := current()
		entry.AttributeValues["owner"] = client.CatalogEntryEngineParamBindingV3{
			Value: &client.CatalogEntryEngineParamBindingValueV3{Literal: lo.ToPtr("team-b")},
		}
		assert.False(t, model.entryUnchanged(desired(), entry))
	})

	t.Run("changed rank", func(t *testing.T) {
		payload := desired()
		payload.Rank = lo.ToPtr(int32(3))
		assert.False(t, model.entryUnchanged(payload, current()))
	})
}

// payloadTooLarge builds the error the client returns for a 413, as the generated client
// sees it after net/http wraps the transport's error.
func payloadTooLarge() error {
	return &url.Error{Op: "Post", URL: "https://api.incident.io", Err: client.HTTPError{
		StatusCode: http.StatusRequestEntityTooLarge,
		Body:       []byte(`{"type":"payload_too_large"}`),
	}}
}

func entriesWithExternalIDs(ids ...string) []client.PartialEntryPayloadV3 {
	return lo.Map(ids, func(id string, _ int) client.PartialEntryPayloadV3 {
		return client.PartialEntryPayloadV3{ExternalId: lo.ToPtr(id)}
	})
}

func TestBulkUpdateSplittingOversized(t *testing.T) {
	t.Run("splits a rejected batch until every request fits", func(t *testing.T) {
		var sent []string
		calls := 0
		err := bulkUpdateSplittingOversized(entriesWithExternalIDs("a", "b", "c", "d", "e", "f", "g"), func(entries []client.PartialEntryPayloadV3) error {
			calls++
			if len(entries) > 2 {
				return payloadTooLarge()
			}
			for _, entry := range entries {
				sent = append(sent, *entry.ExternalId)
			}
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c", "d", "e", "f", "g"}, sent)
		assert.Greater(t, calls, 1)
	})

	t.Run("names an entry that is too large on its own", func(t *testing.T) {
		err := bulkUpdateSplittingOversized(entriesWithExternalIDs("small", "huge"), func(entries []client.PartialEntryPayloadV3) error {
			if lo.ContainsBy(entries, func(entry client.PartialEntryPayloadV3) bool { return *entry.ExternalId == "huge" }) {
				return payloadTooLarge()
			}
			return nil
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "external_id=huge")
	})

	t.Run("stops at the first failure without resending what committed", func(t *testing.T) {
		var sent []string
		err := bulkUpdateSplittingOversized(entriesWithExternalIDs("a", "b", "c", "d"), func(entries []client.PartialEntryPayloadV3) error {
			if len(entries) > 2 {
				return payloadTooLarge()
			}
			if *entries[0].ExternalId == "c" {
				return errors.New("validation failed")
			}
			for _, entry := range entries {
				sent = append(sent, *entry.ExternalId)
			}
			return nil
		})

		require.Error(t, err)
		assert.Equal(t, []string{"a", "b"}, sent)
	})

	t.Run("returns other errors without splitting", func(t *testing.T) {
		calls := 0
		err := bulkUpdateSplittingOversized(entriesWithExternalIDs("a", "b", "c", "d"), func([]client.PartialEntryPayloadV3) error {
			calls++
			return errors.New("validation failed")
		})

		require.Error(t, err)
		assert.Equal(t, 1, calls)
	})
}

// The real client turns a 413 into an error that isPayloadTooLarge recognises, after
// retryablehttp declines to retry it.
func TestIsPayloadTooLargeThroughTheClient(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte(`{"type":"payload_too_large","status":413,"errors":[{"code":"payload_too_large","message":"The request payload is too large"}]}`))
	}))
	defer server.Close()

	apiClient, err := client.New(context.Background(), "test-key", server.URL, "test")
	require.NoError(t, err)

	_, err = apiClient.CatalogV3BulkUpdateEntriesWithResponse(context.Background(), client.CatalogBulkUpdateEntriesPayloadV3{
		CatalogTypeId: "type",
		Entries:       entriesWithExternalIDs("a"),
	})

	require.Error(t, err)
	assert.True(t, isPayloadTooLarge(err))
	assert.Equal(t, 1, calls)
}
