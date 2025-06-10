package client

import (
	"encoding/json"
	"fmt"
)

type HTTPError struct {
	StatusCode int
	Body       []byte
}

// Error returns a string representation of a failed HTTP request. It will try
// to present the response as pretty-printed JSON first, and will fall back to
// showing the response as a string if that's not possible.
func (e HTTPError) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("status %d: empty response body", e.StatusCode)
	}

	if jsonStr, err := e.bodyAsJSON(); err == nil {
		return fmt.Sprintf("status %d:\n\n%s", e.StatusCode, string(jsonStr))
	}

	return fmt.Sprintf("status %d: %s", e.StatusCode, string(e.Body))
}

// bodyAsJSON attempts to format the HTTP response body as pretty-printed JSON.
func (e HTTPError) bodyAsJSON() ([]byte, error) {
	var jsonData any

	err := json.Unmarshal(e.Body, &jsonData)
	if err != nil {
		return nil, err
	}

	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return nil, err
	}

	return prettyJSON, nil
}
