# Default type definitions shipped with swissshepherd.
#
# These provide minimal defaults. Providers should override types in their
# own swissshepherd.hcl to define bylines, frontmatter requirements, and
# other provider-specific conventions.
#
# The order of section blocks within a type IS the canonical doc order.
# section_presence enforces this order when its enforce_order option is true.

type "resource" {
  schema_kind   = "resource"
  website_paths = ["website/docs/r/{name}.html.markdown"]
  title_prefix  = "Resource"

  section "title"      { required = true }
  section "example"    { required = true }
  section "arguments"  { required = true }
  section "attributes" { required = true }
  section "timeouts"   {}
  section "import"     {}
  section "signature"  { forbidden = true }

  region_aware = true
}

type "data_source" {
  schema_kind   = "data_source"
  website_paths = ["website/docs/d/{name}.html.markdown"]
  title_prefix  = "Data Source"

  section "title"      { required = true }
  section "example"    { required = true }
  section "arguments"  { required = true }
  section "attributes" { required = true }
  section "timeouts"   {}
  section "import"     { forbidden = true }
  section "signature"  { forbidden = true }

  region_aware = true
}

type "ephemeral" {
  schema_kind   = "ephemeral"
  website_paths = ["website/docs/ephemeral-resources/{name}.html.markdown"]
  title_prefix  = "Ephemeral"

  section "title"      { required = true }
  section "example"    { required = true }
  section "arguments"  { required = true }
  section "attributes" { required = true }
  section "timeouts"   { forbidden = true }
  section "import"     { forbidden = true }
  section "signature"  { forbidden = true }

  region_aware = true
}

type "function" {
  schema_kind   = "function"
  website_paths = ["website/docs/functions/{name}.html.markdown"]
  title_prefix  = "Function"

  arguments_heading              = "Arguments"
  allow_missing_arguments_byline = true

  section "title"      { required = true }
  section "example"    { required = true }
  section "signature"  { required = true }
  section "arguments"  { required = true }
  section "attributes" { forbidden = true }
  section "timeouts"   { forbidden = true }
  section "import"     { forbidden = true }

  region_aware = false
}

type "list_resource" {
  schema_kind   = "list_resource"
  website_paths = ["website/docs/list-resources/{name}.html.markdown"]
  title_prefix  = "List Resource"

  section "title"      { required = true }
  section "example"    { required = true }
  section "arguments"  { required = true }
  section "attributes" { forbidden = true }
  section "timeouts"   { forbidden = true }
  section "import"     { forbidden = true }
  section "signature"  { forbidden = true }

  region_aware = true
}

type "action" {
  schema_kind   = "action"
  website_paths = ["website/docs/actions/{name}.html.markdown"]
  title_prefix  = "Action"

  section "title"      { required = true }
  section "example"    { required = true }
  section "arguments"  { required = true }
  section "attributes" { forbidden = true }
  section "timeouts"   { forbidden = true }
  section "import"     { forbidden = true }
  section "signature"  { forbidden = true }

  region_aware = false
}

type "guide" {
  schema_kind   = "none"
  website_paths = ["website/docs/guides/{name}.html.markdown"]

  region_aware = false
}

type "index" {
  schema_kind   = "none"
  website_paths = ["website/docs/index.html.markdown"]

  region_aware = false
}
