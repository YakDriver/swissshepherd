// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/schema"
)

// TestLabelCorrectness_OptionalComputedLabeledRequired is the issue #68 repro:
// an Optional+Computed field documented as (Required) must be flagged. Both
// pure Optional and Optional+Computed must read (Optional); only (Required) is
// wrong for an Optional+Computed field.
func TestLabelCorrectness_OptionalComputedLabeledRequired(t *testing.T) {
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

// TestLabelCorrectness_PureOptionalLabeledRequired: a pure Optional scalar at
// the root labeled (Required) must be flagged.
func TestLabelCorrectness_PureOptionalLabeledRequired(t *testing.T) {
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

// TestLabelCorrectness_RequiredLabeledOptional: a Required scalar labeled
// (Optional) must be flagged in the other direction.
func TestLabelCorrectness_RequiredLabeledOptional(t *testing.T) {
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

// TestLabelCorrectness_CorrectLabels_NoFindings: correct labels (Required ->
// (Required), Optional -> (Optional), Optional+Computed -> (Optional)) produce
// no correctness finding.
func TestLabelCorrectness_CorrectLabels_NoFindings(t *testing.T) {
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

// TestLabelCorrectness_ForcesNewExtra: a trailing trait such as "Forces new
// resource" must not defeat detection — the parser sets Required/Optional from
// the leading token, so an Optional field labeled "(Required, Forces new
// resource)" is still flagged.
func TestLabelCorrectness_ForcesNewExtra(t *testing.T) {
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

// TestLabelCorrectness_UnresolvedHeading_NoGuess: an argument under a heading
// that does not resolve to a schema path yields no correctness finding — the
// check never guesses.
func TestLabelCorrectness_UnresolvedHeading_NoGuess(t *testing.T) {
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

// TestLabelCorrectness_DuplicateNameDoesNotHideWrongLabel guards the issue #68
// review finding: a field listed in BOTH Argument Reference (with a wrong
// label) and Attribute Reference must still be flagged. The template-bleed
// duplicate-name guard suppresses only the missing-label warning for unlabeled
// bleed items; it must not silence the schema-backed correctness check for a
// genuinely labeled argument.
func TestLabelCorrectness_DuplicateNameDoesNotHideWrongLabel(t *testing.T) {
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

// TestLabelCorrectness_ComputedOnlyNotFlagged: a computed-only field is out of
// scope for correctness (its placement is checkComputedMisplacement's concern),
// so no "use (...)" correctness finding fires for it.
func TestLabelCorrectness_ComputedOnlyNotFlagged(t *testing.T) {
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
