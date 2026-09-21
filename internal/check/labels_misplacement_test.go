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

// findSeverity returns the severity of the first result whose Message contains
// sub, and whether such a result exists. Severity is the load-bearing §9
// decision, so tests pin it explicitly rather than relying on hasMsg alone.
func findSeverity(results []check.Result, sub string) (check.Severity, bool) {
	for _, r := range results {
		if strings.Contains(r.Message, sub) {
			return r.Severity, true
		}
	}
	return 0, false
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
// synthetic, heading-less AttributeBlocks entry. It has no subsection to move,
// so it yields a per-attribute move for the configurable argument (§8 case 5) —
// not a "move this subsection" collapse, and not the misleading strip-label.
func TestLabels_LabeledDotRefBulletMoved(t *testing.T) {
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
		t.Errorf("a heading-less synthetic reference has no subsection to collapse: %+v", results)
	}
	if !hasMsg(results, `argument "subnet_id" in block "network" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("labeled dot-notation reference to a configurable arg must yield a per-attribute move: %+v", results)
	}
	if hasMsg(results, "should not have") {
		t.Errorf("must not push the author to strip the accurate label: %+v", results)
	}
}

// A bare heading resolves to an exact root-level schema block when one exists
// (§4): `### notification` maps to the root `notification` block (not the nested
// outer.notification), and that configurable block is flagged as misplaced.
func TestLabels_BareHeadingResolvesToExactRootBlock(t *testing.T) {
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
	if !hasMsg(results, `block "notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("bare heading must resolve to the exact root-level block and be flagged: %+v", results)
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

// A dotted heading claims an exact schema path (§4): `### header.match` resolves
// to header.match itself even though a deeper foo.header.match shares the leaf,
// and it is flagged as the misplaced configurable block. (The pre-redesign
// ownership resolver treated this as ambiguous and stayed silent.)
func TestLabels_DottedKeyExactResolves(t *testing.T) {
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
	if !hasMsg(results, `block "header.match" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("dotted heading must resolve to its exact schema path and be flagged: %+v", results)
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

// Comment 1: when two heading forms document the same sole schema path
// (### outer.notification and ### notification both cover outer.notification),
// only the most-specific full-path heading owns the path and is marked moved.
// The alternate leaf heading must not fall through to the misleading strip-label
// warnings for pure-config fields covered by the move (computed fields still do).
func TestLabels_AlternateHeadingsSameMovedPath(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`outer.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
* ` + "`state`" + ` - (Optional) State.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer"}},
		"outer": {Path: "outer", ChildBlocks: []string{"notification"}},
		// sole schema path with leaf "notification":
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{
			{Name: "comparison_operator", Required: true},
			{Name: "state", Computed: true},
		}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	if !hasMsg(results, `block "outer.notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("expected the full-path heading to be flagged misplaced: %+v", results)
	}
	if hasMsg(results, `attribute "comparison_operator" in block "notification" should not have`) {
		t.Errorf("alternate leaf heading's pure-config label must be suppressed (covered by the move): %+v", results)
	}
	// A computed field carrying an erroneous label in the alternate keeps guidance.
	if !hasMsg(results, `attribute "state" in block "notification" should not have (Optional) label`) {
		t.Errorf("computed field in the alternate heading must keep its strip-label: %+v", results)
	}
}

// Comment 2: a labeled dot-path reference (outer[*].notification) creates a
// heading-less synthetic "outer" block. When ### outer.notification is moved,
// the reference bullet must be suppressed — the heading-less parent resolves by
// exact schema path.
func TestLabels_HeadinglessParentReferenceSuppressed(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`outer[*].notification`" + ` - (Optional) Notif ref.

### ` + "`outer.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                   {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer"}},
		"outer":              {Path: "outer", ChildBlocks: []string{"notification"}},
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
		t.Errorf("heading-less parent's reference bullet to the moved block must be suppressed: %+v", results)
	}
}

// A block listed in skip_blocks (e.g. the default "timeouts") is opted out of
// checks entirely, so the misplacement classification must not emit a new move
// error for it even when it is documented under Attribute Reference with
// configurable labels.
func TestLabels_SkipBlocksExemptFromMisplacement(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`timeouts`" + ` Block

* ` + "`create`" + ` - (Optional) Create timeout.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":         {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"timeouts"}},
		"timeouts": {Path: "timeouts", Attributes: []schema.Attribute{{Name: "create", Optional: true}}},
	}}

	// Default SkipBlocks includes "timeouts".
	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("a skip_blocks entry must not produce a misplacement error: %+v", results)
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

// A configurable root scalar that merely links to a moved subsection for context
// is judged on its own: it is a misplaced root argument (#62) and gets its own
// per-attribute move (WARN), independent of the linked subsection's collapse.
func TestLabels_ScalarLinkingToMovedSubsectionStillFlagged(t *testing.T) {
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
	if !hasMsg(results, `argument "extra" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("a configurable root scalar must get its own move (#62), independent of the linked subsection: %+v", results)
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

// A mixed block (a pure-config field plus a computed-only field documented in
// the same subsection) must NOT collapse (§5): it emits a per-attribute move for
// the configurable field and leaves the computed-only field under Attribute
// Reference with its strip-label guidance. This is the 3/3 fix — a wholesale
// "move this subsection" would drag the computed field to the wrong section.
func TestLabels_MixedBlockPerAttributeMoveKeepsComputedStrip(t *testing.T) {
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
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("a mixed block must not collapse to a wholesale subsection move: %+v", results)
	}
	if !hasMsg(results, `argument "comparison_operator" in block "notification" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the configurable field must get a per-attribute move: %+v", results)
	}
	if !hasMsg(results, `attribute "state" in block "notification" should not have (Optional) label`) {
		t.Errorf("the computed-only field must keep its strip-label guidance: %+v", results)
	}
}

// A moved outer.notification must not swallow an unrelated configurable
// notification child at the root: dedup is path-based, not leaf-based (2/3). The
// root's own notification reference is a distinct misplaced configurable block
// and gets its own move.
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
	if !hasMsg(results, `argument "notification" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("unrelated root notification reference must get its own move, not be suppressed by a same-leaf path: %+v", results)
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

// A configurable root *scalar* that shares its leaf name with a moved nested
// block (outer.notification) is judged independently: it is itself a misplaced
// root argument (#62) and gets its own per-attribute move (WARN), never
// suppressed by the nested block's move. Detection is per attribute, so a shared
// leaf name causes no cross-talk.
func TestLabels_SharedLeafConfigurableScalarStillFlagged(t *testing.T) {
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
	if !hasMsg(results, `argument "notification" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("root configurable scalar sharing the moved block's leaf must get its own move (#62); got: %+v", results)
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

// Severity is the load-bearing, gated §9 decision. Pin all three classes so a
// regression that flips one silently (e.g. root → ERROR, exactly the #62 FP
// risk being gated) is caught: collapse and nested per-attribute moves are
// ERROR, and a root-scalar move (#62) is WARN with no block clause.
func TestLabels_MoveSeverities(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`instance_type`" + ` - (Optional) Instance type.

### ` + "`cost_filter`" + ` Block

* ` + "`values`" + ` - (Required) Values.

### ` + "`auto_adjust_data`" + ` Block

* ` + "`auto_adjust_type`" + ` - (Required) Type.
* ` + "`last_auto_adjust_time`" + ` - (Optional) Last time.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "instance_type", Optional: true},
		}, ChildBlocks: []string{"cost_filter", "auto_adjust_data"}},
		"cost_filter": {Path: "cost_filter", Attributes: []schema.Attribute{{Name: "values", Required: true}}},
		"auto_adjust_data": {Path: "auto_adjust_data", Attributes: []schema.Attribute{
			{Name: "auto_adjust_type", Required: true},
			{Name: "last_auto_adjust_time", Computed: true}, // computed-only → block is mixed
		}},
	}}
	results := labelResults(t, src, rs)

	// Collapse (pure-config nested block) → ERROR.
	if sev, ok := findSeverity(results, `block "cost_filter" is documented under Attribute Reference but is a configurable argument block`); !ok || sev != check.SeverityError {
		t.Errorf("collapse severity: ok=%v sev=%v, want ERROR; results=%+v", ok, sev, results)
	}
	// Nested per-attribute move (mixed block) → ERROR.
	if sev, ok := findSeverity(results, `argument "auto_adjust_type" in block "auto_adjust_data"`); !ok || sev != check.SeverityError {
		t.Errorf("nested per-attribute severity: ok=%v sev=%v, want ERROR; results=%+v", ok, sev, results)
	}
	// Root scalar move (#62) → WARN, gated per §9.
	if sev, ok := findSeverity(results, `argument "instance_type" is documented under Attribute Reference`); !ok || sev != check.SeverityWarning {
		t.Errorf("root move severity: ok=%v sev=%v, want WARN; results=%+v", ok, sev, results)
	}
	// The root move must carry no block clause.
	if hasMsg(results, `in block "(root)"`) {
		t.Errorf("root move must not carry a block clause: %+v", results)
	}
}

