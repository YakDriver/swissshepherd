# `prose_casing` rule (design)
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

Status: design, not implemented. Tracks #100.

Flags a word in doc prose that should be cased as an initialism or
AWS-style mixed-case term — `id` or `Id` → `ID`, `json` → `JSON`,
`dynamodb` → `DynamoDB`, `oauth` → `OAuth` — but isn't. Schema-independent:
it can appear anywhere in a doc, the same scheduling category as `anchors`
and `banned_glosses`.

## Placement

A standalone rule, not a `schema_docs` sub-check and not an extension of
`banned_glosses`. `AGENTS.md` decides this directly: "Concerns that are
schema-independent and appear anywhere in a doc (for example, dead anchors)
belong in a standalone rule with its own scoping and `severity`."

It is a `FileRule`, built the way `GlossRule` is. The issue proposed modeling
it on `anchors`, which is the right scheduling comparison but the wrong
implementation template: `AnchorsRule.Check` takes a `CheckContext` and reads
the already-parsed `ctx.Doc`, while this check needs raw line text and byte
offsets to mask code spans, fences, and URLs before scanning — which is what
`GlossRule.CheckFile(FileCheckContext)` and `ctx.Content` provide.

It is also a different question from `banned_glosses`, which bans a
spelled-out phrase in favor of its abbreviation ("Amazon Resource Name" →
"ARN"): there the word is wrong. Here the word is right and only its case is
wrong. Folding the two together would make `glossRegexp`'s one pattern shape
carry two unrelated find/fix semantics.

## Matching

Configure the **canonical** form of each word. Match case-insensitively, then
report when the matched text differs from the canonical form:

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

The canonical form is **not** derived by upcasing the match — it's the
literal string configured for that word, held as-is. The lookup is keyed by
`strings.ToLower(got)` precisely so the stored value can carry AWS's actual
casing, including the mid-word capitals that are common in its vocabulary:
`DynamoDB`, `OAuth`, `iSCSI`, `GameLift`, `OpsWorks`, `SageMaker`. A rule that
only produced all-caps output (`DYNAMODB`) would be wrong for most of
`names/caps.csv`'s true entries, not just `ID`/`ARN`-style initialisms — this
rule has to reproduce whatever casing was configured, letter for letter.

The message explicitly offers both fixes, not just the capitalization one,
because the check cannot tell which is right: `arn` in `the arn format is`
should become `ARN`, but `arn` in `` the `arn` attribute `` (missing
backticks that should have been there) should stay lowercase and gain
backticks instead. Both repairs make the finding disappear, so the message
names both rather than assuming the reader wants the capitalized form.

An earlier draft of this design matched each wrong casing as an exact literal
(`arn`, never `Arn`), one entry per wrong form. Measured against
terraform-provider-aws, that misses 72 of 184 occurrences — `Id` alone is 52,
because sentence-initial and bullet-initial text is where miscasing happens
most. Canonical matching needs one entry per word instead of one per
miscasing, compiles to a single regex instead of a per-entry loop, and makes
"the correctly cased form is never flagged" a property of the `got == want`
comparison rather than a constraint the word list has to encode. It is also
the established idiom for this: vale's `Microsoft.Terms` uses
`extends: substitution` with `ignorecase: true` and a canonical replacement.

Findings come out ordered by byte position within each line, so the
determinism invariant holds structurally. The word list is still iterated in
sorted order when the regex is compiled, as `NewGlossRule` does, so the
alternation — and therefore the message text — never depends on map
iteration order.

Longest-first alternation ordering so `https` wins over `http`.

### Plurals and possessives

Checked, not deferred — `CPUs` and "a CPU's performance" are exactly the
shape of thing this rule exists to catch, and silently skipping them would
leave half of the plural/possessive occurrences of every entry unflagged.
Three forms per word, all explicit canonical entries rather than inferred by
suffix stripping, consistent with "no suffix inference" elsewhere in this
design (`glossRegexp`'s automatic plural handling is deliberately not reused
here — see below):

```
"cpu"   -> "CPU"
"cpus"  -> "CPUs"
"cpu's" -> "CPU's"
```

