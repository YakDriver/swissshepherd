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

// TestLabels_OptionalComputedLabeledRequired is the issue #68 repro:
// an Optional+Computed field documented as (Required) must be flagged. Both
// pure Optional and Optional+Computed must read (Optional); only (Required) is
// wrong for an Optional+Computed field.
func TestLabels_OptionalComputedLabeledRequired(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`document_metadata_configuration_updates`" + ` - (Optional) Config. See [` + "`search`" + ` Block](#search-block).

### ` + "`search`" + ` Block

* ` + "`displayable`" + ` - (Required) Whether the field is returned. The default is ` + "`true`" + `.
* ` + "`facetable`" + ` - (Required) Whether the field can be faceted. The default is ` + "`false`" + `.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "document_metadata_configuration_updates", Optional: true}}, ChildBlocks: []string{"search"}},
		"search": {Path: "search", Attributes: []schema.Attribute{
			{Name: "displayable", Optional: true, Computed: true},
			{Name: "facetable", Optional: true, Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)

	if !hasMsg(results, `argument "displayable" in block "search" is labeled (Required) but is optional in the schema; use (Optional)`) {
		t.Errorf("expected correctness finding for displayable; got: %+v", results)
	}
	if !hasMsg(results, `argument "facetable" in block "search" is labeled (Required) but is optional in the schema; use (Optional)`) {
		t.Errorf("expected correctness finding for facetable; got: %+v", results)
	}
}

// TestLabels_PureOptionalLabeledRequired: a pure Optional scalar at
// the root labeled (Required) must be flagged.
func TestLabels_PureOptionalLabeledRequired(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`description`" + ` - (Required) A description.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "description", Optional: true}}},
	}}

	results := labelResults(t, src, rs)

	// Root-level: message omits the block clause.
	if !hasMsg(results, `argument "description" is labeled (Required) but is optional in the schema; use (Optional)`) {
		t.Errorf("expected correctness finding for description; got: %+v", results)
	}
}

// TestLabels_RequiredLabeledOptional: a Required scalar labeled
// (Optional) must be flagged in the other direction.
func TestLabels_RequiredLabeledOptional(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Optional) The name.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}},
	}}

	results := labelResults(t, src, rs)

	if !hasMsg(results, `argument "name" is labeled (Optional) but is required in the schema; use (Required)`) {
		t.Errorf("expected correctness finding for name; got: %+v", results)
	}
}

// TestLabels_CorrectLabels_NoFindings: correct labels (Required ->
// (Required), Optional -> (Optional), Optional+Computed -> (Optional)) produce
// no correctness finding.
func TestLabels_CorrectLabels_NoFindings(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) The name.
* ` + "`description`" + ` - (Optional) A description.
* ` + "`kms_key_id`" + ` - (Optional) KMS key.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "description", Optional: true},
			{Name: "kms_key_id", Optional: true, Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)

	if hasMsg(results, "in the schema; use ") {
		t.Errorf("expected no correctness findings for correct labels; got: %+v", results)
	}
}

// TestLabels_ForcesNewExtra: a trailing trait such as "Forces new
// resource" must not defeat detection — the parser sets Required/Optional from
// the leading token, so an Optional field labeled "(Required, Forces new
// resource)" is still flagged.
func TestLabels_ForcesNewExtra(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`bucket`" + ` - (Required, Forces new resource) Bucket name.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "bucket", Optional: true}}},
	}}

	results := labelResults(t, src, rs)

	if !hasMsg(results, `argument "bucket" is labeled (Required) but is optional in the schema; use (Optional)`) {
		t.Errorf("expected correctness finding despite Forces new resource trait; got: %+v", results)
	}
}

// TestLabels_UnresolvedHeading_NoGuess: an argument under a heading
// that does not resolve to a schema path yields no correctness finding — the
// check never guesses.
func TestLabels_UnresolvedHeading_NoGuess(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name. See [` + "`mystery`" + ` Block](#mystery-block).

### ` + "`mystery`" + ` Block

* ` + "`ghost`" + ` - (Required) Not in schema.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}},
	}}

	results := labelResults(t, src, rs)

	if hasMsg(results, `argument "ghost"`) && hasMsg(results, "in the schema; use ") {
		t.Errorf("must not emit a correctness finding for an unresolved heading; got: %+v", results)
	}
}

