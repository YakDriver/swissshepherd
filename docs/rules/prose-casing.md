# `prose_casing` rule (design)
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

Status: design, not implemented. Tracks #100.

Flags a word in doc prose that should be cased as an initialism or
AWS-style mixed-case term — `id`/`Id` → `ID`, `json` → `JSON`,
`dynamodb` → `DynamoDB`, `oauth` → `OAuth` — but isn't. Schema-independent:
it can appear anywhere in a doc, the same scheduling category as `anchors`
and `banned_glosses`.

## Placement

A standalone rule, not a `schema_docs` sub-check: schema-independent
concerns that can appear anywhere in a doc get their own scoping and
severity, per `AGENTS.md`.

It is a `FileRule`, built the way `GlossRule` is: it needs raw line text and
byte offsets to mask code spans, fences, and URLs before scanning, which
`GlossRule.CheckFile(FileCheckContext)` and `ctx.Content` provide. It is not
a `Rule` over the parsed `ctx.Doc` the way `AnchorsRule` is — that shape fits
checks over already-extracted structure (heading anchors, in-page links),
not raw-text masking.

It is a different question from `banned_glosses`, which bans a spelled-out
phrase in favor of its abbreviation ("Amazon Resource Name" → "ARN"): there
the word is wrong. Here the word is right and only its case is wrong.

## Matching

Each configured word has a canonical form. Matching is case-insensitive;
a match is reported when its text differs from the canonical form:

```go
// one regex for the whole rule, not one per entry
re := regexp.MustCompile(`(?i)\b(?:` + strings.Join(lowered, "|") + `)\b`)
for _, loc := range re.FindAllStringIndex(masked, -1) {
	got := raw[loc[0]:loc[1]]
	want := canon[strings.ToLower(got)]
	if got == want || glued(raw, loc[0], loc[1]) {
		continue
	}
	// report: avoid %q; use %q instead, or add backticks if this is a
	// correct, lowercase technical reference (code, value, or command)
}
```

The canonical form is held exactly as configured, never derived by
upcasing the match. The lookup is keyed by `strings.ToLower(got)` so the
stored value can carry AWS's actual casing, including mid-word capitals:
`DynamoDB`, `OAuth`, `iSCSI`, `GameLift`, `OpsWorks`, `SageMaker`. The rule
reproduces whatever casing was configured, letter for letter — it is not
an all-caps-only rule.

The message offers both fixes, not just capitalization, because the check
cannot tell which is right: `arn` in `the arn format is` should become
`ARN`, but `arn` in `` the `arn` attribute `` (missing backticks) should
stay lowercase and gain backticks instead.

Findings are ordered by byte position within each line. The word list is
iterated in sorted order when the regex is compiled, so the alternation —
and the message text — never depends on map iteration order. Alternation is
longest-first so `https` wins over `http`.

### Plurals and possessives

Plural and possessive forms are checked, not skipped: `CPUs` and "a CPU's
performance" are the same defect as the singular. Each needs its own
canonical entry rather than a suffix rule, because suffix inference would
mishandle words whose plural isn't formed by adding `s` (`DNS`) and because
an apostrophe is not a word character, so `\bcpu\b` already matches the
`cpu` inside `cpu's` — the possessive needs its own alternation branch
(`cpu's`, matched literally) so the `'s` survives into the suggestion:

```
"cpu"   -> "CPU"
"cpus"  -> "CPUs"
"cpu's" -> "CPU's"
```

Not every word needs all three forms — `JSON` has no plural in practice.
The default list carries only the forms that occur in practice;
`enforce_casing` lets a provider add others (`IDs`, `VPC's`).

## Config shape

```hcl
check "prose_casing" {
  # Off by default. See "Rollout".
  enabled  = true
  severity = "warning"

  # Canonical forms, added to the built-in default list. Matching is
  # case-insensitive, so an entry here whose lowercase form the default
  # list already has replaces the default's casing for that word.
  enforce_casing = ["ARN", "VPC", "KMS", "IAM", "DynamoDB"]

  # Removed from the merged list (default ∪ enforce_casing) before
  # compiling — the way to opt out of a word the default list enforces
  # that this provider's docs use lowercase on purpose. A bare entry
  # removes the word everywhere; a "type/name" entry removes it for one
  # target only.
  ignore_words = ["ami", "aws_emr_cluster/ssh"]

  # The scoping every check already has.
  ignore_targets = ["aws_quicksight_group"]
  skip_frontmatter = true
}
```

- `EnforceCasing []string` (`hcl:"enforce_casing,optional"`) — canonical
  forms the provider adds or overrides. A list, not a `wrong→right` map,
  because the wrong forms are derived from the canonical one, not
  enumerated.