`\b` does not treat `'` as a word character, so `\bcpu\b` alone already
matches the `cpu` inside `cpu's` — the apostrophe form needs its own
alternation branch (`cpu's`, matched literally) rather than relying on `\b`
to find it, and its own canonical entry so the possessive `'s` survives into
the suggestion rather than being dropped. The plural needs no such care:
`cpus` is a single word-character run, matched by extending the alternation
with the literal `cpus`, not by appending `s?` to every pattern the way
`GlossRule.recommend` does — that would re-introduce exactly the kind of
inferred-suffix guessing this design otherwise avoids (an inferred plural
cover a word whose plural isn't formed by just adding `s`, like `DNS` or any
word already ending in `s`).

Cost: three entries instead of one, for any word whose plural or possessive
actually occurs. Not every word needs all three — `JSON` has no plural in
practice — so the default list carries only the forms measured as occurring
corpus-wide, and `enforce_casing` lets a provider add `IDs`/`VPC's` without
waiting on this list.

## Config shape

```hcl
check "prose_casing" {
  # Off by default. See "Rollout".
  enabled  = true
  severity = "warning"

  # Canonical forms. Merges with the built-in default list; matching is
  # case-insensitive, so an entry whose lowercase form the default list
  # already has replaces the default's casing for that word.
  enforce_casing = ["ARN", "VPC", "KMS", "IAM", "DynamoDB"]

  # Removed from the merged list (default ∪ enforce_casing) before
  # compiling. For a word this provider uses in lowercase deliberately.
  # Accepts "type/name" qualification to scope a word to one target.
  ignore_words = ["ami", "aws_emr_cluster/ssh"]

  # The scoping every check already has.
  ignore_targets = ["aws_quicksight_group"]
  skip_frontmatter = true
}
```

- `EnforceCasing []string` (`hcl:"enforce_casing,optional"`) — canonical
  forms the provider adds. A list, not a `wrong→right` map, because the wrong
  forms are derived rather than enumerated. Imperative name per `AGENTS.md`.
- `IgnoreWords []string` (`hcl:"ignore_words,optional"`) — word-granularity
  exclusion, matched case-insensitively. Lets a provider silence one noisy
  word without disabling the check or forking the default list. A bare entry
  (`"ami"`) removes the word everywhere; a `type/name`-qualified entry
  (`"aws_emr_cluster/ssh"`) removes it for one target only, resolved the way
  `Override.Matches` resolves its targets. Independent of `IgnoreTargets`,
  which still scopes the whole check by file. Short and stable, so no `_file`
  variant, per the rule as narrowed in #79.
- Reuses `CheckConfig`'s existing `Severity`, `SkipFrontmatter`,
  `IgnoreTargets`/`IgnoreTargetsFile`, `Prefixes`/`IgnorePrefixes`. No new
  scoping mechanism.
- `Load` rejects a config whose merged list contains two canonical forms
  differing only in case, and any `enforce_casing` entry that is empty. A
  rule that can suggest its own input is a real failure mode, not a
  hypothetical: vale's `Terms.yml` carries a comment recording that it
  shipped once and "made the rule flag its own suggestion."

## Default list

Vendor-neutral only. Nothing specific to one provider's domain ships in the
default: `ARN`, `VPC`, `KMS`, `IAM`, `DynamoDB` and the rest of
terraform-provider-aws's vocabulary belong in that provider's
`enforce_casing`, because a default list is a claim about every provider.

```
ID   API  URL  URI  HTTP HTTPS TLS SSL
JSON XML  HTML SQL  DNS  IP    CPU GPU
UUID GUID CLI  SDK  UI   CSS   JWT
```

Three exclusion criteria, in order of how much they cost to get wrong:

1. **The lowercase form is an ordinary English word.** This rule infers
   uppercase from lowercase, so any such entry fires on correct prose. That
   rules out `GET`, `POST`, `PATH`, `NET`, `NOTE`, `LESS`, `RAM`, `ZIP`,
   `JAR`, `ASP`, `DEBUG` — all of which appear in vale's
   `Microsoft.Acronyms` exceptions. Vale's list is a useful vocabulary
   source but cannot be imported wholesale, because its direction of
   inference is the reverse of ours: it asks whether an **already
   capitalized** token is well-known enough to need no definition, so a
   lowercase English homograph costs it nothing and costs us a false
   positive on every occurrence.
