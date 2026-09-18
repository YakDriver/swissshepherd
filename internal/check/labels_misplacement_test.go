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

// A dotted documentation key that does not exactly match any schema path must
// NOT fall back to a leaf scan that could resolve it to an unrelated block with
// the same leaf. outer.notification (absent) must not be driven off
// other.notification, so no misplacement ERROR is emitted.
func TestLabels_DottedKeyExactMissDoesNotLeafGuess(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`outer.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"other"}},
		"other": {Path: "other", ChildBlocks: []string{"notification"}},
		// Only other.notification exists; outer.notification (the doc key) does
		// not. The shared leaf "notification" is unique, which would tempt a
		// leaf guess.
		"other.notification": {Path: "other.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("absent dotted key must not leaf-guess to an unrelated block and emit an ERROR: %+v", results)
	}
}

// A labeled dot-notation reference (network[*].subnet_id - (Required)) creates a
// synthetic, heading-less AttributeBlocks entry. It must not be treated as a
// misplaced subsection (there is no subsection to move); the per-attribute
// strip-label finding must be retained.
func TestLabels_SyntheticDotRefKeepsStripLabel(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`network[*].subnet_id`" + ` - (Required) Subnet.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":        {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"network"}},
		"network": {Path: "network", Attributes: []schema.Attribute{{Name: "subnet_id", Required: true}}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("synthetic dot-notation reference has no subsection to move: %+v", results)
	}
	if !hasMsg(results, `attribute "subnet_id" in block "network" should not have (Required) label`) {
		t.Errorf("strip-label guidance for a labeled dot-notation reference must be retained: %+v", results)
	}
}

// A bare subsection heading whose leaf is shared by both a root schema block
// and a nested block (notification vs outer.notification) is ambiguous. The
// conservative resolver must refuse to guess, so no ERROR-severity misplacement
// finding is emitted from the root block on an ambiguous bare key.
func TestLabels_AmbiguousBareHeadingNotResolvedToRoot(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"notification", "outer"}},
		// Both a root notification block and a nested outer.notification share
		// the leaf "notification", so the bare "### notification Block" heading
		// is ambiguous.
		"notification":       {Path: "notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
		"outer":              {Path: "outer", ChildBlocks: []string{"notification"}},
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("ambiguous bare heading must not resolve to the root block and emit an ERROR: %+v", results)
	}
}

// A real subsection heading that is PRECEDED by a dot-notation reference to the
// same block (which first creates the map entry with an empty Heading) must
// still be classified. The parser must backfill Heading on the pre-existing
// synthetic entry, otherwise checkLabels skips the block and emits misleading
// strip-label warnings instead of the move error.
func TestLabels_RealHeadingAfterSyntheticReferenceStillMoved(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`notification[*].comparison_operator`" + ` - (Required) Operator.

### ` + "`notification`" + ` Block

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
		t.Errorf("a real heading preceded by a synthetic dot-notation reference must still be classified: %+v", results)
	}
	if hasMsg(results, "should not have") {
		t.Errorf("must not emit strip-label warnings for the moved block's configurable args: %+v", results)
	}
}

// ChildBlocks entries may be stored as full dot-paths (as many fixtures and the
// coverage checker's leafName normalization assume), not only leaf names. A
// parent whose child entry is the full path outer.notification must still be
// detected as configurable (2/3).
func TestLabels_FullPathChildBlockDetected(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`outer`" + ` Block

* ` + "`notification`" + ` - (Optional) Notif.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                   {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer"}},
		"outer":              {Path: "outer", ChildBlocks: []string{"outer.notification"}}, // full-path entry
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "outer" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("full-path ChildBlocks entry must still be recognized as a configurable child: %+v", results)
	}
}

// The descendant traversal must also handle full-path ChildBlocks entries: a
// grandchild stored as wrapper.notification.setting must be reached rather than
// mis-joined into wrapper.notification.wrapper.notification.setting (3/3).
func TestLabels_FullPathDescendantRecursion(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`wrapper`" + ` Block

* ` + "`notification`" + ` - (Optional) Notif.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":        {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"wrapper"}},
		"wrapper": {Path: "wrapper", ChildBlocks: []string{"notification"}}, // leaf entry
		// structural child whose own child is stored as a full path:
		"wrapper.notification":         {Path: "wrapper.notification", ChildBlocks: []string{"wrapper.notification.setting"}},
		"wrapper.notification.setting": {Path: "wrapper.notification.setting", Attributes: []schema.Attribute{{Name: "threshold", Required: true}}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "wrapper" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("full-path grandchild must be reached by the descendant traversal: %+v", results)
	}
}

