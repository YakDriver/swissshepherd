# `prose_casing` rule
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

Status: implemented. Tracks #100; multi-word names #105; matcher
performance #104.

Flags a word or name in doc prose that should be cased as an initialism,
an AWS-style mixed-case term, or a multi-word product name — `id`/`Id` →
`ID`, `json` → `JSON`, `dynamodb` → `DynamoDB`, `apigateway` → `API
Gateway` — but isn't. Schema-independent:
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

Each configured entry has a canonical form: the entry trimmed, with inner
whitespace collapsed to single spaces, its letters' casing kept exactly as
configured and never derived by upcasing the match: `DynamoDB`, `OAuth`,
`iSCSI`, `API Gateway`. An entry is one or more words separated by spaces;
each word is a run of `[0-9A-Za-z_]`, the characters Go's `\b` treats as
word characters.

From the canonical form the rule derives a **match key**, the canonical
lowercased with spaces removed (`API Gateway` → `apigateway`), and its
**breaks**, the key offsets where the canonical has a space (`[3]`).

A run of adjacent words in the text matches an entry when:

1. the words are separated only by spaces or tabs in the raw line,
2. their lowercase forms, joined, equal the key, and
3. every break in the text is also a break in the canonical form.

So text may run a canonical's words together but never split one:

| text | `API Gateway` | `DataSync` |
|---|---|---|
| `api gateway`, `API gateway`, `api  gateway` | match | — |
| `apigateway`, `ApiGateway` | match | — |
| `datasync`, `Datasync` | — | match |
| `data sync` | — | no match |

The third condition is what keeps ordinary words ordinary: `DataSync`
must not match "Provides a SSM resource data sync", nor `AppConfig` an
OpenSearch `### App Config` heading. Both lines are in the
terraform-provider-aws corpus.

A match is reported when its text differs from the canonical form, except
when the only difference is extra spaces or a tab between words, which
render identically. At each word the longest match wins and consumes its
words, so `api gateway` is one finding rather than an extra `api` → `API`,
and a correct `API Gateway` hides the default `API` entry inside it.
Matches are leftmost and non-overlapping, ordered by byte position within
each line.

The message offers both fixes, not just capitalization, because the check
cannot tell which is right: `arn` in `the arn format is` should become
`ARN`, but `arn` in `` the `arn` attribute `` (missing backticks) should
stay lowercase and gain backticks instead. `ssh` is the clearest case —
`` `ssh` `` and `SSH` are both correct, bare `ssh` is not, and which one
applies depends on whether the sentence names the command or the protocol.
A literal isn't always lowercase — an enum value (`Vpc`), a field name
(`Id`), an environment variable prefix (`CODEBUILD_`) — so the backticks
branch doesn't claim it is:

```
avoid "Cloudwatch"; use "CloudWatch" instead, or add backticks around it if it's a literal (code, a value, or a field name)
```

A match containing a space or tab can't be a literal identifier or value,
so it gets only the capitalization fix:

```
avoid "Auto Scaling Group"; use "Auto Scaling group" instead
```

A finding with two candidate fixes names a defect without naming its
remedy, which is a deliberate concession: the alternative is guessing, and
guessing here produces the destructive fix roughly half the time. The
consequence is that this rule is not a mechanical autofix target — each
finding needs a reader's judgment about which branch applies.

### Phrase entries supply context

