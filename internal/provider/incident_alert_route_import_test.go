package provider

import (
	"errors"
	"testing"

	"github.com/incident-io/terraform-provider-incident/internal/client"
)

// TestIsAPINotYetAvailable covers the detector that drives the import
// probe-and-fallback: only a v3 API response carrying the
// `api_not_yet_available` error code should trigger the v2 fallback; every
// other error must be surfaced.
func TestIsAPINotYetAvailable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "api_not_yet_available in errors array",
			err:  client.HTTPError{StatusCode: 403, Body: []byte(`{"type":"resource_forbidden","status":403,"errors":[{"code":"api_not_yet_available","message":"not available"}]}`)},
			want: true,
		},
		{
			name: "different forbidden code",
			err:  client.HTTPError{StatusCode: 403, Body: []byte(`{"errors":[{"code":"missing_permissions"}]}`)},
			want: false,
		},
		{
			name: "not found",
			err:  client.HTTPError{StatusCode: 404, Body: []byte(`{"errors":[{"code":"not_found"}]}`)},
			want: false,
		},
		{
			name: "non-http error",
			err:  errors.New("boom"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAPINotYetAvailable(tc.err); got != tc.want {
				t.Errorf("isAPINotYetAvailable = %v, want %v", got, tc.want)
			}
		})
	}
}