// With a full-path ChildBlocks entry, a parent reference bullet to a moved child
// must still be recognized so no redundant strip-label is emitted alongside the
// move error (1/3, exercising both detection and suppression).
func TestLabels_FullPathChildNoDoubleReport(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`outer`" + ` Block

* ` + "`notification`" + ` - (Optional) Notif.

### ` + "`outer.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                   {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer"}},
		"outer":              {Path: "outer", ChildBlocks: []string{"outer.notification"}}, // full-path entry
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
	if !hasMsg(results, `block "outer.notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected outer.notification move error: %+v", results)
	}
	if hasMsg(results, "should not have") {
		t.Errorf("full-path child must be recognized so no redundant strip-label is emitted: %+v", results)
	}
}

// 1/4: a partial {Parent}-style dotted heading key whose leaf is shared by
// several schema paths (an exact header.match alongside a deeper
// foo.header.match) is ambiguous. The classifier must use most-specific
// ownership and refuse to guess, so no ERROR is emitted from the exact block.
func TestLabels_AmbiguousDottedKeyNotClassified(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`header.match`" + ` Block

* ` + "`prefix`" + ` - (Required) Prefix.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":             {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"header", "foo"}},
		"header":       {Path: "header", ChildBlocks: []string{"match"}},
		"header.match": {Path: "header.match", Attributes: []schema.Attribute{{Name: "prefix", Required: true}}},
		"foo":          {Path: "foo", ChildBlocks: []string{"header"}},
		"foo.header":   {Path: "foo.header", ChildBlocks: []string{"match"}},
		// A second block shares the leaf "match"; with only a partial
		// header.match heading, the composite matcher makes header.match the
		// most-specific owner of BOTH, so the key is ambiguous.
		"foo.header.match": {Path: "foo.header.match", Attributes: []schema.Attribute{{Name: "prefix", Required: true}}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("ambiguous partial dotted key must not be classified via an exact match: %+v", results)
	}
}

// 2/4: resolution must carry the canonical map key, not schema.Block.Path.
// With Path left unset (as many manually assembled schemas / fixtures do), a
// nested moved block and a parent reference bullet must still match by full
// path, so the bullet is suppressed rather than double-reported.
func TestLabels_ResolutionUsesMapKeyNotBlockPath(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`outer`" + ` Block

* ` + "`notification`" + ` - (Optional) Notif.

### ` + "`outer.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	// Path fields deliberately omitted — only the map keys are canonical.
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                   {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer"}},
		"outer":              {ChildBlocks: []string{"notification"}},
		"outer.notification": {Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
	if !hasMsg(results, `block "outer.notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected outer.notification to be flagged misplaced: %+v", results)
	}
	if hasMsg(results, `attribute "notification" in block "outer" should not have`) {
		t.Errorf("parent reference bullet must be suppressed via the map-key path even when Block.Path is unset: %+v", results)
	}
}

// 3/4: a real subsection distinguished by non-empty Heading (with HeadingLine
// unset, as an exported-API caller may construct) must still be classified;
// HeadingLine is only the reported location, not the synthetic marker.
func TestLabels_RealHeadingWithoutHeadingLineStillMoved(t *testing.T) {
	t.Parallel()

	d := &doc.Document{
		ArgumentBlocks: map[string]*doc.DocBlock{},
		AttributeBlocks: map[string]*doc.DocBlock{
			"notification": {
				Name:    "notification",
				Heading: "`notification` Block", // real heading text, but HeadingLine left 0
				Attributes: []doc.DocAttribute{
					{Name: "comparison_operator", Required: true, Line: 5},
				},
			},
		},
	}
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":             {ChildBlocks: []string{"notification"}},
		"notification": {Path: "notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
	if !hasMsg(results, `block "notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("a real heading (non-empty Heading) with unset HeadingLine must still be classified: %+v", results)
	}
}

// A synthetic, heading-less entry created by an unlabeled dot-path reference
// (outer.notification) must not steal subsection ownership from a real
// ### notification heading. Otherwise the real subsection resolves to no schema
// owner and its configurable labels wrongly receive strip-label warnings.
func TestLabels_SyntheticEntryDoesNotStealOwnership(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`outer.notification[*].foo`" + ` - Foo attribute.

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                   {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer"}},
		"outer":              {Path: "outer", ChildBlocks: []string{"notification"}},
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}, {Name: "foo", Computed: true}}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("real subsection must resolve despite a synthetic same-path dot-reference entry: %+v", results)
	}
	if hasMsg(results, `should not have (Required)`) {
		t.Errorf("real subsection's configurable label must not receive a strip-label warning: %+v", results)
	}
}

// A scalar attribute that carries an invalid label and merely links to a moved
// subsection for context is a defect of its own; moving the subsection does not
// fix it, so its strip-label warning must remain (not be suppressed by the link).
func TestLabels_ScalarLinkingToMovedSubsectionNotSuppressed(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`extra`" + ` - (Optional) Extra. See [notification](#notification-block).

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		// extra is a real scalar attribute of the root, not the notification block.
		"":             {Attributes: []schema.Attribute{{Name: "name", Required: true}, {Name: "extra", Optional: true}}, ChildBlocks: []string{"notification"}},
		"notification": {Path: "notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected notification move error: %+v", results)
	}
	if !hasMsg(results, `attribute "extra" in block "(root)" should not have (Optional) label`) {
		t.Errorf("a scalar linking to the moved subsection for context must keep its own strip-label: %+v", results)
	}
}

