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
//
// A typed map behind its own mutex rather than a sync.Map, so reading one back needs no type
// assertion: there is no wrong-type case to report, and so nothing here has to panic.
var (
	alertSourceWriteMutexesLock sync.Mutex
	alertSourceWriteMutexes     = map[string]*sync.Mutex{}
)

// alertSourceWriteMutex returns the mutex for sourceID, creating it on first use.
func alertSourceWriteMutex(sourceID string) *sync.Mutex {
	alertSourceWriteMutexesLock.Lock()
	defer alertSourceWriteMutexesLock.Unlock()

	mutex, ok := alertSourceWriteMutexes[sourceID]
	if !ok {
		mutex = &sync.Mutex{}
		alertSourceWriteMutexes[sourceID] = mutex
	}

	return mutex
}

// lockForAlertSource runs fn holding the write lock for sourceID.
//
// Process-wide, so it orders the writes of one apply. Two applies running at once still race,
// and rely on the server's retry as before.
func lockForAlertSource[T any](
	ctx context.Context,
	sourceID string,
	fn func(context.Context) (T, error),
) (T, error) {
	mutex := alertSourceWriteMutex(sourceID)

	mutex.Lock()
	defer mutex.Unlock()

	return fn(ctx)
}
