// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

var labelTemplates = doc.HeadingTemplates{"`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

func labelResults(t *testing.T, src string, rs *schema.ResourceSchema) []check.Result {
	t.Helper()
	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", labelTemplates)
	if err != nil {
		t.Fatal(err)
	}
	return (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
}

func hasMsg(results []check.Result, sub string) bool {
	for _, r := range results {
		if strings.Contains(r.Message, sub) {
			return true
		}
	}
	return false
}

// A configurable nested block mistakenly documented under Attribute Reference
// with accurate (Required)/(Optional) labels must yield ONE misplacement error
// (move to Argument Reference) and must NOT push the author to strip the
// labels. This is the core of issue #60.
func TestLabels_ConfigurableBlockUnderAttributeReference(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`notification`" + ` - (Optional) Notification. See [` + "`notification`" + ` Block](#notification-block).

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
* ` + "`threshold`" + ` - (Optional) Threshold.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"notification"}},
		"notification": {Path: "notification", Attributes: []schema.Attribute{
			{Name: "comparison_operator", Required: true},
			{Name: "threshold", Optional: true},
		}},
	}}

	results := labelResults(t, src, rs)

	if !hasMsg(results, `block "notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected misplacement error for notification block; got: %+v", results)
	}
	if hasMsg(results, "should not have") {
		t.Errorf("must NOT tell author to strip accurate labels; got: %+v", results)
	}
	// Exactly one misplacement finding per block (not one per attribute).
	n := 0
	for _, r := range results {
		if strings.Contains(r.Message, "move this subsection to Argument Reference") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("expected exactly 1 misplacement finding, got %d: %+v", n, results)
	}
}

// A computed field referenced by dot-path under Attribute Reference (no label)
// must not be treated as misplaced, even though its block is configurable in
// the schema. Guards the false positive found in TestSchemaDocsRule_Complete.
func TestLabels_ComputedDotPathReferenceNotMisplaced(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`network`" + ` - (Optional) Network. See [` + "`network`" + ` Block](#network-block).

### ` + "`network`" + ` Block

* ` + "`subnet_id`" + ` - (Required) Subnet.

## Attribute Reference

* ` + "`network[*].private_ip`" + ` - Private IP address.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"network"}},
		"network": {Path: "network", Attributes: []schema.Attribute{
			{Name: "subnet_id", Required: true},
			{Name: "private_ip", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("dot-path computed reference must not be flagged as misplaced: %+v", results)
	}
	if hasMsg(results, "should not have") {
		t.Errorf("unlabeled computed reference must not be flagged: %+v", results)
	}
}

// A genuinely computed-only attribute carrying an erroneous label keeps the
// original "should not have label" guidance — the labels rule still helps when
// the schema confirms the attribute is read-only.
func TestLabels_ComputedOnlyAttrKeepsStripGuidance(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`endpoint`" + ` - (Optional) Endpoint. See [` + "`endpoint`" + ` Block](#endpoint-block).

### ` + "`endpoint`" + ` Block

* ` + "`address`" + ` - (Optional) Address.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"endpoint"}},
		// endpoint block: address is computed-only, so an (Optional) label is wrong.
		"endpoint": {Path: "endpoint", Attributes: []schema.Attribute{{Name: "address", Computed: true}}},
	}}

	// Document endpoint under Attribute Reference so it lands in AttributeBlocks.
	src = strings.Replace(src, "## Argument Reference\n\n* `name` - (Required) Name.\n* `endpoint` - (Optional) Endpoint. See [`endpoint` Block](#endpoint-block).\n",
		"## Argument Reference\n\n* `name` - (Required) Name.\n\n## Attribute Reference\n\n* `endpoint` - Endpoint. See [`endpoint` Block](#endpoint-block).\n", 1)

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("computed-only attr must not be flagged as misplaced: %+v", results)
	}
	if !hasMsg(results, `attribute "address" in block "endpoint" should not have (Optional) label`) {
		t.Errorf("expected retained strip-label guidance for computed-only attr; got: %+v", results)
	}
}

