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
- **Title section problems** — missing title, wrong heading level, bad `<Kind>: ` prefix, code blocks misplaced above the first `##` heading

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
```

Then run:

```bash
# Check everything
swissshepherd

# Check a single target (auto-detects type)
swissshepherd --target aws_iam_role

# Disambiguate when a name exists as both resource and data source
swissshepherd --target aws_instance --type resource

# Check all targets matching a prefix
swissshepherd --prefix aws_dms_

# Check all data sources for a service
swissshepherd --prefix aws_s3_ --type data_source
```

That's it. swissshepherd builds the provider, generates the schema via Terraform, runs the checks, and cleans up.

### A note on paths

Relative paths in the config file (`provider_dir`, `schema_json`, and every `*_file` option) are interpreted relative to the **current working directory** at the time swissshepherd runs — the same convention as `terraform`, `docker`, `go`, and `make`. The location of the config file itself has no effect on path resolution.

In practice: `cd` into the provider repo root and invoke swissshepherd from there. This works whether the config sits at the root or in a subdirectory like `.ci/`:

```bash
# Config at repo root
cd terraform-provider-foo
swissshepherd

# Config in .ci/, paths still described relative to repo root
cd terraform-provider-foo
swissshepherd --config .ci/swissshepherd.hcl
```

## Usage

```bash
# With explicit config file
swissshepherd --config path/to/swissshepherd.hcl

# With CLI flags (no config file needed)
swissshepherd --provider-dir . --provider-source registry.terraform.io/hashicorp/aws

# With pre-generated schema (skips build)
swissshepherd --schema-json schema.json --provider-source registry.terraform.io/hashicorp/aws

# Regenerate cached schema (when provider code changes)
swissshepherd --refresh-schema

# JSON output
swissshepherd --target aws_vpc --json
```

CLI flags override config file values.

### Schema caching

For large providers (e.g. AWS), building the schema takes minutes. Set `schema_json` to a relative path in your config to cache it:

```hcl
provider_dir    = "."
schema_json     = "terraform-providers-schema/schema.json"
```

On first run, swissshepherd builds the provider and writes the schema to that path. Subsequent runs reuse the cached file instantly. Add the directory to `.gitignore`.

When the provider code changes, regenerate with `--refresh-schema` or simply delete the cached file.

## Configuration

```hcl
# .swissshepherd.hcl

provider_source = "registry.terraform.io/hashicorp/aws"
provider_dir    = "."

# Schema caching: relative paths resolve against provider_dir.
# If the file exists, it's used directly. If missing (or --refresh-schema),
# the provider is built and the schema is generated and cached here.
schema_json = "terraform-providers-schema/schema.json"

# Suppress "doc file not found" warnings for schema aliases that have no doc.
# ignore_file_missing = ["aws_alb", "aws_alb_listener"]
# ignore_file_missing_file = "website/ignore-file-missing.txt"

# Suppress all schema+doc rule findings for deprecated/removed resource stubs.
# File rules (frontmatter) still run.
# ignore_contents_check = ["aws_kms_secret", "aws_db_security_group"]
# ignore_contents_check_file = "website/ignore-contents-check.txt"

# Map schema names to doc names when they differ (doc is still checked).
# Keys can be "name" (all types) or "type/name" (scoped to one type).
# file_aliases = {
#   "list_resource/aws_ebs_volume" = "aws_ec2_ebs_volume"
# }

check "schema_docs" {
  enabled = true

  # Sub-check toggles (all default true; set false to disable individually)
  coverage    = true   # every schema attr documented, every documented attr in schema
  ordering    = true   # alphabetical within required/optional groups
  description = true   # descriptions don't start with bad prefixes
  heading     = true   # block headings match preferred style
  format      = true   # no code blocks, single-line attrs, uninterrupted lists
  labels      = true   # arguments have (Required)/(Optional), attributes do not

  # Path scoping — limit this check to specific targets (see Path scoping below)
  types    = ["resource", "data_source"]
  prefixes = ["aws_s3", "aws_appflow"]

  block_heading_styles = [
    "`{Block}` Block",
    "{Block} Block",
    "`{Block}`",
    "{Block}",
    "{Title}",
  ]
  preferred_block_heading_styles = ["`{Block}` Block"]

  # Coverage options (all have sensible defaults)
  ignore_deprecated   = true
  implicit_attributes = ["id", "tags_all"]   # never flagged as undocumented
  phantom_allowlist   = ["tags", "tags_all"] # never flagged as phantom
  skip_blocks         = ["timeouts"]         # blocks skipped entirely

  # Description options
  # bad_prefixes = ["A ", "An ", "The ", "Specifies ", "Indicates ", "Describes ", "Defines "]

  # Format options
  # no_code_blocks       = true
  # single_line_attrs    = true
  # uninterrupted_lists  = true
}

