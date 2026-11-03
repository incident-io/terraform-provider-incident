package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/samber/lo"

	"github.com/incident-io/terraform-provider-incident/v7/internal/client"
)

// catalogEntryWriteLocks holds a *sync.Mutex per catalog entry ID. The API merges a
// partial update into the entry as it read it, then writes the whole entry, so two
// resources writing the same entry at once can undo each other's write.
var catalogEntryWriteLocks sync.Map

// catalogEntryPartialWrite is a write to part of a catalog entry that something else, such
// as an incident.io sync, owns.
type catalogEntryPartialWrite struct {
	// CustomAliases replaces the entry's custom aliases. Nil leaves them alone.
	CustomAliases *[]string
	// UpdateAttributes lists the attributes to write: those missing from AttributeValues
	// are cleared, and every other attribute is left alone.
	UpdateAttributes []string
	AttributeValues  map[string]client.CatalogEngineParamBindingPayloadV3
}

// writeCatalogEntryPartially writes part of an existing entry, and returns the entry as
// written. It returns nil, with no error, if the entry doesn't exist.
func writeCatalogEntryPartially(ctx context.Context, apiClient *client.ClientWithResponses, entryID string, write catalogEntryPartialWrite) (*client.CatalogEntryV3, diag.Diagnostics) {
	var diags diag.Diagnostics

	lock, _ := catalogEntryWriteLocks.LoadOrStore(entryID, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	// Read the entry fresh, so we send back the name and rank it has now: an update always
	// writes them.
	current, err := apiClient.CatalogV3ShowEntryWithResponse(ctx, entryID, nil)
	if err != nil {
		if httpErr := (client.HTTPError{}); errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return nil, diags
		}
		diags.AddError("Client Error", fmt.Sprintf("Unable to read catalog entry, got error: %s", err))
		return nil, diags
	}

	entry, catalogType := current.JSON200.CatalogEntry, current.JSON200.CatalogType
	if write.CustomAliases != nil && !catalogType.SupportsCustomAliases {
		diags.AddError(
			"Catalog type doesn't take custom aliases",
			fmt.Sprintf("Entries of the %q catalog type don't take custom aliases. Only catalog types that incident.io syncs do, "+
				"and not those whose aliases you edit elsewhere, such as alert priorities.", catalogType.Name),
		)
		return nil, diags
	}

	result, err := apiClient.CatalogV3UpdateEntryWithResponse(ctx, entryID, client.CatalogUpdateEntryPayloadV3{
		Name:            entry.Name,
		Rank:            lo.ToPtr(entry.Rank),
		CustomAliases:   write.CustomAliases,
		AttributeValues: lo.Ternary(write.AttributeValues == nil, map[string]client.CatalogEngineParamBindingPayloadV3{}, write.AttributeValues),
		// Always set, even when empty: without it the API replaces every attribute an API
		// key can set. Leaving out aliases and the external ID keeps the synced ones.
		UpdateAttributes: lo.ToPtr(lo.Ternary(write.UpdateAttributes == nil, []string{}, write.UpdateAttributes)),
	})
	if err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to update catalog entry, got error: %s", err))
		return nil, diags
	}

	return &result.JSON200.CatalogEntry, diags
}
