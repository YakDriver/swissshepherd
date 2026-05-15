# Default type definitions shipped with swissshepherd.
#
# These provide minimal defaults. Providers should override types in their
# own swissshepherd.hcl to define bylines, frontmatter requirements, and
# other provider-specific conventions.

type "resource" {
  schema_kind   = "resource"
  website_paths = ["website/docs/r/{name}.html.markdown"]
  title_prefix  = "Resource"

  require_attributes = "required"
  require_import     = "optional"
  require_timeouts   = "optional"
  require_signature  = "forbidden"

  region_aware = true
}

type "data_source" {
  schema_kind   = "data_source"
  website_paths = ["website/docs/d/{name}.html.markdown"]
  title_prefix  = "Data Source"

  require_attributes = "required"
  require_import     = "forbidden"
  require_timeouts   = "optional"
  require_signature  = "forbidden"

  region_aware = true
}

type "ephemeral" {
  schema_kind   = "ephemeral"
  website_paths = ["website/docs/ephemeral-resources/{name}.html.markdown"]
  title_prefix  = "Ephemeral"

  require_attributes = "required"
  require_import     = "forbidden"
  require_timeouts   = "forbidden"
  require_signature  = "forbidden"

  region_aware = true
}

type "function" {
  schema_kind   = "function"
  website_paths = ["website/docs/functions/{name}.html.markdown"]
  title_prefix  = "Function"

  arguments_heading              = "Arguments"
  allow_missing_arguments_byline = true

  require_attributes = "forbidden"
  require_import     = "forbidden"
  require_timeouts   = "forbidden"
  require_signature  = "required"

  region_aware = false
}

type "list_resource" {
  schema_kind   = "list_resource"
  website_paths = ["website/docs/list-resources/{name}.html.markdown"]
  title_prefix  = "List Resource"

  require_attributes = "forbidden"
  require_import     = "forbidden"
  require_timeouts   = "forbidden"
  require_signature  = "forbidden"

  region_aware = true
}

type "action" {
  schema_kind   = "action"
  website_paths = ["website/docs/actions/{name}.html.markdown"]
  title_prefix  = "Action"

  require_attributes = "forbidden"
  require_import     = "forbidden"
  require_timeouts   = "forbidden"
  require_signature  = "forbidden"

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