// §8 case 3b — the negative twin for the sole inference step (§4): a bare
// heading whose unique-leaf match is an Optional+Computed attribute must NOT be
// flagged as misplaced. configurableArgAtPath excludes Optional+Computed, so
// unique-leaf resolution can never promote it to a move.
func TestLabels_UniqueLeafOptionalComputedNotMoved(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`endpoint`" + ` Block

* ` + "`address`" + ` - (Optional) Address.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":        {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"cluster"}},
		"cluster": {Path: "cluster", ChildBlocks: []string{"endpoint"}},
		// Sole path carrying leaf "endpoint"; its address is Optional+Computed.
		"cluster.endpoint": {Path: "cluster.endpoint", Attributes: []schema.Attribute{{Name: "address", Optional: true, Computed: true}}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move it to Argument Reference") || hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("unique-leaf resolution to an Optional+Computed attr must not produce a move: %+v", results)
	}
}

// Review comment 1/2 (split headings): the dotted heading that owns the schema
// path documents ONLY an unlabeled computed field, while a bare alternate
// heading carries the (Required) configurable field. The two-pass classified
// only the owning heading and emitted a misleading strip-label on the alternate.
// The redesign judges each heading independently: the bare alternate resolves to
// the sole matching path via unique-leaf, so the configurable field is flagged as
// a move — never strip-labeled.
func TestLabels_SplitHeadingConfigInBareAlternateFlagged(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`outer.notification`" + ` Block

* ` + "`last_updated`" + ` - Timestamp of the last update.

### ` + "`notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer"}},
		"outer": {Path: "outer", ChildBlocks: []string{"notification"}},
		// Sole path with leaf "notification": comparison_operator is config,
		// last_updated is computed-only.
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{
			{Name: "comparison_operator", Required: true},
			{Name: "last_updated", Computed: true},
		}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	if !hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("config in the bare alternate heading must be flagged as a move: %+v", results)
	}
	if hasMsg(results, `attribute "comparison_operator"`) && hasMsg(results, "should not have") {
		t.Errorf("must NOT emit the misleading strip-label for the alternate heading's configurable field: %+v", results)
	}
}