// Point 1: a child block whose only field is Optional+Computed is not a
// configurable argument block (the placement rule excludes Optional+Computed),
// so a parent bullet referencing it must NOT be flagged as misplaced. Guards
// against reusing the coverage helper (hasConfigurableAttributes), which counts
// Optional+Computed.
func TestLabels_ChildBlockOptionalComputedOnlyNotMisplaced(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`wrapper`" + ` Block

* ` + "`settings`" + ` - (Optional) Settings. See [` + "`settings`" + ` Block](#settings-block).
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":        {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"wrapper"}},
		"wrapper": {Path: "wrapper", ChildBlocks: []string{"settings"}},
		// settings' only field is Optional+Computed — not a pure-config arg.
		"wrapper.settings": {Path: "wrapper.settings", Attributes: []schema.Attribute{{Name: "mode", Optional: true, Computed: true}}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("child block with only Optional+Computed field must not be flagged misplaced: %+v", results)
	}
}

// Point 2: a structural child block with no direct configurable attributes but
// a configurable descendant IS a configurable reference. When that subsection
// is moved, the parent reference bullet must be suppressed (no redundant
// strip-label alongside the move error).
func TestLabels_StructuralChildWithConfigurableDescendantSuppressed(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`notification`" + ` - (Optional) Notif. See [` + "`notification`" + ` Block](#notification-block).

### ` + "`notification`" + ` Block

* ` + "`setting`" + ` - (Optional) Setting. See [` + "`setting`" + ` Block](#setting-block).
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"notification"}},
		// notification has NO direct attributes; its config lives in a descendant.
		"notification":         {Path: "notification", ChildBlocks: []string{"setting"}},
		"notification.setting": {Path: "notification.setting", Attributes: []schema.Attribute{{Name: "threshold", Required: true}}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("structural block with configurable descendant must be flagged misplaced: %+v", results)
	}
	if hasMsg(results, `attribute "notification" in block "(root)" should not have`) {
		t.Errorf("parent reference bullet to the moved block must be suppressed, not double-reported: %+v", results)
	}
}

// Point 3: a moved block that mixes a pure-config field with a Computed field
// carrying an erroneous label must emit the move error AND retain the
// strip-label guidance for the Computed field — the move only accounts for the
// configurable fields.
func TestLabels_MovedBlockRetainsStripLabelForComputedField(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
* ` + "`state`" + ` - (Optional) State.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"notification"}},
		"notification": {Path: "notification", Attributes: []schema.Attribute{
			{Name: "comparison_operator", Required: true}, // pure-config → covered by move
			{Name: "state", Computed: true},               // computed-only, erroneously labeled
		}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected move error for notification block: %+v", results)
	}
	if !hasMsg(results, `attribute "state" in block "notification" should not have (Optional) label`) {
		t.Errorf("computed field's strip-label guidance must be retained inside a moved block: %+v", results)
	}
	if hasMsg(results, `attribute "comparison_operator"`) {
		t.Errorf("pure-config field must not get a strip-label (the move covers it): %+v", results)
	}
}

// Point 4: a moved outer.notification must not suppress an unrelated
// configurable notification child at the root — suppression matches the full
// resolved schema path, not the leaf name.
func TestLabels_MovedPathDoesNotSuppressUnrelatedLeaf(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`notification`" + ` - (Optional) Root notif. See [` + "`notification`" + ` Block](#notification-block).

### ` + "`outer.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"notification", "outer"}},
		// root's own notification child is configurable but NOT documented as a
		// subsection here, so it is never marked moved.
		"notification": {Path: "notification", Attributes: []schema.Attribute{{Name: "threshold", Required: true}}},
		"outer":        {Path: "outer", ChildBlocks: []string{"notification"}},
		// The moved block is outer.notification — same leaf, different path.
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	if !hasMsg(results, `block "outer.notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected outer.notification to be flagged misplaced: %+v", results)
	}
	if !hasMsg(results, `attribute "notification" in block "(root)" should not have (Optional) label`) {
		t.Errorf("unrelated root notification reference must not be suppressed by a moved same-leaf path: %+v", results)
	}
}

// Point 5: a child block reference is resolved relative to the parent's schema
// path, not by a global leaf lookup. With a same-leaf block elsewhere in the
// tree (which makes a global leaf lookup ambiguous and would previously miss
// the misplacement), the nested block must still be detected as configurable.
func TestLabels_ChildResolvedRelativeToParent(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`outer`" + ` Block

* ` + "`notification`" + ` - (Optional) Notif. See [` + "`notification`" + ` Block](#notification-block).
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer", "notification"}},
		"outer": {Path: "outer", ChildBlocks: []string{"notification"}},
		// outer.notification is configurable...
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
		// ...and a same-leaf root-level notification also exists, making a
		// global leaf lookup ambiguous.
		"notification": {Path: "notification", Attributes: []schema.Attribute{{Name: "foo", Computed: true}}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "outer" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("nested child must be resolved relative to parent path despite same-leaf ambiguity: %+v", results)
	}
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
