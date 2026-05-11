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