- `IgnoreWords []string` (`hcl:"ignore_words,optional"`) — word-granularity
  exclusion, matched case-insensitively, applied after merging
  `enforce_casing` into the default list and before compiling the matcher.
  This is how a provider that enables the check but disagrees with one
  default entry removes it, without forking the whole list. A bare entry
  (`"ami"`) removes the word everywhere; a `type/name`-qualified entry
  (`"aws_emr_cluster/ssh"`) removes it for one target, resolved the way
  `Override.Matches` resolves its targets. Independent of `IgnoreTargets`,
  which scopes the whole check by file rather than one word. Short and
  stable, so no `_file` variant.
- Reuses `CheckConfig`'s existing `Severity`, `SkipFrontmatter`,
  `IgnoreTargets`/`IgnoreTargetsFile`, `Prefixes`/`IgnorePrefixes`. No new
  scoping mechanism.
- `Load` rejects a config whose merged list contains two canonical forms
  differing only in case, and any `enforce_casing` entry equal to its own
  lowercase form (a rule that suggests its own input is a config error, not
  a finding).

## Default list

Vendor-neutral only. AWS-specific vocabulary (`ARN`, `VPC`, `KMS`, `IAM`,
`DynamoDB`, ...) belongs in a provider's `enforce_casing`, not the default,
because the default list is a claim about every provider that uses this
check.

```
ID   API  URL  URI  HTTP HTTPS TLS SSL SSH
JSON XML  HTML SQL  DNS  IP    CPU GPU
UUID GUID CLI  SDK  UI   CSS   JWT
```

A word is excluded from the default list when:

1. **Its lowercase form is an ordinary English word.** The rule infers
   uppercase from lowercase, so such a word would fire on correct prose:
   `GET`, `POST`, `PATH`, `NET`, `NOTE`, `LESS`, `RAM`, `ZIP`, `JAR`, `ASP`,
   `DEBUG`.
2. **Its lowercase form is commonly a literal config value.** `TCP` and
   `UDP`: `tcp`/`udp` are protocol values a user types
   (`protocol = "tcp"`), not prose. Available via `enforce_casing` for a
   provider whose docs don't use them as values.
3. **It's vendor-specific.** Belongs in a provider's `enforce_casing`.

Two-letter entries (`ID`, `IP`, `UI`) carry more false-positive risk than
longer ones and should be measured individually before being added.

The list lives as a Go slice literal in `internal/check/prose_casing.go`,
not generated from any provider's `names/caps.csv`: most of that file's
entries (`Acl`, `AcmPca`, `Dnssec`, `Microvms`) are Go-identifier casing
variants, not English prose words, and the ones that are prose words are
AWS-specific.

## Masking

Reuses `GlossRule`'s pipeline as-is: skip frontmatter when configured, skip
fenced code blocks (`isFenceDelimiter`, `frontmatterEnd`), and blank out
inline code spans, markdown link targets, autolinks, and bare URLs
(`maskUnscannable`), gating each line on a cheap combined regex before the
scan. Masking replaces with equal-length spaces so byte offsets stay
aligned.

Three additional cases need masking beyond `GlossRule`'s pipeline, because
in each the underlying defect is an unbackticked literal, and a casing
finding there would misname the defect:

**Colon-delimited literals.** `arn:aws:ec2:…`, `s3://bucket/prefix`,
`java.sql.Timestamp::valueOf`. Mask any `word:` token and what follows it.

**Glued tokens.** A match adjacent to `-`, `_`, `/`, or `@`, or to a `.`
with an alphanumeric on its far side, is part of a compound identifier, not
a prose word: `execute-api`, `ssh-rsa`, `text/html`, `index.html`,
`~/.ssh/authorized_keys`, `10.0.1.6@tcp`. The far-side test on `.` keeps
sentence-final `…inside your rest api.` reportable while rejecting
`index.html`. A match enclosed in double quotes is skipped for the same
reason (`only "url" can be used`).

**Heading lines, and link text pointing at an in-page anchor, are skipped
entirely.** A block heading carries the schema's own block name, and the
provider's accepted heading styles include unbackticked forms —
`{Title}` renders `ip_filter` as `Ip Filter`. Capitalizing it breaks the
heading's resolution against the schema path, turning a style finding into
a coverage error:

```
#### client_authentication tls Argument Reference      ← tls is the schema block name
### Ip Filter Argument Reference                       ← {Title} of ip_filter
```

Link text targeting an in-page anchor (`[Ip Filter](#ip-filter-argument-reference)`)
mirrors a heading this rule cannot report, so it is skipped for the same
reason. Link text with an external target is ordinary prose and is still
scanned.

## Never guess

Go's `\b` is defined against `\w` = `[0-9A-Za-z_]`, so a following word
character — underscore included — already prevents a match: `\barn\b`
cannot match in `arn_foo`. Markdown emphasis markers (`*arn*`) do not need
special handling either; emphasized prose is still prose and should still
be reported.