// Review comment 2/2 (leaf over-match): a real other.notification block is moved,
// and a phantom ### outer.notification heading (no such schema path) documents a
// configurable-looking field. The two-pass leaf fallback resolved the phantom's
// leaf to other.notification and silently dropped its warning. The redesign
// resolves dotted headings by EXACT path only: the phantom is unresolved, so its
// labeled field keeps its own strip-label (never silently dropped, never borrows
// the real block's move).
func TestLabels_PhantomDottedHeadingNotSuppressedByLeaf(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`other.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.

### ` + "`outer.notification`" + ` Block

* ` + "`comparison_operator`" + ` - (Required) Operator.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"other"}},
		"other": {Path: "other", ChildBlocks: []string{"notification"}},
		// other.notification exists; outer.notification (the phantom heading) does
		// NOT. The shared leaf "notification" is unique, which tempted the old
		// leaf fallback.
		"other.notification": {Path: "other.notification", Attributes: []schema.Attribute{{Name: "comparison_operator", Required: true}}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	// The real block is moved.
	if !hasMsg(results, `block "other.notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("real other.notification must be flagged as a move: %+v", results)
	}
	// The phantom heading's warning must NOT be silently dropped — it keeps its
	// own strip-label because it is unresolved (exact-only dotted resolution).
	if !hasMsg(results, `attribute "comparison_operator" in block "outer.notification" should not have (Required) label`) {
		t.Errorf("phantom outer.notification warning must not be silently dropped via leaf over-match: %+v", results)
	}
	// And the phantom must not borrow the real block's move.
	if hasMsg(results, `block "outer.notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("phantom dotted heading must not resolve to a move: %+v", results)
	}
}

// Review comment 1/4: collapse must cover only the fields documented in the
// collapsing subsection. The dotted owner documents one configurable field
// (pure-config → collapses); a bare alternate resolving to the same path
// documents a DIFFERENT configurable field plus a computed field. Collapsing the
// owner must not silence the alternate's distinct configurable field — it gets
// its own per-attribute move.
func TestLabels_SplitHeadingDistinctConfigInAlternateEmitted(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`outer.notification`" + ` Block

* ` + "`x`" + ` - (Required) X.

### ` + "`notification`" + ` Block

* ` + "`y`" + ` - (Required) Y.
* ` + "`z`" + ` - (Optional) Z.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"outer"}},
		"outer": {Path: "outer", ChildBlocks: []string{"notification"}},
		"outer.notification": {Path: "outer.notification", Attributes: []schema.Attribute{
			{Name: "x", Required: true},
			{Name: "y", Required: true},
			{Name: "z", Computed: true},
		}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	if !hasMsg(results, `block "outer.notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("owner subsection must collapse: %+v", results)
	}
	if !hasMsg(results, `argument "y" in block "notification" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("distinct configurable field in the alternate heading must not be silenced by the owner's collapse: %+v", results)
	}
	if hasMsg(results, `attribute "y"`) && hasMsg(results, "should not have") {
		t.Errorf("the alternate's configurable field must move, not strip: %+v", results)
	}
	if !hasMsg(results, `attribute "z" in block "notification" should not have (Optional) label`) {
		t.Errorf("computed field in the alternate keeps its strip-label: %+v", results)
	}
}

