---
name: changelog-entry
description: >-
  Write or review a CHANGELOG.md entry for the incident.io Terraform provider.
  Use whenever adding a bullet under `## Unreleased`, editing an existing entry,
  or reviewing a PR's changelog. Produces short, scannable entries — one per
  consumer-visible change, not per PR — that name the resource or data source
  affected, end with the PR number, and tell a provider consumer what changed and
  what they must do, pushing full detail to the docs. Trigger when editing
  CHANGELOG.md, when a change adds/changes/removes a resource, data source,
  attribute or behaviour, or when asked to "write a changelog entry", "tidy the
  changelog", or "is this changelog bullet ok".
---

# Writing changelog entries

The changelog is read by **consumers of the provider**, scanning to answer two
questions: *what changed*, and *do I have to do anything*. Write for that reader.
It is not a place to explain how the change was built or why — that is what commit
messages, PR descriptions, and code comments are for.

Entries go as a flat bullet list under `## Unreleased` at the top of
`CHANGELOG.md` (create the heading if it is missing).

## The rules

1. **One entry per consumer-visible change, not per PR.** The unit is a change a
   consumer sees, not a merged pull request. A feature built over five PRs is one
   entry; a PR that fixes three unrelated bugs is three. Crucially, **a change that
   only modifies other still-unreleased work collapses into it** — if an attribute
   was added and then removed before any release, or a not-yet-shipped resource had
   its behaviour tweaked, the consumer meeting it in the next release never saw the
   churn, so it gets no entry of its own. (Full per-PR traceability already lives in
   GitHub's auto-generated release notes; the changelog is the curated layer on
   top.)

2. **Name the resource or data source, in backticks.** Every entry identifies what
   it touches: `incident_api_key`, `incident_alert_route`, the `incident_status`
   data source, or "the provider" for provider-wide config. A reader greps the
   changelog for the resource they use — an entry that doesn't name one is invisible
   to them.

3. **Lead with an imperative verb**: Add, Fix, Remove, Deprecate, Accept, Reject,
   Require, Document. Say what changed, then the consumer impact — what now works,
   what breaks, or what they must change. Sentence case, no bullet-internal
   headings.

4. **End with the PR number(s)** as `(#123)`, or `(#123, #124, #125)` when the one
   change was built over several PRs — still one bullet, but linking every PR so a
   reader can reach them all.

5. **One to two sentences.** If you are writing a third, the detail belongs in the
   docs. A changelog entry *names* a change and *points* to where the detail lives;
   it does not reproduce it. See "Where detail belongs" below.

6. **Mark breaking changes** with a bold `**Breaking**:` prefix, and say what a
   consumer must do to upgrade (or link the migration guide).

7. **Link, don't inline.** When a change needs real explanation — rotation
   semantics, a migration path, a filter syntax — link the registry docs page or
   the guide and stop. Do not paste schema descriptions or examples into the
   changelog; they already live in the generated docs.

8. **Internal work gets an inline `Internal:` entry, kept terse.** Tests, lint
   rules, refactors, CI, dependency bumps — anything with no observable change to
   the provider's behaviour — go in the same list, one line each, prefixed
   `Internal:`, with the PR number. Keep the *why* in the code/PR, not here. Note
   this is distinct from rule 1's collapse: churn on unshipped work is not "internal
   work", it is *nothing*, and gets no line at all.

## Where detail belongs, not the changelog

Before expanding an entry, check whether the detail already has a home — it almost
always does, and the changelog should point there rather than duplicate it:

- **How an attribute behaves, its allowed values, rotation/import semantics** →
  the schema `Description` on the attribute, which generates the registry docs
  page (`docs/resources/*.md`, `docs/data-sources/*.md`). Improve the description;
  the changelog just says "Add `x`, see docs".
- **How to write the resource** → the example under
  `examples/resources/<name>/resource.tf`.
- **Upgrade/migration steps** → the relevant guide under `docs/guides/` (e.g. the
  migrating-to-vN guide). Link it.
- **Why the change was made / how it was implemented / test rationale** → the
  commit message, PR description, or a code comment next to the code. Never the
  changelog.

## Recipes

- New resource or data source:
  `Add the \`incident_x\` resource, which manages <one line>. See the [docs](…). (#123)`
- New attribute:
  `Add \`attr\` to \`incident_x\`, which <what it does for the consumer>. (#123)`
- Bug fix:
  `Fix \`incident_x\` <symptom the consumer saw>. (#123)`
- Breaking:
  `**Breaking**: \`incident_x\` <what changed>. To upgrade, <action> — see [the guide](…). (#123)`
- Internal:
  `Internal: <what changed>. (#123)`

## Examples

`references/examples.md` has real before/after rewrites drawn from this
changelog's own over-long entries. Read it when you want a worked example of
cutting a paragraph-length bullet down, collapsing a feature's unshipped churn
into one entry, or are unsure how much detail to keep.
