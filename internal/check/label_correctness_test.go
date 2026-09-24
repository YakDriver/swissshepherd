// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
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

// TestLabelCorrectness_ReadOnlyOnOptional: a (Read-Only) label on a field that
// is Optional in the schema is wrong and must be reported (use (Optional)).
func TestLabelCorrectness_ReadOnlyOnOptional(t *testing.T) {
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

// TestLabelCorrectness_ReadOnlyOnRequired: a (Read-Only) label on a Required
// field must be reported (use (Required)).
func TestLabelCorrectness_ReadOnlyOnRequired(t *testing.T) {
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

// TestLabelCorrectness_ReadOnlyOnComputedOnly: a (Read-Only) label on a
// genuinely read-only (computed-only) attribute is correct — no finding.
func TestLabelCorrectness_ReadOnlyOnComputedOnly(t *testing.T) {
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

// TestLabelCorrectness_ReadOnlyFlagOff_NoCorrectnessFinding: with
// allow_inline_read_only = false, a (Read-Only) label is not an accepted
// argument label, so it takes the missing-label path rather than producing a
// Read-Only correctness finding.
func TestLabelCorrectness_ReadOnlyFlagOff_NoCorrectnessFinding(t *testing.T) {
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

// TestLabelCorrectness_ReadOnlyUnderAttributeReference: a (Read-Only) label on an
// attribute documented under Attribute Reference is not allowed — attributes
// carry no label there — so it is flagged for stripping. This holds regardless
// of allow_inline_read_only (that flag only permits inline Read-Only in
// Argument Reference).
func TestLabelCorrectness_ReadOnlyUnderAttributeReference(t *testing.T) {
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

// TestLabelCorrectness_UnlabeledUnderAttributeReference: a properly unlabeled
// Read-Only attribute under Attribute Reference is correct — no strip finding.
func TestLabelCorrectness_UnlabeledUnderAttributeReference(t *testing.T) {
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

// TestLabelCorrectness_ComputedOnlyMislabeledPermissive: with
// allow_inline_read_only = true, checkComputedMisplacement is suppressed and
// coverage accepts inline computed-only bullets, so labelCorrectness must catch
// a computed-only field labeled (Optional)/(Required) and direct it to
// (Read-Only). (Copilot #71 review.)
func TestLabelCorrectness_ComputedOnlyMislabeledPermissive(t *testing.T) {
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

// TestLabelCorrectness_ComputedOnlyMislabeledStrict: with
// allow_inline_read_only = false, a computed-only field mislabeled in Argument
// Reference is checkComputedMisplacement's concern, so labelCorrectness must not
// emit a (Read-Only) "use" finding (avoid double-reporting).
func TestLabelCorrectness_ComputedOnlyMislabeledStrict(t *testing.T) {
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

// TestLabelCorrectness_ContradictoryLabelNotBypassed guards against a
// contradictory bullet like "(Read-Only, Optional)" — the parser sets a boolean
// per trait, so both fields are true. The documented label must be built from
// all categories so it cannot coincide with the single-valued schema label and
// slip through. (Copilot #71 review.)
func TestLabelCorrectness_ContradictoryLabelNotBypassed(t *testing.T) {
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

// TestLabelCorrectness_AttrRefContradictoryStripsAll: a contradictory bullet
// under Attribute Reference, e.g. "(Required, Read-Only)" on a computed-only
// field, must name every label to remove in one finding so the fix does not
// require a second lint pass. (Copilot #71 review.)
func TestLabelCorrectness_AttrRefContradictoryStripsAll(t *testing.T) {
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

// TestLabelCorrectness_ReadOnlyLabeledConfigurableMoves: a configurable field
// mislabeled (Read-Only) under Attribute Reference must be directed to move to
// Argument Reference (where its label is then corrected), not merely told to
// strip the label — which would leave the misplaced field silently accepted.
// (Copilot #71 review.)
func TestLabelCorrectness_ReadOnlyLabeledConfigurableMoves(t *testing.T) {
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

// TestLabelCorrectness_ComputedOnlyMislabeledCoverageDisabled: with the coverage
// sub-check disabled and labels enabled, checkComputedMisplacement never runs, so
// labels must itself report a computed-only argument mislabeled (Required)/
// (Optional) rather than dropping the only finding. (Copilot #71 review.)
func TestLabelCorrectness_ComputedOnlyMislabeledCoverageDisabled(t *testing.T) {
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