// TestLabels_DuplicateNameDoesNotHideWrongLabel guards the issue #68
// review finding: a field listed in BOTH Argument Reference (with a wrong
// label) and Attribute Reference must still be flagged. The template-bleed
// duplicate-name guard suppresses only the missing-label warning for unlabeled
// bleed items; it must not silence the schema-backed correctness check for a
// genuinely labeled argument.
func TestLabels_DuplicateNameDoesNotHideWrongLabel(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`displayable`" + ` - (Required) Whether the field is returned. The default is ` + "`true`" + `.

## Attribute Reference

* ` + "`displayable`" + ` - Whether the field is returned.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "displayable", Optional: true, Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)

	if !hasMsg(results, `argument "displayable" is labeled (Required) but is optional in the schema; use (Optional)`) {
		t.Errorf("duplicate-name guard must not hide a wrong argument label; got: %+v", results)
	}
}

// TestLabels_ComputedOnlyNotFlagged: a computed-only field is out of
// scope for correctness (its placement is checkComputedMisplacement's concern),
// so no "use (...)" correctness finding fires for it.
func TestLabels_ComputedOnlyNotFlagged(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`arn`" + ` - (Optional) ARN.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "arn", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)

	if hasMsg(results, `argument "arn"`) && hasMsg(results, "in the schema; use ") {
		t.Errorf("computed-only field must not get a correctness finding; got: %+v", results)
	}
}

// --- Gap A: (Read-Only) label correctness in Argument Reference ---

// labelResultsRO runs the labels checks with allow_inline_read_only = true, so a
// (Read-Only) label inline in Argument Reference is permitted and correctness
// applies to it.
func labelResultsRO(t *testing.T, src string, rs *schema.ResourceSchema) []check.Result {
	t.Helper()
	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", labelTemplates)
	if err != nil {
		t.Fatal(err)
	}
	ro := true
	return (&check.SchemaDocsRule{IgnoreDeprecated: true, AllowInlineReadOnly: &ro}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
}

// TestLabels_ReadOnlyOnOptional: a (Read-Only) label on a field that
// is Optional in the schema is wrong and must be reported (use (Optional)).
func TestLabels_ReadOnlyOnOptional(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`endpoint`" + ` - (Read-Only) Endpoint address.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "endpoint", Optional: true},
		}},
	}}

	results := labelResultsRO(t, src, rs)
	if !hasMsg(results, `argument "endpoint" is labeled (Read-Only) but is optional in the schema; use (Optional)`) {
		t.Errorf("expected Read-Only correctness finding for endpoint; got: %+v", results)
	}
}

// TestLabels_ReadOnlyOnRequired: a (Read-Only) label on a Required
// field must be reported (use (Required)).
func TestLabels_ReadOnlyOnRequired(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Read-Only) The name.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}},
	}}

	results := labelResultsRO(t, src, rs)
	if !hasMsg(results, `argument "name" is labeled (Read-Only) but is required in the schema; use (Required)`) {
		t.Errorf("expected Read-Only correctness finding for name; got: %+v", results)
	}
}

// TestLabels_ReadOnlyOnComputedOnly: a (Read-Only) label on a
// genuinely read-only (computed-only) attribute is correct — no finding.
func TestLabels_ReadOnlyOnComputedOnly(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`arn`" + ` - (Read-Only) ARN of the thing.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "arn", Computed: true},
		}},
	}}

	results := labelResultsRO(t, src, rs)
	if hasMsg(results, `argument "arn"`) && hasMsg(results, "in the schema; use ") {
		t.Errorf("computed-only field labeled (Read-Only) must not be flagged; got: %+v", results)
	}
}

// TestLabels_ReadOnlyFlagOff_NoCorrectnessFinding: with
// allow_inline_read_only = false, a (Read-Only) label is not an accepted
// argument label, so it takes the missing-label path rather than producing a
// Read-Only correctness finding.
func TestLabels_ReadOnlyFlagOff_NoCorrectnessFinding(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`endpoint`" + ` - (Read-Only) Endpoint address.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "endpoint", Optional: true},
		}},
	}}

	// Default rule: allow_inline_read_only is false.
	results := labelResults(t, src, rs)
	if hasMsg(results, "is labeled (Read-Only) but is") {
		t.Errorf("no Read-Only correctness finding expected when allow_inline_read_only is false; got: %+v", results)
	}
}

// --- Gap B: (Read-Only) label not allowed under Attribute Reference ---

// TestLabels_ReadOnlyUnderAttributeReference: a (Read-Only) label on an
// attribute documented under Attribute Reference is not allowed — attributes
// carry no label there — so it is flagged for stripping. This holds regardless
// of allow_inline_read_only (that flag only permits inline Read-Only in
// Argument Reference).
func TestLabels_ReadOnlyUnderAttributeReference(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`arn`" + ` - (Read-Only) ARN of the thing.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "arn", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `attribute "arn" in block "(root)" should not have (Read-Only) label`) {
		t.Errorf("expected strip finding for (Read-Only) label under Attribute Reference; got: %+v", results)
	}
}

