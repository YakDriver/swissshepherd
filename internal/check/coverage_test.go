// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

func TestSchemaDocsRule_Complete(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	// Filter to errors only (warnings about timeouts block are expected since we don't doc those attrs)
	var errors []check.Result
	for _, r := range results {
		if r.Severity == check.SeverityError {
			errors = append(errors, r)
		}
	}

	// The complete doc should have no errors for root and network blocks.
	// Timeouts block attrs (create, delete) are in schema but not in the doc's ### block format,
	// so they'll show up. That's expected — timeouts are documented differently.
	for _, r := range errors {
		if r.Block != "timeouts" {
			t.Errorf("unexpected error: %s (block: %s)", r.Message, r.Block)
		}
	}
}

func TestSchemaDocsRule_Incomplete(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance_incomplete.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	// Should report missing: description (root), network block entirely, timeouts block
	messages := resultMessages(results)

	// description is missing from root
	if !slices.ContainsFunc(messages, func(s string) bool {
		return strings.Contains(s, `"description"`) && strings.Contains(s, "(root)")
	}) {
		t.Error("expected error about missing 'description' in root block")
	}

	// network block is not documented at all
	if !slices.ContainsFunc(messages, func(s string) bool {
		return strings.Contains(s, `"network"`) && strings.Contains(s, "not documented")
	}) {
		t.Error("expected error about undocumented 'network' block")
	}
}

func TestSchemaDocsRule_SkipsImplicit(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	// Should NOT report 'id' as missing
	for _, r := range results {
		if strings.Contains(r.Message, `"id"`) {
			t.Errorf("should not report 'id' as missing: %s", r.Message)
		}
	}
}

func resultMessages(results []check.Result) []string {
	msgs := make([]string, len(results))
	for i, r := range results {
		msgs[i] = r.Message
	}
	return msgs
}

// TestSchemaDocsRule_MultiLevelParentDisambiguation tests that blocks with
// the same leaf name but different parent paths are correctly disambiguated.
// Regression test for issue where s3.s3_output_format_config.aggregation_config
// and upsolver.s3_output_format_config.aggregation_config were confused.
func TestSchemaDocsRule_MultiLevelParentDisambiguation(t *testing.T) {
	t.Parallel()

	// Schema with two similar paths that differ only in the second-to-last segment
	rs := &schema.ResourceSchema{
		Blocks: map[string]*schema.Block{
			"destination.s3.format.config": {
				Path: "destination.s3.format.config",
				Attributes: []schema.Attribute{
					{Name: "common_attr", Optional: true},
					{Name: "s3_only_attr", Optional: true},
				},
			},
			"destination.upsolver.format.config": {
				Path: "destination.upsolver.format.config",
				Attributes: []schema.Attribute{
					{Name: "common_attr", Optional: true},
					{Name: "upsolver_only_attr", Optional: true},
				},
			},
		},
	}

	// Documentation with headings that include the disambiguating parent
	d := &doc.Document{
		ArgumentBlocks: map[string]*doc.DocBlock{
			"s3.format.config": {
				Name:    "s3.format.config",
				Heading: "destination s3 format config Block",
				Attributes: []doc.DocAttribute{
					{Name: "common_attr", Optional: true},
					{Name: "s3_only_attr", Optional: true},
				},
			},
			"upsolver.format.config": {
				Name:    "upsolver.format.config",
				Heading: "destination upsolver format config Block",
				Attributes: []doc.DocAttribute{
					{Name: "common_attr", Optional: true},
					{Name: "upsolver_only_attr", Optional: true},
				},
			},
		},
		AttributeBlocks: map[string]*doc.DocBlock{},
	}

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "test_resource", Schema: rs, Doc: d})

	if len(results) != 0 {
		t.Errorf("expected no errors, got %d:", len(results))
		for _, r := range results {
			t.Logf("  %s: %s", r.Severity, r.Message)
		}
	}
}

