<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

# AGENTS.md

swissshepherd is a documentation linter for Terraform providers. It compares a provider schema (`terraform providers schema -json`) against the provider's Markdown docs. The schema is the source of truth.

This file covers repo-wide rules that are expensive to get wrong. It doesn't list what the code does: for that, read `cmd/check.go` (rule wiring), `docs/README.md` (user-facing behavior), and `docs/rules/*.md` (design rationale). Don't rely on a summary of them.

Before writing or editing Go, read `.agents/skills/go-conventions/SKILL.md`. It overrides habits from other languages and overrides existing code that contradicts it.

## Done means

A change isn't done until all of these hold:

1. `make fmt && make vet && go test -count=1 ./... && make modern-check && go run github.com/YakDriver/copyplop@latest check` passes.
2. A bug fix has a regression test that fails without the fix.
3. Any user-visible behavior change updates `docs/README.md` and the matching `docs/rules/*.md`.
4. A change that alters findings has a measured corpus diff (see below) in the PR description.
5. `git diff` contains no unrelated edits.
6. Every exclusion, skip, suppression, or deferral the change adds names what it leaves unchecked and what reports it instead, in the PR or design doc. If nothing does, link a tracking issue. A deferral ("coverage reports this") needs a test that the other check fires in that exact case. "The false positive went away" isn't evidence a suppression is right: #77's `existsInSiblingBlock` silenced a finding by accepting unverified fields, and #80 found two deferrals where neither check reported.

## Priorities when rules conflict

No false positives > the reported finding names the real defect > existing clean provider configs stay clean > catching more true findings > minimal diff > style.

A linter that cries wolf gets disabled. A missed finding costs less than a wrong one.

## Invariants (must)

- **Never guess.** Don't infer a field's section or label when the schema can't settle it: the heading doesn't resolve to a schema path, the path is in `skip_blocks`, the block is `ConfigUnknown`, or the name isn't a scalar at that path. In those cases emit no placement or label finding. This doesn't suppress coverage: an undocumented field still gets a neutral "is not documented" finding.
- **Report the real defect.** A finding's suggested fix must never damage correct docs. Example: an argument documented under Attribute Reference gets "move it to Argument Reference", not "remove its (accurate) label". See `docs/rules/argument-attribute-misplacement.md`.
- **The section determines labels.** Under Argument Reference, each entry carries the one schema-correct label: `(Required)` if Required, `(Optional)` if configurable (including Optional+Computed), and `(Read-Only)` if computed-only, which is allowed there only when `allow_inline_read_only = true` (otherwise the fix is to move it). Under Attribute Reference, entries carry no labels.
- **Output is deterministic.** Never let output, dedup, or signatures depend on Go map iteration order. Iterate `slices.Sorted(maps.Keys(m))`, and sort anything that feeds a comparison. Run twice and diff to confirm. This has caused flapping findings more than once.
- **Dedup by schema path, never by leaf name.** Sibling blocks often share leaf names (`match`, `fields`, `header`); dedup by leaf silently drops or misattributes findings.
- **Configurable object-typed fields have unknowable per-field flags.** Require them to be documented, but accept either section. See `docs/rules/object-typed-attributes.md`.
- **Anchor slugs match GitHub/Registry rendering** (underscores are kept, dots and backticks dropped). Slug logic has regressed before; change it only alongside a test.

## Defaults (may bend with a stated reason)

- Extend an existing `schema_docs` sub-check rather than adding a toggle. A toggle splits one check's question in two, and providers then configure both, per target too (`override` blocks). Concerns that are schema-independent and appear anywhere in a doc (for example, dead anchors) belong in a standalone rule with its own scoping and `severity`.
- New checks that add many findings ship opt-in, or as `severity = "warning"`, so providers can roll them out gradually.
- Gate expensive per-line scans with a cheap combined pre-filter (see `internal/check/gloss.go`).
- Config options use imperative names (`require_*`, `ignore_*`, `allow_*`, `skip_*`, `enforce_*`), and a list option that can grow long or is edited often has a `_file` variant. Today those are `ignore_targets`, `ignore_contents_check`, `allow_subcategories`, `ignore_missing`, `ignore_extra`, `ignore_resources`, and `skip_blocks`. Short, stable lists such as `types`, `targets`, or `block_heading_styles` don't need one; add a variant when one starts to grow. Breaking config changes are allowed before 1.0 but must be shown before and after in the PR.

