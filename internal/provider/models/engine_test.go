package models

import (
	"strings"
	"testing"
)

func TestNormaliseJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "simple json object",
			input:    `{"b":2,"a":1}`,
			expected: `{"a":1,"b":2}`,
			wantErr:  false,
		},
		{
			name:     "json with HTML characters",
			input:    `{"label":"Alert -> Title","other":"<test> & stuff"}`,
			expected: `{"label":"Alert -> Title","other":"<test> & stuff"}`,
			wantErr:  false,
		},
		{
			name:     "nested json with HTML characters",
			input:    `{"attrs":{"label":"Alert -> Title","name":"alert.title"},"type":"varSpec"}`,
			expected: `{"attrs":{"label":"Alert -> Title","name":"alert.title"},"type":"varSpec"}`,
			wantErr:  false,
		},
		{
			name:     "json with escaped unicode",
			input:    `{"label":"Alert -\u003e Title"}`,
			expected: `{"label":"Alert -> Title"}`,
			wantErr:  false,
		},
		{
			name:     "complex nested structure",
			input:    `{"content":[{"content":[{"attrs":{"label":"Alert -> Title","missing":false,"name":"alert.title"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`,
			expected: `{"content":[{"content":[{"attrs":{"label":"Alert -> Title","missing":false,"name":"alert.title"},"type":"varSpec"}],"type":"paragraph"}],"type":"doc"}`,
			wantErr:  false,
		},
		{
			name:     "json with various special HTML characters",
			input:    `{"test":"<>&'\"","label":"Alert -> Title"}`,
			expected: `{"label":"Alert -> Title","test":"<>&'\""}`,
			wantErr:  false,
		},
		{
			name:     "invalid json",
			input:    `{invalid json}`,
			expected: "",
			wantErr:  true,
		},
		{
			name:     "json with newlines and formatting",
			input:    "{\n  \"b\": 2,\n  \"a\": 1\n}",
			expected: `{"a":1,"b":2}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normaliseJSON(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("normaliseJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("normaliseJSON() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNormaliseJSON_PreservesHTMLCharacters(t *testing.T) {
	// This test specifically validates that HTML characters are not escaped
	// which is critical for the alert route incident template issue
	input := `{"attrs":{"label":"Alert -> Title","description":"Test <summary> with & special > characters"}}`
	
	result, err := normaliseJSON(input)
	if err != nil {
		t.Fatalf("normaliseJSON() unexpected error: %v", err)
	}
	
	// Ensure the result contains the actual characters, not escaped versions
	if !strings.Contains(result, `"Alert -> Title"`) {
		t.Errorf("Expected result to contain %s, but got: %s", `"Alert -> Title"`, result)
	}
	
	if !strings.Contains(result, `"Test <summary> with & special > characters"`) {
		t.Errorf("Expected result to contain %s, but got: %s", `"Test <summary> with & special > characters"`, result)
	}
	
	// Ensure HTML escape sequences are NOT present
	escapeSequences := []string{`\u003e`, `\u003c`, `\u0026`}
	for _, seq := range escapeSequences {
		if strings.Contains(result, seq) {
			t.Errorf("Result should not contain HTML escape sequence %s, but got: %s", seq, result)
		}
	}
}