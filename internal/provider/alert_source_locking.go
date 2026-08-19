package provider

import (
	"context"
	"sync"
)

// Writes to one alert source serialise here. A source is a single blob, so every attribute
// write reads the whole thing, changes its own part and writes it back — siblings racing each
// other all pin the same version, and the server rejects whoever is second.
//
// Its own retry covers a handful, but the budget is fixed and unspaced, so it runs out
// somewhere above fifteen attributes on one source: apply and destroy both fail, and a failed
// destroy leaves resources behind. Serialising costs nothing, because the organisation-wide
// config lock the write takes already commits them one at a time.
//
// Keyed per source rather than one global lock, because a version is per source and two
// sources have no reason to wait for each other.
var alertSourceWriteMutexes sync.Map

// lockForAlertSource runs fn holding the write lock for sourceID.
//
// Process-wide, so it orders the writes of one apply. Two applies running at once still race,
// and rely on the server's retry as before.
func lockForAlertSource[T any](
	ctx context.Context,
	sourceID string,
	fn func(context.Context) (T, error),
) (T, error) {
	value, _ := alertSourceWriteMutexes.LoadOrStore(sourceID, &sync.Mutex{})
	mutex, ok := value.(*sync.Mutex)
	if !ok {
		// Unreachable: alertSourceWriteMutexes only ever stores *sync.Mutex.
		panic("alertSourceWriteMutexes contained a non-*sync.Mutex value")
	}

	mutex.Lock()
	defer mutex.Unlock()

	return fn(ctx)
}