## Stop and redesign

If you are on the third patch for another heading or schema permutation, stop. You are probably analyzing at the wrong granularity: #61 took eight review rounds operating per-subsection when the defect was per-attribute. Write down the core rule in a `docs/rules/` design doc, then reimplement it against that rule.

If two existing implementations disagree, don't adopt whichever you found first as precedent. Check the design docs and git history for the intended behavior, or ask.

## Issues and review comments

Treat an issue, a review comment, or a fix proposed in either as a hypothesis, including ones an agent wrote. Before acting:

- Confirm the defect against the code and, where it's about findings, the corpus. Issues get facts wrong: #74's premise was a coverage bug, and #86 claims a nondeterminism its dedup key rules out.
- Ask whether the proposed fix is the right design, not just whether it removes the symptom. What else does it change? Which invariant could it break? Does it push provider docs toward something worse, such as bullets that restate field names?
- If the issue or proposal is wrong, say so, with evidence, and correct the issue. Don't implement it to close it.

## Don't

- Don't refactor adjacent code (especially `internal/check/schema_docs.go`) while fixing something in it.
- Don't create new packages or `*_helpers.go` files to isolate a helper. Put it in the file that uses it.
- Don't claim "no regression" or "byte-identical" without a measured corpus diff.
- Don't change provider docs or provider configs to make a swissshepherd change look clean. If the tool is wrong, fix the tool.

## Tests

- Use stdlib `testing` only. Tests are table-driven and call `t.Parallel()` in every test and subtest, except tests that use `t.Chdir` or `t.Setenv` (Go panics if these are combined with `t.Parallel()`). Mark those with a `// Not parallel: <reason>` comment instead.
- Default to black-box `_test` packages. White-box tests are fine for pure unexported primitives.
- Add a test to the existing file for its source file, not a new file for a new topic. `foo.go` is tested in `foo_test.go`, plus `foo_internal_test.go` for white-box tests. A new test file needs a new source file.
- Two large source files are split by area instead. `internal/check/schema_docs.go` has one file per sub-check: `coverage_test.go` (coverage, phantom fields and blocks, Read-Only coverage, shared sections, the #77 acceptance cases), `labels_test.go`, and `schema_docs_test.go` for the rest. `internal/doc/doc.go` has `blocks_test.go`, `listitem_test.go`, `anchors_test.go`, and `doc_test.go`.
- Name a test `Test<Rule or sub-check>_<Case>`, using the prefix its file already uses (`TestCoverage_`, `TestLabels_`), so `-run TestCoverage` selects that file's tests.
- When a real provider doc exposes a bug, freeze the pre-fix doc in `testdata/` as a fixture.
- Test the negative cases: every never-guess condition must produce no findings.
- For nondeterminism, run the check many times in-process and assert identical output.

## Corpus validation

Any change that can alter findings must be measured against a real provider. terraform-provider-aws is the reference: its `.ci/swissshepherd-weak.hcl` is what its CI runs, and `.ci/swissshepherd-full.hcl` enables more. The full config does not set every option (for example `nested_object_attributes`, which the weak config sets), so measure both.

```bash
PROVIDER_DIR=/path/to/terraform-provider-aws make corpus   # or scripts/corpus.sh BASE; BASE defaults to main
```

`scripts/corpus.sh` builds BASE in a temporary worktree and the working tree (uncommitted changes included), and runs both configs in the provider with its cached schema. It sorts with `LC_ALL=C` (locale collation once made `comm` report findings that didn't exist), runs the new build twice and fails if the outputs differ, and removes the worktree. It prints counts per config and leaves `base-`, `new-`, `added-`, and `removed-<config>.txt` in `$CORPUS_OUT` for spot checks.

Then:

- Spot-check every added weak-config finding, and report the before and after counts in the PR. The weak config's error count may rise: more findings are better when they are correct. A change is worse only if it adds false positives or lets a real defect pass silently.
- Spot-check added full-config findings against the provider's schema, and report the false-positive count.
- Put the script's summary lines and the two SHAs it prints in the PR.

## PRs

Keep the title under 70 characters. The body covers: summary, root cause (for bugs), fix, and testing. For changes that can alter findings, it also covers corpus results (baseline vs. branch, added/removed, weak-config impact). Link issues with `Closes #N`.
