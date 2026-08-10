# Object-typed attributes, proto5, and the `ConfigUnknown` escape hatch
<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

This page explains why swissshepherd cannot always tell whether a nested object
field is a user-supplied argument or a provider-computed value, and why the
[`schema_docs`](schema_docs.md) coverage check deliberately relaxes its
Argument-vs-Attribute-Reference enforcement for some object-typed attributes.
If you are wondering "why does `ConfigUnknown` exist?", this is the answer.

## The short version

An object-typed attribute — `list(object({...}))`, `set(object({...}))`,
`map(object({...}))`, or bare `object({...})` — encodes its fields as a **cty
type**, which records field *names and types only*. It carries **no per-field
`Required` / `Optional` / `Computed`**. The only configurability signal is on
the *parent* attribute. So when the parent is configurable, swissshepherd has no
schema-derived way to know whether each field belongs in `## Argument Reference`
(an argument) or `## Attribute Reference` (read-only). Rather than guess, it
requires the field to be documented *somewhere* and accepts either section.

This is not a limitation swissshepherd can fix — the information is genuinely
absent from the schema. The sections below explain how it gets lost.

## A concrete example

`aws_appsync_source_api_association`
(`internal/service/appsync/source_api_association.go`):

```go
"source_api_association_config": schema.ListAttribute{ // proto5 Optional+Computed nested block.
    CustomType: fwtypes.NewListNestedObjectTypeOf[sourceAPIAssociationConfigModel](ctx),
    Optional:   true,
    Computed:   true,
    PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
    Validators:    []validator.List{listvalidator.SizeAtMost(1)},
    ElementType: types.ObjectType{
        AttrTypes: fwtypes.AttributeTypesMust[sourceAPIAssociationConfigModel](ctx),
    },
},

type sourceAPIAssociationConfigModel struct {
    MergeType fwtypes.StringEnum[awstypes.MergeType] `tfsdk:"merge_type"`
}
```

`merge_type` is a **required** argument (sent to AWS on Create and Update). But
the metadata loss is entirely in one line: `ElementType: types.ObjectType{...}`.
`AttrTypes` is a `map[string]attr.Type` — names to types, nothing else. There is
no slot for "required." In the provider schema JSON this serializes as:

```json
"source_api_association_config": {
  "type": ["list", ["object", {"merge_type": "string"}]],
  "optional": true,
  "computed": true
}
```

`merge_type`'s required-ness has evaporated. swissshepherd's loader defaults
every such field to `Computed: true`, because that is the only safe assumption
when no flags exist — and then, before this fix, the coverage check demanded
each field appear in `## Attribute Reference`, even though `merge_type` is
(correctly) documented as `(Required)` in `## Argument Reference`.

## Why the provider is written this way

Note the author's own comment: *"proto5 Optional+Computed nested block."* This
is a forced tradeoff, not an oversight. The AWS provider muxes the SDKv2 and
Plugin Framework providers together over **protocol v5** (`tf5muxserver`, see
`internal/provider/factory.go`). Over proto5 there is no representation that
gives you **both** per-field schema **and** Optional+Computed semantics:

| Representation | Per-field flags? | Optional+Computed? | proto5? |
|---|---|---|---|
| `schema.ListNestedBlock` (a nested *block*) | yes | **no** — blocks have no `Computed` | yes |
| `schema.ListNestedAttribute` (a nested *attribute*) | yes | yes | **no** — nested attributes are proto6 |
| `schema.ListAttribute{ElementType: types.ObjectType{...}}` | **no** | yes | yes |

The resource needs Optional+Computed (`UseStateForUnknown` — the provider fills
the value in when the user omits it), which rules out blocks. Nested attributes
would be ideal but do not exist in proto5. So the only proto5-compatible option
is the object-typed `ListAttribute`, which drops the per-field metadata. Every
finding of this family traces back to this constraint.

## How terraform-plugin-docs handles the same schema

tfplugindocs is deterministic precisely because it only classifies what the
schema fully specifies. Its renderer (`internal/schemamd/render.go`) branches on
attribute *shape*:

- **Framework nested type** (`AttributeNestedType != nil`) → it groups each
  child into Required / Optional / Read-Only using that child's own flags. This
  is the rich path. It renders as `(Attributes)` / `(Attributes List)`.
- **cty object type** (`AttributeType.IsObjectType()` or collection of object)
  → it lists each field as bare `` - `name` (type) `` under the *parent's*
  single group, with **no** per-field Required/Optional/Read-Only split —
  because there is nothing to split on.

So tfplugindocs never "deduces" per-field configurability for object types; it
declines to assert it. swissshepherd's relaxed behavior mirrors that stance.

### Contrast: awscc

The awscc provider is proto6-native and generates Framework **nested
attributes** from CloudFormation schemas, which carry per-field `required`
arrays and `readOnlyProperties`. So awscc's nested fields *do* have real flags,
render as `(Attributes)` with proper Required/Optional/Read-Only subsections,
and would be checked section-precisely. The difference is not the tool or the
schema format — it is whether the author used a shape that preserves the flags.

## What swissshepherd does: the parent-configurability model

When [`nested_object_attributes`](schema_docs.md) is enabled, expansion sets a
`ConfigUnknown` flag on each synthesized nested block based on the parent chain:

- **Parent is Computed-only** → its fields are necessarily read-only, so
  `ConfigUnknown = false`. Coverage enforces documentation in
  `## Attribute Reference`, as usual. (This is the common data-source-output
  case, e.g. a computed `items` list.)
- **Parent is Optional and/or Required** → per-field configurability is
  unknowable, so `ConfigUnknown = true`. Coverage still requires each field to
  be documented, but accepts **either** Argument or Attribute Reference and does
  not dictate a section. A genuinely undocumented field is still reported, with
  a neutral "is not documented" message.

`ConfigUnknown` is, in effect, a **proto5-shape detector**: it is true only
where the lossy `ElementType: types.ObjectType` shape was forced, and false
wherever the author had a richer option.

## Future alignment

There is nothing to fix in swissshepherd here — the escape hatch is correct
given the schema. The real remedy lives in the provider: once a resource is
served over **proto6**, it can use `schema.ListNestedAttribute` with a
`NestedObject.Attributes` map, which is Optional+Computed **and** carries
per-field flags:

```go
"source_api_association_config": schema.ListNestedAttribute{
    Optional: true,
    Computed: true,
    // ...
    NestedObject: schema.NestedAttributeObject{
        Attributes: map[string]schema.Attribute{
            "merge_type": schema.StringAttribute{
                CustomType: fwtypes.StringEnumType[awstypes.MergeType](),
                Required:   true,
            },
        },
    },
},
```

That serializes with a `nested_type` carrying `merge_type: {required: true}`.
When that happens, swissshepherd's loader reads the real flag, expansion leaves
`ConfigUnknown = false`, and coverage automatically resumes enforcing the exact
section — no linter change required. The escape hatch self-heals the moment the
schema shape improves.