// Review comment 2/4 (+ 3/4 severity): a labeled child-block reference at the
// root must NOT be suppressed when the child's own subsection documents only
// computed/unlabeled fields (emits no move) — the parent reference is then the
// only actionable misplacement. And because it references a nested block, it is
// ERROR, not the WARN reserved for genuine root scalars.
func TestLabels_RootChildRefNotSuppressedWhenChildClean(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`notification`" + ` - (Optional) Notification. See [` + "`notification`" + ` Block](#notification-block).

### ` + "`notification`" + ` Block

* ` + "`last_updated`" + ` - Timestamp of the last update.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"notification"}},
		// notification is a configurable child block, but its subsection here
		// documents only the computed last_updated (no move from the subsection).
		"notification": {Path: "notification", Attributes: []schema.Attribute{
			{Name: "comparison_operator", Required: true},
			{Name: "last_updated", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)

	sev, ok := findSeverity(results, `argument "notification" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`)
	if !ok {
		t.Fatalf("root child-block reference must not be suppressed when the child subsection emits no move: %+v", results)
	}
	if sev != check.SeverityError {
		t.Errorf("a child-block reference is a nested move → ERROR, not WARN; got %v", sev)
	}
	if hasMsg(results, `attribute "last_updated" should not have`) {
		t.Errorf("unlabeled computed field must not be flagged: %+v", results)
	}
}

// Review comment 4/4: a subsection documenting a configurable scalar plus a
// read-only child block must NOT collapse — a wholesale move would drag the
// read-only child documentation into Argument Reference. The scalar gets a
// per-attribute move; the read-only child stays put.
func TestLabels_MixedChildBlockDoesNotCollapse(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`thing`" + ` Block

* ` + "`mode`" + ` - (Required) Mode.
* ` + "`readonly`" + ` - Read-only sub-block. See [` + "`readonly`" + ` Block](#readonly-block).
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"thing"}},
		"thing": {Path: "thing", ChildBlocks: []string{"readonly"}, Attributes: []schema.Attribute{{Name: "mode", Required: true}}},
		// readonly is a child block whose entire subtree is computed-only.
		"thing.readonly": {Path: "thing.readonly", Attributes: []schema.Attribute{{Name: "url", Computed: true}}},
	}}

	results := labelResults(t, src, rs)

	if hasMsg(results, `block "thing" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("a subsection with a read-only child block must not collapse into a wholesale move: %+v", results)
	}
	if !hasMsg(results, `argument "mode" in block "thing" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the configurable scalar must get a per-attribute move: %+v", results)
	}
	if hasMsg(results, `"readonly"`) {
		t.Errorf("the read-only child block must not be moved or stripped: %+v", results)
	}
}

// Review comment 4/4 (ConfigUnknown variant): a documented ConfigUnknown child
// block — whose per-field configurability is unknowable — conservatively blocks
// collapse, so a subsection mixing it with a configurable scalar emits a
// per-attribute move rather than a wholesale subsection move.
func TestLabels_ConfigUnknownChildBlockDoesNotCollapse(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

### ` + "`thing`" + ` Block

* ` + "`mode`" + ` - (Required) Mode.
* ` + "`opaque`" + ` - Object-typed sub-block. See [` + "`opaque`" + ` Block](#opaque-block).
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"thing"}},
		"thing": {Path: "thing", ChildBlocks: []string{"opaque"}, Attributes: []schema.Attribute{{Name: "mode", Required: true}}},
		// opaque is synthesized from an object-typed attribute: per-field flags
		// are unreliable, so it must not be swept into a collapse.
		"thing.opaque": {Path: "thing.opaque", ConfigUnknown: true, Attributes: []schema.Attribute{{Name: "field", Optional: true}}},
	}}

	results := labelResults(t, src, rs)

	if hasMsg(results, `block "thing" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("a ConfigUnknown child block must conservatively block collapse: %+v", results)
	}
	if !hasMsg(results, `argument "mode" in block "thing" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the configurable scalar must still get a per-attribute move: %+v", results)
	}
}
