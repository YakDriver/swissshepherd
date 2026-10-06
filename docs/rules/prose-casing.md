# `prose_casing` rule (design)
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

Status: design, not implemented. Tracks #100.

Flags a standalone word in doc prose that should be cased as an initialism —
`arn` → `ARN`, `account id` → `account ID` — but currently reads with the
wrong case. Schema-independent; can appear anywhere in a doc, the same
scheduling category as `anchors` and `banned_glosses`.

## Why this isn't `banned_glosses`

`GlossRule` (`internal/check/gloss.go`) bans a spelled-out phrase in favor of
its abbreviation: "Amazon Resource Name" → use "ARN". The word is wrong; the
fix is a different, shorter word. This rule corrects the casing of a word
that is already correct — only its case is wrong: "arn" → "ARN". Folding
single-word casing fixes into `banned_glosses`' config (a
`phrase→abbreviation` map keyed by the long form) would need a phrase key
equal to the abbreviation itself, which the gloss semantics don't expect
(gloss always treats the key as the thing to replace, the value as the thing
to show in parens) and would make `GlossRule`'s one regex shape carry two
unrelated find/fix patterns. New rule, not an extension.

## Why this isn't modeled on `AnchorsRule`

The issue proposed modeling this on `anchors`. `AnchorsRule` is scheduling-wise
the right comparison (schema-independent, runs over the whole doc) but
implementation-wise the wrong template: it's a `Rule` that reads the already
-parsed `ctx.Doc` (heading anchors, in-page links — structures the parser
already extracted). A casing check needs raw line text and byte offsets to
mask out code spans, fences, and URLs before scanning, which is exactly what
`FileRule` + `ctx.Content` gives `GlossRule`. This rule is a `FileRule` built
the same way `GlossRule` is, not a `Rule` built the way `AnchorsRule` is.

## Config shape

```hcl
check "prose_casing" {
  enabled  = true
  severity = "warning"

  # Merges with the built-in default list (see below). A provider entry
  # for a word the default list already has overrides the default's casing
  # for that word; it does not duplicate it.
  casing = {
    "db"  = "DB"
    "sfn" = "SFN"
  }

  # Entries here are removed from the merged list (default ∪ casing) before
  # the check runs. Use for a word the default list gets wrong for this
  # provider, or a specific case this provider accepts.
  ignore_words = ["ami"]

  # Same scoping fields every other check has.
  ignore_targets = ["aws_quicksight_group"]
  skip_frontmatter = true
}
```

- `Casing map[string]string` (`hcl:"casing,optional"`) — provider-supplied
  additions/overrides to the default list, same `wrong→right` shape as
  `names/caps.csv`'s two columns, but — unlike `banned_glosses`, which ships
  with no default and does nothing until configured — this one ships with a
  small built-in default (below) and `Casing` only adds to or overrides it.
  This is the "balance of a good default list that merges with the
  provider's configured list" asked for: zero-config gets the obvious,
  low-risk initialisms; a provider adds the ones specific to its own domain
  (service-name abbreviations, internal product terms) without forking the
  whole list.
- `IgnoreWords []string` (`hcl:"ignore_words,optional"`) — the standard
  ignore mechanic, but at word granularity rather than target granularity.
  Removes entries from the merged map before compiling, letting a provider
  turn off one noisy word without disabling the whole check or losing the
  rest of the default list. This is independent of `IgnoreTargets`
  (`CheckConfig.AppliesTo`), which still applies normally for excluding
  whole files/resources.
- Reuses `CheckConfig`'s existing `Severity`, `SkipFrontmatter`,
  `IgnoreTargets`/`IgnoreTargetsFile`, `IgnorePrefixes` — no new scoping
  mechanism needed.

### Default list: general vs. specific

The issue flagged that `caps.csv`'s 125 entries split into two kinds, and
that split is the right axis for deciding what ships as the zero-config
default versus what a provider must opt into:

- **General, low-risk initialisms** — short, common, standalone English
  acronyms where the lowercase form is rarely a legitimate word on its own
  in Terraform docs prose: `api`, `arn`, `cpu`, `dns`, `http`, `https`,
  `json`, `kms`, `sql`, `ssl`, `tls`, `url`, `vpc`, `xml`, `yaml`. These ship
  as the built-in default — the "good default list" — because the false
  -positive rate for a bare `\bvpc\b` or `\barn\b` match in prose is low
  once code spans/URLs are masked out, and getting these right is most of
  the value in #100's original "account id" / `arn`/`vpc`/`json` complaint.
- **Specific, provider/domain compounds** — AWS service names and
  product-specific terms (`AcmPca`, `DynamoDB`, `ElastiCache`, `GameLift`,
  `OpsWorks`, `SageMaker`, `WorkSpaces`...). These are mostly multi-morpheme
  product names, not generic words; whether a given doc should say
  "elasticache" or "ElastiCache" depends on whether it's naming the AWS
  service (capitalize) or something else (rare, but possible). Lower
  confidence, provider-specific — these are exactly what `Casing` lets a
  provider opt into deliberately, not what ships by default.