check "frontmatter" {
  enabled = true

  require_subcategory    = true
  require_page_title     = true
  require_description    = true
  forbid_layout          = true
  forbid_sidebar_current = true

  allowed_subcategories_file = "website/allowed-subcategories.txt"

  # Targets listed here may have subcategory: "" without failing the allowlist.
  # Useful for function docs that carry no subcategory by convention.
  # allow_empty_subcategory_targets = ["arn_build", "arn_parse"]
}

check "title_section" {
  enabled = true
  # allowed_prefixes = ["Resource", "Data Source", "Ephemeral", "Function", "List Resource", "Action"]
}

check "section_presence" {
  enabled = true
  # No config — reads require_import/require_timeouts/require_signature/require_attributes from the type block.
  # Timeouts are schema-driven: section required iff schema has a timeouts block.
}

check "timeouts_section" {
  enabled = true
  # No config — validates documented timeout actions match schema bidirectionally.
}

check "import_section" {
  enabled = true
  require_identity_section = true  # validate identity import block + ### Identity Schema subsection (default: true)
}

check "example_section" {
  enabled = true
  allowed_languages = ["terraform", "hcl"]         # code block languages allowed (default: terraform, hcl)
}

check "signature_section" {
  enabled = true
  # No config — validates function signature code block contains function name and parameters.
}
```

All rules are enabled by default. Add a `check` block with `enabled = false` to disable one.

## Path scoping

Every `check` block accepts path-scoping fields that limit which targets the rule runs against. This is the primary mechanism for rolling out checks incrementally across a large provider — one service or one type at a time.

```hcl
check "schema_docs" {
  enabled = true

  # Type axis: only run against these type names (empty = all types)
  types = ["resource", "data_source"]

  # Name axis: run against targets whose name has one of these prefixes
  # OR whose name is listed exactly in targets. Both lists are OR'd.
  prefixes = ["aws_s3", "aws_appflow"]
  targets  = ["aws_instance", "aws_accessanalyzer_archive_rule"]

  # Deny list: always excluded, even when allowlists include them.
  ignored_targets      = ["aws_legacy_resource"]
  ignored_targets_file = "website/ignore-ordering.txt"
  ignored_prefixes     = ["aws_appstream"]
}
```

**Semantics:**

1. `ignored_targets` and `ignored_prefixes` win unconditionally — a matching name is never checked.
2. When `types` is non-empty, the target's type must be in the list.
3. When either `prefixes` or `targets` is non-empty, the target's name must satisfy at least one (prefix match OR exact match). Both empty means "any name".

**Use cases:**

```bash
# PR gate: check one specific resource
swissshepherd --target aws_s3_bucket --type resource

# Service gate: check all S3 resources
swissshepherd --prefix aws_s3_ --type resource

# Full CI: check everything
swissshepherd
```

## Type blocks

swissshepherd ships with built-in definitions for every Terraform documentation category. You can override a default or add a new one:

```hcl
# Override the resource type to use a custom doc layout
type "resource" {
  schema_kind   = "resource"
  website_paths = [
    "docs/resources/{name}.md",
    "website/docs/r/{name}.html.markdown",
  ]
  title_prefix = "Resource"
  # ... other fields
}

