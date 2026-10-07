# `prose_casing` rule
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

Status: implemented. Tracks #100.

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
stay lowercase and gain backticks instead. `ssh` is the clearest case —
`` `ssh` `` and `SSH` are both correct, bare `ssh` is not, and which one
applies depends on whether the sentence names the command or the protocol.

A finding with two candidate fixes names a defect without naming its
remedy, which is a deliberate concession: the alternative is guessing, and
guessing here produces the destructive fix roughly half the time. The
consequence is that this rule is not a mechanical autofix target — each
finding needs a reader's judgment about which branch applies.

Findings are ordered by byte position within each line. The word list is
iterated in sorted order when the regex is compiled, so the alternation —
and the message text — never depends on map iteration order. Alternation is
longest-first so `https` wins over `http`.

### Plurals and possessives

Plural and possessive forms are checked, not skipped: `CPUs` and "a CPU's
performance" are the same defect as the singular. Each needs its own
canonical entry rather than a suffix rule. Inference would have to choose
the canonical plural for the provider — `IDs`, `ID's`, and `IDS` are all
written in practice — and an apostrophe is not a word character, so
`\bcpu\b` already matches the `cpu` inside `cpu's`. The possessive
therefore needs its own alternation branch so the `'s` survives into the
suggestion:

```
"cpu"   -> "CPU"
"cpus"  -> "CPUs"
"cpu's" -> "CPU's"
```

Longest-first alternation makes `cpu's` win at that position. A
typographic apostrophe (`cpu’s`) falls through to the plain `cpu` branch,
which still yields the right result, since replacing `cpu` with `CPU`
leaves `CPU’s`.

Not every word needs all three forms. The default list carries only the
forms that occur in practice; `enforce_casing` lets a provider add others.

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
  (`"ami"`) removes the word everywhere; a `target/word` entry
  (`"aws_emr_cluster/ssh"`) removes it for one target only — the target
  first, as a bare resource name or `type/name`, split at the last `/` so
  the trailing word is always unambiguous. Independent of `IgnoreTargets`,
  which scopes the whole check by file rather than one word. Short and
  stable, so no `_file` variant.
- Reuses `CheckConfig`'s existing `Severity`, `SkipFrontmatter`,
  `IgnoreTargets`/`IgnoreTargetsFile`, `Prefixes`/`IgnorePrefixes`. No new
  scoping mechanism.
- `Load` rejects an `enforce_casing` list with two entries differing only
  in case, or an entry equal to its own lowercase form (a rule that
  suggests its own input is a config error, not a finding). It cannot
  check `enforce_casing` against the check's built-in default list without
  an import cycle (`internal/check` already imports `internal/config`), so
  that half of the check — a provider override colliding with a default
  entry's case — runs in `check.NewProseCasingRule` at construction time
  instead.

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

`SSH` stays in despite appearing in literal forms, because those forms are
all glued (`ssh-rsa`, `~/.ssh/authorized_keys`) and already masked. What
remains is the genuine case: bare lowercase `ssh` is wrong either way, and
the correct text is `` `ssh` `` when it names the command or `SSH` when it
names the protocol. The two-fix message covers exactly that, which is why
`SSH` does not need criterion 2's treatment and `tcp`/`udp` — which appear
bare and unglued inside value lists — do.

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

Two cases need masking beyond `GlossRule`'s pipeline. In both, adjacent
punctuation is a reliable structural signal that the token is a literal
rather than prose, so the match can be dropped outright instead of
reported with a choice of fixes:

**Colon-delimited literals.** `arn:aws:ec2:…`, `s3://bucket/prefix`,
`java.sql.Timestamp::valueOf`. Mask any `word:` token and what follows it.

**Glued tokens.** A match adjacent to `-`, `_`, `/`, or `@`, or to a `.`
with an alphanumeric on its far side, is part of a compound identifier, not
a prose word: `execute-api`, `ssh-rsa`, `text/html`, `index.html`,
`~/.ssh/authorized_keys`, `10.0.1.6@tcp`. The far-side test on `.` keeps
sentence-final `…inside your rest api.` reportable while rejecting
`index.html`. A match enclosed in double quotes is skipped for the same
reason (`only "url" can be used`).

### Headings

Headings get no carve-out: a bare `## tls Argument Reference` heading (the
unbackticked `{Block}` style, one of the accepted `block_heading_styles`) is
prose as far as this rule is concerned, and `tls` is reported like any other
word. Skipping headings wholesale would lose real findings, because for
`{Title}` styles capitalizing is both correct and safe — `titleToSnake`
lowercases each word before joining (`doc.go`), so `### Ip Filter Argument
Reference` still resolves to `ip_filter` after the fix.

What a heading does change is which fix is safe to suggest. `{Block}` and
`{Path}` validate through `isSnakeCaseSegment`, which accepts only
`[a-z0-9_]`, so capitalizing one makes the heading stop resolving, the block
become undocumented, and a cosmetic warning become a coverage error:

```
### json                        ->  ### JSON        isSnakeCaseSegment("JSON") = false
#### client_authentication tls  ->  #### … TLS      same
```

Rather than infer the style, test the suggestion against the heading matcher
and withhold the fix that would break it:

```go
if isHeadingLine {
	before := templates.Match(headingText)
	after := templates.Match(strings.Replace(headingText, got, want, 1))
	if before != "" && after != before {
		// capitalizing would break resolution: suggest backticks only
	}
}
```