- **`id` is neither list.** It is deliberately excluded from the built-in
  default for the same reason `caps.csv` excludes it: "id" collides with
  ordinary English inside other words ("avoid", "paid", "valid", "liquid"),
  so a bare word-boundary match on "id" has a materially higher
  false-positive rate than any other candidate entry, default or
  provider-supplied. A provider that wants `id` → `ID` enforced opts in
  explicitly via `casing = { id = "ID" }`, accepting that risk themselves,
  rather than it firing on every provider the day this check ships.

The default list is a Go literal in the check package (`internal/check/`),
not generated from `names/caps.csv` and not embedded HCL like
`config/defaults.hcl`'s types — it is swissshepherd's own curated list, open
to revision via normal PRs, independent of what any one provider's
`caps.csv` happens to contain. Pulling from `caps.csv` directly was the
original issue's suggestion; rejected per the issue's own finding that most
of that file's entries are Go-identifier artifacts (`Acl`, `Dnssec`,
`Microvms`) that are not English prose words at all.

## Matching

Same masking pipeline as `GlossRule`, reused as-is (`isFenceDelimiter`,
`frontmatterEnd`, `maskUnscannable`, the cheap combined-gate-regex-before-
per-entry-scan structure) — skip frontmatter (if configured), fenced code
blocks, inline code spans, markdown link targets, autolinks, bare URLs.

Per entry, match the wrong-cased word as a whole word
(`\b` + `regexp.QuoteMeta(wrong)` + `\b`), case-sensitively against the
**exact** wrong casing, not case-insensitively against any casing that isn't
already right:

- Match the literal lowercase (or otherwise-wrong) form only — `arn`, not
  `Arn`/`ARN`/`aRn`. The right-cased form must never be flagged; this is a
  correction rule, not a "this word must always look like X" rule.
- Word-boundary on both sides (`\b...\b`), so `arn` doesn't match inside
  `tarn` or `yarn`, and `vpc` doesn't match inside a longer identifier
  fragment that masking didn't already remove.
- No case-insensitive prefix handling like `GlossRule`'s optional "Amazon
  "/"AWS " — that's a phrase-banning concern. A casing entry matches exactly
  one literal string.
- Plurals (`arns`, `vpcs`) are a separate, explicit entries question, not
  automatic pluralization like `GlossRule.recommend` — see Open questions.

### Never-guess boundary

Per `AGENTS.md`'s invariant, when a match is structurally ambiguous, skip
it rather than report speculatively. Concretely: a matched word immediately
preceded by a backtick, or immediately followed by an underscore or another
word character that word-boundary matching alone wouldn't already exclude
(e.g., defensive double-checks around Markdown emphasis markers `*`/`_` that
can abut a word without whitespace), is not reported. This rule has no
schema to consult, so "never guess" here means "never flag a match that the
masking pipeline might have mishandled," not a schema-shaped exclusion.

## Open questions

1. **Plurals/possessives.** Does `arns`/`ARNs`/`VPC's` need its own entries,
   or a `GlossRule.recommend`-style automatic suffix rule? Leaning toward
   requiring explicit entries (`"arns" = "ARNs"`) in the default list where
   the plural is common, rather than inferring it — avoids guessing at
   possessive apostrophes and keeps the matching fully literal.
2. **`id` default-off, but how is "default-off" expressed?** Either (a) `id`
   simply isn't in the built-in default map, so it's absent unless a
   provider adds it via `casing`, or (b) it's present but the check carries
   a separate `enable_id bool` escape hatch. Leaning toward (a) — no special
   flag, just absence — consistent with "no new scoping mechanism" and with
   how `caps.csv` itself handles exclusion (by omission, with a comment).
3. **Where does the default list live if it needs its own tests/updates
   independent of a provider's caps.csv?** Proposed: a Go map literal in
   `internal/check/prose_casing.go`, documented inline, PR-reviewable like
   any other code — not generated, not shared with terraform-provider-aws's
   `names/caps.csv` machinery (that generator is Go-identifier-specific and
   explicitly out of scope per the original issue triage).
4. **Severity default.** Proposed `warning`, matching `AGENTS.md`'s "new
   checks that add many findings ship opt-in, or as warning" — but unlike
   `banned_glosses`, this check is **not** opt-in by presence of config (it
   has a non-empty default list), so it needs `enabled` to default to
   `false`, or a conscious decision that it defaults to `true` with
   `severity = "warning"` and an empty corpus run before deciding. Needs a
   corpus run against terraform-provider-aws before deciding which.
5. **Corpus validation required before implementation**, per `AGENTS.md`: this
   change can alter findings, so a real `make corpus` run against
   terraform-provider-aws's `website/docs/` is required once an
   implementation exists, to get real added/removed counts and spot-check
   false positives — the earlier naive Python scan in this design's research
   phase (thousands of raw hits for `https`, `arn`, etc.) was not corpus
   -validated and should not be cited as a finding count in any PR.
