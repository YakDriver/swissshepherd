# swissshepherd

A documentation checker for Terraform providers. Validates that provider documentation accurately reflects the provider schema.

## What it does

swissshepherd compares a Terraform provider's JSON schema (from `terraform providers schema -json`) against its markdown documentation and reports:

- **Missing documentation** — schema attributes or blocks with no corresponding doc entry
- **Phantom documentation** — documented attributes that don't exist in the schema
- **Ordering violations** — arguments or attributes not in alphabetical order
- **Description style issues** — descriptions starting with articles (A, An, The) or fluff words (Specifies, Indicates, Describes, Defines)
- **Misplaced computed attributes** — computed-only attributes documented in Argument Reference instead of Attribute Reference

## Installation

```bash
go install github.com/YakDriver/swissshepherd@latest
```

## Usage

### Check a single resource

```bash
swissshepherd check \
  --schema-json terraform-providers-schema/schema.json \
  --docs-path website/docs \
  --provider-source registry.terraform.io/hashicorp/aws \
  --resource aws_iam_role
```

### Check all resources

```bash
swissshepherd check \
  --schema-json terraform-providers-schema/schema.json \
  --docs-path website/docs \
  --provider-source registry.terraform.io/hashicorp/aws
```

### JSON output

```bash
swissshepherd check --resource aws_vpc --json
```

## Configuration

Create a `.swissshepherd.hcl` file in your project root to avoid repeating CLI flags:

```hcl
provider_source = "registry.terraform.io/hashicorp/aws"
schema_json     = "terraform-providers-schema/schema.json"
docs_path       = "website/docs"

check "completeness" {
  enabled = true

  ignore_resources   = ["aws_legacy_resource"]
  ignore_data_sources = ["aws_kms_secrets"]
}
```

Then run:

```bash
swissshepherd check
```

CLI flags override config file values.

## Generating the schema JSON

```bash
# Build the provider
go build -o terraform-provider-aws .

# Set up Terraform to use the local binary
mkdir -p terraform-plugin-dir/registry.terraform.io/hashicorp/aws/99.99.99/$(go env GOOS)_$(go env GOARCH)
cp terraform-provider-aws terraform-plugin-dir/registry.terraform.io/hashicorp/aws/99.99.99/$(go env GOOS)_$(go env GOARCH)/

# Generate schema
echo 'data "aws_partition" "example" {}' > example.tf
terraform init -plugin-dir terraform-plugin-dir
terraform providers schema -json > schema.json

# Clean up
rm -rf terraform-plugin-dir example.tf .terraform .terraform.lock.hcl terraform-provider-aws
```

## Rules

| Rule | Description |
|------|-------------|
| `completeness` | Every schema attribute is documented; every documented attribute exists in schema |
| `ordering` | Arguments and attributes are alphabetically ordered within required/optional groups |
| `description_style` | Descriptions don't start with weak prefixes (A, An, The, Specifies, Indicates, Describes, Defines) |
| `computed_attribute` | Computed-only attributes appear in Attribute Reference, not Argument Reference |

## Design

- Uses `terraform providers schema -json` as the source of truth — no Go source parsing
- Parses markdown with [goldmark](https://github.com/yuin/goldmark) AST
- Each rule implements a simple interface: `Check(resource, schema, doc) []Result`
- Supports both registry-style (`docs/resources/`) and legacy (`website/docs/r/`) doc layouts
- HCL configuration via [hcl/v2](https://github.com/hashicorp/hcl)

## License

MPL-2.0
