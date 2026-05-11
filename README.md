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

| Rule | Description |
|------|-------------|
| `completeness` | Every schema attribute is documented; every documented attribute exists in schema |
| `ordering` | Arguments and attributes are alphabetically ordered within required/optional groups |
| `description_style` | Descriptions don't start with weak prefixes (A, An, The, Specifies, Indicates, Describes, Defines) |
| `computed_attribute` | Computed-only attributes appear in Attribute Reference, not Argument Reference |

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
- Each rule implements `Check(resource, schema, doc) []Result`
- Supports both registry-style (`docs/resources/`) and legacy (`website/docs/r/`) doc layouts
- HCL configuration via [hcl/v2](https://github.com/hashicorp/hcl)

## License

MPL-2.0
