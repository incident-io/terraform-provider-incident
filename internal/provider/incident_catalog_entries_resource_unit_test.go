package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

func catalogLiteral(value string) client.CatalogEngineParamBindingPayloadV3 {
	return client.CatalogEngineParamBindingPayloadV3{
		Value: &client.CatalogEngineParamBindingValuePayloadV3{Literal: lo.ToPtr(value)},
	}
}

func updateFor(entryID string, values map[string]client.CatalogEngineParamBindingPayloadV3, cleared ...string) updatePayload {
	return updatePayload{
		entryID: entryID,
		payload: &catalogEntryModelPayload{
			Payload:             client.CatalogCreateEntryPayloadV3{AttributeValues: values},
			ClearedAttributeIDs: cleared,
		},
	}
}

func batchEntryIDs(batch bulkUpdateBatch) []string {
	return lo.Map(batch.entries, func(u updatePayload, _ int) string { return u.entryID })
}

func TestBatchBulkUpdates(t *testing.T) {
	t.Run("an entry clearing an attribute doesn't share update_attributes with siblings", func(t *testing.T) {
		// "clearing" clears owner; "sibling" leaves owner out entirely, which must keep
		// meaning "leave it alone" rather than being swept up in the clear.
		toUpdate := []updatePayload{
			updateFor("clearing", map[string]client.CatalogEngineParamBindingPayloadV3{"description": catalogLiteral("a")}, "owner"),
			updateFor("sibling", map[string]client.CatalogEngineParamBindingPayloadV3{"description": catalogLiteral("b")}),
		}

		batches := batchBulkUpdates(toUpdate, nil, 100)
		require.Len(t, batches, 2)

		assert.Equal(t, []string{"clearing"}, batchEntryIDs(batches[0]))
		require.NotNil(t, batches[0].updateAttributes)
		assert.Equal(t, []string{"description", "owner"}, *batches[0].updateAttributes)

		assert.Equal(t, []string{"sibling"}, batchEntryIDs(batches[1]))
		assert.Nil(t, batches[1].updateAttributes)
	})

	t.Run("entries with the same update_attributes share a batch", func(t *testing.T) {
		toUpdate := []updatePayload{
			updateFor("one", map[string]client.CatalogEngineParamBindingPayloadV3{"description": catalogLiteral("a")}, "owner"),
			updateFor("two", map[string]client.CatalogEngineParamBindingPayloadV3{"description": catalogLiteral("b")}, "owner"),
			updateFor("three", map[string]client.CatalogEngineParamBindingPayloadV3{"description": catalogLiteral("c")}),
		}

		batches := batchBulkUpdates(toUpdate, nil, 100)
		require.Len(t, batches, 2)
		assert.Equal(t, []string{"one", "two"}, batchEntryIDs(batches[0]))
		assert.Equal(t, []string{"three"}, batchEntryIDs(batches[1]))
	})

	t.Run("managed_attributes applies to every entry", func(t *testing.T) {
		managed := []string{"description", "owner"}
		toUpdate := []updatePayload{
			updateFor("clearing", map[string]client.CatalogEngineParamBindingPayloadV3{"description": catalogLiteral("a")}, "owner"),
			updateFor("sibling", map[string]client.CatalogEngineParamBindingPayloadV3{"description": catalogLiteral("b")}),
		}

		batches := batchBulkUpdates(toUpdate, &managed, 100)
		require.Len(t, batches, 1)
		assert.Equal(t, []string{"clearing", "sibling"}, batchEntryIDs(batches[0]))
		assert.Same(t, &managed, batches[0].updateAttributes)
	})

	t.Run("groups are chunked to the batch size", func(t *testing.T) {
		toUpdate := []updatePayload{
			updateFor("one", nil),
			updateFor("two", nil),
			updateFor("three", nil),
		}

		batches := batchBulkUpdates(toUpdate, nil, 2)
		require.Len(t, batches, 2)
		assert.Equal(t, []string{"one", "two"}, batchEntryIDs(batches[0]))
		assert.Equal(t, []string{"three"}, batchEntryIDs(batches[1]))
	})
}

func TestIncidentCatalogEntriesResourceBuildModelClearedAttribute(t *testing.T) {
	plan := &IncidentCatalogEntriesResourceModel{
		ID:                types.StringValue("type"),
		ManagedAttributes: types.SetNull(types.StringType),
		Entries: map[string]CatalogEntryModel{
			"one": {
				AttributeValues: map[string]CatalogEntryAttributeBindingModel{
					"description": {Value: types.StringValue(""), ArrayValue: types.ListNull(types.StringType)},
				},
			},
		},
	}
	entries := []client.CatalogEntryV3{{
		Id:         "entry",
		Name:       "One",
		ExternalId: lo.ToPtr("one"),
		AttributeValues: map[string]client.CatalogEntryEngineParamBindingV3{
			"description": {Value: &client.CatalogEntryEngineParamBindingValueV3{Literal: lo.ToPtr("set in the dashboard")}},
		},
	}}

	r := &IncidentCatalogEntriesResource{}

	t.Run("after apply, a value the API kept is an error", func(t *testing.T) {
		_, diags := r.buildModel(client.CatalogTypeV3{Id: "type"}, entries, plan, true)
		assert.True(t, diags.HasError())
	})

	t.Run("on read, a value set since the clear is reported as drift", func(t *testing.T) {
		model, diags := r.buildModel(client.CatalogTypeV3{Id: "type"}, entries, plan, false)
		require.False(t, diags.HasError(), "%v", diags)
		assert.Equal(t, "set in the dashboard", model.Entries["one"].AttributeValues["description"].Value.ValueString())
	})
}