# Add a provider-specific category
type "widget" {
  schema_kind   = "none"   # no schema backing; content-only
  website_paths = ["docs/widgets/{name}.md"]
  title_prefix  = "Widget"
}
```

The `{name}` placeholder in `website_paths` is replaced with the provider-prefix-stripped target name (e.g., `aws_instance` → `instance`). Templates are tried in order; the first existing file wins.

Built-in types: `resource`, `data_source`, `ephemeral`, `function`, `list_resource`, `action`, `guide`, `index`.

## Heading styles

The `block_heading_styles` list in the `schema_docs` check controls which `###` heading formats are recognized as block documentation. Each entry is a template with placeholders:

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
| `schema_docs` | schema + doc | Schema coverage, ordering, description style, heading style, format, and labels for argument/attribute sections |
| `title_section` | schema + doc | Level-1 heading is present, at level 1, begins with one of the allowed `<Kind>: ` prefixes, and contains no code blocks |
| `section_presence` | schema + doc | Required sections are present; forbidden sections are absent. Schema-driven for timeouts (if-and-only-if), type-level fallback otherwise |
| `timeouts_section` | schema + doc | Documented timeout actions match the schema (bidirectional) |
| `import_section` | schema + doc | Import section style (no passive voice, no "e.g."), structure (code blocks present, correct types, ordering), and identity-aware validation |
| `example_section` | schema + doc | Example code blocks use allowed languages and contain the resource name |
| `signature_section` | schema + doc | Function signature code block present, contains function name and schema parameter names |
| `frontmatter` | raw file | YAML frontmatter field presence/absence and subcategory allowlist |

Rules fall into two categories: `schema + doc` rules compare the parsed markdown AST against the schema (including type-level requirements); `raw file` rules operate on the file bytes so they can catch whitespace, line-structure, and frontmatter issues the AST normalizes away.

### Section headings

Section headings are currently hardcoded. The parser recognizes these level-2 headings:

| Section | Expected heading | Used by |
|---------|-----------------|---------|
| Example | `Example Usage` | `section_presence`, `example_section` |
| Arguments | `Argument Reference` | `section_presence`, `schema_docs` |
| Attributes | `Attribute Reference` | `section_presence`, `schema_docs` |
| Timeouts | `Timeouts` | `section_presence`, `timeouts_section` |
| Import | `Import` | `section_presence`, `import_section` |
| Signature | `Signature` | `section_presence` |

These may become configurable in a future release if providers need custom heading text.

### Output format

Each finding includes the rule name, target, file path (with line number when available), and message:

```
ERROR  [section_presence] aws_vpn_gateway (website/docs/d/vpn_gateway.html.markdown): section ## Timeouts is not allowed for type "data_source"
WARN   [schema_docs] aws_instance (website/docs/r/instance.html.markdown:42): attribute "mode" in block "spec.tls" is documented but missing the " - " separator
```

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

The subcategory allowlist is set inside the `check "frontmatter"` block via `allowed_subcategories` (inline list) or `allowed_subcategories_file` (newline-separated file). When the allowlist is non-empty, a frontmatter `subcategory` value not on the list fails. An empty allowlist is equivalent to "anything goes". The allowlist only fires when `subcategory` is present in the file — use `require_subcategory` alongside it if absence should also fail.

Some doc types (e.g. Terraform provider functions) use `subcategory: ""` to signal "no category" rather than omitting the key. By default that empty string is validated against the allowlist and fails. Use `allow_empty_subcategory_targets` to name the specific targets where an empty subcategory is permitted:

```hcl
check "frontmatter" {
  allow_empty_subcategory_targets = ["arn_build", "arn_parse", "trim_iam_role_path", "user_agent"]
}
```

All other targets with `subcategory: ""` continue to fail.

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
- Documentation categories (resource, data source, ephemeral, function, list resource, action, guide, index) are defined in `type` blocks — either the embedded defaults or user overrides. Adding a new Terraform category is a config change, not a code change
- Doc file paths resolve from `type.website_paths` templates; `provider_dir` anchors relative paths when set, otherwise CWD
- Per-check path scoping (`types`, `prefixes`, `targets`, `ignored_targets`, `ignored_prefixes`) lets each rule roll out independently across a large provider
- HCL configuration via [hcl/v2](https://github.com/hashicorp/hcl)
- YAML frontmatter parsed with [yaml.v3](https://gopkg.in/yaml.v3)

## License

MPL-2.0