2. **The lowercase form is commonly a literal config value.** `TCP` and
   `UDP` are excluded on this ground. In Terraform docs `tcp` and `udp` are
   protocol values a user types (`protocol = "tcp"`). Measured: 2 findings
   between them, both false positives, zero true positives. They remain
   available via `enforce_casing` for a provider whose docs don't use them
   as values.
3. **Vendor-specific.** Above.

`SSH` stays in. It was excluded in an earlier draft on the theory that `ssh`
-as-a-verb ("can be used to ssh to the master node") reads better lowercase
than its noun form capitalized, making the two uses genuinely conflict. They
don't: "I'm going to SSH into the server" capitalizes fine as a verb too, the
same way "I'll FTP the file over" or "please CC the team" do for other
initialisms used verbally. There's no sentence where lowercase `ssh` is
correct and `SSH` is wrong, so this isn't a case the rule needs to special
-case — it's the ordinary case, same as every other entry.

`CPU` stays in by default, not provisionally. An earlier draft flagged it as
the first candidate to move to opt-in because its one false positive
(`valid values are cpu and memory`, an unbackticked enum value) is a
real cost. That's true of every entry in this list to varying degrees — it's
why the message suggests backticks as well as capitalization — and isn't a
reason specific to `CPU`. It stays until a provider surfaces a concrete,
repeated cost that the backticks suggestion doesn't resolve; "it had one
false positive in one corpus" is not that.

`ID` stays in, reversing the earlier draft. That draft excluded it citing
`caps.csv`'s rationale — "id" collides with ordinary English inside other
words ("avoid", "paid", "valid"). That rationale belongs to a Go-identifier
linter doing substring matching and does not survive the change of matching
regime: under `\bid\b` there is no word boundary before the `id` in `avoid`,
`paid`, `valid`, or `liquid`, so the collision cannot occur. Measured, `id`
and `Id` are 240 findings, 82% of the rule's total output, and the sample is
uniformly the defect #100 describes:

```
cloudhsm_v2_cluster.html.markdown:78   * `cluster_id` - The id of the CloudHSM cluster.
budgets_budget.html.markdown:445       * `id` - id of resource.
autoscaling_group.html.markdown:820    - `id` - Auto Scaling Group id.
ami_copy.html.markdown:43              * `source_ami_id` - (Required) Id of the AMI to copy. This id must be valid in the region
```

`ID` is the highest-value entry in the list, not the one to defer. Its volume
is a rollout question, handled below, not a correctness one.

Two-letter entries (`ID`, `IP`, `UI`) deserve the extra scrutiny they got
here: vale's `Microsoft.Acronyms` matches `\b([A-Z]{3,5})\b` and declines to
reason about two-letter acronyms at all. All three measured clean under word
boundaries and the masking below, so they ship; a fourth two-letter candidate
should be measured before it's added.

The list lives as a Go slice literal in `internal/check/prose_casing.go`,
revised by ordinary PR. Not generated from any provider's `names/caps.csv`:
most of that file's 125 entries (`Acl`, `AcmPca`, `Dnssec`, `Microvms`) are
Go-identifier casing variants that are not English prose words, and the ones
that are, are AWS-specific.

## Masking

`GlossRule`'s pipeline, reused as-is: skip frontmatter when configured, skip
fenced code blocks (`isFenceDelimiter`, `frontmatterEnd`), and blank out
inline code spans, markdown link targets, autolinks, and bare URLs
(`maskUnscannable`), gating each line on a cheap combined regex before the
scan. Masking replaces with equal-length spaces so byte offsets stay aligned.

That pipeline alone is not sufficient. Measured, it leaves three classes of
false positive, each of which would produce a finding whose suggested fix
damages correct docs — the second priority in `AGENTS.md`, and the "report
the real defect" invariant. In every case the underlying defect is a literal
that should have been in backticks, so a casing finding also misnames it.

