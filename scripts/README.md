## parse_tf_error.go

A simple tool for best-effort cleaning Terraform cty error messages to make them more readable and easier to debug.
In all likelihood this won't produce valid JSON, but it will make the error message more readable.

### Usage

```bash
# Parse an error from a file
go run scripts/parse_tf_error.go error_file.txt

# Preserve newlines in the output
go run scripts/parse_tf_error.go -n error_file.txt

# Write output to a file
go run scripts/parse_tf_error.go -o cleaned.txt error_file.txt
```

### What it does

- Removes pipe characters (│) and normalizes whitespace
- Cleans up cty syntax:
  - Converts `cty.StringVal("abc")` to `"abc"`
  - Converts `cty.BoolVal(true)` to `true`
  - Converts `cty.NullVal()` to `null`
  - Replaces bare `cty.String` and similar types with `null`
- Removes parentheses that break JSON format
- Produces a cleaner, more readable error message

This is particularly useful for parsing "Provider produced inconsistent result after apply" errors that contain complex cty structures.