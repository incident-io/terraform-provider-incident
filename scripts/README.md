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

## docsfilenames

Runs from `go generate` straight after `tfplugindocs`, and renames the docs page
of any resource whose name already starts with `incident_` so the filename is
the full resource name: `docs/resources/incident_role.md` becomes
`docs/resources/incident_incident_role.md`.

tfplugindocs strips a single `incident_` when naming pages. The Terraform
Registry sorts its docs nav by that filename but labels each entry with the
resource name, adding `incident_` back only when the filename doesn't already
start with it - so `incident_incident_role` was labelled `incident_role`, which
isn't a resource we have, and sorted under "i" between `escalation_path` and
`policy`. Naming the page after the resource fixes both.

### Usage

It only needs running as part of docs generation:

```bash
go generate ./...
```