**Colon-delimited literals.** `arn:aws:ec2:…` in prose (`ebs_volume`,
`cloudfront_distribution`, `mwaa_environment`), `s3://bucket/prefix`,
`java.sql.Timestamp::valueOf`. Mask any `word:` token and what follows it.
This matters to any provider that puts `ARN` in `enforce_casing`: 6 of 46
bare `arn` matches in terraform-provider-aws are inside an ARN literal.

**Glued tokens.** A match adjacent to `-`, `_`, `/`, or `@`, or to a `.` with
an alphanumeric on its far side, is part of a compound identifier, not a
prose word: `execute-api` (5 occurrences), `ssh-rsa` (3), `text/html` (2),
`index.html`, `hdfs-site.xml`, `core-site.xml`, `~/.ssh/authorized_keys`,
`10.0.1.6@tcp`. The far-side test on `.` is what keeps sentence-final
`…inside your rest api.` reportable while rejecting `index.html`. A match
enclosed in double quotes is skipped for the same reason
(`only "url" can be used`). This suppresses 28 matches corpus-wide.

**Heading lines are skipped entirely.** A block heading carries the schema's
own name, and the provider's accepted styles include unbackticked forms —
`"{Block} Block"` is in terraform-provider-aws's `block_heading_styles`, and
`{Title}` renders `ip_filter` as `Ip Filter`. `doc.go` documents `{Block}` as
matching "a snake_case name (no spaces, lowercase with underscores)", so
capitalizing it breaks resolution and converts a cosmetic warning into a
coverage error:

```
#### client_authentication tls Argument Reference      ← tls is the schema block name
### Ip Filter Argument Reference                       ← {Title} of ip_filter
```

Vale draws the same boundary, giving headings their own scope and their own
rule (`Microsoft.HeadingAcronyms`, `scope: heading`). Skipping headings
suppresses 10 matches.

**Link text pointing at an in-page anchor is skipped** for the same reason:
`[Ip Filter](#ip-filter-argument-reference)` mirrors a heading this rule
cannot report, and flagging one without the other would leave the doc
internally inconsistent. Link text with an external target is prose and is
still scanned — `see [cpu architecture](https://docs.aws.amazon.com/…)` is
reported.

## Never guess

The earlier draft proposed skipping a match "immediately followed by an
underscore or another word character that word-boundary matching alone
wouldn't already exclude," and skipping matches abutting Markdown emphasis
markers. Both are wrong. Go's `\b` is defined against `\w` = `[0-9A-Za-z_]`,
so a following word character — underscore included — already prevents the
match; `\barn\b` cannot match in `arn_foo`. And `*arn*` is emphasized prose
that should be reported, so suppressing it drops true positives.

The real ambiguities, all of which produce no finding:

