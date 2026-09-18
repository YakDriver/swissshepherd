# Argument/Attribute-Reference Misplacement — Design & Rationale
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

> **Status:** working design doc for the attribute-granular misplacement rule
> (issues #60, #62), tracking PR #61. Living document — to be cleaned into
> reference prose once the implementation lands. Captures the redesign of the
> "configurable argument documented under Attribute Reference" check.

## 1. Why we're reopening this

PR #61 implemented #60 as a two-pass, **whole-subsection** reclassification bolted
onto `checkLabels`. It has taken ~8 rounds of review, each patching one more
heading permutation. That is a design smell, not bad luck. The rounds cluster
around three assumptions that real docs violate:

1. **One heading owns one schema path.** Reality: a path is documented across
   split/alternate headings (`### outer.notification` *and* `### notification`).
2. **A heading is homogeneous** (all-configurable or all-computed). Reality:
   subsections mix configurable args and computed-only outputs.
3. **Leaf/path identity is clean.** Reality: synthetic dot-path bullets, phantom
   headings, and leaf collisions all occur.

The three still-open review comments are each a direct consequence:

- **3/3 (mixed blocks):** "move this *subsection* to Argument Reference" is
  *unfollowable* for a subsection that also documents computed-only fields —
  moving it wholesale makes those fields trip `checkComputedMisplacement`
  (schema_docs.go:489/516). The primary finding contradicts another rule. This
  is a correctness defect in the advice itself, not a suppression gap.
- **1/3 (split headings):** pass 1 inspects only the *owning* heading's labels,
  so labeled config living in an alternate heading never marks the path moved,
  and the alternate gets the misleading strip-label.
- **2/3 (leaf-only suppression):** last round's `uniqueSchemaPathForLeaf`
  suppression matches on leaf alone and silently drops a warning for an
  unrelated dotted heading.

Root cause: the check operates at **subsection granularity** with an
**ownership-based** heading→path resolver and a **homogeneity** assumption. The
defect it is trying to describe is per **attribute** ("this configurable
argument is under the wrong section"). Every mismatch between those two
granularities becomes a special case.

## 2. Reframing the requirement

Re-reading the issues from scratch:

- **#60** (nested blocks): don't tell authors to strip *correct* labels; the real
  defect is section placement — surface a "move it" finding instead.
- **#62** (top-level scalars): same defect at the root, and the author explicitly
  asks for **attribute-level** phrasing: `argument "X" is documented under
  Attribute Reference but is a configurable argument; move it to Argument
  Reference`, noting "That's a distinct code path, not a relaxation of the
  `blockName == ""` guard."

So the true requirement is one rule, stated per attribute:

> **A labeled attribute documented under Attribute Reference that is a purely
> configurable argument in the schema (`Required`/`Optional` and not `Computed`)
> is misplaced. Tell the author to move that argument to Argument Reference,
> instead of telling them to strip its (correct) label.**

Nested blocks (#60) and root scalars (#62) are the same rule at different paths.
The whole-subsection message is a *presentation* optimization for the common case
where an entire block is misplaced — not the detection unit.

**#60 scope after this redesign (crisp statement).** #60 is *resolved for
configurable blocks documented under schema-name, dotted, or unique-leaf-headed
subsections* — the overwhelming majority. It is **not** 100% closed: prose-headed
configurable blocks (§12: 9 measured labeled shared-shape cases across 3
resources) intentionally retain the legacy strip-label behavior, since resolving
them requires the anchor bridge §12 declined to build. That residual is an
accepted, measured limitation tracked as a follow-up — not an oversight.

## 3. Core principle

**Detect per attribute. Aggregate for the message only.**

The per-attribute primitive already exists:
`configurableArgAtPath(rs, path, attrName) bool` returns true iff, at schema
`path`, `attrName` is a configurable argument (a `Required`/`Optional` non-
`Computed` scalar, or a child block whose subtree contains a pure-config field).

Given that primitive, misplacement no longer needs ownership resolution,
"moved path" sets, alternate-heading suppression, or reference-bullet
suppression. Each labeled attribute is judged on its own.

### Classification of a labeled attribute `A` under Attribute Reference, in a block resolved to schema path `P`

| Condition | Finding |
|---|---|
| `configurableArgAtPath(P, A)` is true | **misplacement** — move `A` to Argument Reference |
| `A` exists in schema at `P` but is not pure-config (computed-only, or Optional+Computed) | **strip-label** warning (unchanged, pre-#60 behavior) |
| `A` not resolvable in schema (`P` unknown, or `A` absent) | **fall back** to strip-label (safe; coverage/phantom rules own "doesn't exist") |

`Optional+Computed` and `ConfigUnknown` remain non-misplacement by construction
(`configurableArgAtPath` already returns false), honoring #62's guard.

## 4. Resolution rules (the part that caused the rounds)

Misplacement does **not** use the coverage "ownership" resolver
(`schemaPathsResolvedByDocKey` / most-specific single owner). Ownership exists to
avoid double-counting *coverage*; it is the wrong tool for "does this documented
argument correspond to a configurable schema field." We resolve a subsection
heading `K` to a schema path `P` with a simpler, stricter rule:

- **Dotted heading** (`K` contains `.`, e.g. `outer.notification`): `P = K`
  **iff `rs.Blocks[K]` exists**; otherwise **unresolved**. A dotted heading
  claims an exact path — never remap it by leaf. *(This is what fixes 2/3
  cleanly: a phantom `outer.notification` never borrows `other.notification`.)*
- **Bare heading** (`K` has no `.`, e.g. `notification`): `P = K` if
  `rs.Blocks[K]` exists (root-level unique name); else
  `P = uniqueSchemaPathForLeaf(K)` if exactly one schema path has that leaf; else
  **unresolved**. *(This is what fixes 1/3: `### notification` resolves to the
  sole `outer.notification` regardless of which heading "owns" it for coverage.)*

> **Inference watch-point.** The bare-heading `unique-leaf` branch is the **only**
> step that *infers* a path rather than reading it exactly. It is double-gated
> (unique leaf **and** `configurableArgAtPath` must confirm pure-config at that
> path), so it cannot promote a non-configurable field. But it is *new behavior
> vs PR #61* — PR #61's ownership resolver returned "not owned" for split
> headings and emitted nothing there. Therefore PR #61's "identical before/after"
> corpus evidence does **not** cover unique-leaf-inferred findings. These must be
> enumerated and cleared explicitly in the corpus delta before shipping at ERROR
> (see §9). The real FP axis is *exact-resolved vs unique-leaf-inferred*, which
> runs through both root and nested; the path-class severity split (§9) is a
> proxy for it and must not hide the inferred-nested slice from validation.
- **Root** (`K == ""`): `P = ""` (top-level scalars — #62).
- **Heading-less synthetic block** (dot-path reference bullets like
  `outer[*].notification`): resolve the parent by exact `rs.Blocks[parent]`; the
  bullet's attribute is the referenced leaf. *(Same as today's heading-less
  fallback, retained.)*

Multiple headings resolving to the same `P` is now **fine** — each heading's
labeled attributes are judged independently. No "steal ownership," no
"suppress the alternate."

## 5. Emission granularity — solving the mixed-block contradiction (3/3)

Detection is per attribute; we choose the *message* per subsection:

For a resolved subsection at path `P` with a real heading, after classifying its
labeled attributes:

- **Collapse to one subsection move** —
  `block "P" ... move this subsection to Argument Reference` — **iff**:
  1. at least one labeled attribute of the subsection is misplaced, **and**
  2. the subsection **documents** no computed-only field.

  **Precision (required):** condition 2 is the intersection of *the subsection's
  own documented bullets* with schema-computed status — **not** "the schema
  subtree at `P` contains a computed leaf." An undocumented computed leaf must
  **not** block a legitimate collapse. Concretely: iterate the subsection's
  documented attributes; if any resolves to a computed-only schema field at `P`,
  do not collapse. This is the #60 budget case: a purely configurable block
  sitting in the wrong section → one clean, followable finding.

- **Otherwise emit per-attribute moves** —
  `argument "A" in block "P" ... move it to Argument Reference` — for each
  misplaced attribute, and **leave computed-only fields where they are** (no
  strip-label, no move).

  This is the mixed block (3/3): the author moves only the configurable bullets;
  the computed outputs correctly remain under Attribute Reference. Following the
  findings now yields a clean document — `checkComputedMisplacement` is never
  provoked.

- **Root (`P == ""`)** never collapses: always per-attribute (#62).

The collapse condition keys on the **schema** (is any documented field
computed-only), so it is deterministic and independent of heading style.

### De-duplication

Emit at most one finding per resolved target:

- per-attribute findings dedup on `(P, A)`;
- a subsection collapse suppresses the per-attribute findings for that `P`;
- a dot-path reference bullet and a subsection that resolve to the same child
  block dedup on that **resolved child schema path** — **path-based, never
  leaf-based** (leaf-based dedup would reintroduce the 2/3 over-match).

### Message templates

Two templates — the presence/absence of the block clause is the natural fallout
of "root never collapses / attribute-level phrasing." Both keep "in the schema"
so per-attribute and collapse findings read as one family. Render every path via
`displayPath()` for consistent formatting.

- **Collapse (nested):** `block "P" is documented under Attribute Reference but
  is a configurable argument block in the schema; move this subsection to
  Argument Reference` *(unchanged from current)*.
- **Per-attribute (nested):** `argument "A" in block "P" is documented under
  Attribute Reference but is a configurable argument in the schema; move it to
  Argument Reference`.
- **Per-attribute (root, `P==""`):** `argument "A" is documented under Attribute
  Reference but is a configurable argument in the schema; move it to Argument
  Reference` *(no block clause — `in block "(root)"` reads badly)*.

## 6. What gets deleted

The new model removes the machinery that generated the rounds:

- the two-pass structure (`movedBlocks`, `movedSchemaPaths`, `movedPath`,
  `misplacedLine`);
- `referencesMovedBlock` (reference-bullet suppression);
- the `uniqueSchemaPathForLeaf`-based alternate suppression **as a suppression**
  (the helper stays, repurposed as bare-heading resolution);
- `headedDocBlocks` and the synthetic-ownership-stealing guard (ownership is no
  longer used for misplacement);
- the anchor/`BlockAnchors` suppression already removed earlier.

Net: one pass over `AttributeBlocks`, a per-attribute classifier, and a
per-subsection message chooser.

## 7. How every open item resolves structurally

| Item | Old behavior | New behavior |
|---|---|---|
| 3/3 mixed block | whole-subsection move → contradicts computed rule | per-attribute move; computed fields stay → clean doc |
| 1/3 split headings | config in alternate heading never flagged; strip-label leaks | bare heading resolves to sole path; attr flagged correctly |
| 2/3 leaf over-match | dotted phantom borrows another path's move, drops warning | dotted heading resolves by exact path only → no borrow |
| prior: synthetic steals ownership | needed `headedDocBlocks` guard | ownership unused; synthetic resolved by exact parent |
| prior: alternate heading strip-label | needed suppression pass | independent per-attr judgment |
| #62 root scalars | out of scope / misleading strip-label | same rule at `P==""`, per-attribute |

## 8. Edge cases to cover in tests

1. Purely configurable nested block under Attribute Reference → **one** subsection move (#60 budget shape).
2. Mixed block (config + computed-only) under Attribute Reference → per-attribute moves for config; **no** finding for computed; no `checkComputedMisplacement` collision.
3. Split headings: config field only in the bare alternate heading → flagged via unique-leaf resolution.
3b. **Negative twin:** bare heading whose unique-leaf match is `Optional+Computed` → **no move** (strip-label/none). Guards the sole inference step in §4.
4. Dotted phantom heading whose leaf collides with a real moved path → **no** misplacement finding (exact-only), phantom handled by coverage.
5. Labeled dot-path reference bullet to a configurable child → per-attribute move; dedup against a sibling subsection for the same child.
6. `Optional+Computed` and `ConfigUnknown` fields → never misplacement (strip-label/none), per #62 guard.
7. `skip_blocks` path → exempt (carry over current filter, applied to `P`).
8. Ambiguous/unresolved heading → legacy strip-label, never a move.
9. Root-level configurable scalar under Attribute Reference (#62) → per-attribute move.
10. Computed-only field with an erroneous label → strip-label (unchanged).

## 9. Severity & corpus validation

Severity is split by **false-positive risk per resolution class**, not by a
time-based "second pass":

| Finding class | Severity | Rationale |
|---|---|---|
| Nested subsection collapse move | **ERROR** now | Whole block configurable; highest confidence; parity with merged behavior |
| Nested per-attribute move, **exact-resolved** | **ERROR** now | Same schema-grounded signal as collapse; only presentation differs — severity must not depend on sibling count |
| Nested per-attribute move, **unique-leaf-inferred** | **ERROR** now — gate measured clear | See below: 0 contested cases on the corpus, so no new-vs-#61 slice |
| Root scalar move (`P==""`) | **WARN**, promote to ERROR after a clean corpus run | Root legitimately holds many computed outputs; `Optional+Computed` common — exactly what #62 asked to gate |

`configurableArgAtPath` already structurally excludes the main root FP vectors
(`Optional+Computed`, `ConfigUnknown`), so WARN-at-root is cheap insurance, not a
permanent posture.

**The split is explicitly transitional, not a permanent two-tier UX.** A user must
never see the *same* "move it" message as ERROR in one doc and WARN in another
purely because of internal resolution method — that is confusing. The only
lasting severity distinction is confidence, and once the root bucket clears its
own corpus run it collapses to **uniform ERROR** with the rest. The WARN tier is
a rollout artifact with a scheduled end, not a design feature.

**Provenance requirement (implements this table).** Severity-by-class is only
possible if each finding records how its path resolved. The resolution class
(`dotted-exact` / `bare-exact` / `unique-leaf` / `root`) must be threaded from
`resolveSubsectionPath` → `classifyAttrPlacement` → the `Result`, and mapped to
severity at emission. See §10 step 2b.

### Measured gate for the unique-leaf bucket (point 2 discharged)

The unique-leaf bucket is **distinct** from §12's prose-bridge bucket: shared
shapes are *ambiguous* leaves (multiple schema paths) → `uniqueSchemaPathForLeaf`
returns false → they land in *unresolved/prose*, not here. Unique-leaf inference
fires only for a bare heading whose leaf maps to *exactly one* non-root schema
path. Measured on terraform-provider-aws (2,662 docs, same harness as §12):

| would-be move finding, by resolution class | subsections |
|---|---|
| dotted-exact | 0 |
| bare-exact | 3 |
| unique-leaf-inferred | 7 (18 cfg attrs, 3 resources) |
| …**contested** (path also documented by a competing heading — the only slice that differs from PR #61) | **0** |

The mainstream nested case (`### endpoint` → `cluster.endpoint`) resolves by
unique-leaf, but PR #61's ownership resolver *already* resolved it the same way
via leaf matching — so it is **not** new behavior. The only genuinely-new slice
is *contested* unique-leaf (split/alternate headings), and there are **0** on the
corpus. That clears the gate: nested unique-leaf-inferred ships at **ERROR** now.
The examples are all real nested configurable blocks (`recurrence.daily_settings`,
`rule.statement.rate_based_statement.custom_key`, …).

**This redesign changes corpus output on purpose** — the PR #61 "identical
before/after, zero new FPs" guarantee is **void by design**. Validation is:

- run current-PR-head vs redesign over terraform-provider-aws;
- categorize **every** delta as *intended* (per §7) with a one-line justification;
- keep the resolution-class buckets (above) as a standing part of the run and
  re-confirm **contested == 0** (or enumerate any that appear);
- **hard invariant:** no delta is a *new false ERROR on a correctly-placed field*.

The PR description must be rewritten to present this categorized delta and drop
the "identical before/after" claim.

## 10. Implementation steps

1. Introduce `classifyAttrPlacement(ctx, P, attr) -> {misplaced|stripLabel|ok}`
   wrapping `configurableArgAtPath` + label state.
2. Introduce `resolveSubsectionPath(rs, docBlocks, K) -> (P, class, ok)`
   implementing the §4 rules (dotted=exact, bare=exact|unique-leaf, root,
   heading-less=exact parent). `class` is the resolution provenance
   (`dotted-exact` / `bare-exact` / `unique-leaf` / `root`). Keep ownership
   resolution untouched for coverage.
2b. **Thread provenance to severity.** Carry `class` from `resolveSubsectionPath`
   through `classifyAttrPlacement` into the emitted `Result` (a field, not parsed
   back out of the message), and map class→severity at emission per the §9 table.
   Without this the §9 table is not implementable.
3. Rewrite `checkLabels`' Attribute-Reference half as a single pass:
   resolve `P`; classify each labeled attr; choose message granularity per §5;
   dedup per §5.
4. Delete the two-pass state and suppression helpers listed in §6.
5. Fold #62 (root) in via `P==""`; close #62 with this PR or note it.
6. Tests: the §8 matrix, each proven to fail on the pre-redesign commit.
7. Corpus validation per §9; record the categorized delta in the PR body.
8. Full gates (`go test ./...`, `gofmt`, `go vet`, `make modern-check`,
   `copyplop check`).

## 11. Decisions (resolved in review)

1. **Scope:** fold #62 (root scalars) in now. Re-splitting would resurrect the
   `blockName==""` special case §6 deletes; root-in is a ~3-line consequence
   (`P==""` never collapses, always per-attribute). Close #62 with this PR.
2. **Severity:** per the §9 table — nested ERROR now (exact-resolved), nested
   unique-leaf-inferred ERROR only if the corpus clears it, root WARN then
   promote.
3. **Wording:** two templates per §5, "in the schema" retained, root drops the
   block clause, `displayPath()` throughout.
4. **Collapse:** keep schema-keyed collapse (avoids the budget-UX regression),
   with the §5 precision — computed check is over *documented* bullets, not the
   schema subtree.

### Cross-cutting invariants for implementation

- Dedup is **path-based**, never leaf-based (2/3 lesson).
- Unique-leaf resolution is the **only** inference point; it is double-gated and
  must have the §8 negative twin plus explicit corpus enumeration.
- Rewrite the PR body: categorized before/after delta; drop "identical
  before/after"; hard invariant = no new false ERROR on a correctly-placed field.

## 12. Deferred extension — prose-heading anchor bridge (NOT in this PR)

**Gap.** §4 resolution is schema-grounded (dotted=exact, bare=exact|unique-leaf),
so a **prose-headed** subsection (`### Budget Notification` rather than
`### notification Block`) resolves to nothing and falls back to strip-label —
i.e. a prose-headed *configurable* block under Attribute Reference still slips
through, which is the #60 failure otherwise fixed. Terraform docs use
descriptive headings frequently, so this is a real gap.

**Proposed mechanism (anchor-based *resolution*, distinct from the deleted
anchor-based *suppression*).** The root argument bullet names a schema key and
links to the subsection:
`` `notification` - (Optional) ... [Budget Notification](#budget-notification) ``.
That yields a chain `schema-key bullet → link anchor → subsection heading`. The
schema **path** comes from the key (reliable), not the slug; the anchor only
identifies which prose heading belongs to that key.

**Why it is defensible where suppression was not.** Suppression used anchors to
*silence* a warning → fragility caused silent misses of real problems.
Gated resolution uses anchors to *find* a candidate the schema then confirms →
fragility causes a miss, never a false alarm.

**Hard constraints if ever built:**

1. **Measure first.** Fold a count into this PR's corpus run: Attribute-Reference
   subsections that (a) fail schema-grounded resolution *and* (b) have a root
   bullet whose schema key is a configurable block with a matching anchor.
   Negligible ⇒ don't build; substantial ⇒ justified.
2. **Own issue, never this PR.** The redesign's premise is shrinking the
   special-case surface; an anchor tier is added machinery and would muddy the
   corpus-delta story.
3. **Lowest-priority tier, double-gated.** Consult only when exact *and*
   unique-leaf fail, and keep the `configurableArgAtPath` gate at the resolved
   path, so a wrong/missing/mis-slugged anchor can only cause a **false negative
   (missed detection)**, never a **false positive (fabricated move)**. This
   containment is the price of re-coupling to the slug subsystem (the one PR #63
   hardened).

### MEASURED (terraform-provider-aws, 2,662 docs) — decision: DO NOT BUILD

Measured with a build-tagged pipeline test (`internal/check/measure_prose_test.go`,
`-tags measure`, reusing the real runner + redesign §4 resolution + a *generalized*
anchor bridge that follows nested parents, not just root):

| metric | count |
|---|---|
| docs parsed | 2,662 |
| prose (unresolved) subsections under Attribute Reference | 70 |
| …with ≥1 labeled attribute | **9** |
| …bridgeable via a configurable-parent anchor | **9** (all) |
| …across resources | **3** (`inspector2_filter`, `ssmcontacts_rotation`, `wafv2_rule_group`) |
| …labeled configurable attrs the bridge would move | 23 |
| …prose+labels with NO bridge (residual) | 0 |

**Findings:**

- The population is **~0.1% of docs / 3 resources** — negligible by the
  measure-first bar.
- All 3 are the **shared-shape** pattern (one subsection reused by many fields
  via a single `#anchor`, e.g. `### Date Filter` linked from ~8 date fields), i.e.
  the shared/renamed-subsection class, not plain prose headings.
- In the flagship case, the **parent block resolves schema-grounded**
  (`### Filter Criteria` → `filter_criteria`), so the redesign already emits the
  actionable primary finding ("move `filter_criteria`"). The bridge would add 23
  per-attribute moves on the shared leaf shapes *on top of* that — more noise,
  not more clarity.
- The only real residual harm is misleading strip-label warnings on those shared
  shapes; "fixing" it via anchors re-introduces the anchor coupling/suppression
  §6 deliberately removed.

**Decision:** do **not** build the anchor bridge. The redesign leaves these 9
subsections at their current strip-label behavior (no regression). Track
shared-subsection handling as a low-priority follow-up (fold into / alongside
#62) with this evidence attached; revisit only if the population grows materially.

### Snapshot caveat & rot guard (applies to §9 buckets too)

Both the §12 (prose/bridge) and §9 (resolution-class) numbers come from
`measure_prose_test.go`, which is `-tags measure` and therefore **never runs in
CI**. They are a point-in-time snapshot of an **already-cleaned corpus** — the
`budgets_budget` misplacements that motivated #60 have since been fixed and
`ignore_targets` masks known offenders, so the counts (9 prose / 3 resources; 7
unique-leaf / 0 contested) will drift as those change and understate the natural
frequency. To keep the claims honest:

- **Re-run cadence:** re-run the measurement whenever `ignore_targets` shrinks
  materially or before promoting the root bucket to ERROR; record the fresh
  numbers in the follow-up issue.
- **Cheap standing signal (preferred):** emit the resolution-class buckets as
  part of the normal corpus validation run (§9) so `contested` drift is visible
  each validation. A tiny committed unit test over a *synthetic* fixture (not the
  live provider) should assert the classifier behavior
  (dotted/bare/unique-leaf/unresolved) so a parser/slug change cannot silently
  reclassify — the corpus numbers may drift, but the classification logic stays
  pinned.
