//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -package=client -generate types,client -o=client.gen.go ../apischema/public-schema-v3-including-secret-endpoints.json
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/securityprovider"
	"github.com/pkg/errors"
)

func New(ctx context.Context, apiKey, apiEndpoint, version string, opts ...ClientOption) (*ClientWithResponses, error) {
	bearerTokenProvider, bearerTokenProviderErr := securityprovider.NewSecurityProviderBearerToken(apiKey)
	if bearerTokenProviderErr != nil {
		return nil, bearerTokenProviderErr
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = maxRetries
	retryClient.Backoff = attentiveBackoff

	base := retryClient.StandardClient()

	// The generated client won't turn validation errors into actual errors, so we do this
	// inside of a generic middleware.
	base.Transport = Wrap(base.Transport, func(req *http.Request, next http.RoundTripper) (*http.Response, error) {
		start := time.Now()

		resp, err := next.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		tflog.Debug(ctx, "Received API response", map[string]any{
			"method":      req.Method,
			"url":         req.URL.String(),
			"path":        req.URL.Path,
			"status_code": resp.StatusCode,
			"duration":    time.Since(start),
		})

		if resp.StatusCode > 299 {
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("status %d: no response body", resp.StatusCode)
			}

			return nil, HTTPError{
				Body:       data,
				StatusCode: resp.StatusCode,
			}
		}

		if err := requireJSONBody(resp); err != nil {
			return nil, err
		}

		return resp, err
	})

	clientOpts := append([]ClientOption{
		WithHTTPClient(base),
		WithRequestEditorFn(bearerTokenProvider.Intercept),
		// Add a user-agent so we can tell which version these requests came from.
		WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			req.Header.Add("user-agent", fmt.Sprintf("terraform-provider-incident/%s", version))
			return nil
		}),
	}, opts...)

	client, err := NewClientWithResponses(apiEndpoint, clientOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "creating client")
	}

	return client, nil
}

// requireJSONBody rejects a success response carrying a body the generated client won't
// parse, which is any body that isn't JSON.
//
// The generated Parse functions only populate the typed body (JSON200, JSON201...) when the
// status matches and the content type contains "json". Otherwise they return the response
// with every typed field nil and no error, and every caller dereferences that field — so the
// provider panics and the plugin crashes, rather than reporting anything useful.
//
// The API schema only ever describes JSON response bodies, so a non-JSON body on a 2xx means
// something between us and the API rewrote the response: a proxy's HTML error or sign-in
// page is the usual culprit. Surfacing that beats a nil dereference.
//
// An empty body is left alone: the schema has 44 success responses that declare no body at
// all, all of them 204s from a delete, and those callers don't touch the typed field.
func requireJSONBody(resp *http.Response) error {
	if isJSONContentType(resp.Header.Get("Content-Type")) {
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("status %d: reading response body: %w", resp.StatusCode, err)
	}

	// Put the body back for the caller, which still has to parse it.
	resp.Body = io.NopCloser(bytes.NewReader(data))

	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}

	return fmt.Errorf(
		"status %d: expected a JSON response body but the content type was %q, so the response could not be read. This usually means something between you and the incident.io API rewrote the response, such as a proxy returning an error page. Body: %s",
		resp.StatusCode, resp.Header.Get("Content-Type"), truncateForError(data))
}

// isJSONContentType matches the test the generated client uses to decide whether to parse a
// body, so the two can't disagree about what counts as JSON.
func isJSONContentType(contentType string) bool {
	return strings.Contains(contentType, "json")
}

// truncateForError keeps an unexpected body short enough to put in a diagnostic.
func truncateForError(data []byte) string {
	const limit = 512
	if len(data) <= limit {
		return string(data)
	}

	return string(data[:limit]) + "... (truncated)"
}

const (
	maxRetries = 10
)

func attentiveBackoff(minDuration, maxDuration time.Duration, attemptNum int, resp *http.Response) time.Duration {
	// Retry for rate limits and server errors.
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		// Check for a 'Retry-After' header.
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			retryAfterDate, err := time.Parse(time.RFC1123, retryAfter)
			if err != nil {
				// If we can't parse the Retry-After, lets just wait for 10 seconds
				return 10 * time.Second
			}

			timeToWait := time.Until(retryAfterDate)

			if timeToWait < 1*time.Second {
				// by default lets back off at least 1 second
				return 1 * time.Second
			}

			return timeToWait
		}

	}
	// otherwise use the default backoff
	return retryablehttp.DefaultBackoff(minDuration, maxDuration, attemptNum, resp)
}

// WithReadOnly restricts the client to GET requests only, useful when creating a client
// for the purpose of dry-running.
func WithReadOnly() ClientOption {
	return WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
		if req.Method != http.MethodGet {
			return fmt.Errorf("read-only client tried to make mutating request: %s %s", req.Method, req.URL.String())
		}

		return nil
	})
}

// RoundTripperFunc wraps a function to implement the RoundTripper interface, allowing
// easy wrapping of existing round-trippers.
type RoundTripperFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Wrap allows easy wrapping of an existing RoundTripper with a function that can
// optionally call the original, or do its own thing.
func Wrap(next http.RoundTripper, apply func(req *http.Request, next http.RoundTripper) (*http.Response, error)) http.RoundTripper {
	return RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return apply(req, next)
	})
}