swissshepherd can't know whether "autoscaling" names the AWS feature or the
generic concept; the provider can, and says so with the entries it
configures. In the AWS corpus the brand appears in phrases ("Auto Scaling
group" 55 times, "EC2 Auto Scaling" 33, "Application Auto Scaling" 20),
while bare "autoscaling" is often generic ("If autoscaling creates drift",
App Runner's "auto scaling configuration"). Likewise "workspaces" is
WorkSpaces in one service's docs and an ordinary noun in Grafana's and
Prometheus's. So a provider configures the phrase, not the bare word:

```hcl
enforce_casing = [
  "Auto Scaling group", "Auto Scaling groups",
  "EC2 Auto Scaling", "Application Auto Scaling",
]
```

Bare `autoscaling` then produces nothing. That misses some brand uses,
which `AGENTS.md` ranks below a false positive.

### Plurals and possessives

Plural and possessive forms are checked, not skipped: `CPUs` and "a CPU's
performance" are the same defect as the singular. Each needs its own
canonical entry rather than a suffix rule. Inference would have to choose
the canonical plural for the provider — `IDs`, `ID's`, and `IDS` are all
written in practice:

```
"CPU"   "CPUs"   "CPU's"
"Auto Scaling group"   "Auto Scaling groups"
```

The last word of an entry may end in `'s`. An apostrophe is not a word
character, so the matcher checks for `'s` (either case) directly after a
match, followed by a non-word character, and prefers the possessive entry
when one exists. A typographic apostrophe (`cpu’s`) isn't `'`, so the
plain `cpu` entry matches, which still yields the right result: replacing
`cpu` with `CPU` leaves `CPU’s`.

Not every word needs all three forms. The default list carries only the
forms that occur in practice; `enforce_casing` lets a provider add others.

### Matcher

One pass per line, no regex:

1. Find the runs of `[0-9A-Za-z_]` with a byte scan, the same word bounds
   `\b` gives.
2. Lowercase each word (ASCII, so byte offsets are unchanged) into a
   reused buffer and look it up in a set of **prefixes**: for every entry,
   its key up to each break, and the whole key. A word not in the set, the
   common case, costs one map lookup and is skipped.
3. Otherwise extend one adjacent word at a time, up to the longest entry's
   word count, while the joined text is still a prefix, recording the
   longest extension that is a whole key with allowed breaks.

Each line is scanned unmasked first as a gate; only lines with a match are
masked and scanned again to report. Masking is the expensive step and runs
on few lines.

Not a regex: a case-insensitive alternation over every entry costs RE2 time
in proportion to the alternation, and with a realistic provider list it
cost about 3x every other check combined (#104). On terraform-provider-aws
`website/docs` (286,009 lines) with ~150 entries, the weak config runs in
1.8s with the rule off, 8.1s with an alternation regex, and 2.2s with this
matcher, with byte-identical findings.

## Config shape

```hcl
check "prose_casing" {
  # Off by default. See "Rollout".
  enabled  = true
  severity = "warning"

  # Canonical forms, added to the built-in default list. An entry with the
  # same match key as a default entry replaces it.
  enforce_casing = ["ARN", "VPC", "DynamoDB", "API Gateway", "Auto Scaling group"]

  # Removed from the merged list (default ∪ enforce_casing) before
  # compiling — the way to opt out of a word the default list enforces
  # that this provider's docs use lowercase on purpose. A bare entry
  # removes the word everywhere; a "type/name" entry removes it for one
  # target only.
  ignore_words = ["ami", "aws_emr_cluster/ssh", "aws_x/Auto Scaling group"]

  # The scoping every check already has.
  ignore_targets = ["aws_quicksight_group"]
  skip_frontmatter = true
}
```

- `EnforceCasing []string` (`hcl:"enforce_casing,optional"`) — canonical
  forms the provider adds or overrides, single words or multi-word names.
  A list, not a `wrong→right` map, because the wrong forms are derived
  from the canonical one, not enumerated. Each entry is trimmed and its
  inner whitespace collapsed to single spaces.
- `IgnoreWords []string` (`hcl:"ignore_words,optional"`) — entry-granularity
  exclusion, matched by match key (so `"auto scaling"` also removes
  `Auto Scaling` and its `autoscaling` form), applied after merging
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
- `Load` rejects an `enforce_casing` list with two entries sharing a match
  key (`ARN` and `Arn`, or `AutoScaling` and `Auto Scaling`), or an entry
  equal to its own lowercase form (a rule that suggests its own input is a
  config error, not a finding). `NewProseCasingRule` rejects an entry with
  a character outside `[0-9A-Za-z_]` and spaces, other than a final `'s`:
  `X-Ray` or `Amazon S3.` could never match, because masking blanks text
  glued to `-`, `.`, `/`, and `@`. It cannot
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
fenced code blocks (`fenceDelimiter`, `frontmatterEnd`), and blank out
inline code spans, markdown link targets, autolinks, and bare URLs
(`maskUnscannable`), gating each line on the matcher run over the unmasked
line. Masking replaces with equal-length spaces so byte offsets stay
aligned.

Because masked text becomes spaces, word adjacency is judged on the raw
line: two words are adjacent only if the raw bytes between them are spaces
or tabs. `` the api `x` gateway `` and `the *api* gateway` are not `api
gateway`.

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

A multi-word canonical form adds a second way to break one: respacing a
`{Title}` heading changes its snake-case name, `### Autoscaling Policy
Configuration` (`autoscaling_policy_configuration`) becoming `### Auto
Scaling Policy Configuration` (`auto_scaling_policy_configuration`).

Only headings the doc parser resolves can break. It resolves a heading
against the schema only at level 3 or deeper inside a level-2 section whose
heading begins with `Argument` or `Attribute` (`doc.ParseWithOptions`), so
the rule tracks the same thing: a level-2 heading sets or clears the
section, a level-1 heading clears it, and heading-shaped lines inside
fences are ignored. Everywhere else — Example Usage, Import, the
introduction — a heading is prose and gets the ordinary message, so
`### With AppMesh Proxy` under Example Usage gets `use "App Mesh"` (#107).

Within those sections, rather than infer the style, test the suggestion
against the heading matcher and withhold the fix that would break it:

```go
if isBlockHeading { // level >= 3, inside Argument/Attribute Reference
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
to the same name after a case-only fix, so capitalization is; a respaced
`{Title}` heading resolves to a different name, so only backticks are
offered; and a prose heading resolves to nothing before, so capitalization
is offered there too. Withholding inside these sections doesn't consult
the schema: a heading there that names no block is already reported by
`schema_docs`, and the rule can't tell which name the author meant.

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
| A bare word a provider configured only as a phrase (`autoscaling` with `Auto Scaling group`) | Often the generic concept; only the provider's entries say otherwise | nothing |
| A name split across a line break (`API` / `Gateway`) | The rule is line-based | nothing |
| A name split by markup or a hyphen (`*api* gateway`, `auto-scaling`) | Not adjacent words; hyphenated text is masked as a possible literal | nothing for the name (`api` alone may still be reported) |

## Tests

`internal/check/prose_casing_test.go`, `TestProseCasing_` prefix,
table-driven with `t.Parallel()`.

- One case per masked region: code span, fence, link target, autolink, bare
  URL, colon literal, frontmatter — each asserting no finding.
- One case per never-guess condition asserting no finding: unterminated
  backtick span, each glue character, quoted literal — plus one asserting
  a sentence-final match *is* reported.
- Section tracking: a respaced `{Title}` heading under Example Usage, before
  any section, after `## Import`, as a level-2 heading, and after a level-1
  reset gets the canonical name; the same heading inside each of Argument,
  Attribute, and Attributes Reference gets backticks only; a fenced
  `## Argument Reference` line doesn't open a section.
- Message shape: a multi-word or tab-separated match gets the
  capitalization fix only; a single token, whatever its case, gets both.
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
  entry silences it for one target and leaves it reported elsewhere;
  matching is by key, spaced or not.
- Multi-word entries: every spacing and casing variant of `API Gateway`
  and `Auto Scaling`; text never splits a canonical word (`data sync`,
  `### App Config`); the longest match consumes a shorter entry; a masked
  span or emphasis between words breaks adjacency; whitespace-only
  differences aren't reported; a phrase entry leaves the bare word alone;
  a multi-word possessive; a `{Block}` heading gets backticks only.
- Entry syntax: unmatchable entries (`X-Ray`, `C++`) are rejected.
- `BenchmarkProseCasing`: ~150 entries over candidate-dense text.

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
