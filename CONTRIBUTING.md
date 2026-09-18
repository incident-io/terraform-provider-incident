# Contributing

## CLA

We're open to contributions! If you open a PR we'll get in touch with a copy
of our Contributor License Agreement.

## Changelog

If your change is something a provider consumer would notice — a new resource
or data source, a new attribute, a behaviour change, a bug fix — add an entry
under `## Unreleased` at the top of `CHANGELOG.md`. The house style:

- **One entry per consumer-visible change, not per PR.** A feature built over
  several PRs is one entry; collapse work that only modifies still-unreleased
  changes into them.
- **Name the resource or data source in backticks** (`incident_alert_route`),
  so readers can grep for what they use.
- **Lead with an imperative verb** (Add, Fix, Remove, Deprecate…), then the
  impact on the consumer. One or two sentences — link the docs for the detail
  rather than reproducing it. Prefix breaking changes with `**Breaking**:`.
- **End with the PR number**, e.g. `(#123)`, or `(#123, #124)` when one change
  spanned several PRs.
- **Internal-only work** (tests, CI, refactors) gets a terse line prefixed
  `Internal:`, if it needs one at all.

The `changelog-entry` skill in `skills/changelog-entry/` is the canonical
version of this style, with worked before/after examples.