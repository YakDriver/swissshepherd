# Field homes: one section per field, no repeats
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

Issue #111. This rule decides which reference section a field belongs in. It supersedes the field-home table in [coverage-path-resolution.md](coverage-path-resolution.md) §2 and the Optional+Computed exemption in [argument-attribute-misplacement.md](argument-attribute-misplacement.md) §3. Where earlier designs disagree with this page, this page wins.

## Core rule

> **Every field at schema path P has exactly one home, and is listed only there.**

| Field at P | Home |
|---|---|
| `Required`, `Optional`, or `Optional` + `Computed` | Argument Reference |
| a child block whose subtree has any `Required` or `Optional` field | Argument Reference |
| Computed-only, or a child block whose subtree has no `Required` or `Optional` field | Attribute Reference; Argument Reference instead is allowed when `allow_inline_read_only = true` |
| any field of a `ConfigUnknown` block (configurable object-typed attribute) | unknowable: either section |

Two consequences, checked separately:

1. **Wrong section.** A field listed only outside its home is reported with a move. This is the existing misplacement and home-section behavior, with Optional+Computed now counted as configurable.
2. **Repeated.** A field listed in both P's Argument Reference section and P's Attribute Reference section is reported, whatever its home. Neither entry is "also fine": the duplicate adds nothing, the Attribute Reference byline already says "in addition to the arguments above", and duplicates drift (`aws_media_packagev2_channel_group`: `` `description` - The same as `description` ``).

## What changed and why

Optional+Computed fields used to have home "either", so they could be listed in Attribute Reference, in Argument Reference, or in both. The flags say the user can set the field; that it can also be computed when unset is a default, not a second identity. It's an argument. Its default behavior belongs in its Argument Reference description.

Child blocks follow the same change: a child block whose subtree has an Optional+Computed field, and no pure-configurable one, used to be "either". It's now an argument.

### Optional+Computed that may be an output

Optional+Computed is sometimes not an input at all. terraform-provider-aws's SDKv2 `tftags.TagsSchemaComputed()` is Optional+Computed and backs resource `tags_all` and data-source `tags`, but most of those fields are only ever set by the provider (hashicorp/terraform-provider-aws#50491: 44 data-source sites read `tags` as a filter, 188 don't). Nothing in the schema or the doc tells the two apart, so a finding that names one fix would be wrong for the other.

A field that is an argument *only* through Optional+Computed (an Optional+Computed scalar, or a child block whose configurable subtree has no `Required` field and no `Optional` field that isn't also `Computed`) therefore gets findings that name both fixes:

- Documented only under Attribute Reference: a **warning**, `argument "tags" is Optional+Computed in the schema but is documented only under Attribute Reference; if it can be set, move it to Argument Reference and label it (Optional); if setting it has no effect, the schema should mark it Computed only`. (A child block says "has only Optional+Computed arguments".) A subsection that documents such a field doesn't collapse into "move this subsection", since that would assert the move.
- Listed in both: still an **error** (the repeat is certain), with the fix "remove the Attribute Reference entry if it can be set; if setting it has no effect, remove the Argument Reference entry instead, and the schema should mark it Computed only".

Its home is still Argument Reference: a correct `tags - (Optional)` filter in Argument Reference draws nothing. `tags_all` is an implicit attribute (below), so it's never told to move. When the provider makes output-only fields Computed, their warnings stop with no change here.

What this gives up: an Optional+Computed field that really is an input, documented only under Attribute Reference, gets a warning instead of an error. It's still reported.

### `ConfigUnknown`

`ConfigUnknown` stays "either": the schema doesn't say whether each field is configurable (see [object-typed-attributes.md](object-typed-attributes.md)), so the home can't be named without guessing. A repeat is still certain, so it's reported, with a neutral fix.

## Repeated-field finding

For each schema path P (sorted, excluding `skip_blocks`) whose Argument Reference section `A` and Attribute Reference section `B` both resolve (§4 of coverage-path-resolution.md), and each schema field F at P (attribute or child block) listed in both:

| Home of F | Fix in the message |
|---|---|
| Argument Reference | remove the Attribute Reference entry |
| Argument Reference, only through Optional+Computed | remove the Attribute Reference entry if it can be set; otherwise the Argument Reference entry, and fix the schema |
| Attribute Reference | remove the Argument Reference entry |
| either (`ConfigUnknown`, computed-only with `allow_inline_read_only = true`, or `implicit_attributes`) | keep one entry |

Message: `argument "F" in block "P" is listed in both Argument Reference (line L1) and Attribute Reference (line L2); <fix>`. `attribute` instead of `argument` when the home is Attribute Reference. Root fields omit `in block`.

Severity: **error**. The `AGENTS.md` default for a high-volume new check is a warning; it bends here by maintainer decision ("no repeats, ever"), with the provider's findings fixed in the same terraform-provider-aws PR that bumps swissshepherd.

Scope and exclusions:

- Runs when `coverage` or `labels` is enabled. Before this rule both reported parts of it, with the wrong fix (below); running under either keeps the deferrals from leaving it unchecked.
- `implicit_attributes` (`id`, `tags_all`) have no enforced home: SDKv2 reports `id` as Optional+Computed whether or not anyone sets it, and coverage doesn't require them. A repeat is still reported, with "keep one entry". A labeled one under Attribute Reference keeps the strip-label finding and is never told to move.
- `ignore_deprecated` relaxes "must be documented", not "listed once": deprecated fields are checked.
- Two entries with the same source line are one bullet the parser recorded twice, not a repeat.
- Dedup: one finding per (Argument Reference section, Attribute Reference section, F). A bare-leaf heading pair serving several paths is one repeated bullet, so one finding, reported at the first path in sorted order.
- Fields not in the schema at P are left to the phantom-field check.

## Deferrals (what stops reporting, and what reports it instead)

| Case | Before | After |
|---|---|---|
| Labeled configurable field under Attribute Reference, also in Argument Reference | `labels` misplacement: "move it to Argument Reference" (wrong: it's already there) | repeated-field finding. Misplacement treats it as a repeat, not a move. |
| Same, for every field of a subsection | `labels` collapse: "move this subsection to Argument Reference" | repeated-field findings per field. A subsection whose misplaced fields are all repeats doesn't collapse. |
| Computed-only field labeled in Argument Reference, also in Attribute Reference | `labels`: "Attribute Reference already documents it, so remove it from Argument Reference" | repeated-field finding, same fix |
| Unlabeled configurable field in both sections | nothing (`outsideHome` skipped it) | repeated-field finding |

Each row has a test that the repeated-field finding fires and the old finding doesn't.

## What stays unchecked

- A repeat across paths: F listed in A for P and in B's section for a different path Q. These are different fields.
- A repeat within one section (two bullets for F under one heading). Not a cross-section question; ordering and the fit rule see some of these. Not addressed here.
