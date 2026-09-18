# Patterns: avoid → prefer

Four failure modes, each with what to write instead and where the cut detail
belongs.

## 1. Churn on unshipped work → one entry for the shipped result

A resource built over several PRs should read as the single thing a consumer
meets in the release, not the path taken to build it. Intermediate steps —
an attribute added then removed, a behaviour tweaked before it ever shipped —
are invisible to the consumer, so they collapse into the one entry (or vanish
entirely). The syntax and behaviour detail lives on the generated docs page.

**Avoid** — the build story, one bullet per step:
> - Add the `incident_escalation_path_template` resource… [140 words on syntax]
> - `incident_escalation_path_template` no longer offers `default_value`…
> - `incident_escalation_path_template` now claims the template it manages…
> - A template with no `params` is rejected when you plan…
> - `incident_escalation_path_template` checks the paths built from it…

**Prefer** — the net result, once:
> - Add the `incident_escalation_path_template` resource and data source, which manage
>   a parameterised escalation path that many paths build from. See the [docs](…). (#589)

## 2. Reproducing the docs → name it and link

When an entry starts explaining how an attribute behaves, its allowed values, or
its rotation/import semantics, it is duplicating the generated docs page. Name the
change, give the one fact a consumer needs to get started, and link the rest.

**Avoid:**
> - Add the `incident_api_key` resource, which manages an API key: the credential an
>   integration authenticates with… [250 words on tokens, rotation, grace periods,
>   import, role checks]

**Prefer:**
> - Add the `incident_api_key` resource and data source, for managing API keys and
>   their account- and team-scoped roles. Tokens are rotated by bumping
>   `token_version`; see the [docs](…). (#123)

## 3. Internal rationale → one `Internal:` line

Tests, CI, and refactors have no consumer-visible effect, so they do not warrant a
paragraph justifying them. Record them in a single line if at all; the reasoning
belongs in the code or the PR.

**Avoid:**
> - Cover the v6→v7 upgrade with acceptance tests, for the half of the migration
>   nothing exercised… [230 words explaining why and how]

**Prefer:**
> - Internal: cover the v6→v7 state-carryover with acceptance tests. (#123)

## 4. Explaining the mechanism → stating the impact

A consumer needs to know what they can now do, or which symptom is fixed — not how
the API or the provider works inside. Keep the impact, cut the walk-through.

**Avoid:**
> - Accept `escalation_config.when_alert_joins_group` on an `incident_alert_route`
>   whose `grouping_config.default.enabled` is false, and keep the value the API
>   returns… [explains the API's grouping model and the old plan-time rejection]

**Prefer:**
> - Fix `incident_alert_route` showing a perpetual diff on
>   `escalation_config.when_alert_joins_group` when the route's own grouping is
>   disabled, so a route works alongside a team grouping preference. (#590)
