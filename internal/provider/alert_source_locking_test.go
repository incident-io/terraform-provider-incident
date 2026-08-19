package provider

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestLockForAlertSourceSerialisesOneSource is the property the lock exists for: Terraform
// applies its resources concurrently, and two attribute writes overlapping on one source is
// what exhausts the server's retry budget.
func TestLockForAlertSourceSerialisesOneSource(t *testing.T) {
	var (
		counting sync.Mutex
		holding  int
		peak     int
		wg       sync.WaitGroup
	)

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_, _ = lockForAlertSource(context.Background(), "01SERIALISED", func(context.Context) (struct{}, error) {
				counting.Lock()
				holding++
				peak = max(peak, holding)
				counting.Unlock()

				time.Sleep(time.Millisecond)

				counting.Lock()
				holding--
				counting.Unlock()

				return struct{}{}, nil
			})
		}()
	}
	wg.Wait()

	if peak != 1 {
		t.Errorf("%d writes to one source ran at once, want them serialised", peak)
	}
}

// TestLockForAlertSourceDoesNotBlockOtherSources covers the keying. A single global lock would
// pass the test above, and would make every alert source in a config wait on every other.
func TestLockForAlertSourceDoesNotBlockOtherSources(t *testing.T) {
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})

	go func() {
		_, _ = lockForAlertSource(context.Background(), "01HOLDER", func(context.Context) (struct{}, error) {
			close(held)
			<-release

			return struct{}{}, nil
		})
	}()

	<-held

	go func() {
		_, _ = lockForAlertSource(context.Background(), "01OTHER", func(context.Context) (struct{}, error) {
			return struct{}{}, nil
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("a write to one source waited on another, so the lock is not keyed per source")
	}

	close(release)
}
