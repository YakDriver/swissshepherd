# Coverage: one section per path, exact fields
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

> **Status:** implemented for #77 in PR #87. §7 records each step as built and measured; where the build deviated from the design, the step says so.

## Acceptance cases

The whole design exists to get these three cases right. Every rule below must preserve all three, and all three are required tests (§7).

Two same-named blocks, `Z` at `X.Y.Z` and `Z` at `T.U.Z`:

| Case | Schema | Doc | Required result |
|---|---|---|---|
| **1. Different fields** | `X.Y.Z` has `A1, A2, A3`; `T.U.Z` has `A3, A4, A5` | Each block must be documented with exactly its own fields | **Flag** whenever either block isn't documented with exactly its fields: a shared section listing the union `A1`–`A5`, a shared section listing only one block's fields, or duplicate `Z` headings whose contents can't be matched to the two paths. Today all of these pass (#77). |
| **2. Identical fields** | both have `A1, A2, A3` | **One** `Z` section listing `A1, A2, A3` | **Don't flag.** That section is correct for both paths. |
| **3. Same names, different children** | both have `A1, A2, A3`, but `A3` is a child block whose contents differ: `X.Y.Z.A3` has `B1`, `T.U.Z.A3` has `B2` | One `Z` section listing `A1, A2, A3` | **Flag.** A quick read finds the two `Z` blocks equal, but they aren't interchangeable: the shared `Z` section's `A3` bullet can lead a reader to only one version of `A3`, so it is wrong for the other path even when the `A3` sections themselves are qualified and correct. |

Two `Z` headings is a defect in every case, including case 2. A reader at either section can't tell from its heading which block it documents. (They don't share an anchor: GitHub and the Registry suffix repeated slugs with `-1`, `-2`, and the anchor comes from the heading text, not the normalized key. But a link to the second occurrence works only through that positional suffix, so reordering the headings breaks it.) The fit rule (§5) always reports a duplicated key: an error when no assignment of occurrences to paths is exact, a warning otherwise.

How the design delivers them: case 1 with one shared section is the field-existence rule (§2) plus the shared-section finding (§6). Case 1 with duplicate headings is the fit rule (§5). Case 2 passes with one section: it has nothing to report. Case 3 is disjunct 4 of the shared-section invariant (§6): "identical" means identical at every depth, the same definition #78 uses.

Not the same as #78: that issue is about *differently* headed sections whose content is identical (`` `spec.http_route.match.header.match` `` and `` `spec.http2_route.match.header.match` ``), which it would suggest replacing with one shared `` `header.match` ``. Those headings are distinct and resolve to distinct paths, so nothing here reports them. A duplicated key is a different thing and is always #77's to report.

## 1. Problem

`coverage` can pass a doc section that lists fields which don't exist at the schema path the section documents. Two mechanisms combine (details in #77):

- `checkCoverage` credits a path with the fields of **every** doc section that loosely matches it (`findAllDocBlocksIn`: exact path, 3- and 2-segment composites, bare leaf), across Argument and Attribute Reference merged (`Doc.Blocks()`). The parser also merges separate headings that normalize to the same key, and attributes bullets under a heading it can't parse to the previous section.
- The "documented attribute does not exist in schema" finding is skipped when **any** same-named schema block has the field (`existsInSiblingBlock`, added in `fa4e9e4`).

So a section that is wrong for a path is accepted, and a correct section can be blamed for a path it doesn't document. The second effect is why the suppression exists; removing the suppression alone turns it into false positives.

## 2. Core rule

> **For each schema path P that resolves to section S, every field listed in S must exist at P.**

This is the existing loop in `checkCoverage` (the documented-field check after `existsInSiblingBlock`) with the suppression deleted and resolution tightened (§4). No set comparison between paths is needed.

The two directions of coverage are checked differently, and neither ever crosses paths:

- **Field existence** (the rule above) is checked per section: each section P resolves to, in Argument Reference and in Attribute Reference, is checked against P alone.
- **Missing fields** are checked against the union of P's resolved sections in both reference sections. This keeps today's behavior where a configurable field documented only under Attribute Reference is a misplacement, not also "not documented".
- **Home section.** The union answers "is it documented at all", not "is it documented in the right place", so on its own it accepts a single-home field found only outside its home. `labels` closes that when the bullet carries a label; when it doesn't, `classifyAttrPlacement` reads it as a proper computed output (`schema_docs.go:1633-1636`) and nothing reports it. Coverage therefore reports a single-home field documented only outside its home and unlabeled. Labeled bullets stay with `labels`, so the two can't double-report.

Consequences, not premises:

- A section may serve several paths (a bare-leaf heading, a short qualified heading). The rule is checked at each of them independently. A section listing a field that is missing at any path it serves fails for that path. A section listing a strict subset passes field existence and fails missing-field coverage instead; the two checks produce different findings.
- A path is never credited with fields from a section it doesn't resolve to.

### Served paths

The paths S *serves* are the schema paths that resolve to S under §4, excluding paths in `skip_blocks` (skipped entirely, so they impose nothing on S). `ConfigUnknown` paths are included: their fields' section is unknowable, but whether a field exists at them is not.

### Field homes

Several rules below depend on which reference section a field belongs in. This is the misplacement rule's classification (argument-attribute-misplacement.md §3), extended to separate "either" from "Attribute Reference only" for child blocks. The implementation reuses `configurableArgAtPath` for the Argument Reference rows.

| Field at path P | Home |
|---|---|
| `Required` or `Optional`, not `Computed` | Argument Reference only |
| `Optional` + `Computed` | either |
| Computed-only | Attribute Reference only; either when `allow_inline_read_only = true` |
| a child block whose subtree has a pure-configurable field (`Required` or `Optional`, not `Computed`) | Argument Reference only |
| a child block whose subtree has `Optional` + `Computed` fields but no pure-configurable field | either |
| a child block whose subtree has no `Required` or `Optional` field | Attribute Reference only; either when `allow_inline_read_only = true` |
| any field of a `ConfigUnknown` block | either |

A field with a single home *must* be listed in that reference section's section for P. A field whose home is "either" may be listed in either one.

## 3. Why per reference section

Paths are legitimately split between the two reference sections, often under different heading styles. Measured example, `aws_bedrockagentcore_oauth2_credential_provider`:

```markdown
## Argument Reference
### `github_oauth2_provider_config` Block        <- configurable fields
## Attribute Reference
### `oauth2_provider_config.github_oauth2_provider_config` Block   <- computed fields
```

Resolving against the merged view picks the exact-path key (the Attribute Reference section) and loses the Argument Reference fields: 68 fields on 15 paths across 7 resources would be falsely reported as undocumented. Resolving each reference section separately avoids this.

## 4. Resolution

Resolution uses headings only. For schema path `P` within one reference section, take the first rule that yields a section:

1. **Exact path.** A section keyed exactly `P`.
2. **Most specific name match.** The existing `findAllDocBlocksIn` order (3-segment composite nearest-first, then 2-segment, then bare leaf), taking only the first match, **excluding any dotted key that is itself a different schema path**. A dotted heading that names a real path claims only that path, as in the misplacement resolver (argument-attribute-misplacement.md §4). The bare leaf is never excluded, even when it is also a root-level path (`### visibility_config` with both `visibility_config` and `rule.visibility_config` in the schema): it is a leaf-name heading and serves every path with that name, and the field checks decide per path whether that is correct. Excluding it was measured in step 1 and produced 8 false "block is not documented" errors on blocks identical to the root one (acceptance case 2).
3. Otherwise unresolved: the path is undocumented in that reference section.

Taking only the first match is what fixes the appmesh case: `spec.grpc_route.match.metadata.match` has its own exact heading, so rule 1 picks it and `spec.grpc_route.match`'s section is no longer added on top. Rule 2's exclusion covers the remaining case, where the deeper path has no heading of its own: a heading naming `a.b` doesn't document an undocumented `a.x.b`.

How strict this is depends on the heading style, by design. A heading naming a full path claims exactly that path. A bare-leaf or short qualified heading claims every path it matches and is checked against each of them. Both follow from §2; the author chooses the scope by choosing the heading.

If two heading occurrences normalize to the same key, P resolves to the key and §5's fit rule decides, not a per-occurrence check. Checking each occurrence against every path would report correct docs: with occurrences `{A1,A2,A3}` and `{A3,A4,A5}` and paths to match, the second occurrence checked against `X.Y.Z` reports "`A4` doesn't exist", and following that deletes a correct field.

### Links

Links don't resolve paths. On a rendered page a reader can't see what links to the section they're on, so a section must be identifiable from its heading. This reverses the #51 shared-subsection behavior in `checkAttributeCoverage`, where a parent bullet's link to a differently named section (`management` → `#### Endpoint`) credited that section to the path. Measured cost, corrected in step 1: on the full config this changes nothing, because that config leaves `nested_object_attributes` off. The weak config sets it, and there the change adds 23 Read-Only coverage errors in 5 docs, all of them sections reached only through a link: `aws_fsx_ontap_file_system` and `aws_fsx_ontap_storage_virtual_machine` (`#### Endpoint`), `aws_wafv2_managed_rule_group` (`### Labels`), and `aws_db_instance` (`### Endpoint`). These are the intended result: the headings don't identify the blocks. A combined heading naming each block (`` #### `intercluster` and `management` ``) shares a section without links. `docs/rules/schema_docs.md` ("Shared subsection") and the #51 tests change accordingly.

