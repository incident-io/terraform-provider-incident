package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A 2xx the generated client can't parse leaves the typed body (JSON200, JSON201...) nil
// while returning no error. Every caller dereferences that field, so without this the
// provider panics and the plugin crashes.
func TestRequireJSONBody(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		body        string
		wantErr     string
	}{
		{
			name:        "JSON body is parsed as usual",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        `{"catalog_type":{"id":"01ABC"}}`,
		},
		{
			// What a proxy or sign-in portal does to an intercepted request.
			name:        "HTML body on a 200 is an error, not a nil dereference",
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body:        "<html><body>Authentication required</body></html>",
			wantErr:     "expected a JSON response body",
		},
		{
			name:        "plain text body on a 201 is an error",
			status:      http.StatusCreated,
			contentType: "text/plain",
			body:        "created",
			wantErr:     "expected a JSON response body",
		},
		{
			// The schema has 44 success responses that declare no body, all 204s from a
			// delete. Those callers never touch the typed field, so they must keep working.
			name:   "empty body with no content type is fine",
			status: http.StatusNoContent,
			body:   "",
		},
		{
			name:        "whitespace-only body is treated as empty",
			status:      http.StatusNoContent,
			contentType: "text/plain",
			body:        "  \n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c, err := New(context.Background(), "inc_test_key", srv.URL, "test")
			require.NoError(t, err)

			// Any endpoint will do: we're exercising the transport, not the operation.
			_, err = c.CatalogV3ListTypesWithResponse(context.Background())

			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			// The body has to reach the user, or there's nothing to debug from.
			assert.Contains(t, err.Error(), strings.TrimSpace(tc.body))
			assert.Contains(t, err.Error(), tc.contentType)
		})
	}
}

// The body is consumed to inspect it, so it has to be put back for the caller to parse.
func TestRequireJSONBodyLeavesTheBodyReadable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"catalog_types":[],"pagination_meta":{"page_size":25}}`))
	}))
	defer srv.Close()

	c, err := New(context.Background(), "inc_test_key", srv.URL, "test")
	require.NoError(t, err)

	result, err := c.CatalogV3ListTypesWithResponse(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result.JSON200, "the typed body should be populated")
	assert.Empty(t, result.JSON200.CatalogTypes)
}

func TestIsJSONContentType(t *testing.T) {
	assert.True(t, isJSONContentType("application/json"))
	assert.True(t, isJSONContentType("application/json; charset=utf-8"))
	assert.True(t, isJSONContentType("application/problem+json"))
	assert.False(t, isJSONContentType("text/html"))
	assert.False(t, isJSONContentType(""))
}
