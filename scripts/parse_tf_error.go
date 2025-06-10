package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/alecthomas/kingpin/v2"
)

func main() {
	app := kingpin.New("parse-tf-error", "Tries to parse Terraform cty error messages").
		Author("terraform-provider-incident")

	inputFile := app.Arg("file", "Error file to parse)").String()
	outputFile := app.Flag("output", "Write output to file instead of stdout").Short('o').String()
	preserveNewlines := app.Flag("preserve-newlines", "Preserve newlines in the output").Short('n').Bool()

	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Read from stdin or file
	var input string
	if *inputFile != "" {
		fmt.Println(inputFile)
		data, err := os.ReadFile(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
		input = string(data)
	} else {
		fmt.Fprintln(os.Stderr, "No input provided. Please provide a file")
		return
	}

	// Clean up pipe characters and extra spaces
	lines := strings.Split(input, "\n")
	var cleanedLines []string
	for _, line := range lines {
		// Remove pipe character and leading/trailing spaces
		line = strings.TrimSpace(strings.TrimPrefix(line, "│"))
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}

	// Join lines with appropriate separator
	var cleaned string
	if *preserveNewlines {
		cleaned = strings.Join(cleanedLines, "\n")
	} else {
		cleaned = strings.Join(cleanedLines, " ")
	}

	// Apply cleanups to simplify the cty syntax
	cleaned = simplify(cleaned)

	// Output the result
	if *outputFile != "" {
		err := os.WriteFile(*outputFile, []byte(cleaned), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println(cleaned)
	}
}

func simplify(input string) string {
	// First, normalize whitespace
	input = regexp.MustCompile(`\s+`).ReplaceAllString(input, " ")

	// Step 1: Remove cty. prefixes and simple expressions
	replacements := []struct {
		pattern string
		replace string
	}{
		// Remove map[string] patterns
		{`map\[string\]cty\.(Value|Type)\{`, "{"},
		// Remove cty prefixes for common types
		{`cty\.ObjectVal\(`, ""},
		{`cty\.ListVal\(`, ""},
		{`cty\.SetVal\(`, ""},
		{`cty\.StringVal\("([^"]*)"\)`, `"$1"`},
		{`cty\.BoolVal\((true|false)\)`, "$1"},
		{`cty\.True`, "true"},
		{`cty\.False`, "false"},
		{`cty\.NullVal\([^)]*\)`, "null"},
		// Remove array declarations
		{`\[\]cty\.Value\{`, "["},
		// Handle bare cty.String and cty.Val (replace with null)
		{`cty\.(String|Val|Bool|Number)(\s|,|})`, "null$2"},
		// Remove type references
		{`cty\.(String|Bool|Number|List|Set|Map|Object)`, ""},
		// Remove Val pattern which is still present
		{`Val\("([^"]*)"\)`, `"$1"`},
		{`Val\(`, ""},
	}

	for _, r := range replacements {
		re := regexp.MustCompile(r.pattern)
		input = re.ReplaceAllString(input, r.replace)
	}

	// Step 2: Clean up parentheses - remove trailing parentheses that break JSON
	// This is the key step to fix the output
	parentheses := []struct {
		pattern string
		replace string
	}{
		{`\)\)`, ")"},
		{`\)\}`, "}"},
		{`\}\)`, "}"},
		{`\]\)`, "]"},
		// Fix broken patterns with parentheses
		{`\(([^():]*)\)`, "$1"},       // Remove simple parentheses around content without colons
		{`\(([^()]*:[^()]*)\)`, "$1"}, // Remove parentheses around key-value pairs
	}

	for _, p := range parentheses {
		re := regexp.MustCompile(p.pattern)
		for re.MatchString(input) { // Keep replacing until no more matches
			input = re.ReplaceAllString(input, p.replace)
		}
	}

	// Step 3: best-effort - remove any remaining unmatched parentheses
	input = strings.ReplaceAll(input, "(", "")
	input = strings.ReplaceAll(input, ")", "")

	return input
}