- **Unterminated inline code spans.** `reInlineCode` requires a closing
  backtick on the same line, so `` Example: `arn:aws:iam::cloudfront:user/… ``
  (`cloudfront_origin_access_identity:42`) is not masked. A line containing
  an odd number of backticks is skipped.
- **Headings and anchor link text**, per above: the schema decides those
  names, and this rule has no schema.
- **Enum values written in prose without backticks.** `valid values are cpu
  and memory`, `If not icmp, icmpv6, tcp, udp, or all` are structurally
  indistinguishable from prose. There is no signal to act on, so these remain
  the rule's known residual false positives: 1 of 291 for the default list
  after the `TCP`/`UDP` exclusion. Stated rather than suppressed.

## Measured

Approximated by mirroring `gloss.go`'s pipeline plus the masks above over
terraform-provider-aws `website/docs` at provider commit `34167e0962c`. The
real numbers come from `make corpus` once an implementation exists; these are
sizing, and supersede the raw unmasked scan quoted in #100 ("thousands of
bare lowercase hits"), which counted matches inside code spans and URLs.

| word | findings | variants |
|---|---:|---|
| `ID` | 240 | `id` 188, `Id` 52 |
| `IP` | 11 | `Ip` 9, `ip` 2 |
| `CPU` | 7 | `cpu` 7 |
| `HTTP` | 6 | `Http` 3, `http` 3 |
| `HTTPS` | 5 | `https` 5 |
| `JSON` | 5 | `json` 5 |
| `URI` | 4 | `uri` 3, `Uri` 1 |
| `CLI` | 3 | `cli` 3 |
| `API`, `URL`, `XML`, `SQL` | 2 each | |
| `HTML`, `TLS` | 1 each | |
| **total** | **291** | |

`DNS`, `GPU`, `UUID`, `GUID`, `SDK`, `UI`, `CSS`, `JWT`, `SSL` produce no
findings on this corpus: no cost, and they guard against regression.

`SSH` is not in this table: it was measured excluded, under the earlier
draft's now-reversed exclusion. Needs its own `make corpus` count once
implemented, re-including `ssh`/`Ssh` in the scan — the 3-finding number
quoted against the old `TCP`/`UDP`/`SSH` exclusion bundle no longer applies
to `SSH` alone and shouldn't be reused for it.

Suppressed by the added masks: 28 glued, 10 heading. Spot-checked, the 51
non-`ID` findings are true positives except the one enum-value case noted
above.

## Rollout

`enabled` defaults to `false`. A rule with a non-empty built-in list that
defaults on would make every provider that upgrades start emitting findings
it never opted into, against `AGENTS.md`'s third priority ("existing clean
provider configs stay clean"). Default-off with a curated list is what makes
the list safe to ship; it is not in tension with being useful, since turning
it on is one line.

First release ships `severity = "warning"`. A provider that opts in and wants
CI enforcement raises it with the existing `severity` key; the check picks the
cautious end rather than choosing for them.

Because `ID` is 82% of the output, a provider can stage adoption with the
mechanisms already here: enable with `ignore_words = ["id"]`, fix the other
51, then drop the ignore. No extra toggle needed.

## Why this ships without per-finding suppression

swissshepherd has no inline ignore mechanism — no directive parsing, nothing
comment-based. The escapes are `ignore_words` (one word everywhere) and
`ignore_targets` (one check, one file). Nothing sits between them, and a
warning that fires on correct prose with no way to silence it is a reason to
question the check rather than to add an escape hatch.

The check is defensible without one because every false positive it can
produce falls into one of two classes, and per-finding suppression is the
wrong remedy for both:

- **An unbackticked literal.** `valid values are cpu and memory`, `If not
  icmp, icmpv6, tcp, udp, or all`, `only "url" can be used`. The fix is
  backticks, which removes the finding through the masking above *and*
  improves the doc — those values were always meant to be code-formatted.
  Suppressing instead would leave the doc worse. This is the whole measured
  residual: 1 of 291.
- **A legitimately lowercase English usage.** A word whose lowercase form is
  always correct — this is the risk a candidate like `GET`/`PATH`/`DEBUG`
  posed (excluded above, criterion 1) before it ever reaches the default
  list, not a residual case any shipped entry is known to produce. If one
  turns up anyway, the remedy is removing the word from the list, because a
  word that reads correctly lowercase once will again.

That claim is the check's standing justification, and it is falsifiable: the
first finding that is neither — a word that must stay lowercase in one doc
and be capitalized in hundreds of others — is the evidence that per-finding
suppression is needed. Until then it would be designed against a finding
shape no rule has produced.

Deferring costs little. `Result` already carries `Line`, so suppression would
be a post-filter in the runner keyed on `(Rule, Line)`, touching no rule.
Only 6 of 13 rule files populate `Line` today, so building the mechanism now
also means auditing the other 7 — it is cheaper later, scoped to the rules
that need it. It also has a cost this repo has paid before: an inline ignore
is a provider doc edit whose only purpose is to silence the tool, and nothing
reports one that has gone stale. That is the shape of #77's
`existsInSiblingBlock` and the #80 deferrals, so the mechanism needs an
"ignore directive that suppressed nothing" report shipped alongside it.

The one gap worth closing now is granularity between word and file:
`ignore_words = ["id"]` costs 240 true positives to silence one line.
`ignore_words` therefore accepts the same `type/name` qualification
`ignore_targets` does (#17), resolved like `Override.Matches`:

```hcl
ignore_words = ["ami", "aws_emr_cluster/ssh"]
```

Word-in-one-target scope, consistent with existing semantics, and the
suppression stays in the config where review can see it rather than in
provider docs.

## What this leaves unchecked

Per `AGENTS.md`: every exclusion names what it leaves unchecked and what
reports it instead. Here, nothing else reports any of these — by design, and
accepted because the alternative in each case is a finding that damages
correct docs.

| Unchecked | Why | Reported by |
|---|---|---|
| Casing inside code spans, fences, link targets, URLs, and colon literals | Not prose; the text is a value | nothing, by design |
| Casing in headings | The schema owns the name; unbackticked styles are accepted and capitalizing breaks resolution | nothing. `block_heading_styles` governs heading form |
| Casing in link text targeting an in-page anchor | Mirrors a heading that can't be reported | nothing, by design |
| Vendor-specific initialisms (`ARN`, `VPC`, …) | Not in the default list | nothing until a provider sets `enforce_casing` |
| `TCP`, `UDP` | Lowercase forms are config values a user types (`protocol = "tcp"`) | nothing until a provider opts in |
| A word in `ignore_words` | Provider opted out, globally or for one target | nothing |
| An unbackticked literal the masks don't catch (`valid values are cpu and memory`) | No structural signal separates it from prose | nothing. Reported *incorrectly* as a casing warning; the fix is backticks, which is also the real defect |
| Initialisms spelled out in full ("Amazon Resource Name") | Different defect | `banned_glosses`, when configured |

## Tests

`internal/check/prose_casing_test.go`, `TestProseCasing_` prefix, table-driven
with `t.Parallel()`.

- One case per masked region: code span, fence, link target, autolink, bare
  URL, colon literal, frontmatter, each asserting **no** finding.
- One case per never-guess condition asserting no finding: unterminated
  backtick span, heading line, anchor link text, each of the four glue
  characters, quoted literal, sentence-final `.` asserting the finding **is**
  made.
- Correctly cased text produces nothing: `ID`, `JSON`, `HTTPS` in prose.
- `Id` and `id` both report, confirming canonical rather than literal
  matching, and `enforce_casing` overriding a default word's casing.
- Determinism: a doc with several words on one line, run many times
  in-process, identical output.
- Fixtures frozen from the docs that exposed each class:
  `cloudhsm_v2_cluster` (`id`), `ami_copy` (`Id` and `id` together),
  `ebs_volume` (ARN literal), `api_gateway_rest_api` (`execute-api`),
  `securityhub_insight` (`Ip Filter` heading and anchor text).
- Config validation: duplicate canonical forms differing only in case, and
  an entry that equals its own suggestion.
- `ignore_words`: a bare entry silences a word everywhere; a `type/name`
  entry silences it for that target and leaves it reported in another.

## Also update

- `docs/README.md`: a rules-table row next to `banned_glosses` (line 194) and
  a config section next to it (line 283).
- No provider config change ships with this; it's off by default. Enabling it
  in terraform-provider-aws is a companion provider PR, after the findings
  from a current `make corpus` run (the 291 figure above predates `SSH`'s
  inclusion and the plural/possessive entries, so it will shift) are triaged.

## Open questions

1. **When does per-finding suppression become necessary?** Resolved as a
   trigger, not a question: the first false positive that is neither an
   unbackticked literal nor a list-curation error. If one appears, it belongs
   in its own issue as a runner-level `(Rule, Line)` filter plus a stale-
   directive report, not bolted onto this rule.
2. **Full count for the plural/possessive forms added above.** `cpus`,
   `cpu's`, and the equivalents for every other default-list word that
   actually pluralizes in Terraform docs prose need a `make corpus` count
   before they're added as entries, the same way every singular form above
   was measured rather than assumed. Likely small relative to the singular
   counts, but "likely small" isn't a corpus number.
3. **`SSH` and `CPU`'s real counts, re-measured with the current default
   list.** The "Measured" table reflects the earlier draft's exclusions;
   both are resolved to stay in by design now, but neither has been counted
   under that decision, only under the old one. First real `make corpus`
   pass should include both.

This design can be refined further during implementation, as usual — the
above are the open items worth tracking going in, not a gate on starting.
