# Doc styles: reference and tfplugindocs
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

Issues #116 and #117. swissshepherd was written for one page shape, the AWS provider's, and providers whose docs `tfplugindocs` generates failed on every page even when the docs were complete. This doc defines the two page shapes swissshepherd reads, how each maps onto its rules, and the finding for a doc whose reference section is missing.

## 1. The two styles

Set `doc_style` at the top of the config. It applies to every type.

| | `reference` (default) | `tfplugindocs` |
|---|---|---|
| Providers | AWS, Google, Azure, most hand-written docs | Providers that run `tfplugindocs generate` with its default templates (random, tls, cloudinit, datadog, cloudflare) |
| Title | `# Resource: name` (prefix from `title_prefix` / `allow_prefixes`) | `# name (Resource)`, `# name (Data Source)`, `# name (Ephemeral Resource)`, `# name (List Resource)`, `# name (Action)`, `# function: name` (`title_format`) |
| Reference sections | `## Argument Reference` and `## Attribute Reference` | One `## Schema` (`## Arguments` for functions) |
| A field's label | Inline: `` * `name` - (Required) ... `` | The group it's listed under: `### Required`, `### Optional`, `### Read-Only` at the root, `Required:` / `Optional:` / `Read-Only:` under each nested heading |
| A field bullet | `` * `name` - (Optional) Description `` | `` - `name` (String, Deprecated) Description ``; older releases write `- **name** (String) ...` |
| Nested heading | Any `block_heading_styles` template | `` ### Nested Schema for `path` `` under an `<a id="nestedblock--path">` anchor |
| Timeouts | `## Timeouts` section | A nested `timeouts` block in `## Schema` |
| Import example | ```` ```terraform ```` `import {` block first, then ```` ```console ```` | Optional `import {` blocks, then ```` ```shell ```` |

Why a config setting and not detection per doc: detection changes which rules apply on a guess, so a half-converted doc would be judged by whichever style it resembles. Providers mix styles by mistake (kubernetes has 7 Argument Reference pages among 116 Schema pages; tfe keeps some Schema pages under `website/docs`), and those pages should be reported, not silently read another way. `file_match`'s `mixed_layout` made the same choice.

Why not a preset of existing options: the tfplugindocs field label lives in a group heading, not on the bullet, and its bullet has no ` - ` separator. No combination of heading templates and section names reads that. Pointing `arguments_heading` at `Schema` in the reference style makes things worse: `### Required` becomes a phantom block and every nested bullet "is missing the ' - ' separator" (cloudinit: 15 errors).

## 2. What the tfplugindocs style changes

The style chooses defaults; every one can still be set in the config, except where noted.

- **Types** come from `internal/config/defaults_tfplugindocs.hcl`: `title_format`, `arguments_heading = "Schema"` (`"Arguments"` for functions), no `attributes` or `timeouts` section, `region_aware` off. A `type` block in the config still replaces the default type wholesale.
- **Parser** (`doc.ParseOptions.Tfplugindocs`): `## Schema` (the type's `arguments_heading`) is the arguments section, and only an exact match counts; the reference style's prefix rule (any heading beginning `Argument`/`Attribute`) is off, so a stray `## Argument Reference` is an unknown section instead of being read in the wrong dialect. Group headings and group paragraphs set the label of the bullets that follow. `` Nested Schema for `path` `` is the only block heading, whatever `block_heading_styles` says; any other heading in `## Schema` (a custom template's `### Example: ...`) documents no block and gets the unparseable-heading warning, rather than a phantom-block error. A bullet is a field only if it starts with a code span or bold name followed by tfplugindocs's `(Type...)`; the lists of allowed values inside a description (``* `log` - ...``, ``- `"geo"`: ...``) aren't fields. `Deprecated` among the traits marks the field deprecated. Each block's list is ordered per group (`SplitByLabel`), as tfplugindocs sorts it.
- **`schema_docs`** (`config.StyledCheck`):
  - `allow_inline_read_only` is always true: there is no other section for Read-Only fields. Setting it false has no effect.
  - `skip_blocks` defaults to none: `timeouts` is documented in `## Schema`.
  - `nested_object_attributes` defaults to true: every object attribute has its own `Nested Schema for` heading with its fields.
  - `description` defaults to off: tfplugindocs copies descriptions from the schema, so a weak description is a schema fix, and the default `bad_prefixes` are AWS style.
  - Label findings name the group: `"boundary" is listed under Read-Only but is optional in the schema; list it under Optional`.
  - A root `id` with no schema description is accepted under Read-Only whatever its flags, because tfplugindocs puts it there (SDKv2 makes `id` Optional+Computed). Older releases listed it by its flags (the ldap provider's docs), which is also accepted. `schema.Attribute.Described` carries whether the schema has a description, so this doesn't guess.
  - On function pages, the `## Arguments` list (`` 1. `name` (Type) ... ``) is compared with the function's parameters, both ways. Labels and ordering don't apply: arguments are positional and unlabeled.
- **`import_section`**: `allow_languages` defaults to `terraform`, `shell`, `console`, and `require_import_block` to false. A `shell` block counts as a CLI block for the "terraform blocks before console blocks" order. The passive-voice and "e.g." warnings stay: they are about prose, not layout.
- **`section_presence`**, **`title_section`**, **`timeouts_section`**: no change in code beyond the configurable headings and `title_format` (§4). The types carry the difference.

What stays the same: field homes collapse to one section, but every other `schema_docs` invariant holds. A field listed twice, a field that doesn't exist, a deprecated field not marked, and a nested block with missing fields are reported as in the reference style.

## 3. A missing reference section (#117)

**Defect.** With no arguments section, coverage checked no root argument: the root block is excluded from "block is not documented", and resolving it found no section. Unless the type required the section, nothing reported it. ldap's tfplugindocs page, read in the reference style with `section "schema"` allowed, passed with its Required `dn` and `object_classes` deleted. The same pages also got findings that called documented fields undocumented: "block `part` is not documented" and "Read-Only attribute `rendered` should be documented in Attribute Reference" on cloudinit, whose `## Schema` documents both.

**Rule.** A field's home sections follow `docs/rules/field-homes.md`: Argument Reference for a configurable field; Attribute Reference for a computed-only field, or either with `allow_inline_read_only`; either for a field of a `ConfigUnknown` block. When every home section of a field is absent from the doc, coverage can't check it. One finding per missing section reports all such fields:

> WARN `no ## Argument Reference section, so 3 arguments (`attributes`, `dn`, `object_classes`) aren't checked against the schema`

- It names the type's heading (`arguments_heading`, `attributes_heading`), so a tfplugindocs page with no `## Schema` gets "no ## Schema section, so N fields ...".
- Field paths are sorted and capped at three, with a count of the rest. The noun is "fields" when Read-Only fields are counted under the arguments section (inline Read-Only).
- It replaces, for those fields only, the per-field "is not documented" and "Read-Only attribute ... should be documented" findings and the per-block "block is not documented" findings. They would say the fields are undocumented when the cause is the missing section, and on a tfplugindocs page they are documented.
- A field listed in the other reference section isn't counted: coverage checks it there, and `outsideHome` reports a configurable field that is listed only under Attribute Reference.
- It's a **warning**. The doc may document the fields in a section swissshepherd doesn't read (`## Schema` read in the reference style), and the remedy is structural, which `section_presence` reports as an error when the type requires the section. As an error it would recreate the problem in #116: a consumer would have to demote all of `schema_docs` to stop failing on a page shape, hiding real coverage errors.

**What reports it.** Under the default types both checks fire on a missing Argument Reference: `section_presence`'s "missing required section" error and this warning (`TestCoverage_MissingHomeSectionDeferral`). Neither depends on the other. When the type doesn't require the section, or `section_presence` is off, this warning is the only report, which is the case #117 was about.

## 4. Bugs found on the way

- **`arguments_heading` was never read.** The type field was documented, and `defaults.hcl` set it for functions, but no code used it: functions worked because `Arguments` begins with `Argument`. It's now the heading the parser accepts (exactly, in addition to the reference style's prefix rule) and the one `section_presence` names. `attributes_heading` is its counterpart.
- **The title prefix list was ambiguous.** `title_section` printed `[Action Data Source ... List Resource Resource]`, which reads as prefixes `List` and `Resource` (#116 read it that way). It now quotes each: `"List Resource: ", "Resource: "`.
- **The timeouts finding flapped.** `section_presence` listed timeout actions in `schema.Block.Attributes` order, which comes from a map. On azurerm the message alternated between runs (`('read', 'update', 'create', 'delete')`). It's sorted now. `checkMissingHomes` sorts its field list for the same reason.
- **An empty object attribute's heading was a phantom block.** tfplugindocs writes `` ### Nested Schema for `x` `` for a nested attribute whose object has no fields (cloudflare `include.everyone`). With no children it wasn't in `nestedAttributeLeaves`. `schema.Attribute.Object` now marks object-typed attributes regardless of children.
- **"Does not support import" wasn't recognized.** `import_section` accepted an import section without code blocks only if it said "cannot import". It now also accepts "cannot be imported" and "does not (currently) support ... import", the wording Google ("This resource does not support import.", 30 findings) and cloudflare use. Such a section is then done: the identity checks no longer run on a resource whose doc says it can't be imported.

## 5. What stays unchecked

| Unchecked | Why | Reported by |
|---|---|---|
| Fields whose every home section is missing | Nothing to check them against | The §3 warning, which names them; `section_presence` when the type requires the section |
| Description style in the tfplugindocs style | Descriptions are the schema's | Nothing by default; `description = true` turns it back on |
| Labels and order of function arguments | Positional, unlabeled | `signature_section` for the signature; §2 for presence both ways |
| A root `id` without a description listed under Read-Only or under its flag's group | Both are what tfplugindocs writes | Nothing, by design |
| Docs written in the other style | One style per provider | Their sections are unknown sections (`section_presence`) and the §3 warning |

## 6. Measured

Corpus: each provider's docs at the tag of its latest registry release, with that release's schema. Each run uses a config with only `provider_source`, then adds `doc_style = "tfplugindocs"`. Errors/warnings; every run twice, identical.

| Provider | Base `716ac6d` | Branch, reference style | Branch, tfplugindocs style |
|---|---|---|---|
| hashicorp/archive 2.8.1 | 24/0 | 8/4 | 0/0 |
| hashicorp/cloudinit 2.4.1 | 12/0 | 8/4 | 0/0 |
| hashicorp/dns 3.6.2 | 91/0 | 80/24 | 0/0 |
| elastic-infra/ldap 2.0.1 | 6/0 | 6/1 | 0/0 |
| hashicorp/local 2.9.1 | 62/0 | 28/13 | 0/0 |
| hashicorp/random 3.9.1 | 82/0 | 64/23 | 0/0 |
| hashicorp/null 3.3.2 | 10/0 | 8/3 | 2/0 |
| hashicorp/time 0.14.2 | 53/7 | 31/14 | 2/7 |
| hashicorp/http 3.6.3 | 12/0 | 7/2 | 3/0 |
| hashicorp/tls 4.4.2 | 75/0 | 36/16 | 7/0 |
| cloudflare/cloudflare 5.29.0 | 6,993/0 | 3,687/1,502 | 23/0 |
| DataDog/datadog 4.30.0 | 2,536/2 | 1,447/384 | 67/2 |
| hashicorp/kubernetes 3.5.0 | 1,352/145 | 785/339 | 645/143 |
| hashicorp/tfe 0.84.0 (mostly reference style) | 904/82 | 667/258 | 158/83 |
| integrations/github 6.13.0 (mostly reference style) | 1,378/150 | 1,334/188 | 467/254 |
| hashicorp/google 8.11.0 | 32,219/34,031 | 32,160/34,035 | |
| hashicorp/azurerm 5.14.0 | 36,998/5,236 | 36,998/5,236 | |

In the reference style, the drop on tfplugindocs providers is the §3 rule: one warning per missing section in place of a per-field error for each field of a `## Schema` page. That style still fails those providers, on titles and sections, as it should; it isn't their style.

The tfplugindocs-style findings that remain were spot-checked; all are real or come from custom templates. Stale docs: datadog lists `auth_config.aws_auth_config_keys` fields as Required that the schema made Optional; kubernetes pages lack `volume_device` and `namespace_selector`; cloudflare documents a removed `filter` and lists the computed `networks` as Optional. Also missing `## Example Usage` (cloudflare, datadog, tls), hand-ordered fields (tls), a title code block (cloudflare), deprecated fields not marked, and custom templates: titles without `(Resource)` (kubernetes, null), `## Schema` before `## Example Usage` (kubernetes), extra sections (http, tls, time), import blocks with no language (kubernetes). Those are configured with `title_format`, the type's section order, and `allow_unknown_sections`. github and tfe are reference-style providers measured in the wrong style; their pages in the other style are what is reported.

Reference-style providers:

- terraform-provider-aws `745bb043908` with `.ci/swissshepherd-full.hcl` and `-weak.hcl` (`make corpus`, base `716ac6d`): unchanged, 0 added and 0 removed in both.
- azurerm: the title prefix message is reworded (1,236 findings, same targets), and one timeouts message now lists actions sorted.
- google: the same title rewording (1,422); 30 "should have at least one code block" errors removed on import sections saying "This resource does not support import."; 29 Read-Only errors on 4 docs with no `## Attributes Reference` became 4 warnings, one per doc.
