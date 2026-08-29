package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/incident-io/terraform-provider-incident/v6/internal/client"
)

func TestAPIErrorWithStatus(t *testing.T) {
	notFound := client.HTTPError{StatusCode: http.StatusNotFound, Body: []byte(`{"message":"gone"}`)}

	for _, tc := range []struct {
		name       string
		err        error
		statusCode int
		want       bool
	}{
		{"matching status", notFound, http.StatusNotFound, true},
		{"different status", notFound, http.StatusConflict, false},
		{"nil error", nil, http.StatusNotFound, false},
		{"not an API error", fmt.Errorf("dial tcp: connection refused"), http.StatusNotFound, false},
		// The client wraps errors on the way out, so unwrapping has to work.
		{"wrapped with pkg/errors", errors.Wrap(notFound, "reading"), http.StatusNotFound, true},
		{"wrapped with fmt", fmt.Errorf("reading: %w", notFound), http.StatusNotFound, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := apiErrorWithStatus(tc.err, tc.statusCode)

			assert.Equal(t, tc.want, ok)
			if tc.want {
				// The caller needs the body to build a diagnostic from.
				assert.Contains(t, got.Error(), "gone")
			}
		})
	}
}

func TestIsNotFoundAndIsConflict(t *testing.T) {
	assert.True(t, isNotFound(client.HTTPError{StatusCode: http.StatusNotFound}))
	assert.False(t, isNotFound(client.HTTPError{StatusCode: http.StatusConflict}))
	assert.False(t, isNotFound(nil))

	assert.True(t, isConflict(client.HTTPError{StatusCode: http.StatusConflict}))
	assert.False(t, isConflict(client.HTTPError{StatusCode: http.StatusNotFound}))
}
