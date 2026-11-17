# Copyright IBM Corp. 2019, 2026
# SPDX-License-Identifier: MPL-2.0

# Default type definitions for doc_style = "tfplugindocs": the pages that
# terraform-plugin-docs generates from its default templates. Titles are
# "<name> (<Kind>)" ("function: <name>" for functions), every schema field is
# documented in one "## Schema" section, and timeouts are a nested block in
# that section rather than a "## Timeouts" section, so no type declares a
# timeouts or attributes section. See docs/rules/doc-style.md.
#
# The order of section blocks within a type IS the canonical doc order.

type "resource" {
  schema_kind   = "resource"
  website_paths = [
    "docs/resources/{name}.md",
    "website/docs/r/{name}.html.markdown",
  ]
  title_format      = "{name} (Resource)"
  arguments_heading = "Schema"

  section "title"     { required = true }
  section "example"   { required = true }
  section "arguments" { required = true }
  section "import"    {}
  section "signature" { forbidden = true }
}

type "data_source" {
  schema_kind   = "data_source"
  website_paths = [
    "docs/data-sources/{name}.md",
    "website/docs/d/{name}.html.markdown",
  ]
  title_format      = "{name} (Data Source)"
  arguments_heading = "Schema"

  section "title"     { required = true }
  section "example"   { required = true }
  section "arguments" { required = true }
  section "import"    { forbidden = true }
  section "signature" { forbidden = true }
}

type "ephemeral" {
  schema_kind   = "ephemeral"
  website_paths = [
    "docs/ephemeral-resources/{name}.md",
    "website/docs/ephemeral-resources/{name}.html.markdown",
  ]
  title_format      = "{name} (Ephemeral Resource)"
  arguments_heading = "Schema"

  section "title"     { required = true }
  section "example"   { required = true }
  section "arguments" { required = true }
  section "import"    { forbidden = true }
  section "signature" { forbidden = true }
}

type "function" {
  schema_kind   = "function"
  website_paths = [
    "docs/functions/{name}.md",
    "website/docs/functions/{name}.html.markdown",
  ]
  title_format      = "function: {name}"
  arguments_heading = "Arguments"

  section "title"     { required = true }
  section "example"   { required = true }
  section "signature" { required = true }
  section "arguments" { required = true }
  section "import"    { forbidden = true }
}

type "list_resource" {
  schema_kind   = "list_resource"
  website_paths = [
    "docs/list-resources/{name}.md",
    "website/docs/list-resources/{name}.html.markdown",
  ]
  title_format      = "{name} (List Resource)"
  arguments_heading = "Schema"

  section "title"     { required = true }
  section "example"   { required = true }
  section "arguments" { required = true }
  section "import"    { forbidden = true }
  section "signature" { forbidden = true }
}

type "action" {
  schema_kind   = "action"
  website_paths = [
    "docs/actions/{name}.md",
    "website/docs/actions/{name}.html.markdown",
  ]
  title_format      = "{name} (Action)"
  arguments_heading = "Schema"

  section "title"     { required = true }
  section "example"   { required = true }
  section "arguments" { required = true }
  section "import"    { forbidden = true }
  section "signature" { forbidden = true }
}

type "guide" {
  schema_kind   = "none"
  website_paths = [
    "docs/guides/{name}.md",
    "website/docs/guides/{name}.html.markdown",
  ]
}

type "index" {
  schema_kind   = "none"
  website_paths = [
    "docs/index.md",
    "website/docs/index.html.markdown",
  ]
}
