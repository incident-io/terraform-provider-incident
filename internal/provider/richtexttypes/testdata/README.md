# `{{ }}` ↔ AST grammar fixtures

Shared cases for the templated-text grammar, read by both this package and the product's own
template parser so the two can't drift.

Each file is a top-level JSON array. Every entry has a `name`, used as the test description.

| File | Entry | Asserts |
|---|---|---|
| `templates.json` | `{template, document, canonical?}` | `ToDocument(template)` equals `document`, and `FromDocument(document)` returns `(canonical ?? template, true)` |
| `documents.json` | `{document, expressible, template?, reason?}` | `FromDocument(document)` returns `(template, expressible)` |
| `invalid.json` | `{template, error}` | `ToDocument(template)` errors, carrying `error`'s slug |

- `canonical` appears only when emission differs from the input — whitespace, filter order, newline
  collapsing.
- `template` is absent when `expressible` is false; `reason` is documentation, not asserted.
- `error` is a stable slug, not message text: `unclosed_variable`, `unknown_filter`,
  `invalid_filter_argument`, `empty_variable_name`, `nested_variable`. Each implementation words its
  own diagnostics.
- `document` is usually the bare ProseMirror doc (`{"type": "doc", …}`), which is the shape we
  *emit*. It is not the only shape `literal` holds: the dashboard's template editor writes the
  `{schema_version, text_node}` envelope, and older rows carry the legacy
  `{root, value_markdown}` one. Both appear here, and both collapse — an
  implementation that only accepts the bare doc gives an imported source a permanent plan diff.
  Key order is presentational — compare canonically.