// TestSchemaDocsRule_NestedBlock_LeafAndDotNotation_Coverage is the
// aws_xray_indexing_rule pattern: a nested schema block is documented
// under its leaf-name H3 heading in Argument Reference (input attrs)
// AND under a dot-notation reference in Attribute Reference (computed
// attrs). The full set of documented attributes for the schema's full
// path must be the union of both doc blocks; previously a single-block
// lookup picked one and silently dropped the other.
func TestSchemaDocsRule_NestedBlock_LeafAndDotNotation_Coverage(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"rule"},
			},
			"rule": {
				ChildBlocks: []string{"rule.probabilistic"},
			},
			"rule.probabilistic": {
				Attributes: []schema.Attribute{
					{Name: "desired_sampling_percentage", Required: true},
					{Name: "actual_sampling_percentage", Computed: true},
				},
			},
		},
	}

	// `desired_sampling_percentage` is documented under the
	// `### `probabilistic` Block` heading (keyed "probabilistic" in the
	// doc model). `actual_sampling_percentage` is documented as a
	// dot-notation reference in Attribute Reference (routed to a doc
	// block keyed "rule.probabilistic"). Both are valid, complementary,
	// and together they fully document the schema's "rule.probabilistic"
	// block.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `rule` - (Required) Rule. See [`rule` Block](#rule-block).\n\n" +
		"### `rule` Block\n\n" +
		"* `probabilistic` - (Optional) Probabilistic config. See [`probabilistic` Block](#probabilistic-block).\n\n" +
		"### `probabilistic` Block\n\n" +
		"* `desired_sampling_percentage` - (Required) Configured sampling percentage.\n\n" +
		"## Attribute Reference\n\n" +
		"* `rule[*].probabilistic[*].actual_sampling_percentage` - Applied sampling percentage.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "desired_sampling_percentage") ||
			strings.Contains(r.Message, "actual_sampling_percentage") {
			t.Errorf("unexpected message about probabilistic attribute: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_DuplicateBlockNames exercises the scenario where
// multiple schema paths share the same leaf block name (e.g. connection_pool.tcp
// and timeout.tcp) and the doc merges them into a single "### tcp Block" section.
// This is the appmesh_virtual_node pattern.
func TestSchemaDocsRule_DuplicateBlockNames(t *testing.T) {
	t.Parallel()

	// Schema: two "tcp" blocks under different parents with different attrs.
	// connection_pool.tcp has max_connections.
	// timeout.tcp has idle (a child block, not an attribute).
	rs := &schema.ResourceSchema{
		Name: "aws_test_resource",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"connection_pool", "timeout"},
			},
			"connection_pool": {
				ChildBlocks: []string{"connection_pool.tcp", "connection_pool.grpc"},
			},
			"connection_pool.tcp": {
				Attributes: []schema.Attribute{{Name: "max_connections", Required: true}},
			},
			"connection_pool.grpc": {
				Attributes: []schema.Attribute{{Name: "max_requests", Required: true}},
			},
			"timeout": {
				ChildBlocks: []string{"timeout.tcp", "timeout.grpc"},
			},
			"timeout.tcp": {
				ChildBlocks: []string{"timeout.tcp.idle"},
			},
			"timeout.tcp.idle": {
				Attributes: []schema.Attribute{
					{Name: "unit", Required: true},
					{Name: "value", Required: true},
				},
			},
			"timeout.grpc": {
				ChildBlocks: []string{"timeout.grpc.idle", "timeout.grpc.per_request"},
			},
			"timeout.grpc.idle": {
				Attributes: []schema.Attribute{
					{Name: "unit", Required: true},
					{Name: "value", Required: true},
				},
			},
			"timeout.grpc.per_request": {
				Attributes: []schema.Attribute{
					{Name: "unit", Required: true},
					{Name: "value", Required: true},
				},
			},
		},
	}

	// Doc: both tcp blocks merged into one "### tcp Block" section.
	// Both grpc blocks merged into one "### grpc Block" section.
	// The idle and per_request blocks are also shared across parents.
	markdown := `## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`connection_pool`" + ` - (Optional) Connection pool. See [` + "`connection_pool`" + ` Block](#connection_pool-block) for details.
* ` + "`timeout`" + ` - (Optional) Timeout. See [` + "`timeout`" + ` Block](#timeout-block) for details.

### ` + "`connection_pool`" + ` Block

* ` + "`grpc`" + ` - (Optional) gRPC pool. See [` + "`grpc`" + ` Block](#grpc-block) for details.
* ` + "`tcp`" + ` - (Optional) TCP pool. See [` + "`tcp`" + ` Block](#tcp-block) for details.

### ` + "`timeout`" + ` Block

* ` + "`grpc`" + ` - (Optional) gRPC timeout. See [` + "`grpc`" + ` Block](#grpc-block-1) for details.
* ` + "`tcp`" + ` - (Optional) TCP timeout. See [` + "`tcp`" + ` Block](#tcp-block-1) for details.

### ` + "`tcp`" + ` Block

* ` + "`max_connections`" + ` - (Required) Max connections.

### ` + "`tcp`" + ` Block

* ` + "`idle`" + ` - (Optional) Idle timeout. See [` + "`idle`" + ` Block](#idle-block) for details.

### ` + "`grpc`" + ` Block

* ` + "`max_requests`" + ` - (Required) Max requests.

### ` + "`grpc`" + ` Block

* ` + "`idle`" + ` - (Optional) Idle timeout. See [` + "`idle`" + ` Block](#idle-block) for details.
* ` + "`per_request`" + ` - (Optional) Per-request timeout. See [` + "`per_request`" + ` Block](#per_request-block) for details.

### ` + "`idle`" + ` Block

* ` + "`unit`" + ` - (Required) Unit.
* ` + "`value`" + ` - (Required) Value.

### ` + "`per_request`" + ` Block

* ` + "`unit`" + ` - (Required) Unit.
* ` + "`value`" + ` - (Required) Value.

## Attribute Reference

This resource exports no additional attributes.
`

	d, err := doc.Parse([]byte(markdown), "aws_test_resource")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	disabled := false
	rule := &check.SchemaDocsRule{IgnoreDeprecated: true, Ordering: &disabled}
	results := rule.Check(check.CheckContext{Resource: "aws_test_resource", Schema: rs, Doc: d})

	// Filter to errors and warnings
	var errors, warnings []string
	for _, r := range results {
		if r.Severity == check.SeverityError {
			errors = append(errors, r.Message)
		} else {
			warnings = append(warnings, r.Message)
		}
	}

	// No errors expected: all schema attrs are documented somewhere.
	if len(errors) > 0 {
		t.Errorf("unexpected errors:\n  %s", strings.Join(errors, "\n  "))
	}

	// No phantom warnings expected: every documented attr exists in some
	// sibling schema block with the same leaf name.
	for _, w := range warnings {
		if strings.Contains(w, "does not exist in schema") {
			t.Errorf("unexpected phantom warning: %s", w)
		}
	}
}

// TestSchemaDocsRule_DuplicateBlockNames_RealPhantom verifies that a truly
// phantom attribute (one that doesn't exist in ANY sibling block) is still
// reported even when blocks share a leaf name.
func TestSchemaDocsRule_DuplicateBlockNames_RealPhantom(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test_resource",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"pool_a", "pool_b"},
			},
			"pool_a": {
				ChildBlocks: []string{"pool_a.tcp"},
			},
			"pool_a.tcp": {
				Attributes: []schema.Attribute{{Name: "max_connections", Required: true}},
			},
			"pool_b": {
				ChildBlocks: []string{"pool_b.tcp"},
			},
			"pool_b.tcp": {
				Attributes: []schema.Attribute{{Name: "timeout", Optional: true}},
			},
		},
	}

	// Doc: merged tcp block documents max_connections, timeout, AND a
	// completely bogus attribute "bogus_attr" that exists in no schema block.
	markdown := `## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`pool_a`" + ` - (Optional) Pool A. See [` + "`pool_a`" + ` Block](#pool_a-block) for details.
* ` + "`pool_b`" + ` - (Optional) Pool B. See [` + "`pool_b`" + ` Block](#pool_b-block) for details.

### ` + "`pool_a`" + ` Block

* ` + "`tcp`" + ` - (Optional) TCP. See [` + "`tcp`" + ` Block](#tcp-block) for details.

### ` + "`pool_b`" + ` Block

* ` + "`tcp`" + ` - (Optional) TCP. See [` + "`tcp`" + ` Block](#tcp-block-1) for details.

### ` + "`tcp`" + ` Block

* ` + "`max_connections`" + ` - (Required) Max connections.
* ` + "`timeout`" + ` - (Optional) Timeout.
* ` + "`bogus_attr`" + ` - (Optional) Does not exist anywhere.

## Attribute Reference

This resource exports no additional attributes.
`

	d, err := doc.Parse([]byte(markdown), "aws_test_resource")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	disabled := false
	rule := &check.SchemaDocsRule{IgnoreDeprecated: true, Ordering: &disabled}
	results := rule.Check(check.CheckContext{Resource: "aws_test_resource", Schema: rs, Doc: d})

	// max_connections and timeout should NOT be phantom (they exist in siblings).
	// bogus_attr SHOULD be phantom.
	var phantomWarnings []string
	for _, r := range results {
		if strings.Contains(r.Message, "does not exist in schema") {
			phantomWarnings = append(phantomWarnings, r.Message)
		}
	}

	if len(phantomWarnings) != 1 {
		t.Fatalf("expected exactly 1 phantom warning (bogus_attr), got %d:\n  %s",
			len(phantomWarnings), strings.Join(phantomWarnings, "\n  "))
	}
	if !strings.Contains(phantomWarnings[0], "bogus_attr") {
		t.Errorf("expected phantom warning for bogus_attr, got: %s", phantomWarnings[0])
	}
}

// TestSchemaDocsRule_DuplicateBlockNames_ChildBlockAsAttr verifies that a
// child block name documented as an attribute in a sibling's merged doc section
// is not reported as phantom. This is the per_request case: timeout.grpc has
// per_request as a child block, but the merged grpc doc section lists it as an
// attribute reference to the per_request block.
func TestSchemaDocsRule_DuplicateBlockNames_ChildBlockAsAttr(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test_resource",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"fast", "slow"},
			},
			"fast": {
				ChildBlocks: []string{"fast.grpc"},
			},
			"fast.grpc": {
				// Only has max_requests — no per_request child block.
				Attributes: []schema.Attribute{{Name: "max_requests", Required: true}},
			},
			"slow": {
				ChildBlocks: []string{"slow.grpc"},
			},
			"slow.grpc": {
				// Has per_request as a child block (not an attribute).
				ChildBlocks: []string{"slow.grpc.per_request"},
			},
			"slow.grpc.per_request": {
				Attributes: []schema.Attribute{
					{Name: "unit", Required: true},
					{Name: "value", Required: true},
				},
			},
		},
	}

	// Doc: merged grpc block has max_requests AND per_request.
	// per_request is a child block of slow.grpc — should not be phantom.
	markdown := `## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`fast`" + ` - (Optional) Fast. See [` + "`fast`" + ` Block](#fast-block) for details.
* ` + "`slow`" + ` - (Optional) Slow. See [` + "`slow`" + ` Block](#slow-block) for details.

### ` + "`fast`" + ` Block

* ` + "`grpc`" + ` - (Optional) gRPC. See [` + "`grpc`" + ` Block](#grpc-block) for details.

### ` + "`slow`" + ` Block

* ` + "`grpc`" + ` - (Optional) gRPC. See [` + "`grpc`" + ` Block](#grpc-block-1) for details.

### ` + "`grpc`" + ` Block

* ` + "`max_requests`" + ` - (Required) Max requests.
* ` + "`per_request`" + ` - (Optional) Per-request timeout. See [` + "`per_request`" + ` Block](#per_request-block) for details.

### ` + "`per_request`" + ` Block

* ` + "`unit`" + ` - (Required) Unit.
* ` + "`value`" + ` - (Required) Value.

## Attribute Reference

This resource exports no additional attributes.
`

	d, err := doc.Parse([]byte(markdown), "aws_test_resource")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	disabled := false
	rule := &check.SchemaDocsRule{IgnoreDeprecated: true, Ordering: &disabled}
	results := rule.Check(check.CheckContext{Resource: "aws_test_resource", Schema: rs, Doc: d})

	// per_request should NOT be phantom — it's a child block of slow.grpc.
	// max_requests should NOT be phantom — it's an attr of fast.grpc.
	for _, r := range results {
		if strings.Contains(r.Message, "does not exist in schema") {
			t.Errorf("unexpected phantom warning: %s", r.Message)
		}
	}

	// No errors expected either.
	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("unexpected error: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_DuplicateBlockNames_MissingAttrStillReported ensures
// that a genuinely undocumented attribute is still reported even when sibling
// blocks exist. The sibling suppression only applies to phantom checks, not
// missing-documentation checks.
func TestSchemaDocsRule_DuplicateBlockNames_MissingAttrStillReported(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test_resource",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"pool_a", "pool_b"},
			},
			"pool_a": {
				ChildBlocks: []string{"pool_a.tcp"},
			},
			"pool_a.tcp": {
				Attributes: []schema.Attribute{
					{Name: "max_connections", Required: true},
					{Name: "max_pending", Optional: true}, // NOT documented
				},
			},
			"pool_b": {
				ChildBlocks: []string{"pool_b.tcp"},
			},
			"pool_b.tcp": {
				Attributes: []schema.Attribute{{Name: "timeout", Optional: true}},
			},
		},
	}

	// Doc: merged tcp block only documents max_connections and timeout.
	// max_pending is missing from the doc entirely.
	markdown := `## Argument Reference

* ` + "`name`" + ` - (Required) Name.
* ` + "`pool_a`" + ` - (Optional) Pool A. See [` + "`pool_a`" + ` Block](#pool_a-block) for details.
* ` + "`pool_b`" + ` - (Optional) Pool B. See [` + "`pool_b`" + ` Block](#pool_b-block) for details.

### ` + "`pool_a`" + ` Block

* ` + "`tcp`" + ` - (Optional) TCP. See [` + "`tcp`" + ` Block](#tcp-block) for details.

### ` + "`pool_b`" + ` Block

* ` + "`tcp`" + ` - (Optional) TCP. See [` + "`tcp`" + ` Block](#tcp-block-1) for details.

### ` + "`tcp`" + ` Block

* ` + "`max_connections`" + ` - (Required) Max connections.
* ` + "`timeout`" + ` - (Optional) Timeout.

## Attribute Reference

This resource exports no additional attributes.
`

	d, err := doc.Parse([]byte(markdown), "aws_test_resource")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	disabled := false
	rule := &check.SchemaDocsRule{IgnoreDeprecated: true, Ordering: &disabled}
	results := rule.Check(check.CheckContext{Resource: "aws_test_resource", Schema: rs, Doc: d})

	// max_pending should be reported as undocumented.
	var found bool
	for _, r := range results {
		if r.Severity == check.SeverityError && strings.Contains(r.Message, "max_pending") {
			found = true
		}
	}
	if !found {
		t.Error("expected error for undocumented attribute 'max_pending' in pool_a.tcp")
	}
}

// TestSchemaDocsRule_PhantomBlockHeading exercises the case where a doc has
// a block heading that does not correspond to any schema block. The
// trigger pattern is from website/docs/r/workspaces_ip_group.html.markdown:
// an "### `rules`" block heading is followed by a stray "#### Arguments"
// subheading. The H4 gets parsed as a phantom "arguments" block.
func TestSchemaDocsRule_PhantomBlockHeading(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "name", Required: true},
				},
				ChildBlocks: []string{"rules"},
			},
			"rules": {
				Attributes: []schema.Attribute{
					{Name: "source", Required: true},
					{Name: "description", Optional: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"This resource supports the following arguments:\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `rules` - (Optional) Rules. See [`rules`](#rules) below.\n\n" +
		"### `rules`\n\n" +
		"#### Arguments\n\n" +
		"* `source` - (Required) Source.\n" +
		"* `description` - (Optional) Description.\n\n" +
		"## Attribute Reference\n\n" +
		"This resource exports no additional attributes.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	want := `block heading "Arguments" in Argument Reference has no matching block in schema`
	found := false
	for _, r := range results {
		if strings.Contains(r.Message, want) {
			found = true
			break
		}
	}
	if !found {
		var msgs []string
		for _, r := range results {
			msgs = append(msgs, r.Message)
		}
		t.Errorf("expected message containing %q, got:\n  %s",
			want, strings.Join(msgs, "\n  "))
	}
}

// TestSchemaDocsRule_NestedAttributeHeadingNotPhantom is a regression
// test for the false positive on aws_workspaces_pool (PR
// hashicorp/terraform-provider-aws#42678). Object-typed nested
// attributes — list(object({...})), set(object), or a bare object —
// are documented with block-style headings and a list of their
// sub-attributes, even though they are attributes rather than blocks.
// They live as Attribute.Children, not rs.Blocks entries, so the
// phantom-block check must recognize them by name and not flag them.
func TestSchemaDocsRule_NestedAttributeHeadingNotPhantom(t *testing.T) {
	t.Parallel()

	// application_settings and timeout_settings are optional
	// list(object(...)) attributes; capacity is a real nested block.
	// nested_object.deep exercises recognition at depth > 1.
	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "name", Required: true},
					{Name: "application_settings", Optional: true, Children: []schema.Attribute{
						{Name: "settings_group", Optional: true},
						{Name: "status", Required: true},
					}},
					{Name: "timeout_settings", Optional: true, Children: []schema.Attribute{
						{Name: "disconnect_timeout_in_seconds", Optional: true},
					}},
					{Name: "nested_object", Optional: true, Children: []schema.Attribute{
						{Name: "deep", Optional: true, Children: []schema.Attribute{
							{Name: "leaf", Optional: true},
						}},
					}},
				},
				ChildBlocks: []string{"capacity"},
			},
			"capacity": {
				Attributes: []schema.Attribute{
					{Name: "desired_user_sessions", Required: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"The following arguments are required:\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `capacity` - (Required) Capacity. Defined below.\n\n" +
		"The following arguments are optional:\n\n" +
		"* `application_settings` - (Optional) Application settings. Defined below.\n" +
		"* `nested_object` - (Optional) Nested object. Defined below.\n" +
		"* `timeout_settings` - (Optional) Timeout settings. Defined below.\n\n" +
		"### `capacity` Block\n\n" +
		"* `desired_user_sessions` - (Required) Desired user sessions.\n\n" +
		"### `application_settings`\n\n" +
		"* `settings_group` - (Optional) Settings group.\n" +
		"* `status` - (Required) Status.\n\n" +
		"### `timeout_settings`\n\n" +
		"* `disconnect_timeout_in_seconds` - (Optional) Disconnect timeout.\n\n" +
		"### `nested_object`\n\n" +
		"* `deep` - (Optional) Deep. Defined below.\n\n" +
		"### `deep`\n\n" +
		"* `leaf` - (Optional) Leaf.\n\n" +
		"## Attribute Reference\n\n" +
		"This resource exports no additional attributes.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "has no matching block in schema") {
			t.Errorf("object-typed nested attribute heading wrongly flagged as phantom: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_PhantomStillFiresWithNestedAttributes confirms the
// nested-attribute allowance does not mask genuine phantom headings: a
// heading whose name matches neither a block nor an object-typed
// attribute must still be reported.
func TestSchemaDocsRule_PhantomStillFiresWithNestedAttributes(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "name", Required: true},
					{Name: "application_settings", Optional: true, Children: []schema.Attribute{
						{Name: "status", Required: true},
					}},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `application_settings` - (Optional) Application settings.\n\n" +
		"### `application_settings`\n\n" +
		"#### Arguments\n\n" +
		"* `status` - (Required) Status.\n\n" +
		"## Attribute Reference\n\n" +
		"This resource exports no additional attributes.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	want := `block heading "Arguments" in Argument Reference has no matching block in schema`
	found := false
	for _, r := range results {
		if strings.Contains(r.Message, want) {
			found = true
			break
		}
	}
	if !found {
		var msgs []string
		for _, r := range results {
			msgs = append(msgs, r.Message)
		}
		t.Errorf("expected phantom message %q, got:\n  %s", want, strings.Join(msgs, "\n  "))
	}
}

// TestSchemaDocsRule_PhantomBlockToggle confirms the phantom-block check
// is gated by the coverage sub-check toggle.
func TestSchemaDocsRule_PhantomBlockToggle(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "name", Required: true},
				},
			},
		},
	}

	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"### `phantom`\n\n" +
		"* `something` - (Optional) Something.\n\n" +
		"## Attribute Reference\n\n" +
		"This resource exports no additional attributes.\n"

	d, err := doc.Parse([]byte(markdown), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	disabled := false
	rule := &check.SchemaDocsRule{Coverage: &disabled}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "in Argument Reference has no matching block in schema") {
			t.Errorf("expected no phantom-block error with Coverage=false, got: %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_Correct(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	// arn is computed-only and is in the Attribute Reference section — should pass
	for _, r := range results {
		if r.Severity == check.SeverityError && strings.Contains(r.Message, "arn") {
			t.Errorf("unexpected error for 'arn': %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_Wrong(t *testing.T) {
	t.Parallel()

	ps, err := schema.LoadFile("../../testdata/schema/test_provider.json", "registry.terraform.io/hashicorp/test")
	if err != nil {
		t.Fatalf("loading schema: %s", err)
	}

	d, err := doc.ParseFile("../../testdata/docs/r/instance_computed_wrong.html.markdown")
	if err != nil {
		t.Fatalf("loading doc: %s", err)
	}

	// SchemaDocsRule: arn is computed-only but NOT in Attribute Reference
	attrRule := &check.SchemaDocsRule{}
	attrResults := attrRule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	var foundMissing bool
	for _, r := range attrResults {
		if strings.Contains(r.Message, "should be documented in Attribute Reference") {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Error("expected error about 'arn' missing from Attribute Reference section")
	}

	// SchemaDocsRule: arn is computed-only but IS in Argument Reference
	argRule := &check.SchemaDocsRule{}
	argResults := argRule.Check(check.CheckContext{Resource: "test_instance", Schema: ps.Resources["test_instance"], Doc: d})

	var foundWrongSection bool
	for _, r := range argResults {
		if strings.Contains(r.Message, "should not appear in Argument Reference") {
			foundWrongSection = true
		}
	}
	if !foundWrongSection {
		t.Error("expected warning about 'arn' appearing in Argument Reference section")
	}
}

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

// TestSchemaDocsRule_NoFalseComputedMisplacement verifies that computed-only
// attributes documented in the Attribute Reference section are NOT flagged as
// misplaced in the Argument Reference section, even when broad heading
// templates cause attribute-section items to bleed into ArgumentBlocks.
func TestSchemaDocsRule_NoFalseComputedMisplacement(t *testing.T) {
	t.Parallel()

	// Templates broad enough to cause bleed (includes "{Block}" which matches anything)
	templates := doc.HeadingTemplates{"`{Block}` Block", "{Block} Block", "{Block}", "{Title}"}

	src := []byte(`# Data Source: aws_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name of the thing.

## Attribute Reference

* ` + "`arn`" + ` - ARN of the thing.
* ` + "`created_time`" + ` - Time at which this thing was created.
* ` + "`description`" + ` - Description of the thing.
`)

	d, err := doc.ParseWithTemplates(src, "aws_thing", templates)
	if err != nil {
		t.Fatal(err)
	}

	rs := &schema.ResourceSchema{
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "name", Optional: true},
					{Name: "arn", Computed: true},
					{Name: "created_time", Computed: true},
					{Name: "description", Computed: true},
				},
			},
		},
	}

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if strings.Contains(r.Message, "should not appear in Argument Reference") {
			t.Errorf("false positive: %s", r.Message)
		}
	}
}

// TestSharedSubsection_HeadingsOnly: sibling blocks may share one
// subsection, but only through its heading. A link from each sibling bullet
// to a differently named subsection doesn't document the siblings, because a
// reader at that subsection can't tell which blocks it covers
// (docs/rules/coverage-path-resolution.md §4). This reverses issue #51's
// link-following. A combined heading naming every sibling does share.
func TestSharedSubsection_HeadingsOnly(t *testing.T) {
	t.Parallel()

	endpoints := func() *schema.ResourceSchema {
		return &schema.ResourceSchema{Name: "aws_test", Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "endpoints", Computed: true, Children: []schema.Attribute{
					{Name: "management", Computed: true, Children: []schema.Attribute{
						{Name: "dns_name", Computed: true},
						{Name: "ip_addresses", Computed: true},
					}},
					{Name: "intercluster", Computed: true, Children: []schema.Attribute{
						{Name: "dns_name", Computed: true},
						{Name: "ip_addresses", Computed: true},
					}},
				}},
			}},
		}}
	}
	labels := func() *schema.ResourceSchema {
		return &schema.ResourceSchema{Name: "aws_test", Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "available_labels", Computed: true, Children: []schema.Attribute{{Name: "name", Computed: true}}},
				{Name: "consumed_labels", Computed: true, Children: []schema.Attribute{{Name: "name", Computed: true}}},
			}},
		}}
	}
	endpointsDoc := func(heading string) string {
		return "## Attribute Reference\n\n" +
			"* `endpoints` - Endpoints. See [`endpoints`](#endpoints-block) below.\n\n" +
			"### `endpoints` Block\n\n" +
			"* `intercluster` - Endpoint. See [Endpoint](#endpoint).\n" +
			"* `management` - Endpoint. See [Endpoint](#endpoint).\n\n" +
			"#### " + heading + "\n\n" +
			"* `dns_name` - DNS name.\n" +
			"* `ip_addresses` - IP addresses.\n"
	}
	labelsDoc := func(heading string) string {
		return "## Attribute Reference\n\n" +
			"* `available_labels` - Labels. See [Labels](#labels) below.\n" +
			"* `consumed_labels` - Labels. See [Labels](#labels) below.\n\n" +
			"### " + heading + "\n\n" +
			"* `name` - Label name.\n"
	}

	testCases := map[string]struct {
		schema     func() *schema.ResourceSchema
		md         string
		wantMisses []string // paths reported as missing Read-Only fields
	}{
		"same-named siblings, linked to a differently named subsection": {
			schema:     endpoints,
			md:         endpointsDoc("Endpoint"),
			wantMisses: []string{"endpoints.intercluster", "endpoints.management"},
		},
		"same-named siblings, combined heading": {
			schema: endpoints,
			md:     endpointsDoc("`intercluster` and `management`"),
		},
		"differently named siblings, linked to a shared subsection": {
			schema:     labels,
			md:         labelsDoc("Labels"),
			wantMisses: []string{"available_labels", "consumed_labels"},
		},
		"differently named siblings, combined heading": {
			schema: labels,
			md:     labelsDoc("`available_labels` and `consumed_labels`"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rs := tc.schema()
			schema.ExpandObjectAttributes(&schema.ProviderSchema{
				DataSources: map[string]*schema.ResourceSchema{"aws_test": rs},
			})
			d, err := doc.ParseWithOptions([]byte(tc.md), "aws_test", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
			if err != nil {
				t.Fatal(err)
			}
			results := (&check.SchemaDocsRule{}).Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})
			var got []string
			for _, r := range results {
				if strings.Contains(r.Message, "should be documented") {
					got = append(got, r.Block)
				}
			}
			slices.Sort(got)
			got = slices.Compact(got)
			if !slices.Equal(got, tc.wantMisses) {
				t.Errorf("paths missing Read-Only fields = %v, want %v\n  %s", got, tc.wantMisses, joinMessages(results))
			}
		})
	}
}

// TestProseLeadIn_NoCascade: a legacy prose lead-in introducing a nested
// block's read-only fields is recognized, so its bullets are covered and don't
// cascade into phantom/root/ordering findings (issue #53).
func TestProseLeadIn_NoCascade(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "catalog_properties", Optional: true, Children: []schema.Attribute{
					{Name: "data_lake_access_properties", Optional: true, Children: []schema.Attribute{
						{Name: "data_lake_access", Optional: true},
						{Name: "managed_workgroup_name", Computed: true},
						{Name: "status_message", Computed: true},
					}},
				}},
			}},
		},
	}
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	md := "## Argument Reference\n\n" +
		"* `catalog_properties` - (Optional) Config. See below.\n\n" +
		"### `catalog_properties` Block\n\n" +
		"* `data_lake_access_properties` - (Optional) Config. See below.\n\n" +
		"#### `data_lake_access_properties`\n\n" +
		"* `data_lake_access` - (Optional) Whether enabled.\n\n" +
		"## Attribute Reference\n\n" +
		"The `catalog_properties[0].data_lake_access_properties[0]` block also exports:\n\n" +
		"* `managed_workgroup_name` - Managed workgroup name.\n" +
		"* `status_message` - Status message.\n"

	d, err := doc.ParseWithOptions([]byte(md), "aws_test", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})
	for _, bad := range []string{"does not exist in schema", "should be documented", "should come before"} {
		if hasMessage(results, bad) {
			t.Errorf("prose lead-in should not cascade (%q), got:\n  %s", bad, joinMessages(results))
		}
	}
}

// objectAttrSchema builds a data source whose computed `items` attribute is an
// object-typed list: items => {arn, foo}. Children live on Attribute.Children,
// exactly as the loader produces them for list(object({...})).
func objectAttrSchema() *schema.ResourceSchema {
	return &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "name", Required: true},
				{Name: "items", Computed: true, Children: []schema.Attribute{
					{Name: "arn", Computed: true},
					{Name: "foo", Computed: true},
				}},
			}},
		},
	}
}

func parseCaptured(t *testing.T, md string) *doc.Document {
	t.Helper()
	d, err := doc.ParseWithOptions([]byte(md), "aws_test",
		doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return d
}

func hasMessage(results []check.Result, substr string) bool {
	for _, r := range results {
		if strings.Contains(r.Message, substr) {
			return true
		}
	}
	return false
}

// TestNestedObject_On_MissingField_Errors: with the schema expanded and the
// doc parsed with nested capture, an undocumented object field is flagged.
func TestNestedObject_On_MissingField_Errors(t *testing.T) {
	t.Parallel()

	rs := objectAttrSchema()
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		DataSources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	// `items` documented inline with arn only — foo is missing.
	md := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"* `items` - List of objects. Each object has the following attributes:\n" +
		"    * `arn` - ARN value.\n"

	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{
		Resource: "aws_test", Schema: rs, Doc: parseCaptured(t, md),
	})

	want := `Read-Only attribute "foo" in block "items" should be documented`
	if !hasMessage(results, want) {
		t.Errorf("expected %q, got:\n  %s", want, joinMessages(results))
	}
}

// TestNestedObject_Off_NoFindings: without expansion and without nested
// capture (the default), object fields are neither required nor style-checked.
func TestNestedObject_Off_NoFindings(t *testing.T) {
	t.Parallel()

	rs := objectAttrSchema() // NOT expanded

	// Default parse: no nested capture. arn has a weak "The" start and foo is
	// absent — neither should produce a finding.
	md := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"## Attribute Reference\n\n" +
		"* `items` - List of objects. Each object has the following attributes:\n" +
		"    * `arn` - The ARN value.\n"

	d, err := doc.Parse([]byte(md), "aws_test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{
		Resource: "aws_test", Schema: rs, Doc: d,
	})

	if hasMessage(results, `block "items"`) {
		t.Errorf("did not expect nested-block findings with toggle off:\n  %s", joinMessages(results))
	}
	if hasMessage(results, `"foo"`) {
		t.Errorf("did not expect a foo finding with toggle off:\n  %s", joinMessages(results))
	}
	if hasMessage(results, `attribute "arn" description should not start`) {
		t.Errorf("did not expect nested arn description finding with toggle off:\n  %s", joinMessages(results))
	}
}

// configurableObjectSchema builds a resource with `scaling_target`, an
// Optional+Computed object-typed attribute whose fields (max_task_count) are
// genuine arguments the schema can't distinguish from computed values.
func configurableObjectSchema() *schema.ResourceSchema {
	return &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {Path: "", Attributes: []schema.Attribute{
				{Name: "name", Required: true},
				{Name: "scaling_target", Optional: true, Computed: true, Children: []schema.Attribute{
					{Name: "max_task_count", Computed: true},
				}},
			}},
		},
	}
}

// TestNestedObject_ConfigUnknown_ArgumentReferenceSatisfies: a configurable
// object's field documented as an (Optional) argument in Argument Reference
// satisfies coverage — it must NOT be demanded in Attribute Reference (issues
// #50/#52).
func TestNestedObject_ConfigUnknown_ArgumentReferenceSatisfies(t *testing.T) {
	t.Parallel()

	rs := configurableObjectSchema()
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	md := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `scaling_target` - (Optional) Scaling. Each object has the following attributes:\n" +
		"    * `max_task_count` - (Optional) Max tasks.\n\n" +
		"## Attribute Reference\n\n" +
		"* `id` - ID.\n"

	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{
		Resource: "aws_test", Schema: rs, Doc: parseCaptured(t, md),
	})

	if hasMessage(results, `max_task_count`) {
		t.Errorf("configurable object field in Argument Reference should satisfy coverage, got:\n  %s", joinMessages(results))
	}
}

// TestNestedObject_ConfigUnknown_UndocumentedIsNeutral: a genuinely
// undocumented field of a configurable object is still flagged, but with a
// neutral "is not documented" message — not a spurious "Attribute Reference"
// demand.
func TestNestedObject_ConfigUnknown_UndocumentedIsNeutral(t *testing.T) {
	t.Parallel()

	rs := configurableObjectSchema()
	schema.ExpandObjectAttributes(&schema.ProviderSchema{
		Resources: map[string]*schema.ResourceSchema{"aws_test": rs},
	})

	// scaling_target documented, but max_task_count omitted entirely.
	md := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `scaling_target` - (Optional) Scaling. Each object has the following attributes:\n" +
		"    * `other` - (Optional) Unrelated.\n\n" +
		"## Attribute Reference\n\n" +
		"* `id` - ID.\n"

	results := (&check.SchemaDocsRule{}).Check(check.CheckContext{
		Resource: "aws_test", Schema: rs, Doc: parseCaptured(t, md),
	})

	if !hasMessage(results, `attribute "max_task_count" in block "scaling_target" is not documented`) {
		t.Errorf("expected neutral not-documented message, got:\n  %s", joinMessages(results))
	}
	if hasMessage(results, `Read-Only attribute "max_task_count"`) {
		t.Errorf("configurable object field must not be demanded in Attribute Reference, got:\n  %s", joinMessages(results))
	}
}

// coverageMissingBlockMsgs returns the sorted set of "block ... is not
// documented" coverage messages for a schema+doc pair.
func coverageMissingBlockMsgs(t *testing.T, src string, rs *schema.ResourceSchema) []string {
	t.Helper()
	d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", labelTemplates)
	if err != nil {
		t.Fatal(err)
	}
	results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(
		check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
	var msgs []string
	for _, r := range results {
		if strings.Contains(r.Message, "is not documented") {
			msgs = append(msgs, r.Message)
		}
	}
	slices.Sort(msgs)
	return msgs
}

// Several undocumented blocks that share a leaf name are deduped to a single
// representative finding. Before issue #65 the representative path was chosen by
// Go's randomized map iteration over rs.Blocks, so the reported path flapped
// between runs. Coverage must now produce identical output every run.
func TestCoverage_UndocumentedSiblingBlocksDeterministic(t *testing.T) {
	t.Parallel()

	// foo.thing and bar.thing are distinct undocumented blocks that share the
	// leaf name "thing"; the leaf-keyed dedup keeps exactly one representative.
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":          {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"foo", "bar"}},
		"foo":       {Path: "foo", ChildBlocks: []string{"foo.thing"}},
		"bar":       {Path: "bar", ChildBlocks: []string{"bar.thing"}},
		"foo.thing": {Path: "foo.thing", Attributes: []schema.Attribute{{Name: "x", Optional: true}}},
		"bar.thing": {Path: "bar.thing", Attributes: []schema.Attribute{{Name: "x", Optional: true}}},
	}}

	// Doc documents name, foo, and bar, but never a "thing" subsection.
	src := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n\n" +
		"### `foo` Block\n\n" +
		"* `thing` - (Optional) Thing. See below.\n\n" +
		"### `bar` Block\n\n" +
		"* `thing` - (Optional) Thing. See below.\n"

	// Go re-randomizes map iteration order on each range statement, so repeating
	// the check many times in one process exercises many orderings.
	first := coverageMissingBlockMsgs(t, src, rs)

	// Exactly one representative for the shared "thing" leaf.
	thingCount := 0
	for _, m := range first {
		if strings.Contains(m, `"thing"`) || strings.Contains(m, ".thing") {
			thingCount++
		}
	}
	if thingCount != 1 {
		t.Fatalf("expected exactly one deduped 'thing' block finding, got %d: %v", thingCount, first)
	}

	for i := 1; i < 100; i++ {
		got := coverageMissingBlockMsgs(t, src, rs)
		if !slices.Equal(got, first) {
			t.Fatalf("coverage output is nondeterministic across runs:\n run0 = %v\n run%d = %v", first, i, got)
		}
	}
}

// The acceptance cases in docs/rules/coverage-path-resolution.md (#77). Two
// same-named blocks, z at x.y.z and z at t.u.z:
//
//   - case 1: different fields; flag unless each is documented exactly.
//   - case 2: identical fields; one shared section passes, duplicate
//     headings are still reported.
//   - case 3: identical names, but child block a3 differs below; flag.
//
// Expectations are stated as severity counts, not message text, so they hold
// while the messages are being redesigned.

// want is the expected outcome of one acceptance row.
type want int

const (
	wantNone     want = iota // no findings at all
	wantErrors               // at least one error and at least one warning
	wantError                // at least one error
	wantWarnOnly             // at least one warning, no errors
)

func (w want) String() string {
	return [...]string{"no findings", "errors and a warning", "an error", "warnings only"}[w]
}

// section is one H3 heading in Argument Reference and the fields it lists.
type section struct {
	heading string
	fields  []string
}

func renderAcceptanceDoc(root []string, sections []section) string {
	var b strings.Builder
	b.WriteString("## Argument Reference\n\nThis resource supports the following arguments:\n\n")
	for _, f := range root {
		fmt.Fprintf(&b, "* `%s` - (Optional) %s.\n", f, f)
	}
	for _, s := range sections {
		fmt.Fprintf(&b, "\n### `%s` Block\n\n", s.heading)
		for _, f := range s.fields {
			fmt.Fprintf(&b, "* `%s` - (Optional) %s.\n", f, f)
		}
	}
	b.WriteString("\n## Attribute Reference\n\nThis resource exports no additional attributes.\n")
	return b.String()
}

func optional(names ...string) []schema.Attribute {
	attrs := make([]schema.Attribute, 0, len(names))
	for _, n := range names {
		attrs = append(attrs, schema.Attribute{Name: n, Optional: true})
	}
	return attrs
}

// twoZSchema builds root -> x -> x.y -> x.y.z and root -> t -> t.u -> t.u.z,
// with the given attributes on each z and optional extra blocks below them.
func twoZSchema(xyz, tuz *schema.Block, extra map[string]*schema.Block) *schema.ResourceSchema {
	blocks := map[string]*schema.Block{
		"":      {ChildBlocks: []string{"x", "t"}},
		"x":     {ChildBlocks: []string{"x.y"}},
		"x.y":   {ChildBlocks: []string{"x.y.z"}},
		"t":     {ChildBlocks: []string{"t.u"}},
		"t.u":   {ChildBlocks: []string{"t.u.z"}},
		"x.y.z": xyz,
		"t.u.z": tuz,
	}
	maps.Copy(blocks, extra)
	return &schema.ResourceSchema{Name: "aws_test", Blocks: blocks}
}

// twoZParents documents the ancestors of both z blocks, each under its exact
// path, so every row differs only in how z (and its children) are documented.
var twoZParents = []section{
	{heading: "x", fields: []string{"y"}},
	{heading: "x.y", fields: []string{"z"}},
	{heading: "t", fields: []string{"u"}},
	{heading: "t.u", fields: []string{"z"}},
}

func runAcceptance(t *testing.T, rs *schema.ResourceSchema, root []string, sections []section) []check.Result {
	t.Helper()
	d, err := doc.Parse([]byte(renderAcceptanceDoc(root, sections)), rs.Name)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	off := false
	rule := &check.SchemaDocsRule{
		IgnoreDeprecated: true,
		Ordering:         &off,
		Description:      &off,
		Format:           &off,
		Byline:           &off,
		// The shared-section finding moves from `heading` into `coverage`
		// (design §6), so these cases must pass or fail on coverage alone.
		Heading: &off,
	}
	return rule.Check(check.CheckContext{Resource: rs.Name, Schema: rs, Doc: d})
}

func assertWant(t *testing.T, results []check.Result, w want) {
	t.Helper()
	var errs, warns []string
	for _, r := range results {
		if r.Severity == check.SeverityError {
			errs = append(errs, r.Message)
		} else {
			warns = append(warns, r.Message)
		}
	}
	ok := false
	switch w {
	case wantNone:
		ok = len(errs) == 0 && len(warns) == 0
	case wantErrors:
		ok = len(errs) > 0 && len(warns) > 0
	case wantError:
		ok = len(errs) > 0
	case wantWarnOnly:
		ok = len(errs) == 0 && len(warns) > 0
	}
	if !ok {
		t.Errorf("want %s; got %d error(s), %d warning(s)\n  errors:   %s\n  warnings: %s",
			w, len(errs), len(warns), strings.Join(errs, "\n            "), strings.Join(warns, "\n            "))
	}
}

func withParents(rows ...section) []section {
	return append(slices.Clone(twoZParents), rows...)
}

// reversedZ returns sections with the z occurrences (heading "z") in reverse
// order, leaving every other section in place. Duplicate headings must not be
// resolved by position (design §5), so every result has to survive this.
func reversedZ(sections []section) []section {
	out := slices.Clone(sections)
	var idx []int
	for i, s := range out {
		if s.heading == "z" {
			idx = append(idx, i)
		}
	}
	for i, j := 0, len(idx)-1; i < j; i, j = i+1, j-1 {
		out[idx[i]], out[idx[j]] = out[idx[j]], out[idx[i]]
	}
	return out
}

func hasDuplicateZ(sections []section) bool {
	n := 0
	for _, s := range sections {
		if s.heading == "z" {
			n++
		}
	}
	return n > 1
}

func TestCoverageAcceptance_case1DifferentFields(t *testing.T) {
	t.Parallel()

	rs := twoZSchema(
		&schema.Block{Attributes: optional("a1", "a2", "a3")},
		&schema.Block{Attributes: optional("a3", "a4", "a5")},
		nil,
	)

	testCases := map[string]struct {
		sections []section
		want     want
	}{
		"one z section listing the union": {
			sections: withParents(section{heading: "z", fields: []string{"a1", "a2", "a3", "a4", "a5"}}),
			want:     wantErrors,
		},
		"one z section listing only x.y.z's fields": {
			sections: withParents(section{heading: "z", fields: []string{"a1", "a2", "a3"}}),
			want:     wantErrors,
		},
		"qualified sections, each exact": {
			sections: withParents(
				section{heading: "x.y.z", fields: []string{"a1", "a2", "a3"}},
				section{heading: "t.u.z", fields: []string{"a3", "a4", "a5"}},
			),
			want: wantNone,
		},
		"two z headings, each exact for one path": {
			sections: withParents(
				section{heading: "z", fields: []string{"a1", "a2", "a3"}},
				section{heading: "z", fields: []string{"a3", "a4", "a5"}},
			),
			want: wantWarnOnly,
		},
		"two z headings, second fits no path": {
			sections: withParents(
				section{heading: "z", fields: []string{"a1", "a2", "a3"}},
				section{heading: "z", fields: []string{"a1", "a2", "a3", "a4"}},
			),
			want: wantError,
		},
		"two z headings, both x.y.z's fields": {
			sections: withParents(
				section{heading: "z", fields: []string{"a1", "a2", "a3"}},
				section{heading: "z", fields: []string{"a1", "a2", "a3"}},
			),
			want: wantError,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertWant(t, runAcceptance(t, rs, []string{"t", "x"}, tc.sections), tc.want)
		})
		if !hasDuplicateZ(tc.sections) {
			continue
		}
		t.Run(name+" (z headings reversed)", func(t *testing.T) {
			t.Parallel()
			assertWant(t, runAcceptance(t, rs, []string{"t", "x"}, reversedZ(tc.sections)), tc.want)
		})
	}
}

func TestCoverageAcceptance_case2IdenticalFields(t *testing.T) {
	t.Parallel()

	rs := twoZSchema(
		&schema.Block{Attributes: optional("a1", "a2", "a3")},
		&schema.Block{Attributes: optional("a1", "a2", "a3")},
		nil,
	)

	testCases := map[string]struct {
		sections []section
		want     want
	}{
		"one z section": {
			sections: withParents(section{heading: "z", fields: []string{"a1", "a2", "a3"}}),
			want:     wantNone,
		},
		"qualified sections": {
			sections: withParents(
				section{heading: "x.y.z", fields: []string{"a1", "a2", "a3"}},
				section{heading: "t.u.z", fields: []string{"a1", "a2", "a3"}},
			),
			want: wantNone,
		},
		"two z headings, second fits no path": {
			sections: withParents(
				section{heading: "z", fields: []string{"a1", "a2", "a3"}},
				section{heading: "z", fields: []string{"a1", "a2", "a3", "a4"}},
			),
			want: wantError,
		},
		// A duplicated key is always reported, even when every occurrence fits
		// every path: a reader can't tell which block each heading documents.
		"two z headings, both exact": {
			sections: withParents(
				section{heading: "z", fields: []string{"a1", "a2", "a3"}},
				section{heading: "z", fields: []string{"a1", "a2", "a3"}},
			),
			want: wantWarnOnly,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertWant(t, runAcceptance(t, rs, []string{"t", "x"}, tc.sections), tc.want)
		})
		if !hasDuplicateZ(tc.sections) {
			continue
		}
		t.Run(name+" (z headings reversed)", func(t *testing.T) {
			t.Parallel()
			assertWant(t, runAcceptance(t, rs, []string{"t", "x"}, reversedZ(tc.sections)), tc.want)
		})
	}
}

func TestCoverageAcceptance_case3DifferentChildren(t *testing.T) {
	t.Parallel()

	// z has a1, a2, and child block a3 at both paths. differs controls whether
	// a3's contents differ below (b1 vs b2) or match (b1 at both).
	schemaFor := func(differs bool) *schema.ResourceSchema {
		tuA3 := optional("b2")
		if !differs {
			tuA3 = optional("b1")
		}
		return twoZSchema(
			&schema.Block{Attributes: optional("a1", "a2"), ChildBlocks: []string{"x.y.z.a3"}},
			&schema.Block{Attributes: optional("a1", "a2"), ChildBlocks: []string{"t.u.z.a3"}},
			map[string]*schema.Block{
				"x.y.z.a3": {Attributes: optional("b1")},
				"t.u.z.a3": {Attributes: tuA3},
			},
		)
	}
	zFields := []string{"a1", "a2", "a3"}

	testCases := map[string]struct {
		differs  bool
		sections []section
		want     want
	}{
		"one z section, one a3 section listing b1": {
			differs: true,
			sections: withParents(
				section{heading: "z", fields: zFields},
				section{heading: "a3", fields: []string{"b1"}},
			),
			want: wantErrors,
		},
		// The row a same-level comparison misses: every section's fields are
		// correct, but the shared z section's a3 bullet can lead to only one
		// version of a3.
		"one z section, qualified a3 sections": {
			differs: true,
			sections: withParents(
				section{heading: "z", fields: zFields},
				section{heading: "x.y.z.a3", fields: []string{"b1"}},
				section{heading: "t.u.z.a3", fields: []string{"b2"}},
			),
			want: wantWarnOnly,
		},
		"everything qualified": {
			differs: true,
			sections: withParents(
				section{heading: "x.y.z", fields: zFields},
				section{heading: "t.u.z", fields: zFields},
				section{heading: "x.y.z.a3", fields: []string{"b1"}},
				section{heading: "t.u.z.a3", fields: []string{"b2"}},
			),
			want: wantNone,
		},
		"two z headings, qualified a3 sections": {
			differs: true,
			sections: withParents(
				section{heading: "z", fields: zFields},
				section{heading: "z", fields: zFields},
				section{heading: "x.y.z.a3", fields: []string{"b1"}},
				section{heading: "t.u.z.a3", fields: []string{"b2"}},
			),
			want: wantWarnOnly,
		},
		// Negative twin: identical at every depth.
		"one z section, one a3 section, a3 identical at both paths": {
			differs: false,
			sections: withParents(
				section{heading: "z", fields: zFields},
				section{heading: "a3", fields: []string{"b1"}},
			),
			want: wantNone,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertWant(t, runAcceptance(t, schemaFor(tc.differs), []string{"t", "x"}, tc.sections), tc.want)
		})
		if !hasDuplicateZ(tc.sections) {
			continue
		}
		t.Run(name+" (z headings reversed)", func(t *testing.T) {
			t.Parallel()
			assertWant(t, runAcceptance(t, schemaFor(tc.differs), []string{"t", "x"}, reversedZ(tc.sections)), tc.want)
		})
	}
}

// TestCoverageAcceptance_nonTransitive guards the evaluation strategy in
// design §6: agreement between served paths isn't transitive, so comparing
// each path to one representative misses conflicts. Child block k of a shared
// p section has field x as Optional+Computed at a.p.k (home: either), absent
// at b.p.k, and Optional at c.p.k (home: Argument Reference). a agrees with b
// and with c, but b and c conflict. Every k section is qualified and correct,
// so only the shared p section's disjunct 4 can report it.
func TestCoverageAcceptance_nonTransitive(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"":      {ChildBlocks: []string{"a", "b", "c"}},
			"a":     {ChildBlocks: []string{"a.p"}},
			"b":     {ChildBlocks: []string{"b.p"}},
			"c":     {ChildBlocks: []string{"c.p"}},
			"a.p":   {ChildBlocks: []string{"a.p.k"}},
			"b.p":   {ChildBlocks: []string{"b.p.k"}},
			"c.p":   {ChildBlocks: []string{"c.p.k"}},
			"a.p.k": {Attributes: []schema.Attribute{{Name: "w", Optional: true}, {Name: "x", Optional: true, Computed: true}}},
			"b.p.k": {Attributes: optional("w")},
			"c.p.k": {Attributes: optional("w", "x")},
		},
	}
	sections := []section{
		{heading: "a", fields: []string{"p"}},
		{heading: "b", fields: []string{"p"}},
		{heading: "c", fields: []string{"p"}},
		{heading: "p", fields: []string{"k"}},
		{heading: "a.p.k", fields: []string{"w", "x"}},
		{heading: "b.p.k", fields: []string{"w"}},
		{heading: "c.p.k", fields: []string{"w", "x"}},
	}

	assertWant(t, runAcceptance(t, rs, []string{"a", "b", "c"}, sections), wantWarnOnly)
}
