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
| `description` | Descriptions don't start with bad prefixes ("A ", "The ", "Specifies ", etc.)                                                     |
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
  bad_prefixes = ["A ", "An ", "The ", "Specifies "]

  # Format options
  no_code_blocks              = true   # no fenced code blocks in arg/attr sections
  single_line_attrs           = true   # each attribute on one line
  uninterrupted_lists         = true   # no paragraphs between list items
  allow_attribute_indentation = true   # allow indented sub-attributes in Attribute Reference (default: true)
}
```

## Schema model: Required / Optional / Read-Only

The `coverage` sub-check enforces presence of every schema attribute at every depth of nesting. swissshepherd uses the same three-category mental model as tfplugindocs:

- **Required** — must be set in configuration. Documented in `## Argument Reference` with `(Required)`.
- **Optional** — may be set in configuration. Documented in `## Argument Reference` with `(Optional)`. Includes attributes that are both Optional and Computed (configurable, so still `(Optional)`).
- **Read-Only** — never set in configuration; always populated by the provider. Documented in `## Attribute Reference`, or — when `allow_inline_read_only = true` — inline in `## Argument Reference` with `(Read-Only)`.

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
