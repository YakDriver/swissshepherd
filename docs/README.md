# swissshepherd
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

<img src="swissshepherd_logo_sm.png" alt="swissshepherd logo" width="90" height="80" align="left">

A documentation linter for Terraform providers. swissshepherd compares a provider's schema against its Markdown documentation and reports missing, phantom, misordered, mislabeled, or misformatted docs.

🐙 **Source code:** <https://github.com/YakDriver/swissshepherd>

<br clear="left">

## Table of contents

- [What it checks](#what-it-checks)
- [Installation](#installation)
- [Quick start](#quick-start)
- [CLI reference](#cli-reference)
- [Configuration](#configuration)
- [Rules reference](#rules-reference)
- [Path scoping](#path-scoping)
- [Type system](#type-system)
- [Heading templates](#heading-templates)
- [Schema generation](#schema-generation)
- [Design](#design)

## What it checks

swissshepherd compares a Terraform provider's schema against its markdown documentation and reports:

- **Missing documentation** — schema attributes or blocks with no corresponding doc entry
- **Phantom documentation** — documented attributes that don't exist in the schema
- **Ordering violations** — arguments or attributes not in alphabetical order
- **Description style issues** — descriptions starting with articles or fluff words
- **Misplaced computed attributes** — computed-only attributes in the Argument Reference section
- **Heading style mismatches** — nested block headings not matching the preferred format
- **Format / structure issues** — code blocks inside argument sections, multi-line attribute descriptions, interrupted attribute lists
- **Label violations** — arguments missing (Required)/(Optional) labels (and optionally (Read-Only) when allow_inline_read_only = true), or attributes that have them
- **Byline mismatches** — section introductory paragraph doesn't match expected text
- **Frontmatter problems** — missing required YAML fields, forbidden fields present, disallowed subcategories
- **Title section problems** — missing title, wrong heading level, bad `<Kind>: ` prefix
- **Section presence and order** — required sections missing, forbidden sections present, sections out of canonical order, stray level-2 headings outside the type's spec, timeouts section iff schema has timeouts
- **Timeout mismatches** — documented timeout actions that don't exist in schema, or schema actions not documented
- **Import section issues** — passive voice, missing code blocks, identity section validation
- **Example section issues** — code blocks with disallowed languages, missing resource name
- **Function signature** — missing signature code block, function name or parameters not present
- **Region argument** — region-aware resources that don't document the `region` argument
- **File size** — documentation files exceeding the 500KB Terraform Registry limit
- **File extension** — wrong file extension for the layout (legacy vs registry)
- **Link style** — reference-style link definitions that should be inline
- **File alignment** — missing doc files, orphan doc files, mixed legacy/registry layouts

## Installation

```bash
go install github.com/YakDriver/swissshepherd@latest
```

Requires [Terraform](https://developer.hashicorp.com/terraform/install) in PATH when using `provider_dir` (for automatic schema generation).

## Quick start

Create `.swissshepherd.hcl` in your provider repo root:

```hcl
provider_source = "registry.terraform.io/hashicorp/aws"
provider_dir    = "."
```

Then run:

```bash
swissshepherd
```

That's it. swissshepherd builds the provider, generates the schema, runs all checks, and reports findings.

### Common invocations

```bash
# Check a single target (auto-detects type)
swissshepherd --target aws_iam_role

# Disambiguate when a name exists as both resource and data source
swissshepherd --target aws_instance --type resource

# Check all targets matching a prefix
swissshepherd --prefix aws_dms_

# Check all data sources
swissshepherd --type data_source

# Check a service's resources
swissshepherd --prefix aws_s3_ --type resource
```

## CLI reference

```
swissshepherd [flags]
swissshepherd check [flags]
```

| Flag | Description |
|------|-------------|
| `--config` | Config file path (default: `.swissshepherd.hcl`) |
| `--schema-json` | Path to pre-generated `terraform providers schema -json` output |
| `--provider-source` | Provider source address (e.g., `registry.terraform.io/hashicorp/aws`) |
| `--provider-dir` | Path to provider source directory (triggers automatic build + schema generation) |
| `--target` | Check a single named target |
| `--type` | Restrict to a specific type (`resource`, `data_source`, `ephemeral`, `function`, `list_resource`, `action`) |
| `--prefix` | Check all targets whose name begins with this prefix |
| `--refresh-schema` | Regenerate cached schema even if `schema_json` file exists |
| `--json` | Output results as JSON |
| `--verbose` | Verbose logging (shows enabled checks, scoping, schema stats) |
| `--version` | Print version and exit |

CLI flags override config file values.

### Output format

```
ERROR  [rule_name] resource_name (path/to/file.md:42): message
WARN   [rule_name] resource_name (path/to/file.md): message
```

Exit code 0 = all checks passed. Exit code 1 = one or more errors found.

## Configuration

All configuration lives in a single HCL file. The full reference:

```hcl
# .swissshepherd.hcl

# ─── Provider identity ───────────────────────────────────────────────────────

provider_source = "registry.terraform.io/hashicorp/aws"
provider_dir    = "."

# ─── Schema caching ─────────────────────────────────────────────────────────
# When set, the schema is cached at this path (relative to provider_dir).
# First run builds the provider and writes here; subsequent runs reuse it.
# Use --refresh-schema to regenerate after provider code changes.

schema_json = "terraform-providers-schema/schema.json"

# ─── Global ignore lists ────────────────────────────────────────────────────

# Suppress all schema+doc rule findings for deprecated/removed stubs.
# File rules (frontmatter, file_check) still run.
ignore_contents_check      = ["aws_kms_secret"]
ignore_contents_check_file = "website/ignore-contents-check.txt"

# Map schema names to doc file names when they differ.
# Keys: plain "name" (all types) or "type/name" (scoped).
file_aliases = {
  "list_resource/aws_ebs_volume" = "aws_ec2_ebs_volume"
  "aws_alb"                      = "aws_lb"
}

# ─── Check blocks ───────────────────────────────────────────────────────────
# Each check block configures one rule. All rules are enabled by default.
# Add `enabled = false` to disable. See "Rules reference" for details.
```

### Path resolution

All relative paths in the config (`provider_dir`, `schema_json`, `*_file` options) resolve against the **current working directory** — the same convention as `terraform`, `go`, and `make`. The config file's location has no effect.

In practice: `cd` into the provider repo root and run swissshepherd from there:

```bash
cd terraform-provider-aws
swissshepherd --config .ci/swissshepherd.hcl
```

### List files

Options ending in `_file` (`ignore_targets_file`, `allow_subcategories_file`, `ignore_missing_file`, etc.) read one entry per line. Empty lines and lines starting with `#` are ignored.

## Rules reference

### Overview

| Rule | Kind | Description |
|------|------|-------------|
| `example_section` | per-target | Example code block validation |
| `file_check` | per-file | File size, extension, and link style validation |
| `file_match` | global | File↔schema alignment: missing docs, orphan files, mixed layouts |
| `frontmatter` | per-file | YAML frontmatter field validation |
| `import_section` | per-target | Import section style and structure |
| `region_argument` | per-target | Region argument presence for region-aware types |
| `schema_docs` | per-target | Schema coverage, ordering, description style, heading style, format, labels, deprecation, and bylines |
| `section_presence` | per-target | Section presence, order, and recognition of unknown level-2 headings |
| `signature_section` | per-target | Function signature validation |
| `timeouts_section` | per-target | Timeout actions match schema bidirectionally |
| `title_section` | per-target | Level-1 heading validation |

**Per-target** rules run once per (resource, data source, etc.) and compare schema against parsed markdown. **Per-file** rules run on raw file bytes. **Global** rules run once per invocation.

The two largest rules — `schema_docs` and `section_presence` — have detailed reference docs in [rules/](rules/). The smaller rules are documented inline below.

---

### `example_section`

Validates example code blocks:
- Code block languages are in the allowed list
- Code blocks contain the resource/data source name

```hcl
check "example_section" {
  allow_languages = ["terraform", "hcl"]  # default
}
```

---

### `file_check`

Validates file-level properties.

```hcl
check "file_check" {
  max_file_size             = 500000                                # bytes (default: 500KB registry limit)
  allow_extensions          = [".md", ".html.markdown", ".html.md"] # accepted extensions
  allow_registry_extensions = [".md"]                               # stricter set for docs/ layout
  inline_links              = true                                  # flag reference-style [ref]: url links
}
```

When `inline_links` is enabled, reference-style link definitions (`[label]: url`) are flagged with a suggestion to use inline `[text](url)` style instead.

---

### `file_match`

Validates file↔schema alignment (runs once per invocation):
- **require_doc**: every schema resource must have a documentation file
- **require_schema**: every documentation file must have a matching schema resource
- **mixed_layout**: can't mix legacy (`website/docs/`) and registry (`docs/`) layouts

```hcl
check "file_match" {
  require_doc    = true   # default: true
  require_schema = true   # default: true
  mixed_layout   = true   # default: true

  ignore_missing      = ["aws_alb", "aws_alb_listener"]       # no doc required
  ignore_missing_file = "website/ignore-file-missing.txt"
  ignore_extra        = ["aws_removed_thing"]                  # orphan doc OK
  ignore_extra_file   = "website/ignore-file-mismatch.txt"
}
```

The `ignore_missing` and `ignore_extra` lists suppress findings for specific targets. Use `ignore_missing_file` / `ignore_extra_file` for file-based lists.

---

### `frontmatter`

Validates YAML frontmatter at the top of each doc file. Requirements can come from both the check block (global) and the type block (per-type via `frontmatter_require` / `frontmatter_forbid`). Both sources are merged — a field required by either is enforced.

```hcl
check "frontmatter" {
  # Require fields to be present
  require_subcategory = true
  require_page_title  = true
  require_description = true
  require_layout      = false  # legacy layout only

  # Forbid fields from being present
  forbid_subcategory    = false
  forbid_page_title     = false
  forbid_description    = false
  forbid_layout         = true   # registry layout
  forbid_sidebar_current = true  # always forbidden in modern docs

  # Subcategory allowlist (empty = anything goes)
  allow_subcategories      = ["S3", "VPC", "IAM"]
  allow_subcategories_file = "website/allowed-subcategories.txt"

  # Targets where subcategory: "" is acceptable
  allow_empty_subcategory_targets = ["arn_build", "arn_parse"]
}
```

---

### `import_section`

Validates import section content and structure:
- No passive voice ("can be imported" → use imperative)
- No "e.g." (use "For example" or just show the example)
- Code blocks present with correct types (`terraform` and `console`)
- Correct ordering (terraform block before console block)
- Identity-aware: when resource has identity schema, validates `### Identity Schema` subsection

```hcl
check "import_section" {
  require_identity_section = true  # default: true
}
```

---

### `region_argument`

Validates that region-aware resources document the `region` argument. Only fires for types with `region_aware = true` in their type block (resources, data sources, ephemerals by default).

```hcl
check "region_argument" {
  ignore_resources      = ["aws_global_accelerator"]
  ignore_resources_file = "website/ignore-region.txt"
}
```

---

### `schema_docs`

The primary rule. Validates argument and attribute documentation against the provider schema. Eight sub-checks: `byline`, `coverage`, `deprecated`, `description`, `format`, `heading`, `labels`, `ordering`. Coverage enforces the Required / Optional / Read-Only model at every depth of nesting and supports inline `(Read-Only)` labels via `allow_inline_read_only`.

See **[rules/schema_docs.md](rules/schema_docs.md)** for the full configuration reference, sub-check details, and the schema model.

---

### `section_presence`

Owns the structural integrity of a doc file: presence, order, and recognition of level-2 sections. Configured in two places — the `type` block declares which sections may appear and in what order; the `check "section_presence"` block toggles enforcement.

See **[rules/section_presence.md](rules/section_presence.md)** for the full configuration reference, custom-section conventions, and schema-driven Timeouts behavior.

---

### `signature_section`

Validates function signature documentation:
- Signature code block is present
- Contains the function name
- Contains all schema parameter names

No config options.

---

### `timeouts_section`

Validates that documented timeout actions match the schema bidirectionally:
- Every schema timeout action must be documented
- Every documented timeout action must exist in the schema

No config options.

---

### `title_section`

Validates the level-1 heading (`# Resource: aws_thing`).

Checks:
- Heading exists and is at level 1
- Begins with an allowed `<Kind>: ` prefix (from `allow_prefixes` or the type's `title_prefix`)
- No code blocks appear above the first `##` heading

```hcl
check "title_section" {
  allow_prefixes = ["Resource", "Data Source", "Ephemeral", "Function"]
}
```

## Path scoping

Every `check` block accepts path-scoping fields that control which targets the rule runs against. This is the primary mechanism for incremental rollout across a large provider.

```hcl
check "schema_docs" {
  # Type axis: only these type names (empty = all)
  types = ["resource", "data_source"]

  # Name axis (OR'd together): prefix match OR exact match
  prefixes = ["aws_s3", "aws_appflow"]
  targets  = ["aws_instance"]

  # Deny list: always excluded, even when allowlists match
  ignore_targets      = ["aws_legacy_resource"]
  ignore_targets_file = "website/ignore-ordering.txt"
  ignore_prefixes     = ["aws_appstream"]
}
```

All four name-axis fields (`prefixes`, `targets`, `ignore_targets`, `ignore_prefixes`) support **type/name notation** for type-scoped entries:

```hcl
check "schema_docs" {
  # Only match aws_thing when it's a data source, not a resource
  ignore_targets = ["data_source/aws_thing"]

  # Only match aws_s3_ prefixed data sources
  prefixes = ["data_source/aws_s3_"]
}
```

Plain entries (no `/`) match any type. Qualified entries (`type/name` or `type/prefix`) only match when the type matches.

**Evaluation order:**

1. `ignore_targets` / `ignore_prefixes` — if matched, target is excluded unconditionally
2. `types` — if non-empty, target's type must be in the list
3. `prefixes` / `targets` — if either is non-empty, target must match at least one; both empty = all names pass

## Type system

swissshepherd ships with built-in type definitions for all standard Terraform documentation categories. Override any default or add new types in your config:

### Built-in types

| Type | Schema kind | Default doc path | Region-aware |
|------|-------------|------------------|--------------|
| `resource` | `resource` | `website/docs/r/{name}.html.markdown` | yes |
| `data_source` | `data_source` | `website/docs/d/{name}.html.markdown` | yes |
| `ephemeral` | `ephemeral` | `website/docs/ephemeral-resources/{name}.html.markdown` | yes |
| `function` | `function` | `website/docs/functions/{name}.html.markdown` | no |
| `list_resource` | `list_resource` | `website/docs/list-resources/{name}.html.markdown` | yes |
| `action` | `action` | `website/docs/actions/{name}.html.markdown` | no |
| `guide` | `none` | `website/docs/guides/{name}.html.markdown` | no |
| `index` | `none` | `website/docs/index.html.markdown` | no |

### Overriding a type

```hcl
type "resource" {
  schema_kind   = "resource"
  website_paths = [
    "docs/resources/{name}.md",
    "website/docs/r/{name}.html.markdown",
  ]
  title_prefix = "Resource"
  region_aware = true

  section "title"      { required = true }
  section "example"    { required = true }
  section "arguments"  { required = true }
  section "attributes" { required = true }
  section "timeouts"   {}
  section "import"     {}
  section "signature"  { forbidden = true }

  frontmatter_require = ["description", "page_title"]
  frontmatter_forbid  = ["sidebar_current"]

  arguments_bylines = [
    "This resource supports the following arguments:",
    "The following arguments are required:",
    "The following arguments are optional:",
  ]
  attributes_bylines = [
    "This resource exports the following attributes in addition to the arguments above:",
  ]
}
```

### Type fields

| Field | Description |
|-------|-------------|
| `schema_kind` | Schema accessor: `resource`, `data_source`, `ephemeral`, `function`, `list_resource`, `action`, `none` |
| `website_paths` | Ordered list of path templates. `{name}` = provider-prefix-stripped target name |
| `title_prefix` | Expected `# <Prefix>: ` in the level-1 heading |
| `arguments_heading` | Override the expected heading text (default: `"Argument Reference"`) |
| `arguments_bylines` | Allowed paragraph texts under `## Argument Reference` |
| `attributes_bylines` | Allowed paragraph texts under `## Attribute Reference` |
| `allow_missing_arguments_byline` | Don't require a byline paragraph |
| `section "<name>" { ... }` | Declares a section in the doc structure. Order is the canonical doc order. Block fields: `required`, `forbidden`. See [`section_presence`](rules/section_presence.md) for details. |
| `frontmatter_require` | Fields that must be present in YAML frontmatter |
| `frontmatter_forbid` | Fields that must be absent |
| `region_aware` | Whether `region_argument` rule applies to this type |

## Heading templates

The `block_heading_styles` list controls which `###` heading formats are recognized as block documentation within argument/attribute sections.

### Placeholders

| Placeholder | Matches | Example | Extracted name |
|-------------|---------|---------|----------------|
| `{Block}` | snake_case identifier | `network_interface` | `network_interface` |
| `{Title}` | Title Case words (→ snake_case) | `Network Interface` | `network_interface` |

### Examples

| Template | Matches heading | Extracted |
|----------|-----------------|-----------|
| `` `{Block}` Block `` | `` `network` Block `` | `network` |
| `{Block} Configuration Block` | `vpc_config Configuration Block` | `vpc_config` |
| `{Block} Argument Reference` | `filter Argument Reference` | `filter` |
| `{Title}` | `Credit Specification` | `credit_specification` |

Note: goldmark strips backticks from inline code in headings, so `` ### `network` Block `` becomes `network Block` in parsed text.

### Preferred style

Set `prefer_block_heading_styles` to emit warnings when a heading matches an accepted style but not the preferred one:

```hcl
check "schema_docs" {
  block_heading_styles        = ["`{Block}` Block", "{Block} Block", "{Block}", "{Title}"]
  prefer_block_heading_styles = ["`{Block}` Block"]
}
```

## Schema generation

When `provider_dir` is set, swissshepherd automatically:

1. Builds the provider with `go build`
2. Creates a temporary Terraform plugin directory
3. Runs `terraform init` + `terraform providers schema -json`
4. Uses the generated schema for all checks
5. Cleans up temporary files

### Caching

For large providers where the build takes minutes, set `schema_json` to cache:

```hcl
provider_dir = "."
schema_json  = "terraform-providers-schema/schema.json"
```

First run builds and caches. Subsequent runs reuse the file instantly. Regenerate with `--refresh-schema` or delete the cached file.

### Pre-generated schema

Skip the build entirely by providing a pre-generated schema:

```bash
swissshepherd --schema-json path/to/schema.json --provider-source registry.terraform.io/hashicorp/aws
```

## Design

- **Schema is the source of truth** — derived from `terraform providers schema -json`, not Go source parsing
- **Markdown parsed with goldmark** — full AST, not regex
- **Two rule interfaces**: `Rule` (schema + AST) and `FileRule` (raw bytes). The runner reads each file once and feeds it to both
- **Type-driven architecture** — documentation categories are config, not code. Adding a new Terraform category is a config change
- **Per-check path scoping** — each rule rolls out independently via `types`, `prefixes`, `targets`, `ignore_targets`, `ignore_prefixes`
- **Config-driven** — HCL configuration via [hcl/v2](https://github.com/hashicorp/hcl); no magic values
- **Global checks** run once per invocation (directory layout, file mismatch)
- **YAML frontmatter** parsed with [yaml.v3](https://gopkg.in/yaml.v3)

## License

MPL-2.0
