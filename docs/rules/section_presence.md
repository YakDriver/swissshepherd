# `section_presence` rule
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

Owns the structural integrity of a doc file: presence, order, and recognition of level-2 sections. Configuration comes from two places — the `type` block declares which sections may appear and in what order, and the `check "section_presence"` block toggles enforcement.

## Type-side configuration

On the `type` block, list the sections this type may contain, in the order they must appear:

```hcl
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
```

The list is **exhaustive**: any level-2 heading in the doc that does not match a declared section is reported as unknown. This applies to canonical names too — leaving `import` off the list and then writing `## Import` in the doc is an error, even though `import` is a canonical name.

Each section block accepts:

- `required = true` — section must be present
- `forbidden = true` — section must be absent
- both unset — section is optional (allowed but not required)
- both set — config error

The order of `section` blocks IS the canonical doc order. A type with no `section` blocks (e.g. `guide`, `index`) skips this rule entirely.

## Canonical section names

`title`, `signature`, `example`, `arguments`, `attributes`, `timeouts`, `import` — recognized by the parser and have fixed heading text by convention (e.g. `arguments` → `## Argument Reference`, `example` → `## Example Usage`).

## Custom section names

Any other lowercase snake_case identifier opts the type into a non-canonical H2 section. The heading text is derived by title-casing the snake_case name. For example:

```hcl
type "ephemeral" {
  schema_kind   = "ephemeral"
  website_paths = ["website/docs/ephemeral-resources/{name}.html.markdown"]
  title_prefix  = "Ephemeral"

  section "title"       { required = true }
  section "example"     { required = true }
  section "arguments"   { required = true }
  section "attributes"  { required = true }
  section "usage_notes" {}    # → ## Usage Notes
}

type "action" {
  schema_kind   = "action"
  website_paths = ["website/docs/actions/{name}.html.markdown"]
  title_prefix  = "Action"

  section "title"                 { required = true }
  section "example"               { required = true }
  section "dependency_management" {}    # → ## Dependency Management
  section "arguments"             { required = true }
}
```

## Check-side configuration

```hcl
check "section_presence" {
  enforce_order          = true   # default
  allow_unknown_sections = false  # default
}
```

- `enforce_order` — when true (default), sections that appear out of the order declared on the type are reported as errors.
- `allow_unknown_sections` — when true, level-2 headings outside the type's section spec are permitted silently (e.g. for free-form provider docs). Default false.

## Schema-driven Timeouts

When a schema is loaded, the schema's timeouts block decides whether the section is required, overriding the type's `required`/`forbidden` flag for `timeouts`. A schema-configured timeouts block missing from the doc is an error; a documented timeouts section with no schema configuration is also an error.