// TestLabels_UnlabeledUnderAttributeReference: a properly unlabeled
// Read-Only attribute under Attribute Reference is correct — no strip finding.
func TestLabels_UnlabeledUnderAttributeReference(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`arn`" + ` - ARN of the thing.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "arn", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, `attribute "arn"`) && hasMsg(results, "should not have") {
		t.Errorf("unlabeled Read-Only attribute must not be flagged; got: %+v", results)
	}
}

// --- Gap A2: computed-only mislabeled (Required)/(Optional) in permissive mode ---

// TestLabels_ComputedOnlyMislabeledPermissive: with
// allow_inline_read_only = true, checkComputedMisplacement is suppressed and
// coverage accepts inline computed-only bullets, so labelCorrectness must catch
// a computed-only field labeled (Optional)/(Required) and direct it to
// (Read-Only). (Copilot #71 review.)
func TestLabels_ComputedOnlyMislabeledPermissive(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`status`" + ` - (Optional) Current status.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "status", Computed: true},
		}},
	}}

	results := labelResultsRO(t, src, rs)
	if !hasMsg(results, `argument "status" is labeled (Optional) but is read-only in the schema; use (Read-Only)`) {
		t.Errorf("expected (Read-Only) correctness finding for computed-only status in permissive mode; got: %+v", results)
	}
}

// TestLabels_ComputedOnlyMislabeledStrict: with
// allow_inline_read_only = false, a computed-only field mislabeled in Argument
// Reference is checkComputedMisplacement's concern, so labelCorrectness must not
// emit a (Read-Only) "use" finding (avoid double-reporting).
func TestLabels_ComputedOnlyMislabeledStrict(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`status`" + ` - (Optional) Current status.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "status", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "use (Read-Only)") {
		t.Errorf("strict mode must defer computed-only mislabel to checkComputedMisplacement; got: %+v", results)
	}
}

// TestLabels_ContradictoryLabelNotBypassed guards against a
// contradictory bullet like "(Read-Only, Optional)" — the parser sets a boolean
// per trait, so both fields are true. The documented label must be built from
// all categories so it cannot coincide with the single-valued schema label and
// slip through. (Copilot #71 review.)
func TestLabels_ContradictoryLabelNotBypassed(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`endpoint`" + ` - (Read-Only, Optional) Endpoint address.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "endpoint", Optional: true},
		}},
	}}

	results := labelResultsRO(t, src, rs)
	if !hasMsg(results, `argument "endpoint" is labeled (Optional), (Read-Only) but is optional in the schema; use (Optional)`) {
		t.Errorf("contradictory (Read-Only, Optional) label must be flagged, not bypassed; got: %+v", results)
	}
}

// TestLabels_AttrRefContradictoryStripsAll: a contradictory bullet
// under Attribute Reference, e.g. "(Required, Read-Only)" on a computed-only
// field, must name every label to remove in one finding so the fix does not
// require a second lint pass. (Copilot #71 review.)
func TestLabels_AttrRefContradictoryStripsAll(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`arn`" + ` - (Required, Read-Only) ARN.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "arn", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `attribute "arn" in block "(root)" should not have (Required), (Read-Only) labels`) {
		t.Errorf("contradictory Attribute Reference label must list all categories in one finding; got: %+v", results)
	}
}

// TestLabels_ReadOnlyLabeledConfigurableMoves: a configurable field
// mislabeled (Read-Only) under Attribute Reference must be directed to move to
// Argument Reference (where its label is then corrected), not merely told to
// strip the label — which would leave the misplaced field silently accepted.
// (Copilot #71 review.)
func TestLabels_ReadOnlyLabeledConfigurableMoves(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`arn`" + ` - ARN.
* ` + "`bucket`" + ` - (Read-Only) Bucket name.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "bucket", Required: true},
			{Name: "arn", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `argument "bucket" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("configurable (Read-Only)-labeled field must get move guidance; got: %+v", results)
	}
	// It must not be told merely to strip the label and left in place.
	if hasMsg(results, `attribute "bucket"`) && hasMsg(results, "should not have") {
		t.Errorf("configurable field must move, not strip-and-stay; got: %+v", results)
	}
}

// TestLabels_ComputedOnlyMislabeledCoverageDisabled: with the coverage
// sub-check disabled and labels enabled, checkComputedMisplacement never runs, so
// labels must itself report a computed-only argument mislabeled (Required)/
// (Optional) rather than dropping the only finding. (Copilot #71 review.)
func TestLabels_ComputedOnlyMislabeledCoverageDisabled(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`status`" + ` - (Optional) Current status.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "status", Computed: true},
		}},
	}}

	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", labelTemplates)
	if err != nil {
		t.Fatal(err)
	}
	no := false
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true, Coverage: &no}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	if !hasMsg(results, `argument "status" is labeled (Optional) but is computed-only in the schema; move it to Attribute Reference and remove the label`) {
		t.Errorf("labels must report computed-only mislabel when coverage is disabled; got: %+v", results)
	}
}

