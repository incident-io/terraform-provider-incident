package provider

import (
	"context"
	"sync"
)

// Global mutex for alert config locking.
var alertConfigMutex sync.Mutex

// lockForAlertConfig ensures that only one alert-related operation can proceed at a time.
//
// Since all alert operations (both sources and attributes) affect the same global
// alert config (for now), we use a single global lock for all operations.
func lockForAlertConfig[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	alertConfigMutex.Lock()
	defer alertConfigMutex.Unlock()

	return fn(ctx)
}