Link correctness belongs to the `anchors` rule. Today it catches dead fragments only; a live link to the wrong section (`aws_wafv2_rule_group`: `allow` → `#action`) is not caught anywhere. Extending `anchors` for that (#82), and an opt-in requirement that every block bullet links to its section (#83), are separate enhancements.

## 5. Sections that resolve to no path

Tightening resolution opens a hole. A section that no path resolves to is entirely unchecked: its fields are compared against nothing. Today many such sections are checked, if wrongly, because loose matching credits them to some path, the parser merges them into another section, or their bullets are attributed to the previous section. After §4 they would silently drop out of phantom-field checking. §5 reports them so that doesn't happen. It is the backstop for §4 and ships with it.

Three cases:

- **Unparseable heading.** An H3/H4 inside Argument or Attribute Reference that matches no configured `block_heading_styles` template.

  **Attribution change (separate from the warning).** Bullets under an unparseable heading belong to no section. Before #77 the parser credited them to whichever section came before, which produced phantom fields, false ordering findings, and misattributed labels there. Dot-path reference bullets (`a[*].b[*].c`) name their own path and are still routed. This removes findings and ships before step 2 (commit sequence); the warning adds findings and ships in step 4. Each diff then moves one way. Orphaned bullets get no field, ordering, description, or label checks until their heading is fixed; the step-4 warning is what reports that (§9).
- **Prose-introduced list.** A colon-terminated paragraph that names a schema block in backticks ("The `cloudwatch_logs` object takes the following arguments:") is a heading in all but syntax: its list documents that block, keyed as a heading for it would be (`suggestHeadingKey`), and gets every check a section gets. Like a heading, it is in effect until the next heading or lead-in, so a code block interrupting its list doesn't end it. The `heading`-style warning asks for a real heading. Three exceptions, each where reading the prose as a heading would credit the wrong block:
  - Prose naming nothing in the schema ("…supports the same arguments as `aws_instance`, with the addition of:") continues its section.
  - Prose before any list under the reference section's own heading (the byline's place) or under a heading that resolves to a schema path introduces that heading's list. Under an unparseable or unresolved heading it opens the block, which also gives those bullets a section.
  - Markdown can't end a list: a bullet after a blank line continues it, and authors resume the enclosing section that way (`aws_codepipeline_custom_action_type`). A trailing run of bullets that are fields of the enclosing section and not of the named block stays in the section, and the warning names the line where the list should end. Crediting them to the named block would report correct fields as nonexistent. A section field anywhere else in the list belongs to the named block, as under a heading, and is reported there.

  Prose that names the current section's own block ("The `rule` block also supports:" under a `rule` heading) is never a candidate.

  Both this and the unparseable-heading warning suggest a heading when the text names a schema block: candidate paths end in a named identifier, and those containing the most other named identifiers as segments win (`source: auth` → `source.auth`). One winner is suggested by leaf if that leaf is unique in the schema, else by full path; several winners sharing a leaf are suggested by the leaf. The suggestion is advisory and resolves nothing.

- **Unresolved section.** A parsed heading that no schema path resolves to, for example because every path it could match has a more specific section, or because a descriptive title (`Managed Query Results Encryption Configuration`) doesn't normalize to a schema name. Excluded to avoid double reporting: Argument Reference headings that `checkPhantomBlocks` already reports (no schema block with that leaf), and Attribute Reference headings naming object-typed attributes (`nestedAttributeLeaves`), which `checkPhantomBlocks` deliberately allows. Also excluded, found in step 4c: headings with no bullets (`### GuardDuty Cleanup Permissions`, prose only), which hide no unchecked fields. The message says why the heading resolves to nothing: no schema block has that name; every block with that name resolves to a more specific heading; or, for a dotted key, no block with that name sits under that parent.
- **Duplicate heading.** Two or more *headings* within one reference section that normalize to the same key. (The same key in Argument and Attribute Reference is the normal split described in §3, not a duplicate.) Position and heading level are not used to tell them apart, so swissshepherd can't know which occurrence documents which path. It doesn't need to: it checks whether **any** assignment of occurrences to paths makes every block exact.

  An occurrence **fits** a path when every property in the §6 invariant agrees: every field it lists exists at the path; every field whose only home is this reference section is listed; and its labels and deprecation markers are correct for the path. For the paths a duplicated key serves:

  | Condition | Result |
  |---|---|
  | Some path has no occurrence that fits it, or some occurrence fits no path | **error**: whichever occurrence documents it, some block lacks its exact fields (acceptance case 1). Names the path, or the occurrence and line, and the closest occurrence by number of mismatched fields |
  | Every path has a fitting occurrence and every occurrence fits some path | **warning**: the content can be matched to the paths, but a duplicated key is still a defect: a reader can't tell from either heading which block it documents. Qualify the headings. |

  There is no passing case. A duplicated key is always reported, including when every occurrence fits every path (acceptance case 2, two `Z` headings): the fields are right but the headings are not. The correct form of case 2 is one section.

  Three refinements from implementation (step 4b):

  - **Headings with no bullets aren't fitted.** "#### ebs_config / See `ebs_config` under `core_instance_group` above." documents nothing; fitting it would report every field as missing, naming the wrong defect. It still counts toward the duplicate.
  - **The closest occurrence is chosen by content first**: fewest fields that don't exist or aren't listed, then fewest differences overall. A heading with the right fields and a wrong label is nearer than one listing a field that isn't there (`aws_cognito_risk_configuration`, two `#### actions`).
  - **Labels count only when present**, as in `labelCorrectness`; an unlabeled bullet is the `labels` check's concern.

  The error fires only when every possible assignment fails, so it is certain, not a guess. Assignment is not a bijection — two paths may share one occurrence that fits both — so the condition above is exact and no matching algorithm is needed: two loops over occurrences and paths decide it.

  **Storage.** The parser keeps one entry per key and adds an ordered list of **occurrences** to it, each with its own fields and heading line. Coverage and the fit rule read the occurrences, never the merged union: the union `{A1…A5}` credited to both paths is exactly what hides case 1 today, and checked against either path without the suppression it would report correct docs. Labels, ordering, descriptions, and misplacement keep reading the merged entry, so their behavior is unchanged and nothing they check becomes unchecked.

  The parser handles three cases when a heading's key is already present (`internal/doc/doc.go:819-845`): a brand-new entry; an entry that pre-existed **without** a heading, whose content was routed there from elsewhere on the page (a dot-path reference bullet like `rule[*].probabilistic[*].x`, or a prose lead-in), which is merged and marked `SpansSubsections`; and a second heading normalizing to the same key, also marked `SpansSubsections`. Only the third case adds an occurrence. Routed content joins the merged entry but no occurrence: with several headings for its key, which one it was meant for is unknowable, so the fit rule ignores it. (The original design assigned it to an occurrence; that can't be done without guessing.) The routing that brings fields documented elsewhere on the page to the right path is otherwise unchanged. `SpansSubsections` is unchanged too, so the misplacement rule's whole-subsection collapse behaves exactly as today.

These are doc-mapping findings, not field defects, and several fire on docs that are readable (e.g. `aws_athena_workgroup`). They ship as **warnings**, per the `AGENTS.md` default for high-volume checks. The one exception is a duplicated key whose occurrences can't be matched to its paths: that is a certain field defect (acceptance case 1), so it is an **error**, like the field-existence finding it stands in for.

Release condition: §5 doesn't have to land in the same commit or PR as §4, and "unresolved section", the case with the most exclusion logic and no measurement yet, may land after the other two. But all three must be in the same swissshepherd release as §4. The backstop exists so that no provider ever runs the tightened resolution without something reporting the sections it leaves unchecked.

## 6. Findings

Each finding names one schema path and one field, so a reader (typically an AI agent fixing docs) can act on it without cross-referencing. Messages are not aggregated for display.

