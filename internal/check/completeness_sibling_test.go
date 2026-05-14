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

// TestArgumentsSectionRule_DuplicateBlockNames exercises the scenario where
// multiple schema paths share the same leaf block name (e.g. connection_pool.tcp
// and timeout.tcp) and the doc merges them into a single "### tcp Block" section.
// This is the appmesh_virtual_node pattern.
func TestArgumentsSectionRule_DuplicateBlockNames(t *testing.T) {
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

	rule := &check.ArgumentsSectionRule{IgnoreDeprecated: true}
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

// TestArgumentsSectionRule_DuplicateBlockNames_RealPhantom verifies that a truly
// phantom attribute (one that doesn't exist in ANY sibling block) is still
// reported even when blocks share a leaf name.
func TestArgumentsSectionRule_DuplicateBlockNames_RealPhantom(t *testing.T) {
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

	rule := &check.ArgumentsSectionRule{IgnoreDeprecated: true}
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

// TestArgumentsSectionRule_DuplicateBlockNames_ChildBlockAsAttr verifies that a
// child block name documented as an attribute in a sibling's merged doc section
// is not reported as phantom. This is the per_request case: timeout.grpc has
// per_request as a child block, but the merged grpc doc section lists it as an
// attribute reference to the per_request block.
func TestArgumentsSectionRule_DuplicateBlockNames_ChildBlockAsAttr(t *testing.T) {
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

	rule := &check.ArgumentsSectionRule{IgnoreDeprecated: true}
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

// TestArgumentsSectionRule_DuplicateBlockNames_MissingAttrStillReported ensures
// that a genuinely undocumented attribute is still reported even when sibling
// blocks exist. The sibling suppression only applies to phantom checks, not
// missing-documentation checks.
func TestArgumentsSectionRule_DuplicateBlockNames_MissingAttrStillReported(t *testing.T) {
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

	rule := &check.ArgumentsSectionRule{IgnoreDeprecated: true}
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