`HeadingTemplates.Match` is exported and pure, so this costs the rule a
`block_heading_styles` value at construction and nothing else. It needs no
case analysis and no schema: a `{Block}` or `{Path}` heading resolves before
and not after, so only backticks are offered; a `{Title}` heading resolves
to the same name either way, so capitalization is; and a prose heading
resolves to nothing before, so capitalization is offered there too.

The backticks branch agrees with `schema_docs`'s separate preferred-style
finding, which already points unbackticked `{Block}` headings toward the
backticked form. The two findings name different defects — one casing, one
heading format — and the backticks fix satisfies both.

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
- **Enum values written in prose without backticks.** `valid values are
  cpu and memory` is structurally indistinguishable from prose. There is no
  signal to act on; this is a known residual false positive, addressed by
  the message's backticks suggestion rather than suppressed.

## Sizing

Approximated by mirroring `gloss.go`'s pipeline plus the masks above over
terraform-provider-aws `website/docs` at provider commit `34167e0962c`.
Real counts come from `make corpus` once an implementation exists; these
are what the decisions above rest on, recorded so a surprising corpus
number has something to contradict. They also supersede the raw unmasked
scan quoted in #100 ("thousands of bare lowercase hits"), which counted
matches inside code spans and URLs.

**301 findings over 15 of the 24 default words.**

| word | findings | variants |
|---|---:|---|
| `ID` | 241 | `id` 189, `Id` 52 |
| `IP` | 14 | `Ip` 11, `ip` 3 |
| `JSON` | 8 | `json` 8 |
| `HTTP` | 7 | `Http` 4, `http` 3 |
| `CPU` | 7 | `cpu` 7 |
| `HTTPS` | 5 | `https` 5 |
| `URI` | 4 | `uri` 3, `Uri` 1 |
| `CLI` | 3 | `cli` 3 |
| `API`, `URL`, `XML`, `TLS`, `SQL` | 2 each | |
| `SSH`, `HTML` | 1 each | |

`SSL`, `DNS`, `GPU`, `UUID`, `GUID`, `SDK`, `UI`, `CSS`, and `JWT` produce
nothing on this corpus: no cost, and they guard against regression.

- `ID` is 80% of the output. That is what makes the staged rollout below
  work, and why `ignore_words` needs to exist.
- The glue and quote masks suppress 33 matches, all literals
  (`execute-api`, `ssh-rsa`, `text/html`, `index.html`, `core-site.xml`,
  `java.sql.Timestamp`, `10.0.1.6@tcp`, `only "url" can be used`).
- `TCP` and `UDP`, excluded by criterion 2, produce 2 findings between
  them, both in one line of value text (`If not icmp, icmpv6, tcp, udp, or
  all`) and both false positives. Zero true positives is the evidence for
  excluding them.
- 9 findings are on heading lines. Of those, 4 would break resolution if
  capitalized (`### json` ×2, `#### json`, `#### client_authentication
  tls`) and get the backticks-only message; 2 are `{Title}` headings where
  capitalizing is correct and safe (`### Ip Filter Argument Reference`,
  `### Source Ip Config`); 3 are prose headings that resolve to no block.
- Spot-checked, the non-`ID` findings are true positives except the single
  unbackticked-enum case noted under "Never guess".

## Rollout

`enabled` defaults to `false`. A non-empty built-in list that defaulted on
would make every provider that upgrades start emitting findings it never
opted into, against `AGENTS.md`'s "existing clean provider configs stay
clean." Turning it on is one config line.

First release ships `severity = "warning"`; a provider that wants CI
enforcement raises it with the existing `severity` key.

A provider adopting the check can stage it: enable with
`ignore_words = ["id"]`, which defers 241 of the 301 findings, fix the
remaining 60, then drop the ignore.

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
  backtick span, each glue character, quoted literal — plus one asserting
  a sentence-final match *is* reported.
- A bare, unbackticked `{Block}`-style heading (`## tls Argument
  Reference`) *is* reported for `tls`, with the backticks-only message,
  because capitalizing would stop the heading resolving. A `{Title}`-style
  heading (`### Ip Filter Argument Reference`) is reported with the
  capitalization fix, since it resolves to `ip_filter` either way. A prose
  heading (`### Usage with subnet id`) likewise.
- Correctly cased text produces nothing: `ID`, `JSON`, `HTTPS` in prose.
- `Id` and `id` both report, confirming canonical rather than literal
  matching; `enforce_casing` overriding a default word's casing; a
  mixed-case canonical (`DynamoDB`, `OAuth`) reproduced letter for letter.
- Plurals and possessives: `cpus` → `CPUs`, `cpu's` → `CPU's`, and a
  typographic `cpu’s` → `CPU’s` through the plain branch.
- Determinism: a doc with several words on one line, run many times
  in-process, identical output.
- Fixtures frozen from real docs: an `id` reference, an `Id`/`id` pair, an
  ARN literal, a glued token (`execute-api`), an unbackticked `{Block}`
  -style heading, a `{Title}`-style heading.
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

1. Full `make corpus` counts, to confirm the sizing above and settle the
   plural and possessive entries the default list should carry.
2. Whether `CPU`'s one unbackticked-enum false positive (`valid values are
   cpu and memory`) recurs often enough in other providers' docs to move it
   to opt-in. 6 of its 7 findings here are true positives.