// TestLabels_NoFalseLabelsWarning verifies that attributes in the
// Attribute Reference section are NOT flagged for missing (Required)/(Optional)
// labels, even when broad heading templates cause them to appear in ArgumentBlocks.
func TestLabels_NoFalseLabelsWarning(t *testing.T) {
	t.Parallel()

	templates := doc.HeadingTemplates{"`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := []byte(`# Data Source: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name of the thing.

## Attribute Reference

* ` + "`arn`" + ` - ARN of the thing.
* ` + "`created_date`" + ` - Creation date.
* ` + "`last_updated_date`" + ` - Last update date.
`)

	d, err := doc.ParseWithTemplates(src, "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_thing", Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "is missing (Required) or (Optional) label") {
			if strings.Contains(r.Message, "arn") || strings.Contains(r.Message, "created_date") || strings.Contains(r.Message, "last_updated_date") {
				t.Errorf("false positive label warning for attribute-section item: %s", r.Message)
			}
		}
	}
}

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
// the schema. Guards the false positive found in TestCoverage_FixtureComplete.
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

// A real subsection heading PRECEDED by a dot-notation reference to the same
// block (which first creates the map entry with an empty Heading) must still be
// classified — never strip-labeled. Because the pre-heading reference bullet
// lives at a different physical location than the heading, the entry must NOT
// collapse into a single "move this subsection" (that move would not relocate
// the earlier bullet); each configurable field gets its own per-attribute move.
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
	// No wholesale collapse — it would leave the pre-heading reference bullet behind.
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("a block spanning a pre-heading reference must not collapse: %+v", results)
	}
	// Both configurable fields are individually flagged as moves.
	if !hasMsg(results, `argument "comparison_operator" in block "notification" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the pre-heading reference bullet must get its own move: %+v", results)
	}
	if !hasMsg(results, `argument "threshold" in block "notification" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the heading's configurable field must get a move: %+v", results)
	}
	if hasMsg(results, "should not have") {
		t.Errorf("must not emit strip-label warnings for the block's configurable args: %+v", results)
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
	if hasMsg(results, `attribute "y" in block "notification" should not have`) {
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

// A subsection that documents a valid misplaced configurable field alongside a
// computed-only field written with a malformed bullet (stored only in
// MalformedAttributes) must NOT collapse: a "move this subsection" would drag
// the computed output into Argument Reference. The malformed computed field
// pins the subsection, forcing a per-attribute move of the configurable field
// only.
func TestLabels_MalformedComputedBlocksCollapse(t *testing.T) {
	t.Parallel()

	// `arn` uses an en-dash separator instead of " - ", so parseListItem
	// rejects it and it lands only in MalformedAttributes.
	src := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"### `logging` Block\n\n" +
		"* `enabled` - (Required) Whether logging is enabled.\n" +
		"* `arn` \u2013 ARN of the logging config.\n"

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"logging"}},
		"logging": {Path: "logging", Attributes: []schema.Attribute{
			{Name: "enabled", Required: true},
			{Name: "arn", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("a subsection with a malformed computed field must not collapse: %+v", results)
	}
	if !hasMsg(results, `argument "enabled" in block "logging" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the configurable field must still get a per-attribute move: %+v", results)
	}
}

