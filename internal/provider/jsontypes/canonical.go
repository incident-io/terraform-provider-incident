package jsontypes

import (
	"bytes"
	"encoding/json"
	"strings"
)

// NormaliseJSON returns a deterministic encoding of the given JSON string with
// object keys sorted lexicographically (json.Marshal sorts map keys) and
// insignificant whitespace removed. HTML escaping is left at Go's default
// (enabled), so '>', '<' and '&' are emitted as '>', '<' and '&'.
//
// This is the form written into Terraform state when the API returns a literal.
// HTML escaping is kept ON deliberately: it matches what HCL's jsonencode
// produces, so the stored state stays byte-stable for the dominant user
// population (and for ImportStateVerify, which compares state byte-for-byte).
// Semantic equality between differently-escaped literals is handled separately
// by canonicalJSON / JSONStringsEqual, so raw-string users no longer hit
// inconsistent-result errors either.
//
// It returns an error if the input is not valid JSON, which callers use to
// decide whether to treat the value as JSON at all.
func NormaliseJSON(jsonString string) (string, error) {
	return encodeCanonical(jsonString, true)
}

// canonicalJSON returns an escaping-insensitive canonical form: keys sorted,
// whitespace removed, and HTML escaping disabled so that '>' and '>' (and the
// '<' / '&' equivalents) collapse to the same bytes. It is only used for
// semantic equality comparisons, never for the value stored in state.
func canonicalJSON(jsonString string) (string, error) {
	return encodeCanonical(jsonString, false)
}

func encodeCanonical(jsonString string, escapeHTML bool) (string, error) {
	if jsonString == "" {
		return "", nil
	}

	var data any
	if err := json.Unmarshal([]byte(jsonString), &data); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(escapeHTML)
	if err := encoder.Encode(data); err != nil {
		return "", err
	}

	// Encode appends a trailing newline.
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// JSONStringsEqual reports whether two strings are semantically equal. If both
// parse as valid JSON, their canonical forms are compared (key-sorted and
// escaping-insensitive, so '>' equals '>'). If either is not valid JSON, it
// falls back to exact string equality. This fallback is essential because not
// every value is JSON (a literal can be a plain reference or string).
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
