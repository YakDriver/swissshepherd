# `schema_docs` rule
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

The primary rule. Validates argument and attribute documentation against the provider schema.

## Sub-checks

All enabled by default; disable individually via the rule's config block.

| Sub-check     | What it validates                                                                                                                 |
|---------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `byline`      | First paragraph after section heading matches expected byline text (from type)                                                    |
| `coverage`    | Every schema attr is documented; every documented attr exists in schema; every block heading in Argument Reference matches a schema block |
| `deprecated`  | Deprecation status matches between schema and docs (both directions)                                                              |
| `description` | Descriptions don't start with weak/redundant/meta prefixes ("The ", "This ", "Contains ", "Used ", etc.)                              |
| `format`      | No code blocks in arg/attr sections; single-line attrs; uninterrupted lists                                                       |
| `heading`     | Block headings match the preferred template style                                                                                  |
| `labels`      | Arguments have (Required)/(Optional) labels (and optionally (Read-Only) when allow_inline_read_only = true); attributes do not                                                                |
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

Two long-standing documentation conventions attach a nested block's fields to it without a dedicated per-path heading. When `nested_object_attributes` is enabled, coverage understands both:

- **Shared subsection.** Structurally-identical sibling blocks are often documented once under a single subsection that each sibling links to, e.g.

  ```markdown
  * `management` - Endpoint ... See [Endpoint](#endpoint).
  * `intercluster` - Endpoint ... See [Endpoint](#endpoint).

  #### Endpoint

  * `dns_name` - ...
  * `ip_addresses` - ...
  ```

  Coverage follows each sibling bullet's in-page link to the shared subsection, so both `endpoints.management` and `endpoints.intercluster` are credited with the `Endpoint` block's fields. The link is followed only for the sibling's own bullet, so unrelated paths are never mis-credited. Siblings with different names sharing one subsection (e.g. `available_labels`/`consumed_labels` under `### Labels`) work the same way.

- **Legacy indexed prose lead-in.** Older docs introduce a nested block's fields with a sentence instead of a heading:

  ```markdown
  The `catalog_properties[0].data_lake_access_properties[0]` block also exports:

  * `managed_workgroup_name` - ...
  * `status_message` - ...
  ```

  swissshepherd recognizes this lead-in (a backtick-quoted dotted/indexed path followed by `block ... supports:`/`exports:`) as a block boundary, keying the following bullets to the dot-path — just as a `#### catalog_properties.data_lake_access_properties` heading would. This avoids a cascade of misattributed coverage, ordering, and "list interrupted" findings.

## Schema model: Required / Optional / Read-Only

The `coverage` sub-check enforces presence of every schema attribute at every depth of nesting. swissshepherd uses the same three-category mental model as tfplugindocs:

- **Required** — must be set in configuration. Documented in `## Argument Reference` with `(Required)`.
- **Optional** — may be set in configuration. Documented in `## Argument Reference` with `(Optional)`. Includes attributes that are both Optional and Computed (configurable, so still `(Optional)`).
- **Read-Only** — never set in configuration; always populated by the provider. Documented in `## Attribute Reference`, or — when `allow_inline_read_only = true` — inline in `## Argument Reference` with `(Read-Only)`.

When a genuinely configurable argument (`Required`/`Optional` and not `Computed`) is instead documented under `## Attribute Reference`, the `labels` sub-check reports a *misplacement* — directing the author to move it to Argument Reference rather than to strip its (correct) label (issues #60, #62). For the design and rationale behind that detection — attribute-granular classification, heading→schema-path resolution, path-based severity (a genuine root scalar is a warning; every nested move is an error), and the measured corpus evidence — see [Argument/Attribute-Reference Misplacement](argument-attribute-misplacement.md).

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
