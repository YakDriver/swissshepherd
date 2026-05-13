# Default type definitions shipped with swissshepherd.
#
# Each block captures everything swissshepherd needs to know about one
# category of provider documentation except the check logic itself: where
# docs live, where the schema lives, and what conventions rules should
# apply. Providers customize by declaring a `type "<name>" { ... }` block
# with the same name in their own config; that override replaces the
# default wholesale.
#
# Adding a new Terraform category in a future release is a two-line change:
# add a block here and register its schema accessor in internal/schema.

type "resource" {
  schema_kind = "resource"
  website_paths = [
    "docs/resources/{name}.md",
    "website/docs/r/{name}.html.markdown",
  ]

  title_prefix = "Resource"

  arguments_bylines = [
    "This resource supports the following arguments:",
    "The following arguments are required:",
    "The following arguments are optional:",
    "This resource does not support any arguments.",
  ]
  attributes_bylines = [
    "This resource exports the following attributes in addition to the arguments above:",
    "This resource exports no additional attributes.",
  ]

  require_attributes = "required"
  require_import     = "optional"
  require_timeouts   = "optional"
  require_signature  = "forbidden"

  frontmatter_require = ["description", "page_title"]
  frontmatter_forbid  = ["sidebar_current"]

  region_aware = true
}

type "data_source" {
  schema_kind = "data_source"
  website_paths = [
    "docs/data-sources/{name}.md",
    "website/docs/d/{name}.html.markdown",
  ]

  title_prefix = "Data Source"

  arguments_bylines = [
    "This data source supports the following arguments:",
    "The following arguments are required:",
    "The following arguments are optional:",
    "This data source does not support any arguments.",
  ]
  attributes_bylines = [
    "This data source exports the following attributes in addition to the arguments above:",
    "This data source exports no additional attributes.",
  ]

  require_attributes = "required"
  require_import     = "forbidden"
  require_timeouts   = "optional"
  require_signature  = "forbidden"

  frontmatter_require = ["description", "page_title"]
  frontmatter_forbid  = ["sidebar_current"]

  region_aware = true
}

type "ephemeral" {
  schema_kind = "ephemeral"
  website_paths = [
    "docs/ephemeral-resources/{name}.md",
    "website/docs/ephemeral-resources/{name}.html.markdown",
  ]

  title_prefix = "Ephemeral"

  arguments_bylines = [
    "This ephemeral resource supports the following arguments:",
    "The following arguments are required:",
    "The following arguments are optional:",
    "This ephemeral resource does not support any arguments.",
  ]
  attributes_bylines = [
    "This ephemeral resource exports the following attributes in addition to the arguments above:",
    "This ephemeral resource exports no additional attributes.",
  ]

  require_attributes = "required"
  require_import     = "forbidden"
  require_timeouts   = "forbidden"
  require_signature  = "forbidden"

  frontmatter_require = ["description", "page_title"]
  frontmatter_forbid  = ["sidebar_current"]

  region_aware = true
}

type "function" {
  schema_kind = "function"
  website_paths = [
    "docs/functions/{name}.md",
    "website/docs/functions/{name}.html.markdown",
  ]

  title_prefix = "Function"

  arguments_heading                = "Arguments"
  allow_missing_arguments_byline   = true
  arguments_bylines = [
    "This function supports the following arguments:",
    "This function does not support any arguments.",
  ]

  require_attributes = "forbidden"
  require_import     = "forbidden"
  require_timeouts   = "forbidden"
  require_signature  = "required"

  frontmatter_require = ["description", "page_title"]
  frontmatter_forbid  = ["sidebar_current"]

  region_aware = false
}

type "list_resource" {
  schema_kind = "list_resource"
  website_paths = [
    "docs/list-resources/{name}.md",
    "website/docs/list-resources/{name}.html.markdown",
  ]

  title_prefix = "List Resource"

  arguments_bylines = [
    "This list resource supports the following arguments:",
    "The following arguments are required:",
    "The following arguments are optional:",
    "This list resource does not support any arguments.",
  ]

  require_attributes = "forbidden"
  require_import     = "forbidden"
  require_timeouts   = "forbidden"
  require_signature  = "forbidden"

  frontmatter_require = ["description", "page_title"]
  frontmatter_forbid  = ["sidebar_current"]

  region_aware = true
}

type "action" {
  schema_kind = "action"
  website_paths = [
    "docs/actions/{name}.md",
    "website/docs/actions/{name}.html.markdown",
  ]

  title_prefix = "Action"

  arguments_bylines = [
    "This action supports the following arguments:",
    "The following arguments are required:",
    "The following arguments are optional:",
    "This action does not support any arguments.",
  ]

  require_attributes = "forbidden"
  require_import     = "forbidden"
  require_timeouts   = "forbidden"
  require_signature  = "forbidden"

  frontmatter_require = ["description", "page_title", "subcategory"]
  frontmatter_forbid  = ["sidebar_current"]

  region_aware = false
}

type "guide" {
  schema_kind = "none"
  website_paths = [
    "docs/guides/{name}.md",
    "website/docs/guides/{name}.html.markdown",
  ]

  frontmatter_require = ["description", "page_title"]
  frontmatter_forbid  = ["sidebar_current"]

  region_aware = false
}

type "index" {
  schema_kind = "none"
  website_paths = [
    "docs/index.md",
    "website/docs/index.html.markdown",
  ]

  frontmatter_require = ["description", "page_title"]
  frontmatter_forbid  = ["sidebar_current", "subcategory"]

  region_aware = false
}