// A combined heading (`### `foo` and `bar“) mirrors one physical subsection to
// alias blocks that resolve independently. When one alias is a configurable
// block and another is computed/read-only, collapsing the whole subsection would
// drag the computed alias's content into Argument Reference. The group must not
// collapse; the configurable alias gets a per-attribute move instead.
func TestLabels_MixedAliasGroupDoesNotCollapse(t *testing.T) {
	t.Parallel()

	src := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"### `foo` and `bar`\n\n" +
		"* `enabled` - (Required) Whether enabled.\n"

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":    {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"foo", "bar"}},
		"foo": {Path: "foo", Attributes: []schema.Attribute{{Name: "enabled", Required: true}}},
		"bar": {Path: "bar", Attributes: []schema.Attribute{{Name: "enabled", Computed: true}}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("a mixed alias group must not collapse the shared subsection: %+v", results)
	}
	if !hasMsg(results, `argument "enabled" in block "foo" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the configurable alias must still get a per-attribute move: %+v", results)
	}
}

// A collapsing subsection must not also emit a strip-label finding for an
// Optional+Computed field it contains: the collapse moves the whole subsection
// to Argument Reference, where that field is a valid argument and must keep its
// (Optional) label. Emitting both would create a new labels error.
func TestLabels_CollapseSuppressesOptionalComputedStrip(t *testing.T) {
	t.Parallel()

	src := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"### `settings` Block\n\n" +
		"* `mode` - (Optional) Mode.\n" +
		"* `state` - (Optional) State.\n"

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"settings"}},
		"settings": {Path: "settings", Attributes: []schema.Attribute{
			{Name: "mode", Optional: true},                  // pure config -> drives the collapse
			{Name: "state", Optional: true, Computed: true}, // Optional+Computed -> would strip
		}},
	}}

	results := labelResults(t, src, rs)
	if !hasMsg(results, `block "settings" is documented under Attribute Reference but is a configurable argument block`) {
		t.Fatalf("expected the subsection to collapse: %+v", results)
	}
	if hasMsg(results, "should not have") {
		t.Errorf("a collapsing subsection must not also strip an Optional+Computed field's label: %+v", results)
	}
}

// Two headings that normalize to the same block key merge into one DocBlock that
// keeps only the first HeadingLine. Collapsing it would emit a single "move this
// subsection" at the first heading, leaving the second physical subsection
// misplaced. The merged entry must be treated as spanning multiple subsections,
// so each configurable field gets its own per-attribute move instead.
func TestLabels_RepeatedHeadingDoesNotCollapse(t *testing.T) {
	t.Parallel()

	src := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"### `foo` Block\n\n" +
		"* `alpha` - (Required) Alpha.\n\n" +
		"### `foo` Block\n\n" +
		"* `beta` - (Required) Beta.\n"

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"foo"}},
		"foo": {Path: "foo", Attributes: []schema.Attribute{
			{Name: "alpha", Required: true},
			{Name: "beta", Required: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("a block documented under repeated headings must not collapse: %+v", results)
	}
	if !hasMsg(results, `argument "alpha" in block "foo" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the first subsection's field must get a per-attribute move: %+v", results)
	}
	if !hasMsg(results, `argument "beta" in block "foo" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the second subsection's field must get a per-attribute move: %+v", results)
	}
}

// A labeled root bullet that references a skip_blocks target (e.g. the default
// "timeouts") must not produce a move: the subsection path is "" (not skipped),
// but the resolved target is "timeouts". Filtering only the path would emit a
// spurious ERROR for a block documented as skipped entirely.
func TestLabels_SkippedTargetRootBulletNotMoved(t *testing.T) {
	t.Parallel()

	src := `# Resource: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

* ` + "`timeouts`" + ` - (Optional) Timeouts.
`
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":         {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"timeouts"}},
		"timeouts": {Path: "timeouts", Attributes: []schema.Attribute{{Name: "create", Optional: true}}},
	}}

	// Default SkipBlocks includes "timeouts".
	results := labelResults(t, src, rs)
	if hasMsg(results, "move it to Argument Reference") || hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("a root bullet referencing a skip_blocks target must not produce a move: %+v", results)
	}
}

// In a combined heading the parser mirrors valid attributes to every alias but
// records a malformed bullet only on the primary alias. A malformed computed-only
// field that is computed under a NON-primary alias must still pin that alias, so
// the shared subsection does not collapse and drag the computed output into
// Argument Reference.
func TestLabels_MalformedComputedOnAliasBlocksCollapse(t *testing.T) {
	t.Parallel()

	// `arn` uses a missing-dash form -> parseListItem rejects it -> it lands only
	// in the primary alias (foo) MalformedAttributes. It is computed under bar.
	src := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"### `foo` and `bar`\n\n" +
		"* `enabled` - (Required) Enabled.\n" +
		"* `arn` (Read-Only) ARN.\n"

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":    {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"foo", "bar"}},
		"foo": {Path: "foo", Attributes: []schema.Attribute{{Name: "enabled", Required: true}}},
		"bar": {Path: "bar", Attributes: []schema.Attribute{
			{Name: "enabled", Required: true},
			{Name: "arn", Computed: true},
		}},
	}}

	results := labelResults(t, src, rs)
	if hasMsg(results, "move this subsection to Argument Reference") {
		t.Errorf("a malformed computed field on a non-primary alias must block the group collapse: %+v", results)
	}
	if !hasMsg(results, `argument "enabled" in block "foo" is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference`) {
		t.Errorf("the configurable field must still get a per-attribute move: %+v", results)
	}
}

// TestLabels_BudgetFixtureMisplacement drives the misplacement rule end-to-end
// on the frozen pre-fix aws_budgets_budget doc — the #60 flagship case. It pins
// the two behaviors the fixture exercises cleanly:
//
//   - Purely configurable blocks whose headings resolve to a schema block
//     (### Cost Filter → cost_filter, ### Filter Expression → filter_expression)
//     collapse to ONE "move this subsection" (ERROR), with no per-attribute noise
//     and no strip-label on their configurable fields — the core #60 fix.
//   - A prose-headed subsection that fails schema resolution (### Budget
//     Notification vs schema `notification`) falls back to the legacy
//     strip-label — the accepted, measured residual (§2, §12), never a move.
//
// (The auto_adjust_data / historical_options subsections in this doc use a
// non-canonical "`name` (Required) -" label form that the parser treats as
// malformed, so they do not drive misplacement; the mixed-block per-attribute
// behavior is covered by TestLabels_MixedBlockPerAttributeMoveKeepsComputedStrip.)
func TestLabels_BudgetFixtureMisplacement(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/budgets_budget_misplaced.html.markdown")
	if err != nil {
		t.Fatal(err)
	}

	// Schema mirrors the budgets_budget blocks the fixture documents with
	// canonical labels. filter_expression's operands are modeled as configurable
	// scalars — the test only needs them to be pure-config so the subsection
	// collapses.
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {ChildBlocks: []string{"cost_filter", "filter_expression", "notification"}},
		"cost_filter": {Path: "cost_filter", Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "values", Required: true},
		}},
		"filter_expression": {Path: "filter_expression", Attributes: []schema.Attribute{
			{Name: "and", Optional: true},
			{Name: "or", Optional: true},
			{Name: "not", Optional: true},
			{Name: "dimensions", Optional: true},
			{Name: "tags", Optional: true},
			{Name: "cost_categories", Optional: true},
		}},
		// Configurable, but documented under the prose heading "Budget
		// Notification", which does not resolve to this schema name.
		"notification": {Path: "notification", Attributes: []schema.Attribute{
			{Name: "comparison_operator", Required: true},
			{Name: "notification_type", Required: true},
			{Name: "threshold", Required: true},
			{Name: "threshold_type", Required: true},
			{Name: "subscriber_email_addresses", Optional: true},
			{Name: "subscriber_sns_topic_arns", Optional: true},
		}},
	}}

	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_budgets_budget", Schema: rs, Doc: d})

	has := func(sub string) bool { return hasMsg(results, sub) }

	// 1. Pure-config blocks collapse to one subsection move (ERROR).
	for _, block := range []string{"cost_filter", "filter_expression"} {
		msg := `block "` + block + `" is documented under Attribute Reference but is a configurable argument block in the schema; move this subsection to Argument Reference`
		if sev, ok := findSeverity(results, msg); !ok || sev != check.SeverityError {
			t.Errorf("%s collapse: ok=%v sev=%v, want ERROR; results=%+v", block, ok, sev, results)
		}
	}

	// 2. A collapsed block emits no per-attribute noise and no strip-label on its
	//    configurable fields (the collapse covers them).
	for _, bad := range []string{
		`attribute "name" in block "cost_filter" should not have`,
		`attribute "values" in block "cost_filter" should not have`,
		`in block "cost_filter" is documented under Attribute Reference but is a configurable argument in the schema`,
		`attribute "and" in block "filter_expression" should not have`,
		`in block "filter_expression" is documented under Attribute Reference but is a configurable argument in the schema`,
	} {
		if has(bad) {
			t.Errorf("collapse must suppress per-attribute/strip noise (%q): %+v", bad, results)
		}
	}

	// 3. Prose-headed unresolved subsection falls back to strip-label (accepted
	//    residual, §2/§12) — never a move.
	if !has(`attribute "comparison_operator" in block "budget_notification" should not have (Required) label`) {
		t.Errorf("prose-headed budget_notification should retain legacy strip-label: %+v", results)
	}
	if has(`in block "budget_notification" is documented under Attribute Reference but is a configurable argument`) ||
		has(`block "budget_notification" is documented under Attribute Reference but is a configurable argument block`) {
		t.Errorf("an unresolved prose heading must never produce a move: %+v", results)
	}
}

// TestLabels_ServedPaths: a section is label-checked against every path it
// serves, so shared and partly qualified sections are checked too (#80).
// Root -> a -> a.z and root -> b -> b.z; w -> w.a -> w.a.z for qualified keys.
func TestLabels_ServedPaths(t *testing.T) {
	t.Parallel()

	block := func(attrs ...schema.Attribute) *schema.Block { return &schema.Block{Attributes: attrs} }
	req := func(n string) schema.Attribute { return schema.Attribute{Name: n, Required: true} }
	opt := func(n string) schema.Attribute { return schema.Attribute{Name: n, Optional: true} }
	ro := func(n string) schema.Attribute { return schema.Attribute{Name: n, Computed: true} }
	twoZ := func(az, bz *schema.Block) *schema.ResourceSchema {
		return &schema.ResourceSchema{Name: "aws_thing", Blocks: map[string]*schema.Block{
			"":    {Attributes: []schema.Attribute{ro("id")}, ChildBlocks: []string{"a", "b"}},
			"a":   {Attributes: []schema.Attribute{opt("n")}, ChildBlocks: []string{"z"}},
			"b":   {Attributes: []schema.Attribute{opt("n")}, ChildBlocks: []string{"z"}},
			"a.z": az,
			"b.z": bz,
		}}
	}
	const parents = "## Argument Reference\n\n* `a` - (Optional) A.\n* `b` - (Optional) B.\n\n" +
		"### `a` Block\n\n* `n` - (Optional) N.\n* `z` - (Optional) Z.\n\n" +
		"### `b` Block\n\n* `n` - (Optional) N.\n* `z` - (Optional) Z.\n\n"
	const attrs = "\n## Attribute Reference\n\n* `id` - ID.\n"
	on, off := true, false

	testCases := map[string]struct {
		rs       *schema.ResourceSchema
		md       string
		coverage *bool
		inline   *bool
		want     []string // substrings of labels findings, in order
		also     string   // a substring some other finding must contain
	}{
		"shared section, label wrong at every path": {
			rs:   twoZ(block(req("x")), block(req("x"))),
			md:   parents + "### `z` Block\n\n* `x` - (Optional) X.\n" + attrs,
			want: []string{`argument "x" in block "z" is labeled (Optional) but is required in the schema; use (Required)`},
		},
		"shared section, label right at every path": {
			rs: twoZ(block(req("x")), block(req("x"))),
			md: parents + "### `z` Block\n\n* `x` - (Required) X.\n" + attrs,
		},
		"partly qualified key": {
			rs: &schema.ResourceSchema{Name: "aws_thing", Blocks: map[string]*schema.Block{
				"":      {Attributes: []schema.Attribute{ro("id")}, ChildBlocks: []string{"w", "z"}},
				"w":     {Attributes: []schema.Attribute{opt("n")}, ChildBlocks: []string{"a"}},
				"w.a":   {Attributes: []schema.Attribute{opt("n")}, ChildBlocks: []string{"z"}},
				"w.a.z": block(opt("x")),
				"z":     block(req("x")),
			}},
			md: "## Argument Reference\n\n* `w` - (Optional) W.\n* `z` - (Optional) Z.\n\n### `w` Block\n\n* `n` - (Optional) N.\n* `a` - (Optional) A.\n\n" +
				"### `w.a` Block\n\n* `n` - (Optional) N.\n* `z` - (Optional) Z.\n\n### `a.z` Block\n\n* `x` - (Required) X.\n\n### `z` Block\n\n* `x` - (Required) X.\n" + attrs,
			want: []string{`argument "x" in block "a.z" is labeled (Required) but is optional in the schema; use (Optional)`},
		},
		// The shared-section finding reports the conflict once; per-path
		// label findings would contradict each other.
		"paths disagree": {
			rs:   twoZ(block(req("x")), block(opt("x"))),
			md:   parents + "### `z` Block\n\n* `x` - (Optional) X.\n" + attrs,
			also: `"x" is (Required) at "a.z" and (Optional) at "b.z"`,
		},
		"paths disagree, coverage off": {
			rs:       twoZ(block(req("x")), block(opt("x"))),
			md:       parents + "### `z` Block\n\n* `x` - (Optional) X.\n" + attrs,
			coverage: &off,
		},
		// A ConfigUnknown block's labels are unknowable, so only the known
		// path decides.
		"one path ConfigUnknown": {
			rs:   twoZ(block(req("x")), &schema.Block{Attributes: []schema.Attribute{ro("x")}, ConfigUnknown: true}),
			md:   parents + "### `z` Block\n\n* `x` - (Optional) X.\n" + attrs,
			want: []string{`argument "x" in block "z" is labeled (Optional) but is required in the schema`},
		},
		"every path ConfigUnknown": {
			rs: twoZ(&schema.Block{Attributes: []schema.Attribute{ro("x")}, ConfigUnknown: true}, &schema.Block{Attributes: []schema.Attribute{ro("x")}, ConfigUnknown: true}),
			md: parents + "### `z` Block\n\n* `x` - (Optional) X.\n" + attrs,
		},
		// Strict mode: Read-Only coverage reports the move for each path.
		"computed-only in a shared section": {
			rs:   twoZ(block(opt("x"), ro("r")), block(opt("x"), ro("r"))),
			md:   parents + "### `z` Block\n\n* `x` - (Optional) X.\n* `r` - (Optional) R.\n" + attrs,
			also: `Read-Only attribute "r" in block "b.z" should be documented in Attribute Reference section`,
		},
		"computed-only in a shared section, coverage off": {
			rs:       twoZ(block(opt("x"), ro("r")), block(opt("x"), ro("r"))),
			md:       parents + "### `z` Block\n\n* `x` - (Optional) X.\n* `r` - (Optional) R.\n" + attrs,
			coverage: &off,
			want:     []string{`argument "r" in block "z" is labeled (Optional) but is computed-only in the schema; move it to Attribute Reference and remove the label`},
		},
		// Attribute Reference documents it at every path, so coverage is
		// satisfied and nothing else reports the labeled duplicate.
		"computed-only also in Attribute Reference": {
			rs:   twoZ(block(opt("x"), ro("r")), block(opt("x"), ro("r"))),
			md:   parents + "### `z` Block\n\n* `x` - (Optional) X.\n* `r` - (Optional) R.\n" + attrs + "\n### `z` Block\n\n* `r` - R.\n",
			want: []string{`argument "r" in block "z" is labeled (Optional) but is computed-only in the schema; Attribute Reference already documents it, so remove it from Argument Reference`},
		},
		"computed-only, inline Read-Only allowed": {
			rs:     twoZ(block(opt("x"), ro("r")), block(opt("x"), ro("r"))),
			md:     parents + "### `z` Block\n\n* `x` - (Optional) X.\n* `r` - (Optional) R.\n" + attrs,
			inline: &on,
			want:   []string{`argument "r" in block "z" is labeled (Optional) but is read-only in the schema; use (Read-Only)`},
		},
		// Several headings for one key: the fit rule compares each heading's
		// labels, so labels doesn't compare their merged bullets.
		"duplicate headings": {
			rs:   twoZ(block(req("x")), block(req("x"), opt("y"))),
			md:   parents + "### `z` Block\n\n* `x` - (Optional) X.\n\n### `z` Block\n\n* `x` - (Required) X.\n* `y` - (Optional) Y.\n" + attrs,
			also: `documents block "a.z" exactly; the closest, at line 16: "x" should be (Required)`,
		},
		"duplicate headings, coverage off": {
			rs:       twoZ(block(req("x")), block(req("x"))),
			md:       parents + "### `z` Block\n\n* `x` - (Optional) X.\n\n### `z` Block\n\n* `x` - (Required) X.\n" + attrs,
			coverage: &off,
			want:     []string{`argument "x" in block "z" is labeled (Optional) but is required in the schema`},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.ParseWithTemplates([]byte(tc.md), "aws_thing", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
			if err != nil {
				t.Fatal(err)
			}
			rule := check.SchemaDocsRule{Ordering: &off, Description: &off, Format: &off, Byline: &off, Heading: &off, Coverage: tc.coverage, AllowInlineReadOnly: tc.inline}
			results := rule.Check(check.CheckContext{Resource: "aws_thing", Schema: tc.rs, Doc: d})
			var got []string
			for _, r := range results {
				if strings.Contains(r.Message, " is labeled ") {
					got = append(got, r.Message)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("labels findings = %q, want %d matching %q; all:\n  %s", got, len(tc.want), tc.want, joinMessages(results))
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("finding %d = %q, want it to contain %q", i, got[i], w)
				}
			}
			if tc.also != "" && !hasMsg(results, tc.also) {
				t.Errorf("missing %q in:\n  %s", tc.also, joinMessages(results))
			}
		})
	}
}
