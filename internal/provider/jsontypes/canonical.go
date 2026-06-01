package jsontypes

import (
	"bytes"
	"encoding/json"
	"strings"
)

// canonicalJSON returns a deterministic, escaping-insensitive encoding of the
// given JSON string: object keys sorted lexicographically (json sorts map
// keys), insignificant whitespace removed, and HTML escaping disabled so that
// '>' and '>' (and the '<' / '&' equivalents) collapse to the same bytes.
//
// It is used only for semantic equality comparisons, never for a value stored
// in state. It returns an error if the input is not valid JSON, which callers
// use to decide whether to treat the value as JSON at all.
func canonicalJSON(jsonString string) (string, error) {
	if jsonString == "" {
		return "", nil
	}

	var data any
	if err := json.Unmarshal([]byte(jsonString), &data); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		return "", err
	}

	// Encode appends a trailing newline.
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// JSONStringsEqual reports whether two strings are semantically equal. If both
// parse as valid JSON, their canonical forms are compared (key-sorted and
// escaping-insensitive, so '>' equals '>'). If either is not valid JSON,
// it falls back to exact string equality. This fallback is essential because
// not every value is JSON (a literal can be a plain reference or string).
func JSONStringsEqual(a, b string) bool {
	if a == b {
		return true
	}

	canonicalA, errA := canonicalJSON(a)
	canonicalB, errB := canonicalJSON(b)
	if errA != nil || errB != nil {
		// At least one side isn't JSON: fall back to exact string equality,
		// which we already know is false here.
		return false
	}

	return canonicalA == canonicalB
}