| Condition | Severity | Finding |
|---|---|---|
| A field listed in S doesn't exist at P | error | "documented argument/attribute `X` in block `P` does not exist in schema (section `S`, line L)". When S also serves paths where `X` exists, append: "`S` also documents `Q1`, `Q2`, `Q3` (and K more), where `X` exists" |
| A field expected at P is listed in no section P resolves to | unchanged (`severity(attr)`) | unchanged message: "attribute `X` in block `P` is not documented". When the section that documents P in the reference section where `X` belongs also serves paths where `X` doesn't exist, append the mirror pointer: "the section documenting `P` (`S`, line L) also documents `Q1`, `Q2` (and K more), where `X` does not exist" |
| S serves paths `Q` and `P`, and some field `X` must be listed in S at `Q` (single home in S's reference section) but doesn't exist at `P` | warning | "section `S` (line L) documents N paths; `X` must be listed for `Q` but doesn't exist at `P`. Qualifying means up to N sections: give `Q` a heading that resolves only to paths where `X` exists, e.g. `` `Q` Block ``" (see Suggested headings) |
| S serves `Q` and `P`; `X` is listed in S or single-home there, and has a different schema-correct label at each (disjunct 2; one finding per conflicting field) | error | "section `S` (line L) documents N paths; `X` is (Required) at `Q` and (Optional) at `P`, so one section cannot label it correctly. Qualifying means up to N sections: give `Q` a heading that resolves only to paths where `X` has the same label, e.g. `` `Q` Block ``" |
| S serves `Q` and `P`; `X` is listed in S or single-home there, and is `Deprecated` at one and not the other (disjunct 3; one finding per conflicting field) | error | "section `S` (line L) documents N paths; `X` is deprecated at `Q` but not at `P`, so one section cannot mark it correctly. Qualifying means up to N sections: …" |
| S serves `Q` and `P`; child block `X` is listed in S or single-home there, and `Q.X` and `P.X` aren't interchangeable (disjunct 4; one finding per conflicting child block) | warning | "section `S` (line L) documents N paths; `X` differs below this level between `Q` and `P` (e.g. `B1` exists at `Q.X` but not at `P.X`), so `S`'s `X` bullet can't lead to the right `X` for both. Qualifying means up to N sections: …" |
| A single-home field is documented only outside its home reference section, with no label | `severity(attr)` | "argument `X` in block `P` is documented under Attribute Reference but is configurable; move it to Argument Reference and label it (Required)/(Optional)" |
| P unresolved in both reference sections, has configurable fields, and no ancestor is also reported | error | "block `P` is not documented", plus when non-zero: "(M paths beneath it are also undocumented; K other undocumented paths share the name `L`)". Counts, not promises: a bare `L` heading documents those K paths only if they are interchangeable; otherwise it trades them for shared-section findings. |
| Unparseable heading (§5) | warning | heading text and line |
| Unresolved section (§5) | warning | heading text and line; "its fields are not checked against the schema" |
| Duplicate heading (§5), occurrences can't be matched to paths | error | the path with no fitting occurrence, or the occurrence (heading, line) that fits no path, and the closest occurrence by number of mismatched fields |
| Duplicate heading (§5), occurrences matchable to paths | warning | all headings and lines; the paths the shared key serves (sorted, capped at three, with a count of the rest); ask the author to give each occurrence the heading of the path it documents, or to keep one section when the paths are interchangeable |

The field-existence finding is one check with an optional pointer, not two findings. Whether `X` exists at some other path S serves changes only the appended clause.

For a key with more than one occurrence, the fit rule (§5) **replaces** the per-path field-existence and missing-field findings and the shared-section finding for that section: none are emitted from it. The shared-section finding would only repeat the fit rule's remedy (qualify the headings), and nothing is lost by dropping it: a duplicated key is always reported, so a child-content difference under it (disjunct 4) still surfaces, as the duplicate-heading warning. Running both would compare the merged union against each path and report correct docs — `{A1,A2,A3}` and `{A3,A4,A5}` under two `Z` headings would be told that `A4` doesn't exist at `X.Y.Z`, and following that deletes a correct field. The fit error stands in for those findings, which is why it carries their severity.

Both field findings need a pointer, and the missing-field one matters more. It is the finding that misleads: given `P{a,b}` and `Q{a,b,c}` sharing a section listing `{a,b}`, "attribute `c` in block `Q` is not documented" reads as "add `c` here", and doing that produces a field-existence error at `P` on the next run. The mirror pointer says `c` doesn't belong in the shared section. Neither pointer carries the remedy: with 325 field errors in `aws_wafv2_web_acl`, "qualify the heading" belongs in the one shared-section warning, not in every field message.

Determinism: the `Q…` list is sorted and capped at three, with a count of the rest. The compared pair is always (representative, first differing path), both in sorted path order, per the cost rule below — never a pair found by iteration. Disjunct 1 reports one representative conflict per section, the first qualifying (`X`, `Q`, `P`) in sorted order (by `Q`, then `X`, then `P`), since there are usually many. Disjuncts 2, 3, and 4 emit one finding per conflicting field or child block.

The existing message says "documented attribute" for fields under Argument Reference. Since this finding replaces it, it says "argument" under Argument Reference and "attribute" under Attribute Reference.

### Suggested headings

The shared-section warning suggests the **full path** of `Q` as an example heading. It needs no verification: rule 1 matches a full-path key exactly, and rule 2's exclusion stops any other path from claiming it, because a full-path key is itself a schema path. The heading always resolves, it can't flap, and no search is involved.

A full-path heading for `Q` clears the reported (`X`, `Q`, `P`) conflict, but not necessarily the finding: `S` may still serve other conflicting paths. What is guaranteed is the upper bound: giving every served path its full path always clears it. So the message states the served-path count N as "up to N sections". N is what separates a two-minute fix from a restructuring decision, and it is already known.

The message states the condition as well as the example, so an agent can choose a shorter heading itself rather than copying the full path:

> "section `S` (line L) documents N paths; `X` must be listed for `Q` but doesn't exist at `P`. Qualifying means up to N sections: give `Q` a heading that resolves only to paths where `X` exists, e.g. `` `Q` Block ``"

The example is rendered with the first `{Path}` template in `prefer_block_heading_styles`, falling back to `` `{Path}` Block ``, as `checkHeadings` does today. Coverage re-verifies whatever heading the author chooses on the next run.

Without this, an agent following a full-path example literally would turn `format_configuration` in `aws_quicksight_analysis` (243 paths) into 243 full-path sections where 9 two-segment headings suffice. The result would be correct docs, but bloated.

The duplicate-heading warning doesn't suggest a heading per occurrence. Position isn't used to resolve duplicates (§5), so swissshepherd doesn't know which path each occurrence documents, and assigning one would be a guess. It lists the paths the shared key serves (sorted, capped at three, with a count of the rest) and asks the author to give each occurrence the heading of the path it documents.

The suggestion is advisory text. It isn't part of the correctness story: the field findings and the trigger stand on their own.

The suggestion is not withheld when N is large (WAFv2 `statement`, below). The example is correct, so it is expensive to follow, not wrong. Withholding it would remove the only concrete thing in the message while leaving the finding in place. The message prints one example heading, not N of them, so suppressing it saves no output. N lets a reader defer to #74 on evidence rather than on the tool's judgment of what is practical.

No config lever is needed or offered for it. `skip_blocks` wouldn't serve: it compares full paths exactly (`slices.Contains(r.skipBlocks(), blockPath)`; the default `"timeouts"` is a root-level name), with no prefix form and no `_file` variant (#79), so covering WAFv2 `statement` would take 156 entries across three resources. The lever that exists is the resource-level `schema_docs` exclusion, and all three WAFv2 resources already use it in the weak config (§8), so these findings surface only in the full config.

### Follow-up: shorter suggestions (optional)

Tracked in #84. Optional polish on an advisory message, not part of #77. If full-path examples prove annoying in practice, an optimizer could suggest the fewest, shortest headings instead. It would have to work within what the resolver matches. §4 accepts exactly four key forms for path `P` with leaf `L`:

- the full path `P`;
- `a.b.L`, where `a.b` is any **adjacent** pair of `P`'s ancestors;
- `a.L`, where `a` is **any** ancestor of `P`, not only the parent;
- the bare leaf `L`;

minus any partial key that is itself a schema path (rule 2's exclusion). Trailing path suffixes are the wrong model: a partial key of four or more segments matches nothing, and non-adjacent keys (`bar_chart_visual.field_wells`) are missed. Candidates would have to be verified with the reverse resolver over **every** path with leaf `L`, since a new `a.L` heading can capture a path that currently resolves to a different section.

A rough sizing, from a simulation of §4 over a subset of such candidates (`a.L` at each ancestor distance, adjacent pairs at the parent distance, bare `L` with full-path overrides, full paths), using the trigger on Argument Reference fields. Counts are upper bounds. The last column is the full-path bound if one bare-leaf section served every path with the name:

| resource | conflicting names | sections suggested | full-path upper bound (sum of N) |
|---|---:|---:|---:|
| `aws_quicksight_analysis` | 44 | 161 | 1,199 |
| `aws_wafv2_web_acl` | 8 | 35 | 2,059 |
| `aws_wafv2_rule_group` | 4 | 24 | 1,644 |
| `aws_wafv2_web_acl_rule` | 8 | 30 | 4,123 |

The gain is concentrated in names that were already cheap to fix: `format_configuration` (243 paths, 9 two-segment headings) and `match_pattern` in `aws_wafv2_web_acl_rule` (2,160 paths, 3 sections). The largest QuickSight cases are one section per visual type (`chart_configuration` 21, `field_wells` 20), the real structure of the API.

WAFv2 `statement` is where no optimizer helps much. It has only 2–3 shapes, because nesting is capped at 3 levels, but no partial key expresses nesting depth: `and_statement.statement` occurs at every level, and `rule.statement` is itself a schema path, so rule 2 excludes it as a qualifier. The best candidate the simulation found is almost entirely full-path headings:

| resource | `statement` paths | shapes | sections | full-path headings | partial headings |
|---|---:|---:|---:|---:|---:|
| `aws_wafv2_web_acl` | 64 | 3 | 20 | 19 (up to 6 segments) | 1 (`statement.statement`) |
| `aws_wafv2_rule_group` | 52 | 3 | 17 | 16 (up to 6 segments) | 1 |
| `aws_wafv2_web_acl_rule` | 40 | 2 | 14 | 13 (up to 5 segments) | 1 |

The `statement` field-existence errors are correct (the shared section lists `and_statement`, `or_statement`, and `not_statement` at a level where they aren't valid). Qualifying is possible but costly, 51 sections of about 20 bullets each across the three resources. How those docs should be structured is outside #77 (see #74).

### Never guess

No field-level finding is produced when the path is in `skip_blocks` (not served, per §2) or the section resolves to no path (reported by §5 instead). `ConfigUnknown` does **not** suppress field existence: a field that doesn't exist at a path doesn't exist there regardless. It only makes the field's home "either" (§2 field homes), which affects missing-field coverage and the shared-section trigger. This follows `AGENTS.md`: never-guess governs placement and labels, and does not suppress coverage.

### Dedup and cascades

`AGENTS.md` requires dedup by schema path, never by leaf. Coverage previously deduped missing blocks by leaf (`reportedMissingBlocks`) and phantom fields by leaf+field (`reportedExtraAttrs`); both are gone, so every finding names one path.

Missing blocks multiply two ways, and the message accounts for both:

- **Shared leaf name.** One missing heading, one finding per path with that leaf. Each finding states how many other undocumented paths share the name.
- **Descendants of an undocumented block.** Only the shallowest undocumented block in each subtree is reported, with the count of undocumented paths beneath it. Under the §9 test nothing becomes unchecked: the descendants can't be reached from the docs, and once the ancestor has a section the next run reports them. The real defect is the missing ancestor section, and a finding naming a descendant would send an agent to write a section nobody can navigate to. "Undocumented" here means the same thing as the finding: configurable fields, not in `skip_blocks`, no section in either reference section. A container ancestor (no configurable fields) or a skipped one is never reported, so it never absorbs descendants.

This is a different axis from leaf dedup and compatible with it: each finding still names one real path, and the hidden count is stated.

### The shared-section finding moves into `coverage`

The `heading` sub-check already warns that a heading "is ambiguous". That finding becomes coverage's shared-section warning above, and `checkHeadings` keeps only the preferred-style check.

Two reasons. First, its trigger has to be coverage's own comparison, not a separate one, and it has to be defined over single-home fields. Today it is a `blockSignature` difference plus `schemaPathsResolvedByDocKey(...) > 1` under loose matching, which fires on sections §2 considers correct:

- Whole-block signatures ignore the split between reference sections. `P{a,b}` and `Q{a,b,c}` with `c` computed-only at `Q` and documented in `Q`'s Attribute Reference section is the legitimate shared-section case: the Argument Reference field sets are identical.
- Signatures ignore coverage's filters. If `Q` is in `skip_blocks`, or `c` is in `phantom` or matched by `shouldSkipAttribute`, the user has declared the difference nonexistent.

Comparing expected field *sets* can't fix this, because some fields have no single home: an `Optional`+`Computed` field, a computed-only field under `allow_inline_read_only = true`, and any field of a `ConfigUnknown` block may appear in either reference section (§2 field homes). "Expected in this section" is undefined for them, so any set comparison reports a difference where the docs are correct.

Anchoring the trigger on what S lists doesn't work either: given `P{a,b}` and `Q{a,b,c}` sharing a section that lists `{a,b}`, no listed field is missing anywhere, so the warning would never fire, and the only finding would be the missing-field error at `Q`, the one finding §6 says misleads without the remedy.

The trigger is therefore defined on the schema, and stated as an invariant rather than a list of conflicts:

> **A shared section is valid only when every schema-derived property swissshepherd compares is identical across the paths it serves.**

Enumerating conflicts one at a time is how `existsInSiblingBlock` happened. The properties are fixed by the checks that consume them, and the list is closed today:

| Property | Compared by | Disjunct |
|---|---|---|
| the field exists at the path | `checkCoverage` | 1 |
| `Required` / `Optional` / computed-only (the label) | `labelCorrectness` | 2 |
| `Deprecated` | `checkDeprecated` (`schema_docs.go:2045-2072`) | 3 |
| a child block's contents, recursively (the child's own paths must themselves be interchangeable under disjuncts 1–4) | every check above, one level down | 4 |

Bullet order and description prefixes can't diverge: `checkOrdering` (547) and `checkDescriptions` (560) are doc-internal and never consult the schema. There is no `ForceNew` comparison anywhere. **Any new check that compares a per-field schema property to a doc bullet has to add a disjunct here, or shared sections silently misrepresent again.**

Concretely, S serves paths `Q` and `P`, and for some field `X`:

> 1. `X`'s only home at `Q` is S's reference section, and `X` doesn't exist at `P`; or
> 2. `X` is listed in S, or its only home at `Q` is S's reference section, and `X` exists at `P` with a different schema-correct label; or
> 3. `X` is listed in S, or its only home at `Q` is S's reference section, and `X` is `Deprecated` at one of them and not the other; or
> 4. `X` is a child block that is listed in S, or whose only home at `Q` is S's reference section, and `Q.X` and `P.X` are not interchangeable: some disjunct 1–4 holds between them, in either reference section.

Only disjunct 1 is restricted to single-home fields, because it is about a field that may be absent from S. Disjuncts 2 and 3 are about a bullet's label or deprecation marker, which the per-path checks compare for whatever S lists, regardless of home. Restricting them too would leave an `Optional`+`Computed` field, listed in a shared section and deprecated at only one served path, producing the contradictory `checkDeprecated` pair with nothing suppressing it.

Disjunct 4 is acceptance case 3. Without it, a shared section passes whenever its paths agree at its own level, even if a child block differs underneath. Field existence, labels, and deprecation all pass at `Z`, and the child `A3`'s own sections may be correct and qualified, yet the shared `Z` section's `A3` bullet can point a reader to only one of them. No check at `A3`'s level can see that, because the defect is in `Z`'s section. The recursion uses the same served-path definition, filters, and field homes at every level, so a difference only in a skipped path or filtered field never propagates upward. Measured on the `hashicorp/aws` schema: of 1,353 groups of same-named blocks within a resource that agree at their own level and occur at more than one path, 24 (in 11 resources) differ somewhere below, most in the three WAFv2 resources (4 each), plus `aws_macie2_classification_job` (`includes`, `excludes`), `aws_cloudfront_distribution` (`forwarded_values`), `aws_appmesh_virtual_node` and `aws_appmesh_virtual_gateway` (`validation`), and `aws_appautoscaling_policy` (`metric_stat`). That measurement compared all fields and flags, before field homes and filters, so it is an upper bound.

Object-typed attributes have children too. With `nested_object_attributes = true` they are expanded into blocks and disjunct 4 covers them. With it off, their children aren't compared at all, which is #81.

Disjunct 1 covers both ways a shared section can be wrong about existence: listing `X` (the union) and omitting it (the intersection). Fields whose home is "either" never force the conflict on their own; if one is listed in S where it doesn't exist, the field-existence error reports it, and moving one bullet is a smaller fix than splitting a heading.

Disjunct 2 is the label case, and it exists because a section carries one label per bullet. `X` `Required` at `Q` and `Optional` at `P` cannot be documented correctly by one shared section, whichever label it uses. Because the correct label follows from `Required`/`Optional`/computed-only, this also covers `X` pure-configurable at `Q` and computed-only at `P`, where the labels are `(Required)` and `(Read-Only)`. Measured: 22 block names across 17 resources have a field whose requiredness differs by path (`metric_name` and `namespace` under `metric` in `aws_appautoscaling_policy`; `interval` under `retain_rule` in `aws_dlm_lifecycle_policy`). The configurable/read-only variant has no current cases, but the definition covers it because the condition is about labels, not about requiredness specifically.

Disjunct 3 is the same argument for deprecation, and it is in scope because §7 moves `checkDeprecated` onto the §4 resolver. `checkDeprecated` checks both directions: "deprecated in schema but not marked in docs", and "marked deprecated in docs but not in schema". So a shared section whose paths disagree gets one finding at each path, and they contradict — mark the bullet and the non-deprecated path fires, leave it and the deprecated path fires. One bullet cannot satisfy both. Not yet measured; the disjunct is defined regardless, for the same reason as the configurable/read-only variant.

Second, disjunct 1 fires only when some other finding also fires, so for that case the finding carries the remedy rather than detecting anything new. If S lists `X`, `P` gets a field-existence error. If S doesn't list `X`, `Q` gets a missing-field error, unless `X` is documented in `Q`'s section in the other reference section, in which case `labels` reports the misplacement. Using the served-path definition (which excludes `skip_blocks`) and coverage's attribute filters (`shouldSkipAttribute`, `phantom`) is what makes this hold; without them the trigger could fire on a path or field coverage never evaluates.

Disjuncts 2, 3, and 4 are different: they detect defects nothing else reports. For disjunct 4, the child's own sections may be qualified and correct, so nothing at the child's level fires; the defect is the parent's shared bullet. `labelCorrectness` resolves a section with `resolveSubsectionPath`, which returns unresolved for exactly the shared and short-qualified keys this finding is about (§9), so it emits nothing there. For deprecation, `checkDeprecated` does emit, but what it emits is a contradictory pair. In both cases field existence passes because `X` exists at both paths, and missing-field coverage passes because `X` is listed. Severity parity holds — a wrong label or deprecation marker is an error everywhere: in `labels`, in `checkDeprecated`, in these disjuncts, and under the fit rule — and these disjuncts are not advisory, and `coverage = false` removes the only coherent check for them.

Because disjuncts 2, 3, and 4 are sole detectors, they emit **one finding per conflicting field** (per conflicting child block for 4), not one per section. Disjunct 1 reports a single representative conflict, because the per-path field-existence errors already enumerate the rest; nothing enumerates a label or deprecation conflict, so reporting only the first would hide the others until it was fixed.

### Cost: compare to a representative, not pairwise

The disjuncts are written as a relation between two served paths, but they must not be evaluated over pairs. `match_pattern` in `aws_wafv2_web_acl_rule` serves 2,160 paths; pairwise is 2.3 million comparisons, each recursing into subtrees for disjunct 4. Implemented that way the check is unusable on exactly the resources this design is about.

Not every relation here is transitive, so "compare each path to one representative" is only valid for some disjuncts. Labels and `Deprecated` are plain values, and equality of values is transitive. Interchangeability is not, because disjunct 1 is one-directional (a field whose home is "either" never forces a conflict). Counterexample, with one field `X` at three served paths: at `A`, `X` is `Optional`+`Computed` (home: either); at `B`, `X` is absent; at `C`, `X` is `Optional` only (home: Argument Reference). `A`–`B` agree (an either-home field's absence never conflicts), `A`–`C` agree (both `(Optional)`), but `B`–`C` conflict (`X` must be listed at `C` and doesn't exist at `B`). A representative `A` would report nothing. So each disjunct is evaluated over the whole served set, which stays O(N × fields) per section:

- **Disjunct 1**: for each field, collect the served paths where it exists and those where its only home is S's reference section; fire when the second set is non-empty and the first isn't all served paths.
- **Disjuncts 2 and 3**: for each field, the representative is the first served path, in sorted order, where the field exists (a single per-section representative has no value for fields absent there). Walk the remaining paths where it exists once, comparing label and `Deprecated`.
- **Disjunct 4**: for each child block `X`, evaluate the set {`P.X` for every served `P` where `X` exists} with the same procedure, as if one section served it: disjunct 1 over the set, 2 and 3 against the per-field representative, and 4 recursively. No pairs are compared. Results are memoized by the sorted set of child paths, since the same subtree sets recur across sections and levels.

The reported pair for disjuncts 2 and 3 is (per-field representative, first differing path in sorted order); for disjuncts 1 and 4 it is the first qualifying (`Q`, `P`) in sorted order. Both are deterministic and name two real paths. The fit rule's "closest occurrence by number of mismatched fields" breaks ties by occurrence order.

The finding must therefore be emitted whenever `coverage` is, and `heading = false` must not remove it. Conditioning a `checkHeadings` finding on `enabled(r.Coverage)` would do that, but coverage already holds the resolution result and the filtered field sets, so there is nothing left for `checkHeadings` to contribute and no shared-resolver contract to keep in sync.

It is emitted per (section, reference section). `checkCoverage` iterates schema paths (`slices.Sorted(maps.Keys(rs.Blocks))`), not sections, so the section→paths map is built from the resolution results and these findings are emitted after the path loop, in sorted section order.

The `labels` precedent doesn't apply in either direction. `labels` suppresses its message because `checkComputedMisplacement` describes the same defect at the same line with a better fix. Here the shared-section warning and the field errors are cause and symptom at different granularities; neither can be dropped, and after the move they are emitted by the same sub-check.

Two consequences to accept deliberately:

- A config with `coverage = false, heading = true` loses ambiguity detection, and with it the only coherent check for cross-path label, deprecation, and child-content conflicts (disjuncts 2–4). Reimplementing the trigger in `checkHeadings` to preserve it would leave two implementations of one comparison, which `AGENTS.md` warns against.
- Severity: disjuncts 1 and 4 are warnings; 2 and 3 are errors (step 6b). Disjunct 1 accompanies at least one coverage or `labels` finding, which carries the defect's own severity (a deprecated field's missing-field finding is a warning, per `severity(attr)`). Disjunct 4 accompanies the child-level findings, or explains a parent bullet no single child section can be right for. Disjuncts 2 and 3 stand alone: no single marker is right for every path, and an error matches what `labelCorrectness`, `checkDeprecated`, and the fit rule emit for the same defect.

## 7. Scope

In scope:

- `checkCoverage`, `checkAttributeCoverage`, and the parser changes in §5.
- **Resolver consolidation.** Today there are two overlapping resolvers: `findAllDocBlocksIn` returns every loosely matching section, and `findDocBlockIn` returns its first match, with a doc comment telling callers to use `findAllDocBlocksIn` when they need full coverage. After §4 that advice is inverted and the multi-match behavior has no consumer: `checkCoverage` (`findAllDocBlocksIn` at `schema_docs.go:177`), `checkAttributeCoverage` (412–413), `schemaPathsResolvedByDocKey` (773), and `checkDeprecated` (via `findDocBlock` at 2022) all want exactly one section per reference section. They move to a single resolve function implementing §4. `findAllDocBlocksIn`'s ordering becomes the ranking step inside it, and the rule 2 exclusion lives there. `findDocBlockIn` folds into it. The `anchors` parameter and the shared-subsection link branch are deleted. No second resolver remains to drift from the first (`AGENTS.md`: "if two existing implementations disagree, don't adopt whichever you found first").
- **`checkDeprecated` and the merged view.** It has the same loose-matching defect as coverage: `findDocBlock` wraps `findDocBlockIn(d.Blocks(), …)`, so a path's deprecation markers can be compared against a section that documents a different path. It resolves per reference section like the coverage checks, which changes its findings, so it belongs in the corpus diff. `Doc.Blocks()` has exactly two production callers, 177 and 1744; once both move, the merged view has none and is deleted. Its loop also ranges `ctx.Schema.Blocks` unsorted (2020) and needs `slices.Sorted` for the same reason `checkCoverage` does (#65). Moving it onto the §4 resolver is what makes a shared section with divergent deprecation emit a contradictory pair, which is why disjunct 3 is in #77 and not a follow-up; the per-path deprecation findings are suppressed for a field disjunct 3 reports, the same way disjunct 2 suppresses per-path label findings in §7's follow-up.
- **Home-section check for unlabeled fields.** A single-home field documented only outside its home with no label is accepted today by both coverage (the §2 union) and `labels` (`placementOK` for an unlabeled bullet). Coverage reports it; labeled bullets stay with `labels`. Small and measured at the root (2 cases), unmeasured nested, so it needs its own line in the corpus diff.
- `existsInSiblingBlock` has one caller (`checkCoverage`) and is deleted, along with the comment in `checkPhantomBlocks` that refers to it.
- `TestCoverage_DuplicateBlockNames` in `internal/check/coverage_test.go` (the `fa4e9e4` regression test) asserts that a merged `### tcp Block` covering `connection_pool.tcp` and `timeout.tcp` passes. That is the behavior being removed, so the test is inverted: the merged section now errors for the fields that don't exist at each path, and the qualified form (`` `connection_pool.tcp` Block `` / `` `timeout.tcp` Block ``) passes. The other tests in that file are reviewed against §2.
- The `heading` ambiguity warning moves into `checkCoverage` as the shared-section finding (§6). Its trigger is rebuilt on the §4 resolver and the §6 invariant (four disjuncts), replacing `ambiguousLeaves`/`blockSignature`, which go with it; `checkHeadings` keeps only the preferred-style check. `schemaPathsResolvedByDocKey` becomes the single reverse resolver inside coverage. Note the gate being removed is `prefer_block_heading_styles` (`r.Preferred`), not `block_heading_styles`: they are independent config keys, and §5's unparseable check legitimately depends on the latter.
- `docs/README.md` and `docs/rules/schema_docs.md`.

Out of scope: wrong-target links are for `anchors` (#82). Consolidating *differently* headed sections with identical content is #78; duplicated keys are reported here (§5).

### Commit sequence

The order is load-bearing, not cosmetic. Deleting `existsInSiblingBlock` before resolution is tightened surfaces #77's categories C and D against correct docs, which is why the issue calls for a redesign instead of a deletion. Each step gets its own corpus diff; six diffs that can each be explained beat one of several thousand lines where no finding can be attributed to a cause.

1. **Resolver consolidation.** The single §4 resolve function, per reference section, replacing all four call sites; `findDocBlockIn` folded in; the `anchors` parameter and link branch deleted. `existsInSiblingBlock` still in place. With `existsInSiblingBlock` still in place, the 714 suppressed findings stay hidden, so category C is not visible in this diff. Expect changes in missing-field findings (credit stops crossing paths), Read-Only coverage (`checkAttributeCoverage`), `checkDeprecated`, and the `heading` ambiguity warning, which now resolves through the new function. *Measured* (provider @ `34167e0962c`, base `6ea7434`): full config +1 error (`aws_autoscaling_group` `mixed_instances_policy.launch_template.override.launch_template_specification`, documented only under another path's full-path heading), plus 4 phantom warnings whose reported path changed (leaf dedup, step 3); weak config 0 → 23 errors, the 23 link-only Read-Only errors in §4 Links. Output identical across runs. (An earlier version of this entry gave 3 → 26, measured in the `terraform-provider-aws` checkout, whose cached schema predates `aws2`'s refreshed one; re-measured in `aws2` like every later step.)
1b. **Orphan bullets under unparseable headings** (§5, attribution change). Parser only; removes findings. *Measured* (`terraform-provider-aws2` @ `34167e0962c`, base = step 1): full config ERROR 14,000 → 13,902, WARN 5,177 → 5,009; removed 100 errors and 168 warnings, added 2. Every removed finding with a line number (254) sits under a heading no template matches; the other 14 are ordering findings that existed only because orphaned bullets were mixed into the previous section's list. The 2 added are real `description` errors (root `id`/`status` starting with "The") that were hidden because the description check's `seen` map is keyed by section name plus field across both reference sections, so an orphaned bullet credited to the root suppressed the real one. Weak config unchanged (23 errors).
2. **Delete `existsInSiblingBlock`.** Expect the ~635 field-existence errors. Category C shows up here as the gap between the 714 findings suppressed today and the ~635 that appear: those are the paths step 1 stopped crediting with another path's section. *Measured* (`terraform-provider-aws2` @ `34167e0962c`, base = step 1b): full config +42 WARN, 0 ERROR, 0 removed; weak config +1 WARN (`aws_arcregionswitch_plan` `parallel_config` at `workflow.step.parallel_config.step`, the field §8 predicted). All 43 checked against the schema: each field is absent at the reported path, 0 false positives. They are warnings, not errors, because the existing phantom finding is `SeverityWarning`; §10 says field-existence findings are errors, which is still to be done. The count is 42 rather than ~635 because leaf+field dedup still collapses repeats; step 3 expands them. Category C does not appear: step 1 already resolved those paths to their own sections.
3. **Path-keyed dedup.** Expect the QuickSight increase (about 312 → 7,500 per resource). *Measured* (`terraform-provider-aws2` @ `34167e0962c`, base = step 2): full config ERROR 13,902 → 42,093, WARN 5,051 → 5,771, 0 removed; weak config unchanged (23 ERROR, 1 WARN). Added 28,191 missing-block errors (QuickSight 7,226 per resource; `aws_wafv2_web_acl_rule` 3,419, `aws_wafv2_rule_group` 1,695, `aws_wafv2_web_acl` 1,187) and 720 phantom warnings. Checked against the schema: every missing block has configurable fields, every phantom field is absent at its path. 76 of the phantoms are at the root and name the wrong defect: bullets introduced by prose with no heading (`aws_iot_topic_rule`: "The `cloudwatch_logs` object takes the following arguments:") are credited to the previous section. The prose isn't a recognized lead-in and there's no heading for §5 to report; see §9.
3b. **Report the shallowest undocumented block per subtree** (§6 Dedup and cascades), with both counts in the message. *Measured* (`terraform-provider-aws2` @ `34167e0962c`, base = step 3): full config ERROR 42,093 → 16,277; missing-block findings 30,644 → 4,828, of which 3,599 are re-worded survivors. `aws_quicksight_analysis` 7,539 → 15 (`definition.sheets` reports 7,403 beneath it), `aws_wafv2_rule_group` 1,708 → 2, `aws_wafv2_web_acl_rule` 3,520 → 2,200. Weak config unchanged. What remains is the shared-leaf multiplier: `aws_wafv2_web_acl` keeps 1,188 `match_pattern` findings, each under a documented ancestor and each stating that 1,187 others share the name.
4a. **Unparseable-heading and prose-list warnings**, with suggested headings; prose lists that belong to another block leave their section. *Measured* (`terraform-provider-aws2` @ `34167e0962c`, base = step 3b): full config ERROR 16,277 → 16,002, WARN 5,771 → 5,515; weak config unchanged. Added 112 unparseable-heading warnings (89 with a suggestion) and 167 prose-list warnings (all with a suggestion). Removed 815 findings credited to the wrong section by orphaned prose lists: 459 phantoms, 269 description, 76 missing-label, 11 ordering. Every removed phantom is a bullet that is not a field at any path its section documents, by construction. Added 5 description errors that the description check's cross-section `seen` map had hidden behind an orphaned bullet of the same name; they are real but report block `(root)` for bullets that belong to a mixed prose list (`aws_elastictranscoder_preset` `video_watermarks`), which stays in place by design. This step moves in both directions; the prose attribution can't land in 1b because deciding it needs the schema.
4a′. **Description check on orphaned bullets.** The check doesn't depend on the block, so it keeps running on bullets that belong to no section. *Measured* (base = 4a): full config +417 description errors, 0 removed; weak config unchanged. The cross-section `seen` dedup that hid some of them is #86.
4b. **Duplicate headings**: occurrences in the parser and the fit rule. Per-path field-existence findings, and missing-field findings for fields whose only home is that reference section, are replaced by the fit rule for a duplicated key. *Measured* (`terraform-provider-aws2` @ `34167e0962c`, base = 4a′; identical output over 20 runs): full config ERROR 16,419 → 16,425, WARN 5,515 → 5,504; weak config +1 WARN, the `aws_bedrockagentcore_memory_strategy` duplicate §8 predicted. Added 8 duplicate-heading warnings and 7 fit errors, all checked against the schema and docs: `aws_cognito_risk_configuration` (`event_action` is Required, documented Optional), `aws_emr_cluster` (`throughput` listed for fleet `ebs_config`, which lacks it), `aws_sagemaker_user_profile` (`built_in_lifecycle_config_arn` in neither `code_editor_app_settings` heading). Removed 19 phantoms and 1 missing field, all on duplicated keys and now covered by the fit rule: `aws_ce_cost_category`'s two `rule` headings, for example, were 9 phantoms and are now one warning, because each heading fits one path.
4c. **Unresolved-section warning.** *Measured* (`terraform-provider-aws2` @ `34167e0962c`, base = 4b): full config +30 WARN, 0 removed, all Attribute Reference headings with bullets and a name no schema block has (`Endpoint`, `Disk IOPS`, `Labels`, `Query String Config`, and `aws_wafv2_rule_group`'s `RateLimit … Block` headings, which sit after `## Attribute Reference` though they document arguments). Weak config +6 WARN: the five link-only docs from step 1, whose Read-Only errors now come with a warning naming the heading. No case of "every block resolves to another heading" occurs in the corpus; it is covered by tests.
5. **Shared-section finding in `coverage`**, with phantoms raised to error. *Measured* (`terraform-provider-aws2` @ `34167e0962c`, base = 4c): full config ERROR 16,425 → 17,497, WARN 5,534 → 4,490; weak config ERROR 23 → 24 (`aws_arcregionswitch_plan` `parallel_config`, now an error), WARN 8 → 12. Of the change: 1,072 phantom warnings became 1,072 phantom errors with the new wording ("documented argument" under Argument Reference) and the pointer clause; 96 missing-field errors gained the pointer clause; the 17 `heading` "is ambiguous" warnings were removed. 45 shared-section warnings were added: 21 existence (disjunct 1), 5 label (2), 19 child-content (4), no deprecation (3). All 45 were checked against the schema by script. Of the 17 old ambiguity warnings, 12 now have a shared-section finding and 5 are duplicated keys, which the fit rule reports. The suite passes from this step on.

  Two defects caught on the corpus, each with a test that fails without the fix: a field that is an attribute at one path and a child block at another was compared as a label (`aws_msk_cluster` `tls`, "(Optional) and (Read-Only)"); and a disjunct-4 finding under a recursive name named one parent twice (WAFv2 `statement`, "between rule.statement and rule.statement"), because the parent was recovered by prefix matching rather than by dropping the leaf.

  Deviation: the field-existence message carries the section and the paths where the field does exist, but not the section's line; the line is already the finding's own location.
6. **Home-section check** for unlabeled single-home fields. *Measured* (base = 5): full config ERROR 17,497 → 17,504, WARN 4,490 → 4,507; weak config ERROR 24 → 24, WARN 12 → 19; nothing removed. All 24 added findings checked against the schema and the doc: each field is `Required` or `Optional`, not `Computed`, and listed only under Attribute Reference without a label. 17 are root scalars, mostly data-source arguments listed only as exported attributes (`aws_elasticache_user.user_name`, `aws_ses_email_identity.email`); 7 are nested (`filter` blocks under Attribute Reference in `aws_ami` and `aws_ami_ids`, a combined "Accepter and Requester" heading, a routed `payment_configuration.query_compute.is_responsible`).

  Scope: scalar attributes only. Coverage requires child blocks through the block-level finding, not a parent bullet, so a child-block bullet in the wrong section has no missing-field finding to extend. The reverse direction (a computed-only field only under Argument Reference, unlabeled) needs no new finding: Read-Only coverage already reports it as not documented in Attribute Reference (error), and `labels` reports the missing label.

  Deviation: severity follows `labels`' move finding, not `severity(attr)` alone: a warning for a root scalar (#62), so one defect doesn't get a different severity depending on whether the bullet has a label. The message ends with the label to add.
6b. **Marker mismatches are errors everywhere** (decision, §10). A wrong label (`labelCorrectness`), a deprecation mismatch (`checkDeprecated`, both directions), and shared-section disjuncts 2 and 3 become errors, matching the fit rule, which already treats them as errors under duplicate headings. A missing label and a label under Attribute Reference stay warnings; the fit rule doesn't compare those either. *Measured* (base = 6): full config ERROR 17,504 → 17,681, WARN 4,507 → 4,330: exactly 177 warnings became the same 177 errors (160 labels, 12 deprecation, 5 shared-section). Weak config ERROR 24 → 28, WARN 19 → 15: the 4 shared-section label conflicts in `aws_bedrockagentcore_gateway_target` and `aws_bedrockagentcore_memory_strategy`, confirmed in step 5. Nothing else changed.
6c. **Prose lists are sections** (decision, §10; §5 rewritten). Replaces 4a's rule, which left a prose list's bullets in no section when none was the enclosing section's and in place otherwise. *Measured* (base = 6b): full config ERROR 17,681 → 18,789, WARN 4,330 → 4,483; weak config unchanged (28 / 15). The prose warning's wording changes (167 → 239 lines, now also after an unresolved or unparseable heading), and description findings on prose bullets now name the block (394 removed, 409 added). Coverage: 56 phantom errors and 202 missing-block errors are gone because their bullets now reach the right block (including §9's former example, `aws_elastictranscoder_preset` `video_watermarks`), along with 14 Read-Only coverage errors and 3 label errors that had judged bullets against the wrong block (each checked against the schema: `aws_iot_topic_rule` `elasticsearch.id`, `aws_security_group` `filter.name` and `aws_ssm_maintenance_window_task` `parameter.name` are all Required there). Added: 1,309 missing-block errors, 1,281 of them in `aws_wafv2_rule_group`, where "Each `rule` supports…" now documents `rule` and its undocumented subtrees are reported one level down, under step 3b's shallowest rule; 7 missing-field and 6 phantom errors; and the checks a section gets that a prose list didn't: 51 ordering errors (9 removed), 93 missing-label warnings (17 removed), 12 label errors, 3 separator warnings, and 4 duplicate keys (for example two `and` prose lists in `aws_macie2_classification_job`, for different paths). A script confirmed every block, missing-field and phantom finding against the schema. The 7 missing-field errors have a bullet of that name elsewhere, each in the wrong place: `aws_directory_service_directory` lists `connect_settings`' arguments under bold prose (`**connect_settings**`, no backticks), which stays in the root section; `aws_lb_listener_rule` documents `query_string`'s `key` and `value` under a `values` wrapper that doesn't exist; `aws_ssm_maintenance_window_task` lists `document_version` only for `automation_parameters`.

  Test that fails without the change: the code-block case in `TestCoverage_OrphanedBullets` fails when a prose lead-in ends at its first list.
6d. **Child subtrees compare every field** (review of #87). Inside a child subtree, disjunct 4 compared labels and deprecation only for single-home fields, so children differing in an `Optional`+`Computed` field's label or deprecation counted as interchangeable, against acceptance case 3. Every field that exists is now compared; `ConfigUnknown` labels never are, at either level. *Measured* (base = 6c): no change in either config. Test that fails without it: `TestCoverage_SharedChildDeprecation`.
6e. **A duplicate heading's defect is reported once** (review of #87). A heading whose closest path already had a fit error with the same differences repeated it. *Measured* (base = 6d): full config ERROR 18,789 → 18,786, the repeats in `aws_cognito_risk_configuration` (line 72) and `aws_sagemaker_user_profile` (lines 67, 97); weak unchanged. Test: `TestCoverage_DuplicateHeadingReportsOnce`.
6f. **Cost test** (§7 Tests). `TestCoverage_SharedSectionCost`: bare-leaf sections serving 4,000 paths with nested children run in about 0.4 s; a pairwise evaluation took 13.6 s at 2,000 paths and fails the 10 s deadline.
6g. **Review fixes** (Copilot review of #87), each with a case in `TestCoverage_ReviewRegressions` that fails without it: a field coverage ignores under `ignore_deprecated` still exists, so it can't force a qualified heading; with `nested_object_attributes`, an object attribute's expanded fields are compared below a shared section (they're a block at `p.f` that `ChildBlocks` doesn't list); child blocks are resolved from the loader's bare names in field homes and the fit rule; a prose list leaves every alias of a combined heading; prose sections resolve by unique leaf, so `labels` checks them; `checkDeprecated` leaves duplicated keys to the fit rule. *Measured* (base = 6f): full config ERROR 18,786 → 18,793, WARN 4,483 → 4,481; weak unchanged. Added 8 label errors on prose lists, each checked against the schema; removed 4 findings on `aws_instance` bullets credited to a combined heading's aliases (they remain under `ephemeral_block_device`); `aws_sagemaker_user_profile`'s fit errors now also name the configurable child blocks both headings omit.

`checkDeprecated` moves onto the new resolver in step 1, and the suppression that keeps it from emitting a contradictory pair arrives with step 5, so those two must land in the same release even if they are separate PRs.

### Tests and fixtures

Per `AGENTS.md`: stdlib `testing`, table-driven, `t.Parallel()`, black-box `_test` packages, and a frozen `testdata/` fixture wherever a real provider doc exposed the behavior.

**Acceptance cases first.** Both cases at the top of this doc are required tests, as a table over the schema `X.Y.Z{A1,A2,A3}` / `T.U.Z{A3,A4,A5}` (case 1) and `X.Y.Z{A1,A2,A3}` / `T.U.Z{A1,A2,A3}` (case 2):

| Doc | Case 1 expected | Case 2 expected |
|---|---|---|
| one `Z` section listing `A1`–`A5` | errors (fields absent at each path) + shared-section warning | n/a |
| one `Z` section listing `A1, A2, A3` | errors (`A1`, `A2` absent at `T.U.Z`; `A4`, `A5` missing) + shared-section warning | **no finding** (the case 2 target) |
| qualified `X.Y.Z` and `T.U.Z` sections, each with exactly its fields | no finding | no finding |
| two `Z` headings, `{A1,A2,A3}` and `{A3,A4,A5}` | duplicate-heading warning only, no field errors | n/a |
| two `Z` headings, `{A1,A2,A3}` and `{A1,A2,A3,A4}` | duplicate-heading error (no occurrence fits `T.U.Z`) | error (the second occurrence fits no path) |
| two `Z` headings, both `{A1,A2,A3}` | error (no occurrence fits `T.U.Z`) | duplicate-heading **warning** — the fields are right, the duplicated key is not |

No row produces nothing for a duplicated key; that is the point of the last row.

Case 3, over the schema `X.Y.Z{A1,A2,A3}` / `T.U.Z{A1,A2,A3}` where `A3` is a child block with `X.Y.Z.A3{B1}` and `T.U.Z.A3{B2}`:

| Doc | Expected |
|---|---|
| one `Z` section, one `A3` section listing `B1` | errors at `A3` (`B1` absent at `T.U.Z.A3`; `B2` missing) + shared-section warnings for both `A3` and `Z` |
| one `Z` section, qualified `X.Y.Z.A3{B1}` and `T.U.Z.A3{B2}` sections | shared-section warning for `Z` (disjunct 4), no field errors; **this is the row that a same-level comparison misses** |
| qualified `X.Y.Z`, `T.U.Z`, `X.Y.Z.A3`, `T.U.Z.A3` sections | no finding |
| two `Z` headings, both `{A1,A2,A3}`, qualified `A3` sections | duplicate-heading warning only (each occurrence fits, but the key is duplicated); no shared-section warning, since the fit rule replaces it for a duplicated key |
| same as the first row, but `A3` identical at both paths (`{B1}` and `{B1}`) | no finding (the negative twin: equal at every depth) |

Each row is also run with the occurrences in reverse order, to prove the result doesn't depend on position.

Each frozen provider doc needs a matching schema fixture: the resource's slice of the cached `terraform-providers-schema/schema.json`, alongside the existing `testdata/schema/test_provider.json`. Without it the fixture can't run.

Fixtures from the measured cases:

- `aws_appmesh_route`: rule 2's exclusion (`spec.grpc_route.match` must not claim `spec.grpc_route.match.metadata.match`).
- `aws_bedrockagentcore_oauth2_credential_provider`: per-reference-section resolution, where the merged view loses the Argument Reference fields.
- `aws_appmesh_virtual_node`: the `fa4e9e4` case, as the inverted `TestCoverage_DuplicateBlockNames`.
- `aws_cognito_risk_configuration` (two `#### actions` with different fields): duplicate headings whose occurrences each fit one path, so the fit rule yields the warning and no field errors.
- `aws_athena_workgroup`: a readable doc whose heading doesn't resolve — a §5 warning and no field error.
- `aws_appautoscaling_policy` (`metric_name` and `namespace` under `metric`): disjunct 2.

Negative cases, each of which must produce nothing:

- A shared section whose served paths have identical fields, labels, and deprecation flags.
- A path in `skip_blocks`; a field in `phantom` or matched by `shouldSkipAttribute` — including when that filtered field is the only difference between two served paths, so the shared-section trigger stays silent.
- A `ConfigUnknown` block: no label or placement finding, but field existence still applies.
- A field whose home is "either" that is absent from one served path: disjunct 1 must not fire.
- A section that resolves to no path: no field-level finding, only the §5 warning.

Also:

- The parser's three merge cases tested separately, so the routing case can't regress into a duplicate.
- Non-transitivity: a shared section serving three paths where `X` is `Optional`+`Computed` at `A`, absent at `B`, and `Optional` only at `C` must report the `B`–`C` conflict, including when the three are child paths under disjunct 4. A representative-based implementation passes every other test and fails this one.
- Determinism: a shared section with many conflicting fields, run repeatedly in-process, asserting identical output. The compared pair must be (representative, first differing path) every time.
- Cost: a section serving a few hundred synthetic paths with nested children, asserting the check completes without pairwise blowup. A pairwise implementation passes every correctness test above and only fails here, so this is the test that protects the §6 cost rule.
- Anchor slugs, per `AGENTS.md`, alongside any change to suggested-heading rendering.

### Follow-up: label correctness per served path

Tracked in #80. Not in #77, but decided here rather than discovered afterwards. `labelCorrectness` resolves a section with `resolveSubsectionPath`, which accepts only an exact path, an exact root-level name, or a leaf unique across the schema (`schema_docs.go:1545-1571`), and returns nil when that fails (1086-1088). Every section this design treats as a legitimate shared section therefore gets no label check at all. Measured over backtick-style Argument Reference headings:

| heading resolves as | sections | bullets | labels checked |
|---|---:|---:|---|
| exact path | 1,808 | 5,499 | yes |
| leaf that occurs once | 1,380 | 3,688 | yes |
| shared or short qualified | ~317 | ~893 | no |

The fix is to check each labeled field in S against each path S serves, using the §4 resolver. Three constraints:

- **The cross-path conflict must not become N label findings.** When served paths disagree about a field's correct label, emitting one finding per path yields contradictory demands, at most one of which can be satisfied, and following any of them breaks the others. That case is disjunct 2's shared-section finding and the per-path label findings are suppressed for it. Per-path label checking is only meaningful once the served paths agree (`AGENTS.md`: report the real defect).
- **The misplacement resolver stays as it is.** `resolveSubsectionPath` keeps its own rules by design (argument-attribute-misplacement.md §4); only label correctness changes resolver. That leaves two notions of "which path does this section document" inside one rule, which `AGENTS.md` warns about, and the concrete hazard is the deferral at `schema_docs.go:1147-1153`: `labelCorrectness` returns nil expecting `checkComputedMisplacement` to report the move, and if the two resolvers disagree about the path, neither reports. Today it is narrow — the deferral only bites at the root, where both return `""` — but the follow-up has to verify it.
- **It needs its own corpus diff.** ~893 bullets have never been label-checked, so the finding count is unmeasured. Landing it inside #77 would bury #77's own diff.

Why the split: disjunct 2 closes the exposure #77 creates, since the design blesses shared sections and its suggestion steers authors toward the short qualified keys `resolveSubsectionPath` can't resolve. This follow-up closes the pre-existing hole, where a shared section's paths all agree on a label and the documented label is simply wrong. #77 doesn't widen that one.

## 8. Measured impact

Design-time estimates, kept for the record: terraform-provider-aws @ `886c0fab585`, `.ci/swissshepherd-full.hcl`, cached schema, instrumented builds of swissshepherd `main` @ `6ea7434`. The figures as built are in §7's step entries and the table below; where they differ, those win. The sizing figures in §6's optional follow-up are upper bounds from a simulation of a subset of candidates and are not part of #77.

Field-existence errors:

- `existsInSiblingBlock` suppresses **714** (path, field) findings on 219 paths in 25 resources. The 84 in #77 is after leaf dedup.
- Taking only the most specific name match (merged view), 79 of the 714 resolve to a correct section and disappear (e.g. `aws_appmesh_route`, `aws_appmesh_gateway_route`, `aws_bedrockagent_knowledge_base`, `aws_fis_experiment_template`, `aws_autoscaling_group`). About 635 remain on 201 paths in 16 resources, most in `aws_wafv2_web_acl` (325), `aws_wafv2_rule_group` (159), and `aws_wafv2_web_acl_rule` (121). Most of the WAFv2 errors come from `statement`, whose heading fix is mostly full-path headings (§6).

§5 warnings:

- Unparseable headings: 112 in 29 docs, most in `aws_msk_cluster` (18), `aws_emrserverless_application` (17), `aws_codebuild_project` (13), `aws_cognito_user_pool` data source (13).
- Duplicate headings: 18 in 12 docs.
- Unresolved sections: 30 (step 4c).
- Shared-section findings replace the `heading` "is ambiguous" warnings: 45 findings against 17 warnings (step 5). Moving the finding into `coverage` adds nothing for terraform-provider-aws: both `.ci` configs set `heading = true` (`swissshepherd-full.hcl:239`, `swissshepherd-weak.hcl:241`) along with `block_heading_styles` and `prefer_block_heading_styles`, so neither gate was suppressing it. The count still moves in both directions because the trigger changes: `blockSignature` (729) compares sorted attribute names only, while the new condition also considers child blocks, applies coverage's filters, and ignores fields whose home is "either".

Shared-section label conflicts (disjunct 2):

- 22 block names across 17 resources have a field whose requiredness differs by path, for example `metric_name` and `namespace` under `metric` in `aws_appautoscaling_policy`, and `interval` under `retain_rule` in `aws_dlm_lifecycle_policy`. The configurable-at-one-path, read-only-at-another variant has no current cases.
- The label-checking gap this sits in is measured in §7's follow-up table: ~317 sections and ~893 bullets in Argument Reference get no label check today.

Shared-section deprecation conflicts (disjunct 3): none on the corpus (step 5).

Shared-section child-content conflicts (disjunct 4): 24 groups in 11 resources agree at their own level but differ below (§6). Upper bound: measured before field homes and filters were applied, and counts schema groups, not doc sections that actually share a heading. As built: 19 findings (step 5).

Home-section check for unlabeled fields: 24 findings, 17 at the root and 7 nested, all confirmed (§7 step 6). The design-time estimate was 2 root-level cases.

Missing-block errors (path-keyed dedup):

- Each QuickSight resource goes from about 312 findings (one per undocumented block name) to about 7,500 (one per undocumented path): `aws_quicksight_analysis` 7,539, `aws_quicksight_dashboard` 7,539, `aws_quicksight_template` 7,538. Estimated from the schema and doc headings; as built, 7,226 per resource (step 3), then 14 or fewer (step 3b).
- This is tolerable because those resources are already excluded from `schema_docs` in the weak config (`resource/aws_quicksight_analysis`, `resource/aws_quicksight_dashboard`, `resource/aws_quicksight_template`, `data_source/aws_quicksight_analysis`), so CI is unaffected. The increase appears only in the full config, where each finding names a path an agent can act on. Their documentation approach is #74.

Weak config (`.ci/swissshepherd-weak.hcl`): the weak config is expected to report more after this change, because it was missing real defects. More correct findings are an improvement; only false positives and silent passes count as worse. Known additions: `aws_arcregionswitch_plan` (1 field, `step`); `aws_bedrockagentcore_memory_strategy` (duplicate-heading warning, `memory_record_schema Block`); and step 1's 23 link-only Read-Only errors in `aws_fsx_ontap_file_system`, `aws_fsx_ontap_storage_virtual_machine`, `aws_wafv2_managed_rule_group`, and `aws_db_instance` (§4 Links). All are to be fixed in the swissshepherd bump PR.

Corrections to #77: `aws_athena_workgroup` and `aws_autoscaling_group` are listed there as docs that are wrong for users. Autoscaling is correct and resolves once composites can't cross into another schema path. Athena gets both, measured in step 2. Its `Encryption Configuration` heading normalizes to the bare leaf `encryption_configuration`, so under headings-only resolution it serves both `encryption_configuration` paths and lists `encryption_option` and `kms_key_arn` where they don't exist: a correct field error. Its `Managed Query Results Encryption Configuration` section resolves to no path, so it also gets the §5 unresolved-section warning once that lands.

### Measurements before merge

`AGENTS.md` requires a measured corpus diff for any change that alters findings. All are on `terraform-provider-aws2` @ `34167e0962c`, both configs, with output identical across runs; §7's step entries give the detail. Counts are on the branch head unless a step is named.

| Measurement | Result |
|---|---|
| Field-existence errors, per reference section | +42 when `existsInSiblingBlock` went (step 2), +720 after path-keyed dedup (step 3). Now 842 under Argument Reference and 180 under Attribute Reference, all errors (step 5) |
| Missing-block errors after path-keyed dedup | QuickSight 7,226 per resource (step 3), then 14 or fewer once only the shallowest undocumented block is reported (step 3b) |
| Unresolved sections (§5) | 30 (step 4c), 28 after prose lists became sections (step 6c) |
| Shared-section findings under the new trigger | 45 (step 5): 21 existence, 5 label, 19 child-content; replace 17 `heading` "is ambiguous" warnings |
| Disjunct 3: paths disagreeing on `Deprecated` | 0 on the corpus; covered by tests |
| Disjunct 4: shared sections whose paths differ below their own level | 19 findings (step 5); unchanged when child subtrees compare every field |
| `checkDeprecated` on the new resolver | no change (step 1) |
| Home-section check | 24: 17 root, 7 nested (step 6) |
| Duplicate headings | 8 warnings and 7 fit errors (step 4b); 4 fit errors after one defect is reported once |
| Weak-config errors | 0 → 23 (step 1, link-only Read-Only), 24 (step 5, arcregionswitch phantom), 28 (step 6b, shared-section label conflicts); 15 warnings. Every one confirmed against the schema |
| Two runs of the new binary, byte-identical | every step |

Every added finding was spot-checked against the provider schema, by script for the field and block findings; false positives found that way were fixed, each with a regression test.

## 9. What stays unchecked

The #77 bug began as a false positive that was made to go away rather than made correct, and it was measured by "the error disappeared" instead of "the doc is judged correctly." The test that catches that is: wherever this design excludes or skips something, what becomes unchecked, and does anything report it? §5 is the answer for sections that resolve to no path, and the §6 invariant is the answer for shared sections. These are the remaining cases, written down so they can't creep. Every row needs either a reporter or a tracking issue.

| Unchecked | Why | Reported by | Tracking |
|---|---|---|---|
| Labels in shared and short-qualified sections (~317 sections, ~893 bullets) | `resolveSubsectionPath` returns unresolved, so `labelCorrectness` emits nothing | Cross-path label conflicts only, via disjunct 2. A label that is uniformly wrong across the served paths is unreported. | #80 (§7 follow-up) |
| Fields of Attribute Reference sections naming object-typed attributes | §5 exempts them from "unresolved section" because `checkPhantomBlocks` deliberately allows those headings. But allowing isn't checking: with `nested_object_attributes` off, which it is in both provider configs, no path resolves to the section and its fields are compared against nothing. This is the original defect's exact shape — a section's contents accepted unverified — so it is the one row that most needs closing. | nothing | #81 |
| Labels and placement inside `ConfigUnknown` blocks | `labelCorrectness` and `configurableArgAtPath` return early for them: per-field `Required`/`Optional` is unknowable (§6 never guess). Field existence still applies. | nothing, by design | — |
| Field, ordering, description, and label checks on bullets under an unparseable heading | The bullets belong to no section (§5) | The unparseable-heading warning (step 4a), which names the heading and suggests one; fixing it restores every check. The `description` check still runs on them, naming the heading instead of a block (step 4a′); the rest wait for the heading. Prose naming a block under such a heading gives its list a section (step 6c). | — |
| A live link to the wrong section | `anchors` catches dead fragments only (`aws_wafv2_rule_group`: `allow` → `#action`) | nothing | #82 |

Not a gap: a provider that leaves `block_heading_styles` unset still gets the §5 backstop. `cmd/check.go` falls back to `doc.DefaultHeadingTemplates()`, which includes `{Block}` and `{Title}`, so headings parse and the unresolved-section check applies.

Closed by this design: a configurable argument documented only under Attribute Reference with no label. It passed because missing-field coverage unions both reference sections and `classifyAttrPlacement` reads an unlabeled bullet as a proper computed output. Coverage now reports it (§2 home section).

The test itself is general, not specific to #77: wherever a change excludes or skips something, state what becomes unchecked and what reports it. It belongs in `AGENTS.md`, not only here (#85).

## 10. Decisions

| Question | Decision |
|---|---|
| Resolve by position or heading hierarchy? | No. A section is identified by its heading alone. |
| Resolve by links? | No. Links are navigation; reverses #51 in `checkAttributeCoverage`. Measured in step 1: no change on the full config, 23 new Read-Only errors in 5 docs on the weak config (`nested_object_attributes = true`), all link-only sections. |
| Severity: field-existence and missing-block findings | Error. |
| Severity: unparseable, unresolved, and duplicate headings (§5) | Warning, except a duplicated key whose occurrences can't be matched to its paths, which is an error (a certain field defect). |
| Severity: wrong label or deprecation marker | Error everywhere: `labels`, `checkDeprecated`, shared-section disjuncts 2 and 3, and the fit rule. One defect gets one severity whether its section is single, shared, or duplicated. A missing label stays a warning. |
| Prose-introduced lists | A heading in all but syntax: the list documents the block the prose names, including a list mixing the section's own fields, except a trailing run that resumes the enclosing section (§5). |
| Wrong-target links | Not coverage; belongs to `anchors`. |
| Aggregate messages? | No. One finding per path and field. |
| `heading` ambiguity warning | Moves into `coverage` as the shared-section finding. |
| Shared-section trigger | Stated as an invariant, not a list: a shared section is valid only when every schema-derived property swissshepherd compares is identical across the paths it serves. Today that is existence (disjunct 1, single-home fields only), label (2), `Deprecated` (3), and a child block's contents, recursively (4); 2–4 apply to any field S lists as well as single-home fields. "Identical" means identical at every depth (acceptance case 3). A new per-field schema comparison must add a disjunct. Not a comparison of expected field sets, which is undefined for fields whose home is "either". |
| Disjuncts 2–4 are sole detectors | Accepted. `resolveSubsectionPath` can't resolve a shared section, so `labelCorrectness` is silent; `checkDeprecated` emits a contradictory pair instead; and a child-content difference (4) is invisible at the child's own level when its sections are qualified. Disjuncts 2 and 3 are errors, 4 a warning (step 6b). Each reports one finding per conflicting field or child block, since nothing else enumerates them. |
| Deprecation divergence in #77 or later? | #77. §7 moves `checkDeprecated` onto the §4 resolver, which is what creates the contradictory pair, so the disjunct ships with the cause. |
| Single-home field documented only outside its home, unlabeled | Reported by coverage. The §2 union answers "documented at all", not "documented in the right place", and `classifyAttrPlacement` reads an unlabeled bullet as a proper output. Labeled bullets stay with `labels`. |
| Label correctness per served path | Follow-up, not #77. Needs its own corpus diff over ~893 never-checked bullets; the cross-path conflict routes to disjunct 2 rather than producing contradictory per-path findings. Decided now so it isn't discovered after #77 ships. |
| Duplicate headings and the parser | One entry per key with an ordered list of occurrences. Coverage and the fit rule read occurrences; other checks read the merged entry, unchanged. Routed heading-less content and `SpansSubsections` are unchanged. Error when no assignment of occurrences to paths is exact; warning otherwise. There is no passing case: a duplicated key hides which block each occurrence documents, so it is reported even when every occurrence fits every path. The fit rule also replaces the shared-section finding for that key. |
| Duplicated keys and the per-path field findings | The fit rule replaces them for that section. Running both would compare the merged union against each path and report correct docs. |
| Relation to #78 | Separate. #78 is about *differently* headed sections with identical content, which it would consolidate under one shorter heading. A duplicated key is #77's, always reported. |
| Evaluation cost | Never pairwise over served paths: each disjunct is evaluated over the whole served set, O(N × fields) per section. Labels and `Deprecated` use a per-field representative (first served path where the field exists). Interchangeability isn't transitive (disjunct 1 is one-directional), so disjunct 4 recurses on the set of child paths, memoized by that sorted set, never on pairs. `match_pattern` serves 2,160 paths, where pairwise is 2.3M comparisons. |
| Implementation order | Six steps (§7), resolver consolidation before deleting `existsInSiblingBlock`, each with its own corpus diff. Deleting first would surface #77's categories C and D against correct docs. |
| Field homes | Extend `configurableArgAtPath`'s classification: pure-configurable → Argument Reference; `Optional`+`Computed` → either; computed-only → Attribute Reference (either under `allow_inline_read_only`); `ConfigUnknown` → either. Child blocks follow their subtree the same way: any pure-configurable field → Argument Reference; only `Optional`+`Computed` → either; no `Required`/`Optional` field → Attribute Reference. |
| `ConfigUnknown` and field existence | Field existence still applies. Only the field's home is unknowable. |
| Suggested headings | The full path of `Q`, plus the condition (a heading that resolves only to paths where `X` exists) so an agent can choose a shorter one. Needs no verification. An optimizer for shorter suggestions is an optional follow-up. Duplicate-heading warnings list candidate paths rather than assign one per occurrence. |
| Withhold the suggestion when N is large (WAFv2 `statement`)? | No. The example is correct, and the message prints one, so withholding saves nothing. The message states the served-path count N instead. |
| `coverage = false, heading = true` loses ambiguity detection | Accepted, rather than keeping a second implementation of the trigger. |
| Pointers on field findings | Both directions. The missing-field pointer is the one that prevents a wrong fix. |