The real ambiguities, all of which produce no finding:

- **Unterminated inline code spans.** A line with an odd number of
  backticks is skipped, since the inline-code mask can't tell where the
  span was meant to end.
- **Headings and anchor link text.** The schema decides those names, and
  this rule has no schema.
- **Enum values written in prose without backticks.** `valid values are
  cpu and memory` is structurally indistinguishable from prose. There is no
  signal to act on; this is a known residual false positive, addressed by
  the message's backticks suggestion rather than suppressed.

## Rollout

`enabled` defaults to `false`. A non-empty built-in list that defaulted on
would make every provider that upgrades start emitting findings it never
opted into, against `AGENTS.md`'s "existing clean provider configs stay
clean." Turning it on is one config line.

First release ships `severity = "warning"`; a provider that wants CI
enforcement raises it with the existing `severity` key.

A provider adopting the check can stage it: enable with the noisiest word
in `ignore_words`, fix the rest, then drop the ignore.

## No in-document suppression

The only escapes are `ignore_words` (one word, everywhere or in one target)
and `ignore_targets` (one check, one file) — both in config, both
reviewable. There is no inline, comment-based directive to silence a single
finding in place.

An inline directive would let a word stay lowercase on one line without
removing it from the list, but nothing would report the directive going
stale once the underlying line changes or the word is fixed elsewhere — the
same failure mode `skip_blocks` and `allow_phantoms` entries already have
when applied too broadly. Config-based exclusion doesn't have this problem:
removing a word from `ignore_words` is a visible, reviewable config change,
and a provider can audit the whole exclusion list in one place rather than
hunting for comments scattered across docs.

If a word in the default list turns out to need per-line, not per-provider,
treatment — correct lowercase in one doc, wrong in hundreds of others —
that is a reason to re-examine the word, not to add a suppression
mechanism: the fix is either removing the word (if it's genuinely
context-dependent) or adding backticks at the one correct lowercase site
(if it's a literal that was never prose).

## What this leaves unchecked

| Unchecked | Why | Reported by |
|---|---|---|
| Casing inside code spans, fences, link targets, URLs, and colon literals | Not prose; the text is a value | nothing, by design |
| Casing in headings | The schema owns the block name; unbackticked heading styles are accepted and capitalizing breaks resolution | nothing |
| Casing in link text targeting an in-page anchor | Mirrors a heading that can't be reported | nothing, by design |
| Vendor-specific initialisms (`ARN`, `VPC`, …) | Not in the default list | nothing until a provider sets `enforce_casing` |
| `TCP`, `UDP` | Lowercase forms are config values a user types (`protocol = "tcp"`) | nothing until a provider opts in |
| A word in `ignore_words` | Provider opted out, globally or for one target | nothing |
| An unbackticked literal the masks don't catch (`valid values are cpu and memory`) | No structural signal separates it from prose | reported *as* a casing warning; the fix (backticks) is also the real defect |
| Initialisms spelled out in full ("Amazon Resource Name") | Different defect | `banned_glosses`, when configured |

## Tests

`internal/check/prose_casing_test.go`, `TestProseCasing_` prefix,
table-driven with `t.Parallel()`.

- One case per masked region: code span, fence, link target, autolink, bare
  URL, colon literal, frontmatter — each asserting no finding.
- One case per never-guess condition asserting no finding: unterminated
  backtick span, heading line, anchor link text, each glue character,
  quoted literal — plus one asserting a sentence-final match *is* reported.
- Correctly cased text produces nothing: `ID`, `JSON`, `HTTPS` in prose.
- `Id` and `id` both report, confirming canonical rather than literal
  matching; `enforce_casing` overriding a default word's casing.
- Determinism: a doc with several words on one line, run many times
  in-process, identical output.
- Fixtures frozen from real docs: an `id` reference, an `Id`/`id` pair, an
  ARN literal, a glued token (`execute-api`), an `Ip Filter`-style heading
  with matching anchor text.
- Config validation: duplicate canonical forms differing only in case, and
  an `enforce_casing` entry equal to its own lowercase form.
- `ignore_words`: a bare entry silences a word everywhere; a `type/name`
  entry silences it for one target and leaves it reported elsewhere.

## Also update

- `docs/README.md`: a rules-table row next to `banned_glosses`, and a
  config section next to it.
- No provider config change ships with this; it's off by default. Enabling
  it in terraform-provider-aws is a companion provider PR, after a
  `make corpus` run is triaged.

## Open questions

1. Full `make corpus` counts, including plural/possessive forms and `SSH`,
   are needed before the default list is finalized.
2. Whether `CPU`'s one known unbackticked-enum false positive recurs enough
   in other providers' docs to move it to opt-in.

This design can be refined further during implementation.
