This repository contains the source code for the incident.io Terraform provider. People might make changes to:
- Implement a new resource 
- Fix a bug reported to the provider

See also CONTRIBUTING.md

# Development guidelines

## Available libraries

- **`lo` (samber/lo)** - Generic utility library for Go. Use for common helpers like `lo.Chunk`, `lo.Map`, `lo.ToPtr`, `lo.Filter`, etc. Prefer this over writing custom helpers.

## Resources

If you're developing a new resource:
- DEVELOPING.md has development guidelines about how to best write your resource schema
- The repository follows a structure of:
  - `internal/provider/` - Resource and data source implementations
  - `internal/provider/models/` - Go models for API objects
  - `internal/client/` - Generated API client
  - `examples/` - Terraform configuration examples

# Debugging 

## Potential fixes for common issues

- **Use sets over lists where possible** - Sets handle ordering differences better
- **ValidateConfig can over-validate** - Be careful with dynamic attributes; validation may fail when attributes are unknown during planning
- **Provider produced inconsistent result** - Often issues with `Computed` attributes. Also watch out for API responses that return nested objects even when the user didn't configure them (e.g. `EmailOptions` always returned for email sources because it contains `email_address`). The `FromAPI` method should return nil when no user-configurable data is present.
- **Adding source-type-specific options** - Follow the pattern: add model struct with `FromAPI`/`ToPayload` in `models/`, add schema block + validation in resource, wire into create/update payloads. Validation goes in `ValidateConfig` to check options match `source_type`. Tests use `testRunTemplate` for Go template-based Terraform configs.
- **Complex error messages** - Use `go run scripts/parse_tf_error.go error_file.txt` to clean up cty errors