// The misplacement error should point at the subsection heading line (the line
// the author must move), not at an attribute inside the block.
func TestLabels_MisplacementReportsHeadingLine(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`notification`" + ` - (Optional) Notification. See [` + "`notification`" + ` Block](#notification-block).

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":             {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"notification"}},
		"notification": {Path: "notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	// Heading line is where "### `notification` Block" sits in src (1-based).
	wantLine := 0
	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, "### ") && strings.Contains(line, "notification") {
			wantLine = i + 1
			break
		}
	}

	for _, r := range labelResults(t, src, rs) {
		if strings.Contains(r.Message, "move this subsection to Argument Reference") {
			if r.Line != wantLine {
				t.Errorf("misplacement finding Line = %d, want heading line %d", r.Line, wantLine)
			}
			return
		}
	}
	t.Fatal("expected a misplacement finding")
}

// A configurable *scalar* attribute that shares its leaf name with a moved
// child block (but is itself an ordinary field, not a reference to that block)
// must not be suppressed. This exercises the root block, which is never marked
// "moved", so its labeled attributes reach the per-attribute path. Suppression
// is gated on the entry being a configurable child block, so the scalar keeps
// its own "should not have label" warning.
func TestLabels_SharedLeafConfigurableScalarNotSuppressed(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`notification`" + ` - (Optional) Whether notifications are enabled.

### ` + "`outer.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		// root has a configurable scalar "notification" and an "outer" child block.
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}, {Name: "notification", Optional: true}}, ChildBlocks: []string{"outer"}},
		"outer": {Path: "outer", ChildBlocks: []string{"notification"}},
		// The moved block is outer.notification — a different block that shares the leaf.
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	if !hasMsg(results, `block "outer.notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected outer.notification to be reported as misplaced; got: %+v", results)
	}
	if !hasMsg(results, `attribute "notification" in block "(root)" should not have (Optional) label`) {
		t.Errorf("root configurable scalar sharing the moved block's leaf must still be flagged; got: %+v", results)
	}
}

// A computed-only scalar attribute that merely shares its leaf name with a
// moved block must still receive its "should not have label" warning — the
// reference-bullet suppression is gated on the attribute actually being a
// configurable reference, not on the name alone.
func TestLabels_SharedLeafComputedScalarNotSuppressed(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.

### ` + "`other`" + ` Block

* ` + "`notification`" + ` - (Optional) Whether notifications are enabled.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":             {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"notification", "other"}},
		"notification": {Path: "notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
		// "other" is not misplaced itself; its "notification" is a computed-only
		// scalar that shares the leaf name of the moved block.
		"other": {Path: "other", Attributes: []schema.Attribute{{Name: "notification", Computed: true}}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected notification block to be reported as misplaced; got: %+v", results)
	}
	if !hasMsg(results, `attribute "notification" in block "other" should not have (Optional) label`) {
		t.Errorf("computed-only scalar sharing the moved block's leaf name must still be flagged; got: %+v", results)
	}
}

// Optional+Computed attributes may legitimately be documented under either
// section, so a labeled Optional+Computed attribute under Attribute Reference
// must not be reported as a misplaced argument.
func TestLabels_OptionalComputedNotMisplaced(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`settings`" + ` - (Optional) Settings. See [` + "`settings`" + ` Block](#settings-block).

## Attribute Reference

* ` + "`settings`" + ` - Settings. See [` + "`settings`" + ` Block](#settings-block).

### ` + "`settings`" + ` Block

* ` + "`mode`" + ` - (Optional) Mode.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"settings"}},
		// mode is Optional AND Computed — valid in either section.
		"settings": {Path: "settings", Attributes: []schema.Attribute{{Name: "mode", Optional: true, Computed: true}}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("Optional+Computed attr must not be flagged as misplaced: %+v", results)
	}
}

// Object-typed attributes whose per-field configurability is unknowable
// (ConfigUnknown) must never trigger the ERROR-severity misplacement finding.
func TestLabels_ConfigUnknownBlockNotMisplaced(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`config`" + ` - (Optional) Config. See [` + "`config`" + ` Block](#config-block).

### ` + "`config`" + ` Block

* ` + "`field`" + ` - (Optional) Field.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"config"}},
		// Synthesized from an object-typed attribute: flags are not reliable.
		"config": {Path: "config", ConfigUnknown: true, Attributes: []schema.Attribute{{Name: "field", Optional: true}}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("ConfigUnknown block must not produce a misplacement error: %+v", results)
	}
}
