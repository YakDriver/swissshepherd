# swissshepherd
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

A documentation checker for Terraform providers. Validates that provider documentation accurately reflects the provider schema.

## What it does

swissshepherd compares a Terraform provider's schema against its markdown documentation and reports:

- **Missing documentation** — schema attributes or blocks with no corresponding doc entry
- **Phantom documentation** — documented attributes that don't exist in the schema
- **Ordering violations** — arguments or attributes not in alphabetical order
- **Description style issues** — descriptions starting with articles or fluff words
- **Misplaced computed attributes** — computed-only attributes in the wrong section
- **Heading style mismatches** — nested block headings not matching the preferred format
- **Format / structure issues** — code blocks inside argument sections, multi-line attribute descriptions, interrupted attribute lists
- **Frontmatter problems** — missing required YAML frontmatter fields, forbidden fields present, disallowed subcategories

## Installation

```bash
go install github.com/YakDriver/swissshepherd@latest
```

Requires [Terraform](https://developer.hashicorp.com/terraform/install) in PATH when using `provider_dir`.

## Quick start

Create `.swissshepherd.hcl` in your provider repo root:

```hcl
provider_source = "registry.terraform.io/hashicorp/aws"
provider_dir    = "."
docs_path       = "website/docs"
```

Then run:

```bash
# Check everything
swissshepherd

# Check a single service
swissshepherd --prefix aws_dms_

# Check a single resource
swissshepherd --resource aws_iam_role
```

That's it. swissshepherd builds the provider, generates the schema via Terraform, runs the checks, and cleans up.

## Usage

```bash
# With explicit config file
swissshepherd --config path/to/swissshepherd.hcl

# With CLI flags (no config file needed)
swissshepherd --provider-dir . --provider-source registry.terraform.io/hashicorp/aws --docs-path website/docs

# With pre-generated schema (skips build)
swissshepherd --schema-json schema.json --docs-path website/docs --provider-source registry.terraform.io/hashicorp/aws

# JSON output
swissshepherd --resource aws_vpc --json
```

CLI flags override config file values.

## Configuration

```hcl
# .swissshepherd.hcl

provider_source = "registry.terraform.io/hashicorp/aws"
provider_dir    = "."           # builds provider + generates schema automatically
docs_path       = "website/docs"

# Or use a pre-generated schema (skips build):
# schema_json = "path/to/schema.json"

# Subcategory allowlist — referenced by the frontmatter check when set
allowed_subcategories_file = "website/allowed-subcategories.txt"

check "completeness" {
  enabled = true

  ignore_resources    = ["aws_legacy_resource"]
  ignore_data_sources = ["aws_kms_secrets"]

  block_heading_styles = [
    "`{Block}` Block",
    "{Block} Block",
    "`{Block}`",
    "{Block}",
    "{Title}",
  ]

  # When set, emits warnings suggesting this as the canonical form
  preferred_block_heading_styles = ["`{Block}` Block"]
}

check "ordering" {
  enabled = true
}

check "description_style" {
  enabled = true
}

check "computed_attribute" {
  enabled = false  # disable this rule
}

check "heading_style" {
  # Only active when preferred_block_heading_styles is set on completeness
  enabled = true
}

check "format_style" {
  enabled = true
}

check "frontmatter" {
  enabled = true

  # Registry-layout resources forbid layout; legacy-layout resources require it.
  # Pick the set that matches your provider's layout.
  require_subcategory = true
  require_page_title  = true
  require_description = true
  forbid_layout          = true
  forbid_sidebar_current = true
}
```

All rules are enabled by default. Add a `check` block with `enabled = false` to disable one.

## Heading styles

The `block_heading_styles` list in the `completeness` check controls which `###` heading formats are recognized as block documentation. Each entry is a template with placeholders:

| Placeholder | Matches | Example heading | Extracted name |
|-------------|---------|-----------------|----------------|
| `{Block}` | A snake_case identifier (lowercase, underscores) | `network_interface` | `network_interface` |
| `{Title}` | Title Case words (converted to snake_case) | `Network Interface` | `network_interface` |

Templates are tried in order — first match wins. Literal text around the placeholder must match exactly.

### Examples

| Template | Matches heading | Result |
|----------|-----------------|--------|
| `` `{Block}` Block `` | `network Block` | `network` |
| `{Block} Block` | `network Block` | `network` |
| `{Block} Configuration Block` | `vpc_config Configuration Block` | `vpc_config` |
| `{Block} Argument Reference` | `filter Argument Reference` | `filter` |
| `` `{Block}` `` | `statement` | `statement` |
| `{Block}` | `redis_settings` | `redis_settings` |
| `{Title}` | `Credit Specification` | `credit_specification` |
| `{Title}` | `CPU Options` | `cpu_options` |

Note: goldmark (the markdown parser) strips backticks from inline code in headings. So `### \`network\` Block` in markdown becomes `network Block` in the parsed text. The templates match against this parsed text.

### Strict vs permissive

To enforce a single heading format across your docs:

```hcl
block_heading_styles = ["`{Block}` Block"]
```

To accept everything during a migration:

```hcl
block_heading_styles = [
  "`{Block}` Block",
  "{Block} Block",
  "{Block} block",
  "{Block} Configuration Block",
  "{Block} Argument Reference",
  "{Block} Attribute Reference",
  "`{Block}`",
  "{Block}",
  "{Title}",
]
```

If `block_heading_styles` is omitted, a sensible default is used.

## Rules

| Rule | Kind | Description |
|------|------|-------------|
| `completeness` | schema + doc | Every schema attribute is documented; every documented attribute exists in schema |
| `ordering` | schema + doc | Arguments and attributes are alphabetically ordered within required/optional groups |
| `description_style` | schema + doc | Descriptions don't start with weak prefixes (`A`, `An`, `The`, `Specifies`, `Indicates`, `Describes`, `Defines`) |
| `computed_attribute` | schema + doc | Computed-only attributes appear in Attribute Reference, not Argument Reference |
| `heading_style` | schema + doc | Nested block headings match the preferred template (requires `preferred_block_heading_styles`) |
| `format_style` | raw file | No code blocks inside argument/attribute sections, single-line attribute entries, uninterrupted attribute lists |
| `frontmatter` | raw file | YAML frontmatter field presence/absence and subcategory allowlist |

Two kinds of rule share a config surface. `schema + doc` rules compare the parsed markdown AST against the schema; `raw file` rules operate on the file bytes so they can catch whitespace, line-structure, and frontmatter issues the AST normalizes away.

## Frontmatter options

Every frontmatter toggle is off by default — the rule does nothing until at least one is set. Mix and match to match your provider's layout:

| Option | Effect |
|--------|--------|
| `require_subcategory` | Fail if `subcategory` is absent |
| `require_page_title` | Fail if `page_title` is absent |
| `require_description` | Fail if `description` is absent |
| `require_layout` | Fail if `layout` is absent (legacy layout) |
| `forbid_subcategory` | Fail if `subcategory` is present |
| `forbid_page_title` | Fail if `page_title` is present |
| `forbid_description` | Fail if `description` is present |
| `forbid_layout` | Fail if `layout` is present (registry layout) |
| `forbid_sidebar_current` | Fail if `sidebar_current` is present (always, in modern docs) |

The subcategory allowlist is set at the top level via `allowed_subcategories` (inline list) or `allowed_subcategories_file` (newline-separated file). When the allowlist is non-empty, a frontmatter `subcategory` value not on the list fails. An empty allowlist is equivalent to "anything goes". The allowlist only fires when `subcategory` is present in the file — use `require_subcategory` alongside it if absence should also fail.

An unterminated or missing frontmatter block is treated the same as an absent block: each `require_*` toggle produces a result, `forbid_*` toggles stay silent.

## How schema generation works

When `provider_dir` is set, swissshepherd:

1. Runs `go build` in the provider directory
2. Sets up a temporary Terraform plugin directory
3. Runs `terraform init` and `terraform providers schema -json`
4. Uses the generated schema for checks
5. Cleans up all temporary files

If `schema_json` is set instead, the build step is skipped entirely.

## Design

- Schema from `terraform providers schema -json` is the source of truth — no Go source parsing
- Markdown parsed with [goldmark](https://github.com/yuin/goldmark) AST
- Two rule interfaces: `Rule.Check(resource, schema, doc) []Result` for schema + AST checks, `FileRule.CheckFile(resource, path, content) []Result` for raw-bytes checks (frontmatter, line-level scans). The runner reads each doc file once and feeds the bytes to both kinds
- Supports both registry-style (`docs/resources/`) and legacy (`website/docs/r/`) doc layouts
- HCL configuration via [hcl/v2](https://github.com/hashicorp/hcl)
- YAML frontmatter parsed with [yaml.v3](https://gopkg.in/yaml.v3)

## License

MPL-2.0
