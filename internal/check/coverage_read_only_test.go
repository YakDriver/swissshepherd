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

// TestSchemaDocsRule_NestedReadOnly_MissingFromBoth_Errors closes the gap
// where a Read-Only (computed-only) attribute on a nested block is silently
// undocumented. The schema declares network.private_ip; the doc has neither
// a nested attr-block heading for it nor a dot-notation reference at root.
// swissshepherd must report it.
func TestSchemaDocsRule_NestedReadOnly_MissingFromBoth_Errors(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"network"},
			},
			"network": {
				Attributes: []schema.Attribute{
					{Name: "subnet_id", Required: true},
					{Name: "private_ip", Computed: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `network` - (Required) Network configuration.\n\n" +
		"### `network` Block\n\n" +
		"* `subnet_id` - (Required) Subnet identifier.\n\n" +
		"## Attribute Reference\n\n" +
		"* `arn` - ARN of the resource.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	want := `Read-Only attribute "private_ip" in block "network" should be documented in Attribute Reference section`
	found := false
	for _, r := range results {
		if strings.Contains(r.Message, want) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected message containing %q, got:\n  %s",
			want, joinMessages(results))
	}
}

// TestSchemaDocsRule_NestedReadOnly_DotNotation_Passes confirms the existing
// AWS provider convention works: a Read-Only nested attribute documented as
// `network[*].private_ip` in the root Attribute Reference satisfies coverage.
func TestSchemaDocsRule_NestedReadOnly_DotNotation_Passes(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"network"},
			},
			"network": {
				Attributes: []schema.Attribute{
					{Name: "subnet_id", Required: true},
					{Name: "private_ip", Computed: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `network` - (Required) Network configuration.\n\n" +
		"### `network` Block\n\n" +
		"* `subnet_id` - (Required) Subnet identifier.\n\n" +
		"## Attribute Reference\n\n" +
		"* `arn` - ARN of the resource.\n" +
		"* `network[*].private_ip` - Private IP address.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "private_ip") {
			t.Errorf("unexpected message about private_ip: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_NestedReadOnly_NestedBlockHeading_Passes confirms the
// alternative form: a `### \`network\` Block` heading inside Attribute
// Reference with private_ip listed beneath also satisfies coverage.
func TestSchemaDocsRule_NestedReadOnly_NestedBlockHeading_Passes(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"network"},
			},
			"network": {
				Attributes: []schema.Attribute{
					{Name: "subnet_id", Required: true},
					{Name: "private_ip", Computed: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `network` - (Required) Network configuration.\n\n" +
		"### `network` Block\n\n" +
		"* `subnet_id` - (Required) Subnet identifier.\n\n" +
		"## Attribute Reference\n\n" +
		"* `arn` - ARN of the resource.\n\n" +
		"### `network` Block\n\n" +
		"* `private_ip` - Private IP address.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "private_ip") {
			t.Errorf("unexpected message about private_ip: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_NestedReadOnly_InlineToggleOff_Errors confirms that a
// (Read-Only) label inline in Argument Reference is NOT accepted as
// documentation when the toggle is off — coverage still requires it in
// Attribute Reference. (Misplacement warning is exercised in step 4.)
func TestSchemaDocsRule_NestedReadOnly_InlineToggleOff_Errors(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"network"},
			},
			"network": {
				Attributes: []schema.Attribute{
					{Name: "subnet_id", Required: true},
					{Name: "private_ip", Computed: true},
				},
			},
		},
	}

	// Alphabetic order: private_ip before subnet_id. Group ordering is
	// not enforced in this phase.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `network` - (Required) Network configuration.\n\n" +
		"### `network` Block\n\n" +
		"* `private_ip` - (Read-Only) Private IP address.\n" +
		"* `subnet_id` - (Required) Subnet identifier.\n\n" +
		"## Attribute Reference\n\n" +
		"* `arn` - ARN of the resource.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	want := `Read-Only attribute "private_ip" in block "network" should be documented in Attribute Reference section`
	found := false
	for _, r := range results {
		if strings.Contains(r.Message, want) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected message containing %q, got:\n  %s",
			want, joinMessages(results))
	}
}

// TestSchemaDocsRule_NestedReadOnly_InlineToggleOn_Passes confirms that with
// the toggle on, a (Read-Only) label inline in Argument Reference satisfies
// coverage — the tfplugindocs-aligned permissive convention.
func TestSchemaDocsRule_NestedReadOnly_InlineToggleOn_Passes(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"network"},
			},
			"network": {
				Attributes: []schema.Attribute{
					{Name: "subnet_id", Required: true},
					{Name: "private_ip", Computed: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `network` - (Required) Network configuration.\n\n" +
		"### `network` Block\n\n" +
		"* `private_ip` - (Read-Only) Private IP address.\n" +
		"* `subnet_id` - (Required) Subnet identifier.\n\n" +
		"## Attribute Reference\n\n" +
		"* `arn` - ARN of the resource.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	enabled := true
	rule := &check.SchemaDocsRule{AllowInlineReadOnly: &enabled}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "private_ip") {
			t.Errorf("unexpected message about private_ip with toggle on: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_TopLevelReadOnly_InlineToggleOn_Passes confirms the
// toggle applies at root too: a top-level Read-Only attribute documented
// inline in Argument Reference with (Read-Only) is accepted.
func TestSchemaDocsRule_TopLevelReadOnly_InlineToggleOn_Passes(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "name", Required: true},
					{Name: "arn", Computed: true},
				},
			},
		},
	}

	// Alphabetic: arn before name.
	markdown := "## Argument Reference\n\n" +
		"* `arn` - (Read-Only) ARN of the resource.\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"This resource exports no additional attributes.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	enabled := true
	rule := &check.SchemaDocsRule{AllowInlineReadOnly: &enabled}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, `"arn"`) {
			t.Errorf("unexpected message about arn with toggle on: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_TopLevelReadOnly_MissingFromAttrs_Errors is a regression
// test for the existing top-level computed coverage check: a Read-Only
// attribute missing from Attribute Reference is reported.
func TestSchemaDocsRule_TopLevelReadOnly_MissingFromAttrs_Errors(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "name", Required: true},
					{Name: "arn", Computed: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"This resource exports no additional attributes.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	want := `Read-Only attribute "arn" should be documented in Attribute Reference section`
	found := false
	for _, r := range results {
		if strings.Contains(r.Message, want) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected message containing %q, got:\n  %s",
			want, joinMessages(results))
	}
}

// TestSchemaDocsRule_DeeplyNestedReadOnly_DotNotation_Passes confirms that a
// multi-level dot-notation reference at the root Attribute Reference
// (e.g. `analyzer_configuration.unused_access_configuration.computed_summary`)
// satisfies coverage for an attribute on a deeply-nested schema block.
// Mirrors the path style produced by tfplugindocs's anchor IDs.
func TestSchemaDocsRule_DeeplyNestedReadOnly_DotNotation_Passes(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"analyzer_configuration"},
			},
			"analyzer_configuration": {
				ChildBlocks: []string{"analyzer_configuration.unused_access_configuration"},
			},
			"analyzer_configuration.unused_access_configuration": {
				Attributes: []schema.Attribute{
					{Name: "unused_access_age", Optional: true},
					{Name: "computed_summary", Computed: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `analyzer_configuration` - (Optional) Analyzer configuration.\n\n" +
		"### `analyzer_configuration` Block\n\n" +
		"### `unused_access_configuration` Block\n\n" +
		"* `unused_access_age` - (Optional) Days for unused access.\n\n" +
		"## Attribute Reference\n\n" +
		"* `arn` - ARN of the resource.\n" +
		"* `analyzer_configuration.unused_access_configuration.computed_summary` - Computed summary of unused access.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "computed_summary") {
			t.Errorf("unexpected message about computed_summary: %s", r.Message)
		}
	}
}

func joinMessages(results []check.Result) string {
	var msgs []string
	for _, r := range results {
		msgs = append(msgs, r.Message)
	}
	return strings.Join(msgs, "\n  ")
}
