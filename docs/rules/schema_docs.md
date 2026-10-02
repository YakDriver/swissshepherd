# `schema_docs` rule
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

The primary rule. Validates argument and attribute documentation against the provider schema.

## Sub-checks

All enabled by default; disable individually via the rule's config block.

| Sub-check     | What it validates                                                                                                                 |
|---------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `byline`      | First paragraph after section heading matches expected byline text (from type)                                                    |
| `coverage`    | Every schema field is documented, in the section its path resolves to; every documented field exists at every path its section documents; a section shared by several paths is right for all of them; headings that document nothing, or the same key twice, are reported (see [Coverage: which section documents a block](#coverage-which-section-documents-a-block)) |
| `deprecated`  | Deprecation status matches between schema and docs (both directions); a mismatch is an error                                     |
| `description` | Descriptions don't start with weak/redundant/meta prefixes ("The ", "This ", "Contains ", "Used ", etc.). One finding per bullet, so same-named bullets in different sections are each reported |
| `format`      | No code blocks in arg/attr sections; single-line attrs; uninterrupted lists                                                       |
| `heading`     | Block headings match the preferred template style. Style only: whether a heading documents the right block is `coverage`'s job     |
| `labels`      | Arguments carry a present, schema-correct label — (Required)/(Optional), or (Read-Only) for read-only attributes when allow_inline_read_only = true; attributes carry none. A wrong label is an error, a missing one a warning |
| `ordering`    | Attributes alphabetical (single-byline lists as one group; split required/optional bylines as separate groups)                    |

## Config

```hcl
check "schema_docs" {
  # Sub-check toggles
  byline      = true
  coverage    = true
  deprecated  = true
  description = true
  format      = true
  heading     = true
  labels      = true
  ordering    = true

  # Heading templates (see "Heading templates" docs)
  block_heading_styles        = ["`{Block}` Block", "{Block}", "{Title}"]
  prefer_block_heading_styles = ["`{Block}` Block"]

  # Coverage options
  ignore_deprecated      = true                     # skip deprecated schema attrs
  implicit_attributes    = ["id", "tags_all"]       # never flagged as undocumented
  allow_phantoms         = ["tags", "tags_all"]     # never flagged as phantom
  skip_blocks            = ["timeouts"]             # blocks skipped entirely
  allow_inline_read_only = false                    # see "Schema model" below

  # Description options
  # Overrides the default weak/redundant/meta starts. Default:
  #   "A ", "An ", "The ", "This ", "It ",
  #   "Indicates ", "Specifies ", "Describes ", "Defines ",
  #   "Contains ", "Determines ", "Identifies ", "Represents ", "Denotes ", "Holds ", "Used "
  bad_prefixes = ["A ", "An ", "The ", "This ", "Specifies "]

  # Format options
  no_code_blocks              = true   # no fenced code blocks in arg/attr sections
  single_line_attrs           = true   # each attribute on one line
  uninterrupted_lists         = true   # no paragraphs between list items
  allow_attribute_indentation = true   # allow indented sub-attributes in Attribute Reference (default: true)

  # Object-typed attribute coverage (opt-in; default false)
  nested_object_attributes    = true   # check fields of list(object)/set(object)/object attributes
}
```

## Object-typed attributes (`nested_object_attributes`)

Legacy `list(object({...}))` / `set(object({...}))` / bare `object({...})` attributes carry their fields as a nested *type*, not as a schema block. By default swissshepherd treats such an attribute as a single leaf: its inner fields are neither required to be documented nor style-checked.

Set `nested_object_attributes = true` to model those fields as nested blocks. When enabled:

- the schema expands each object-typed attribute into dot-path blocks (e.g. `items`, `items.dns_entry`), so `coverage`, `description`, `ordering`, and `labels` apply to their fields at every depth; and
- the doc parser captures the inline-indented sub-bullets those fields are conventionally documented with (the "Each object has the following attributes:" pattern), matching them against the expanded schema.

The cty type encoding of an object (`list/set/map(object({...}))`, `object`) records field names and types but **no per-field** `Required`/`Optional`/`Computed`. swissshepherd therefore uses the *parent* attribute's configurability:

- when the parent is **Computed-only**, its fields are necessarily read-only and must be documented in `## Attribute Reference` (the usual Read-Only coverage rule); but
- when the parent is **Optional and/or Required**, each field's configurability is unknowable, so a field may be documented in **either** Argument Reference (as an argument) or Attribute Reference — coverage still requires it to be documented *somewhere*, but no specific section is enforced. (If AWS later migrates such an attribute to a Framework `nested_type`, which does carry per-field flags, section-precise checking resumes automatically.)

This is **off by default** because enabling it surfaces a large number of new findings in docs that previously passed. Roll it out gradually — stage with `ignore_targets` / `skip_blocks`, or enable it once the affected docs are clean.

For the full rationale behind the parent-configurability model and the `ConfigUnknown` flag — why some object fields cannot be classified into a specific section, the proto5 constraint that causes it, and the future-alignment path — see [Object-typed attributes, proto5, and the `ConfigUnknown` escape hatch](object-typed-attributes.md).

### Nested fields documented under a shared or prose-introduced subsection

Coverage decides which section documents a block from section headings alone, never from links or position, because a reader looking at a section can't see what links to it (see [Coverage: one section per path](coverage-path-resolution.md)). Two conventions attach a nested block's fields to it without a dedicated per-path heading:

- **Shared subsection.** Structurally identical sibling blocks can be documented once, under a heading that names each of them:

  ```markdown
  * `management` - Endpoint ... See [`intercluster` and `management`](#intercluster-and-management).
  * `intercluster` - Endpoint ... See [`intercluster` and `management`](#intercluster-and-management).

  #### `intercluster` and `management`

  * `dns_name` - ...
  * `ip_addresses` - ...
  ```

  A subsection with a heading that doesn't name the blocks, such as `#### Endpoint` linked from both bullets, doesn't document them: the bullets' links are navigation, and coverage reports the blocks' fields as undocumented. Earlier versions followed such links (issue #51); that was reversed in #77.

- **Legacy indexed prose lead-in.** Older docs introduce a nested block's fields with a sentence instead of a heading:

  ```markdown
  The `catalog_properties[0].data_lake_access_properties[0]` block also exports:

  * `managed_workgroup_name` - ...
  * `status_message` - ...
  ```

  swissshepherd recognizes this lead-in (a backtick-quoted dotted/indexed path followed by `block ... supports:`/`exports:`) as a block boundary, keying the following bullets to the dot-path — just as a `#### catalog_properties.data_lake_access_properties` heading would. This avoids a cascade of misattributed coverage, ordering, and "list interrupted" findings.

- **Other prose lead-ins** ("The `cloudwatch_logs` object takes the following arguments:") also document the block they name, with a style warning asking for a heading. See [Prose-introduced lists](#prose-introduced-lists).

## Schema model: Required / Optional / Read-Only

The `coverage` sub-check enforces presence of every schema attribute at every depth of nesting. swissshepherd uses the same three-category mental model as tfplugindocs:

- **Required** — must be set in configuration. Documented in `## Argument Reference` with `(Required)`.
- **Optional** — may be set in configuration. Documented in `## Argument Reference` with `(Optional)`. Includes attributes that are both Optional and Computed (configurable, so still `(Optional)`).
- **Read-Only** — never set in configuration; always populated by the provider. Documented in `## Attribute Reference`, or — when `allow_inline_read_only = true` — inline in `## Argument Reference` with `(Read-Only)`.

When a genuinely configurable argument (`Required`/`Optional` and not `Computed`) is instead documented under `## Attribute Reference`, the `labels` sub-check reports a *misplacement* — directing the author to move it to Argument Reference rather than to strip its (correct) label (issues #60, #62). For the design and rationale behind that detection — attribute-granular classification, heading→schema-path resolution, path-based severity (a genuine root scalar is a warning; every nested move is an error), and the measured corpus evidence — see [Argument/Attribute-Reference Misplacement](argument-attribute-misplacement.md). When such an argument carries no label at all, `labels` can't tell it from a computed output, so `coverage` reports it instead, with the label to add: `argument "name" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference and label it (Required)`. Severity matches the labeled case: a warning at the root, an error when nested (a warning if the field is deprecated).

### Label correctness

The `labels` sub-check validates that a documented argument's label is both **present and correct** — a single question, "is the label right?", not two separate ones. A present label whose value contradicts the schema is as much a defect as a missing one: `(Required)` on an attribute that is actually `Optional` misleads users about what they must set.

The invariant is single-valued: exactly one label is correct for each attribute — `(Required)` when the schema attribute is `Required`, `(Optional)` when it is configurable but not required (pure `Optional` or `Optional`+`Computed`), and `(Read-Only)` when it is computed-only. The other two category labels are wrong; for example an `Optional`+`Computed` field must read `(Optional)`, so both `(Required)` and `(Read-Only)` on it are reported.

Correctness lives inside the `labels` sub-check rather than behind a separate toggle. `schema_docs` shares one `ignore_targets`/`prefixes` scope across all sub-checks, so a second toggle would only add a global on/off, never per-target granularity. Validating the label's value was always the intent of `labels`; the earlier presence-only behavior was a gap in that check, not a deliberately narrower feature.

A section is checked against every block it documents, decided the way `coverage` decides it (see "Coverage: which section documents a block" below). A shared section, such as one `` `encryption_configuration` `` heading documenting two blocks, or a partly qualified one, such as `` `a.z` `` for block `w.a.z`, is checked like an exact one, and its finding names the section as written.

The check never fires on a guess. It reports nothing when:

- the section documents no block (including blocks in `skip_blocks`);
- the name is not a scalar attribute at any block it documents whose labels are known (e.g. a child-block reference bullet). A `ConfigUnknown` block (an object-typed synthesized block whose per-field configurability is unknowable — see [Object-typed attributes](object-typed-attributes.md)) doesn't count;
- those blocks disagree about the label: no label is right for every one of them. `coverage` reports that once as a shared-section finding; with `coverage` off, nothing reports it; or
- the section's key has several headings and `coverage` is on: the duplicate-heading check compares each heading's labels instead.

Label additions such as `(Required, Forces new resource)` do not affect detection: the required/optional state is read from the leading token, and trailing traits are left as authored.

Correctness also covers the inline `(Read-Only)` label permitted when `allow_inline_read_only = true`: it is valid only for a genuinely read-only (computed-only) attribute. A configurable (`Required`/`Optional`) field mislabeled `(Read-Only)` is reported the same way, directing the author to the correct `(Required)`/`(Optional)` label. Conversely, a computed-only field mislabeled `(Required)`/`(Optional)` is reported with `use (Read-Only)` — but only under `allow_inline_read_only = true`; in strict mode a computed-only field does not belong in Argument Reference at all, which `coverage` reports instead, so `labels` defers to avoid a double finding. The same holds for a `(Read-Only)` label in strict mode, which isn't an Argument Reference label. When Attribute Reference already documents the field, `coverage` is satisfied, so `labels` reports it: `argument "r" in block "z" is labeled (Optional) but is computed-only in the schema; Attribute Reference already documents it, so remove it from Argument Reference`.

Under `## Attribute Reference` the rule is the mirror image: attributes carry **no label at all**. A `(Read-Only)` label there is flagged for removal, just as a stray `(Required)`/`(Optional)` label is — a configurable field is additionally directed to move to Argument Reference (see [Argument/Attribute-Reference Misplacement](argument-attribute-misplacement.md)). This holds regardless of `allow_inline_read_only`, which only governs inline `(Read-Only)` in Argument Reference.

**Interaction with `ordering`.** In docs that split arguments under `The following arguments are required:` / `optional:` bylines, correcting a label changes the group the argument belongs to, so the byline semantics require relocating it under the matching byline and re-alphabetizing. Note that `ordering` does not enforce that physical placement: it rebuilds the required/optional groups from each bullet's *parsed label* — not from the byline it sits under — and checks each derived group alphabetically. It therefore *may* surface an alphabetical violation when a corrected bullet is left under the wrong byline, but only if the bullet's name breaks alphabetical order within its label group; a corrected bullet whose name still sorts correctly stays under the wrong byline with no finding. Treat the relocation as a manual step that pairs with the label fix, not something `ordering` will always catch.

For nested blocks, Read-Only attributes can be documented in any of the following equivalent forms:

- Under a nested-block heading inside `## Attribute Reference`:

  ````markdown
  ### `network` Block

  * `private_ip` - Private IP address.
  ````

- As a dot-notation reference at the root level of `## Attribute Reference`. Multi-level paths are supported, matching the path style produced by tfplugindocs's anchor IDs:

  ```markdown
  * `network[*].private_ip` - Private IP address.
  * `analyzer_configuration.unused_access_configuration.computed_summary` - Summary of unused access.
  ```

- Inline inside the `### \`block\`` heading in Argument Reference, with the `(Read-Only)` label, when `allow_inline_read_only = true`:

  ````markdown
  ### `network` Block

  * `private_ip` - (Read-Only) Private IP address.
  * `subnet_id` - (Required) Subnet identifier.
  ````

The default (`allow_inline_read_only = false`) preserves the AWS provider's traditional separation: `## Argument Reference` for configurable attributes, `## Attribute Reference` for Read-Only ones. Setting the toggle to `true` permits the tfplugindocs-aligned permissive convention without requiring all docs to convert at once.

## Coverage: which section documents a block

A section's identity comes from its heading alone. Links and position never decide which block a section documents: a reader looking at a section can't see what links to it. The design, its acceptance cases, and the measured corpus impact are in [Coverage: one section per path](coverage-path-resolution.md) (#77).

### Resolution

For each schema path, `coverage` looks for one section in Argument Reference and one in Attribute Reference. For `w.x.y.z` it tries these heading keys in order, taking the first one that names a section:

1. the full path, `w.x.y.z`;
2. two adjacent ancestors and the leaf, nearest first: `x.y.z`, then `w.x.z`;
3. one ancestor and the leaf, nearest first: `y.z`, `x.z`, then `w.z`;
4. the bare leaf, `z`.

A dotted key that is itself a different schema path is skipped, so `x.z` never documents `w.x.y.z` when the schema also has a block at `x.z`. Paths in `skip_blocks` are skipped entirely.

Every field a section lists must exist at **every** path that resolves to it. A field the schema expects at a path counts as documented if it's listed in either of that path's sections, so the misplacement findings, not "is not documented", handle a field in the wrong reference section.

### Shared sections

A section with a short heading (`` ### `z` Block ``) can serve several paths. That's fine when the paths are interchangeable, and wrong when they aren't. A shared section is valid only if, across every path it serves:

- every field that must be listed for one path (its only home is this reference section) exists at all of them;
- each listed field has the same label and the same deprecation status; and
- child blocks it lists are themselves interchangeable at every depth.

Otherwise `coverage` reports the section once per conflicting field, naming two of the paths and suggesting a qualified heading:

```text
section "z" (line 24) in Argument Reference documents 2 paths; "a2" must be listed for "x.y.z" but doesn't exist at "t.u.z". Qualifying means up to 2 sections: give "x.y.z" a heading that resolves only to paths where "a2" exists, e.g. "`x.y.z` Block"
```

Existence and child-content conflicts are warnings, because the per-path field findings already report the errors. Label and deprecation conflicts are errors: no single marker is right for every path. This replaces the `heading` sub-check's former "is ambiguous" warning, so it now needs `coverage` enabled.

The per-path field findings point at the cause too. A phantom field in a shared section names the other paths where the field does exist; a missing field names the other paths the section serves where it doesn't, so the fix is a qualified heading, not another bullet.

### Headings that document nothing

| Finding | Severity | Meaning |
|---|---|---|
| `heading "…" … isn't a recognized block heading, so its bullets aren't checked against the schema` | warning | An H3+ heading in a reference section matches no `block_heading_styles` template. Its bullets belong to no section; only `description` still checks them. Suggests a heading when the text names a schema block. |
| `heading "…" … documents no block (…), so its fields aren't checked against the schema` | warning | The heading parses, but no schema path resolves to its key. Headings with no bullets are skipped. |
| `block "P" is not documented (M paths beneath it are also undocumented; K other undocumented paths share the name "L")` | error | No section resolves to `P`, which has configurable fields. Only the shallowest undocumented block in a subtree is reported; the counts say how much lies beneath it. |

### Duplicate headings

When two headings in one reference section normalize to the same key, `coverage` keeps both occurrences and asks whether some assignment of headings to the paths that key serves makes every block exact: listed fields exist, fields whose only home is this section are listed, and labels and deprecation markers are right.

- If no assignment works, each path with no fitting heading, and each heading that fits no path, is an **error** naming the closest match and its differences. A heading isn't reported again when its closest path already is, with the same differences.
- If one works, it's still a **warning**: a reader can't tell from the headings which block each documents. The fix is to give each occurrence the heading of its path.

The per-path field findings and the shared-section finding are not emitted for a duplicated key; the fit rule stands in for them.

### Prose-introduced lists

A colon-terminated paragraph that names a schema block in backticks is treated as a heading for that block:

```markdown
The `cloudwatch_logs` object takes the following arguments:

* `role_arn` - (Required) IAM role ARN.
```

The list documents `cloudwatch_logs` and gets every check a section gets. Like a heading, the prose stays in effect until the next heading or lead-in, so a code block interrupting its list doesn't end it. A warning asks for a real heading: `list introduced by prose ("…") in Argument Reference documents "cloudwatch_logs" without a block heading; use a block heading, e.g. "`cloudwatch_logs` Block"`.

This is a parsing decision, not a coverage one: it applies whether or not `coverage` is enabled, so `ordering`, `description`, `labels`, and `format` judge a prose list's bullets against the block it names. The same holds for bullets under an unparseable heading, which belong to no section for every sub-check.

Exceptions, where reading prose as a heading would credit the wrong block:

- Prose that names nothing in the schema ("…the same arguments as `aws_instance`, with the addition of:") continues its section.
- Prose that names the section's own block ("The `rule` block also supports:" under a `rule` heading) continues it.
- Before any list, prose directly under a reference section heading, or under a heading that resolves to a schema path, introduces that heading's own list. Under an unparseable or unresolved heading, it opens the named block.
- Markdown can't end a list, so authors sometimes resume the enclosing section after a blank line. A trailing run of bullets that are fields of the enclosing section, and not of the named block, stays in the enclosing section, and the warning adds `The list runs on into fields of the enclosing section from line N; end it before them`. A section field anywhere else in the list belongs to the named block and is reported there.

### Phantom fields

A listed field that doesn't exist at a path its section serves is an **error**: `documented argument "x" in block "p" does not exist in schema` under Argument Reference, `documented attribute …` under Attribute Reference.

## Coverage: phantom block headings

The `coverage` sub-check also flags H3+ headings inside `## Argument Reference` whose name doesn't match any schema block. Common patterns this catches:

- Stray subsection headings like `#### Arguments` or `#### Nested Blocks` under a real block heading. The H3 already declares the block; nested "Arguments" / "Nested Blocks" subheadings are noise that the parser interprets as phantom blocks.
- Title-Case descriptive headings (`### Tool Specification`, `### Restore Configuration`) for nested blocks where the schema name is different. Use `` ### `tool_specification` Block `` or another canonical heading style.
- Combined headings (`### egress and ingress`) when the schema doesn't actually have those blocks (e.g. when the underlying type is attribute-as-blocks).

The check is restricted to `## Argument Reference`. Block-style headings under `## Attribute Reference` (e.g. `### Endpoint`, `### master_user_secret`) commonly document the structure of computed attributes that have no Block representation in the schema, and are not flagged.

## Ordering: single vs. split lists

The `ordering` sub-check adapts to how arguments are presented in the doc:

- **Single combined list** — when the byline is `This resource supports the following arguments:`, `` Each `block` supports: ``, or any single byline preceding one list, all attributes (Required and Optional) are checked as one alphabetical sequence.
- **Split lists** — when the doc uses two bylines: `The following arguments are required:` followed (after the required list) by `The following arguments are optional:`, each group is checked independently. Required attributes alphabetical among themselves; Optional attributes alphabetical among themselves.

The signal comes directly from the byline text, not the order of attributes. Mixing labels in a single-list block (e.g., a `(Required)` after several `(Optional)` items) is allowed as long as the names are alphabetical overall.
