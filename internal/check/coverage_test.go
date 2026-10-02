// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"fmt"
	"maps"
	"regexp"
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

var (
	phantomRe = regexp.MustCompile(`^documented (?:argument|attribute) "([^"]+)" in block "[^"]+" does not exist in schema`)
	sharedRe  = regexp.MustCompile(`^section "[^"]*" \(line \d+\) in (?:Argument|Attribute) Reference documents \d+ paths; `)
)

// TestSchemaDocsRule_DuplicateBlockNames covers same-named blocks under
// different parents with different fields (the aws_appmesh_virtual_node
// pattern). A merged section is checked against each path it serves, so a
// field listed there that doesn't exist at a path is reported for that path,
// even when a same-named sibling has it (#77). Before #77 the sibling's field
// suppressed the finding (existsInSiblingBlock, fa4e9e4). Qualified headings
// that document each path exactly pass.
func TestSchemaDocsRule_DuplicateBlockNames(t *testing.T) {
	t.Parallel()

	appmesh := &schema.ResourceSchema{Name: "aws_test_resource", Blocks: map[string]*schema.Block{
		"":                         {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"connection_pool", "timeout"}},
		"connection_pool":          {ChildBlocks: []string{"connection_pool.tcp", "connection_pool.grpc"}},
		"connection_pool.tcp":      {Attributes: []schema.Attribute{{Name: "max_connections", Optional: true}}},
		"connection_pool.grpc":     {Attributes: []schema.Attribute{{Name: "max_requests", Optional: true}}},
		"timeout":                  {ChildBlocks: []string{"timeout.tcp", "timeout.grpc"}},
		"timeout.tcp":              {ChildBlocks: []string{"timeout.tcp.idle"}},
		"timeout.tcp.idle":         {Attributes: []schema.Attribute{{Name: "unit", Required: true}, {Name: "value", Required: true}}},
		"timeout.grpc":             {ChildBlocks: []string{"timeout.grpc.idle", "timeout.grpc.per_request"}},
		"timeout.grpc.idle":        {Attributes: []schema.Attribute{{Name: "unit", Required: true}, {Name: "value", Required: true}}},
		"timeout.grpc.per_request": {Attributes: []schema.Attribute{{Name: "unit", Required: true}, {Name: "value", Required: true}}},
	}}
	pools := &schema.ResourceSchema{Name: "aws_test_resource", Blocks: map[string]*schema.Block{
		"":           {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"pool_a", "pool_b"}},
		"pool_a":     {ChildBlocks: []string{"pool_a.tcp"}},
		"pool_a.tcp": {Attributes: []schema.Attribute{{Name: "max_connections", Required: true}}},
		"pool_b":     {ChildBlocks: []string{"pool_b.tcp"}},
		"pool_b.tcp": {Attributes: []schema.Attribute{{Name: "timeout", Optional: true}}},
	}}
	grpcs := &schema.ResourceSchema{Name: "aws_test_resource", Blocks: map[string]*schema.Block{
		"":                      {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"fast", "slow"}},
		"fast":                  {ChildBlocks: []string{"fast.grpc"}},
		"fast.grpc":             {Attributes: []schema.Attribute{{Name: "max_requests", Required: true}}},
		"slow":                  {ChildBlocks: []string{"slow.grpc"}},
		"slow.grpc":             {ChildBlocks: []string{"slow.grpc.per_request"}},
		"slow.grpc.per_request": {Attributes: []schema.Attribute{{Name: "unit", Required: true}, {Name: "value", Required: true}}},
	}}

	// sec renders one H3 section; bullets are "name" or "name>link" for a
	// child-block bullet.
	sec := func(heading string, bullets ...string) string {
		var b strings.Builder
		fmt.Fprintf(&b, "### `%s` Block\n\n", heading)
		for _, f := range bullets {
			fmt.Fprintf(&b, "* `%s` - (Optional) %s.\n", f, f)
		}
		return b.String() + "\n"
	}
	page := func(root []string, sections ...string) string {
		var b strings.Builder
		b.WriteString("## Argument Reference\n\n* `name` - (Required) Name.\n")
		for _, r := range root {
			fmt.Fprintf(&b, "* `%s` - (Optional) %s.\n", r, r)
		}
		b.WriteString("\n")
		for _, s := range sections {
			b.WriteString(s)
		}
		return b.String() + "## Attribute Reference\n\nThis resource exports no additional attributes.\n"
	}

	testCases := map[string]struct {
		rs         *schema.ResourceSchema
		md         string
		want       []string // "path field" pairs reported as not existing
		wantOther  []string // other findings, in order
		wantShared int      // shared-section findings (§6)
	}{
		// Two tcp and two grpc headings, each listing exactly one path's
		// fields. The fit rule matches them, so no field is wrong, but the
		// duplicated headings are still reported (§5).
		"appmesh: duplicate tcp and grpc headings": {
			rs: appmesh,
			md: page([]string{"connection_pool", "timeout"},
				sec("connection_pool", "grpc", "tcp"), sec("timeout", "grpc", "tcp"),
				sec("tcp", "max_connections"), sec("tcp", "idle"),
				sec("grpc", "max_requests"), sec("grpc", "idle", "per_request"),
				sec("idle", "unit", "value"), sec("per_request", "unit", "value")),
			wantOther: []string{
				`2 headings in Argument Reference normalize to "grpc" (lines 25, 29), so a reader can't tell which block each documents; give each the heading of the path it documents (connection_pool.grpc, timeout.grpc)`,
				`2 headings in Argument Reference normalize to "tcp" (lines 17, 21), so a reader can't tell which block each documents; give each the heading of the path it documents (connection_pool.tcp, timeout.tcp)`,
			},
		},
		// One merged tcp section: checked against each path it serves.
		"appmesh: one shared tcp section listing both paths' fields": {
			rs: appmesh,
			md: page([]string{"connection_pool", "timeout"},
				sec("connection_pool", "grpc", "tcp"), sec("timeout", "grpc", "tcp"),
				sec("tcp", "idle", "max_connections"),
				sec("connection_pool.grpc", "max_requests"), sec("timeout.grpc", "idle", "per_request"),
				sec("idle", "unit", "value"), sec("per_request", "unit", "value")),
			want:       []string{"connection_pool.tcp idle", "timeout.tcp max_connections"},
			wantShared: 1,
		},
		"appmesh: qualified sections": {
			rs: appmesh,
			md: page([]string{"connection_pool", "timeout"},
				sec("connection_pool", "grpc", "tcp"), sec("timeout", "grpc", "tcp"),
				sec("connection_pool.tcp", "max_connections"), sec("timeout.tcp", "idle"),
				sec("connection_pool.grpc", "max_requests"), sec("timeout.grpc", "idle", "per_request"),
				sec("idle", "unit", "value"), sec("per_request", "unit", "value")),
		},
		"pools: merged tcp section with a field no block has": {
			rs: pools,
			md: page([]string{"pool_a", "pool_b"},
				sec("pool_a", "tcp"), sec("pool_b", "tcp"),
				sec("tcp", "max_connections", "timeout", "bogus_attr")),
			want: []string{
				"pool_a.tcp bogus_attr", "pool_a.tcp timeout",
				"pool_b.tcp bogus_attr", "pool_b.tcp max_connections",
			},
			wantShared: 1,
		},
		"grpc: merged section lists another path's child block": {
			rs: grpcs,
			md: page([]string{"fast", "slow"},
				sec("fast", "grpc"), sec("slow", "grpc"),
				sec("grpc", "max_requests", "per_request"),
				sec("per_request", "unit", "value")),
			want:       []string{"fast.grpc per_request", "slow.grpc max_requests"},
			wantShared: 1,
		},
		"grpc: qualified sections": {
			rs: grpcs,
			md: page([]string{"fast", "slow"},
				sec("fast", "grpc"), sec("slow", "grpc"),
				sec("fast.grpc", "max_requests"), sec("slow.grpc", "per_request"),
				sec("per_request", "unit", "value")),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.Parse([]byte(tc.md), tc.rs.Name)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			off := false
			rule := &check.SchemaDocsRule{IgnoreDeprecated: true, Ordering: &off, Labels: &off, Heading: &off}
			results := rule.Check(check.CheckContext{Resource: tc.rs.Name, Schema: tc.rs, Doc: d})

			var got, other []string
			shared := 0
			for _, r := range results {
				if m := phantomRe.FindStringSubmatch(r.Message); m != nil {
					got = append(got, r.Block+" "+m[1])
					continue
				}
				if sharedRe.MatchString(r.Message) {
					shared++
					continue
				}
				other = append(other, r.Message)
			}
			if shared != tc.wantShared {
				t.Errorf("shared-section findings = %d, want %d", shared, tc.wantShared)
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("phantoms = %q\nwant      %q", got, tc.want)
			}
			if !slices.Equal(other, tc.wantOther) {
				t.Errorf("other findings =\n  %s\nwant\n  %s", strings.Join(other, "\n  "), strings.Join(tc.wantOther, "\n  "))
			}
		})
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

// TestCoverage_OrphanedBullets: headings the parser can't read, and lists
// introduced by prose, leave their bullets in no section. Coverage reports
// them so those fields aren't silently unchecked, and suggests a heading when
// the text names a schema block (#77).
func TestCoverage_OrphanedBullets(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                             {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"source", "secondary_sources", "action", "error_action"}},
		"source":                       {Attributes: optional("location"), ChildBlocks: []string{"source.auth"}},
		"source.auth":                  {Attributes: optional("resource")},
		"secondary_sources":            {Attributes: optional("location"), ChildBlocks: []string{"secondary_sources.auth"}},
		"secondary_sources.auth":       {Attributes: optional("resource")},
		"action":                       {ChildBlocks: []string{"action.cloudwatch_logs"}},
		"action.cloudwatch_logs":       {Attributes: optional("role_arn")},
		"error_action":                 {ChildBlocks: []string{"error_action.cloudwatch_logs"}},
		"error_action.cloudwatch_logs": {Attributes: optional("role_arn")},
	}}
	page := func(body string) string {
		return "## Argument Reference\n\n* `name` - (Required) Name.\n\n" + body
	}

	testCases := map[string]struct {
		md   string
		want []string // orphan findings
	}{
		"unparseable heading naming a qualified block": {
			md:   page("### source: auth\n\n* `resource` - (Optional) Resource.\n"),
			want: []string{"heading \"source: auth\" in Argument Reference isn't a recognized block heading, so its bullets aren't checked against the schema; use a block heading, e.g. \"`source.auth` Block\""},
		},
		"unparseable heading naming no block": {
			md:   page("### Waiting for Capacity\n\n* `resource` - (Optional) Resource.\n"),
			want: []string{"heading \"Waiting for Capacity\" in Argument Reference isn't a recognized block heading, so its bullets aren't checked against the schema"},
		},
		// The list holds the section's own fields, so it continues the
		// section: no finding, and the fields stay documented.
		"prose continuing the section": {
			md: page("* `source` - (Optional) Source.\n\nThis resource supports the same arguments as `aws_instance`, with the addition of:\n\n* `action` - (Optional) Action.\n"),
		},
		"prose naming a leaf shared by several blocks": {
			md:   page("The `cloudwatch_logs` object takes the following arguments:\n\n* `role_arn` - (Optional) Role.\n"),
			want: []string{"list introduced by prose (\"The `cloudwatch_logs` object takes the following arguments:\") in Argument Reference has no block heading, so its bullets aren't checked against the schema; use a block heading, e.g. \"`cloudwatch_logs` Block\""},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.ParseWithTemplates([]byte(tc.md), "aws_thing", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
			if err != nil {
				t.Fatal(err)
			}
			results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
			var got []string
			for _, r := range results {
				if strings.Contains(r.Message, "aren't checked against the schema") {
					got = append(got, r.Message)
				}
				if strings.Contains(r.Message, `"(root)" does not exist`) {
					t.Errorf("orphaned bullet credited to the root: %s", r.Message)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("orphan findings =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

// TestDescriptions_OrphanedBullets: bullets that belong to no section still
// get the description check, which doesn't depend on the block. The finding
// names the heading or prose instead (#77).
func TestDescriptions_OrphanedBullets(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                       {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"action"}},
		"action":                 {ChildBlocks: []string{"action.cloudwatch_logs"}},
		"action.cloudwatch_logs": {Attributes: optional("role_arn")},
	}}
	testCases := map[string]struct {
		body string
		want string
	}{
		"under an unparseable heading": {
			body: "### Waiting for Capacity\n\n* `role_arn` - (Optional) The role.\n",
			want: `attribute "role_arn" description should not start with "The" (under heading "Waiting for Capacity")`,
		},
		"in an orphaned prose list": {
			body: "* `name` - (Required) Name.\n\nThe `cloudwatch_logs` object takes the following arguments:\n\n* `role_arn` - (Optional) The role.\n",
			want: `attribute "role_arn" description should not start with "The" (in the list introduced by prose at line 5)`,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.ParseWithTemplates([]byte("## Argument Reference\n\n"+tc.body), "aws_thing", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
			if err != nil {
				t.Fatal(err)
			}
			results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
			if !hasMessage(results, tc.want) {
				t.Errorf("missing %q in:\n  %s", tc.want, joinMessages(results))
			}
		})
	}
}

// TestCoverage_DuplicateHeadingClosestPrefersContent: when naming the closest
// block for a heading that fits none, a block whose fields match but whose
// label differs is nearer than one with a missing field
// (aws_cognito_risk_configuration's two `actions` headings).
func TestCoverage_DuplicateHeadingClosestPrefersContent(t *testing.T) {
	t.Parallel()

	// t.u.z has a2 and a3, both Required; x.y.z has a2, a3, a4, all Optional.
	// The second heading lists a2 and a3 as (Optional): against t.u.z that's
	// two wrong labels, against x.y.z one missing field. By count x.y.z is
	// closer; by content t.u.z is.
	rs := twoZSchema(
		&schema.Block{Attributes: optional("a2", "a3", "a4")},
		&schema.Block{Attributes: []schema.Attribute{{Name: "a2", Required: true}, {Name: "a3", Required: true}}},
		nil,
	)
	sections := withParents(section{heading: "z", fields: []string{"a2", "a3", "a4"}}, section{heading: "z", fields: []string{"a2", "a3"}})
	results := runAcceptance(t, rs, []string{"t", "x"}, sections)
	want := `heading "z Block" (line 30) in Argument Reference documents no block exactly; the closest is "t.u.z": "a2" should be (Required); "a3" should be (Required)`
	if !hasMessage(results, want) {
		t.Errorf("missing %q in:\n  %s", want, joinMessages(results))
	}
}

// TestCoverage_DuplicateHeadingFitSortsDifferences: the fit rule lists the
// closest heading's differences. The provider schema builds attribute slices
// from maps, so their order varies between runs; the list must be sorted or
// the message flaps (#77). The attributes here are deliberately unsorted.
func TestCoverage_DuplicateHeadingFitSortsDifferences(t *testing.T) {
	t.Parallel()

	many := make([]schema.Attribute, 0, 12)
	for _, n := range []string{"k", "c", "h", "a", "j", "e", "b", "l", "f", "d", "i", "g"} {
		many = append(many, schema.Attribute{Name: n, Optional: true})
	}
	rs := twoZSchema(&schema.Block{Attributes: many}, &schema.Block{Attributes: optional("a")}, nil)
	sections := withParents(section{heading: "z", fields: []string{"a"}}, section{heading: "z", fields: []string{"a", "m"}})

	results := runAcceptance(t, rs, []string{"t", "x"}, sections)
	want := `documents block "x.y.z" exactly; the closest, at line 24: "b" isn't listed; "c" isn't listed; "d" isn't listed; and 8 more`
	if !hasMessage(results, want) {
		t.Errorf("missing %q in:\n  %s", want, joinMessages(results))
	}
}

// TestCoverage_UnresolvedSections: a parsed heading that no schema path
// resolves to has its fields compared against nothing, so it's reported
// (#77). Not reported: headings with no bullets, Argument Reference headings
// checkPhantomBlocks already reports, and Attribute Reference headings naming
// object-typed attributes.
func TestCoverage_UnresolvedSections(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {Attributes: []schema.Attribute{
			{Name: "name", Required: true},
			{Name: "endpoints", Computed: true, Children: []schema.Attribute{{Name: "dns_name", Computed: true}}},
		}, ChildBlocks: []string{"a", "b"}},
		"a":       {Attributes: optional("x"), ChildBlocks: []string{"a.match"}},
		"a.match": {Attributes: optional("y")},
		"b":       {Attributes: optional("x")},
	}}
	page := func(args, attrs string) string {
		return "## Argument Reference\n\n* `name` - (Required) Name.\n* `a` - (Optional) A.\n* `b` - (Optional) B.\n\n" + args +
			"## Attribute Reference\n\n" + attrs
	}
	const exact = "### `a` Block\n\n* `x` - (Optional) X.\n* `match` - (Optional) M.\n\n### `a.match` Block\n\n* `y` - (Optional) Y.\n\n### `b` Block\n\n* `x` - (Optional) X.\n\n"

	testCases := map[string]struct {
		md   string
		want []string
	}{
		"titled heading in Attribute Reference": {
			md:   page(exact, "### Disk IOPS\n\n* `mode` - Mode.\n"),
			want: []string{`heading "Disk IOPS" in Attribute Reference documents no block (no schema block has that name), so its fields aren't checked against the schema`},
		},
		"heading with no bullets": {
			md: page(exact, "### GuardDuty Cleanup Permissions\n\nSome prose.\n"),
		},
		"bare leaf whose only path has a more specific heading": {
			md:   page(exact+"### `match` Block\n\n* `y` - (Optional) Y.\n\n", ""),
			want: []string{`heading "match Block" in Argument Reference documents no block (every block named "match" resolves to another heading), so its fields aren't checked against the schema`},
		},
		"dotted key with no such path": {
			md:   page(exact+"### `b.match` Block\n\n* `y` - (Optional) Y.\n\n", ""),
			want: []string{`heading "b.match Block" in Argument Reference documents no block (no block named "match" sits under "b"), so its fields aren't checked against the schema`},
		},
		"object-typed attribute heading in Attribute Reference": {
			md: page(exact, "### `endpoints` Block\n\n* `dns_name` - DNS name.\n"),
		},
		"Argument Reference heading with no schema block (checkPhantomBlocks reports it)": {
			md: page(exact+"### `nope` Block\n\n* `z` - (Optional) Z.\n\n", ""),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.ParseWithTemplates([]byte(tc.md), "aws_thing", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block", "{Title}"})
			if err != nil {
				t.Fatal(err)
			}
			results := (&check.SchemaDocsRule{IgnoreDeprecated: true}).Check(check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
			var got []string
			for _, r := range results {
				if strings.Contains(r.Message, "documents no block (") {
					got = append(got, r.Message)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("unresolved findings =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

// TestCoverage_SharedSectionMessages pins two details of the shared-section
// finding found on the corpus (#77): a field that is an attribute at one path
// and a child block at another isn't a label conflict, and a disjunct-4
// finding names the two parent paths whose children differ.
func TestCoverage_SharedSectionMessages(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		rs       *schema.ResourceSchema
		sections []section
		want     []string // substrings, each in some shared-section finding
		wantNot  string   // substring no finding may contain
	}{
		"attribute at one path, child block at another": {
			rs: twoZSchema(
				&schema.Block{Attributes: optional("a1", "tls")},
				&schema.Block{Attributes: optional("a1"), ChildBlocks: []string{"t.u.z.tls"}},
				map[string]*schema.Block{"t.u.z.tls": {Attributes: optional("b1")}},
			),
			sections: withParents(section{heading: "z", fields: []string{"a1", "tls"}}, section{heading: "t.u.z.tls", fields: []string{"b1"}}),
			wantNot:  "can't label it correctly",
		},
		// Recursion through one name (WAFv2's statement.and_statement...):
		// section a serves a and a.k.a, whose k children differ. The finding
		// must name the two parents, not one twice.
		"children differ below, recursive name": {
			rs: &schema.ResourceSchema{Name: "aws_test", Blocks: map[string]*schema.Block{
				"":        {ChildBlocks: []string{"a"}},
				"a":       {Attributes: optional("a1"), ChildBlocks: []string{"a.k"}},
				"a.k":     {Attributes: optional("b1"), ChildBlocks: []string{"a.k.a"}},
				"a.k.a":   {Attributes: optional("a1"), ChildBlocks: []string{"a.k.a.k"}},
				"a.k.a.k": {Attributes: optional("b1")},
			}},
			sections: []section{
				{heading: "a", fields: []string{"a1", "k"}},
				{heading: "a.k", fields: []string{"a", "b1"}},
				{heading: "a.k.a.k", fields: []string{"b1"}},
			},
			want: []string{`"k" differs below this level between "a" and "a.k.a"`},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := []string{"t", "x"}
			if _, ok := tc.rs.Blocks["a"]; ok {
				root = []string{"a"}
			}
			results := runAcceptance(t, tc.rs, root, tc.sections)
			for _, w := range tc.want {
				if !hasMessage(results, w) {
					t.Errorf("missing %q in:\n  %s", w, joinMessages(results))
				}
			}
			if tc.wantNot != "" && hasMessage(results, tc.wantNot) {
				t.Errorf("unexpected %q in:\n  %s", tc.wantNot, joinMessages(results))
			}
		})
	}
}

// TestCoverage_OutsideHome covers the §2 home-section check: a
// pure-configurable argument documented only under Attribute Reference with
// no label is reported by coverage. Labeled bullets stay with labels' move
// finding, and fields with no single home are never reported.
func TestCoverage_OutsideHome(t *testing.T) {
	t.Parallel()

	const msg = "is documented under Attribute Reference but is a configurable argument in the schema"
	testCases := map[string]struct {
		root, child []schema.Attribute
		unknown     bool   // child is ConfigUnknown
		args, attrs string // root-level bullets
		nArgs       string // bullets of child's Argument Reference section; "" omits it
		nAttrs      string // bullets of child's Attribute Reference section; "" omits it
		want        []string
		wantWarn    bool
	}{
		"root required, unlabeled under attributes": {
			root:     []schema.Attribute{{Name: "name", Required: true}, {Name: "arn", Computed: true}},
			attrs:    "* `arn` - ARN.\n* `name` - Name.\n",
			want:     []string{`argument "name" ` + msg + "; move it to Argument Reference and label it (Required)"},
			wantWarn: true,
		},
		"nested optional, unlabeled under attributes": {
			child:  []schema.Attribute{{Name: "size", Optional: true}},
			nAttrs: "* `size` - Size.\n",
			want:   []string{`argument "size" in block "c" ` + msg + "; move it to Argument Reference and label it (Optional)"},
		},
		// The computed sibling keeps labels from collapsing the section into
		// one "move this subsection" finding.
		"labeled: left to labels": {
			child:  []schema.Attribute{{Name: "size", Optional: true}, {Name: "state", Computed: true}},
			nAttrs: "* `size` - (Optional) Size.\n* `state` - State.\n",
			want:   []string{`argument "size" in block "c" ` + msg + "; move it to Argument Reference"},
		},
		"also under arguments": {
			child:  []schema.Attribute{{Name: "size", Optional: true}},
			nArgs:  "* `size` - (Optional) Size.\n",
			nAttrs: "* `size` - Size.\n",
		},
		"optional+computed has two homes": {
			child:  []schema.Attribute{{Name: "size", Optional: true, Computed: true}},
			nAttrs: "* `size` - Size.\n",
		},
		"config unknown": {
			child:   []schema.Attribute{{Name: "size", Optional: true}},
			unknown: true,
			nAttrs:  "* `size` - Size.\n",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := append([]schema.Attribute{{Name: "id", Computed: true}}, tc.root...)
			rs := &schema.ResourceSchema{Name: "aws_test", Blocks: map[string]*schema.Block{
				"":  {Attributes: root, ChildBlocks: []string{"c"}},
				"c": {Attributes: tc.child, ConfigUnknown: tc.unknown},
			}}
			md := "## Argument Reference\n\n" + tc.args + "* `c` - (Optional) C.\n\n"
			if tc.nArgs != "" {
				md += "### `c` Block\n\n" + tc.nArgs + "\n"
			}
			md += "## Attribute Reference\n\n* `id` - ID.\n" + tc.attrs + "\n"
			if tc.nAttrs != "" {
				md += "### `c` Block\n\n" + tc.nAttrs
			}
			d, err := doc.Parse([]byte(md), rs.Name)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			off := false
			rule := &check.SchemaDocsRule{Ordering: &off, Description: &off, Format: &off, Byline: &off, Heading: &off}
			results := rule.Check(check.CheckContext{Resource: rs.Name, Schema: rs, Doc: d})
			var got []string
			for _, r := range results {
				if strings.Contains(r.Message, msg) {
					got = append(got, r.Message)
					if (r.Severity == check.SeverityWarning) != tc.wantWarn {
						t.Errorf("severity = %v, want warning = %t", r.Severity, tc.wantWarn)
					}
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got:\n  %s\nwant:\n  %s\nall:\n  %s", strings.Join(got, "\n  "), strings.Join(tc.want, "\n  "), joinMessages(results))
			}
		})
	}
}

// TestSchemaDocs_MarkerMismatchIsError pins that a wrong label or deprecation
// marker is an error wherever it's found: in a single section (labels,
// deprecation), in a section shared by paths that disagree (shared-section
// disjuncts 2 and 3), and under duplicate headings (the fit rule).
func TestSchemaDocs_MarkerMismatchIsError(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		rs   *schema.ResourceSchema
		md   string
		want string // substring of the finding that must be an error
	}{
		"wrong label": {
			rs:   &schema.ResourceSchema{Name: "aws_test", Blocks: map[string]*schema.Block{"": {Attributes: []schema.Attribute{{Name: "name", Required: true}}}}},
			md:   "## Argument Reference\n\n* `name` - (Optional) Name.\n",
			want: `argument "name" is labeled (Optional) but is required in the schema`,
		},
		"deprecated in schema, not in docs": {
			rs:   &schema.ResourceSchema{Name: "aws_test", Blocks: map[string]*schema.Block{"": {Attributes: []schema.Attribute{{Name: "name", Optional: true, Deprecated: true}}}}},
			md:   "## Argument Reference\n\n* `name` - (Optional) Name.\n",
			want: "is deprecated in schema but not marked as deprecated in docs",
		},
		"deprecated in docs, not in schema": {
			rs:   &schema.ResourceSchema{Name: "aws_test", Blocks: map[string]*schema.Block{"": {Attributes: []schema.Attribute{{Name: "name", Optional: true}}}}},
			md:   "## Argument Reference\n\n* `name` - (Optional, **Deprecated**) Name.\n",
			want: "is marked deprecated in docs but not in schema",
		},
		"shared section, label differs": {
			rs: twoZSchema(
				&schema.Block{Attributes: []schema.Attribute{{Name: "a1", Required: true}}},
				&schema.Block{Attributes: optional("a1")},
				nil,
			),
			md:   renderAcceptanceDoc([]string{"t", "x"}, withParents(section{heading: "z", fields: []string{"a1"}})),
			want: `"a1" is (Optional) at "t.u.z" and (Required) at "x.y.z"`,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.Parse([]byte(tc.md), tc.rs.Name)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			off := false
			rule := &check.SchemaDocsRule{Ordering: &off, Description: &off, Format: &off, Byline: &off, Heading: &off}
			results := rule.Check(check.CheckContext{Resource: tc.rs.Name, Schema: tc.rs, Doc: d})
			i := slices.IndexFunc(results, func(r check.Result) bool { return strings.Contains(r.Message, tc.want) })
			if i < 0 {
				t.Fatalf("no finding containing %q in:\n  %s", tc.want, joinMessages(results))
			}
			if results[i].Severity != check.SeverityError {
				t.Errorf("%q: severity %v, want error", results[i].Message, results[i].Severity)
			}
		})
	}
}

// TestCoverage_UndocumentedSubtreeReportsShallowest: only the shallowest
// undocumented block in a subtree is reported, with the count of undocumented
// paths beneath it. Descendants can't be reached from the docs, so the real
// defect is the missing ancestor section (#77). An ancestor without
// configurable fields is never reported, so it must not absorb descendants.
func TestCoverage_UndocumentedSubtreeReportsShallowest(t *testing.T) {
	t.Parallel()

	opt := func(n string) []schema.Attribute { return []schema.Attribute{{Name: n, Optional: true}} }
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":         {Attributes: []schema.Attribute{{Name: "name", Required: true}}, ChildBlocks: []string{"a", "c"}},
		"a":        {Attributes: opt("x"), ChildBlocks: []string{"a.b"}},
		"a.b":      {Attributes: opt("y"), ChildBlocks: []string{"a.b.leaf"}},
		"a.b.leaf": {Attributes: opt("z")},
		"c":        {ChildBlocks: []string{"c.d"}}, // container: no configurable fields
		"c.d":      {Attributes: opt("w"), ChildBlocks: []string{"c.d.leaf"}},
		"c.d.leaf": {Attributes: opt("z")},
	}}
	src := "# Resource: aws_thing\n\n## Argument Reference\n\n* `name` - (Required) Name.\n"

	testCases := map[string]struct {
		skip []string
		want []string
	}{
		"shallowest undocumented block per subtree": {
			want: []string{
				`block "a" is not documented (2 paths beneath it are also undocumented)`,
				`block "c.d" is not documented (1 path beneath it is also undocumented)`,
			},
		},
		// A skipped ancestor isn't a finding, so it absorbs nothing.
		"skipped ancestor": {
			skip: []string{"a"},
			want: []string{
				`block "a.b" is not documented (1 path beneath it is also undocumented)`,
				`block "c.d" is not documented (1 path beneath it is also undocumented)`,
			},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d, err := doc.ParseWithTemplates([]byte(src), "aws_thing", labelTemplates)
			if err != nil {
				t.Fatal(err)
			}
			results := (&check.SchemaDocsRule{IgnoreDeprecated: true, SkipBlocks: tc.skip}).Check(
				check.CheckContext{Resource: "aws_thing", Schema: rs, Doc: d})
			var got []string
			for _, r := range results {
				if strings.Contains(r.Message, "is not documented") {
					got = append(got, r.Message)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("missing-block findings = %q\nwant %q", got, tc.want)
			}
		})
	}
}

// Several undocumented blocks that share a leaf name are each reported under
// their own path: findings are keyed by path, never by leaf (#77). Before #77 a
// leaf-keyed dedup kept one representative, and before #65 that representative
// was chosen by Go's randomized map iteration, so the reported path flapped.
// Coverage must produce identical output every run.
func TestCoverage_UndocumentedSiblingBlocksDeterministic(t *testing.T) {
	t.Parallel()

	// foo.thing and bar.thing are distinct undocumented blocks that share the
	// leaf name "thing"; each is reported.
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

	want := []string{
		`block "bar.thing" is not documented (1 other undocumented path shares the name "thing")`,
		`block "foo.thing" is not documented (1 other undocumented path shares the name "thing")`,
	}
	if !slices.Equal(first, want) {
		t.Fatalf("missing-block findings = %q, want %q", first, want)
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
		// A heading with no bullets ("See `z` above.") documents nothing, so it
		// isn't fitted; the other heading serves both identical paths.
		"two z headings, one with no bullets": {
			sections: withParents(
				section{heading: "z", fields: []string{"a1", "a2", "a3"}},
				section{heading: "z"},
			),
			want: wantWarnOnly,
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

// The heading ambiguity warning moved into coverage as the shared-section
// finding (#77, docs/rules/coverage-path-resolution.md §6). These tests kept
// their scenarios and now assert that finding.

// TestSchemaDocsRule_FullPathHeadings_DescendantsOwnHeadings is a
// regression test for the self-suggesting ambiguity false positive on
// aws_appmesh_gateway_route / aws_appmesh_route. A parent block
// (spec.http2_route.match) and its descendants
// (…match.header.match, …match.query_parameter.match) all share the
// leaf "match" and are structurally distinct, so the leaf is ambiguous.
// But every block has its own exact full-path heading, so the parent
// heading is NOT ambiguous.
//
// The old resolver evaluated the parent's doc key in isolation and, via
// the non-contiguous composite matcher (parts[0].parts[1].leaf), counted
// the descendants as also "resolving" to the parent key — producing an
// ambiguity warning whose suggested fix was the identical string the
// author had already written. With most-specific-match resolution, each
// descendant is owned by its own full-path heading, so the parent
// resolves to exactly one block.
func TestSchemaDocsRule_FullPathHeadings_DescendantsOwnHeadings(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"spec"},
			},
			"spec":             {ChildBlocks: []string{"spec.http2_route"}},
			"spec.http2_route": {ChildBlocks: []string{"spec.http2_route.match"}},
			// Three distinct blocks sharing the leaf "match" -> ambiguous leaf.
			"spec.http2_route.match": {
				Attributes:  []schema.Attribute{{Name: "prefix", Optional: true}},
				ChildBlocks: []string{"spec.http2_route.match.header", "spec.http2_route.match.query_parameter"},
			},
			"spec.http2_route.match.header":       {ChildBlocks: []string{"spec.http2_route.match.header.match"}},
			"spec.http2_route.match.header.match": {Attributes: []schema.Attribute{{Name: "exact", Optional: true}}},
			"spec.http2_route.match.query_parameter": {
				ChildBlocks: []string{"spec.http2_route.match.query_parameter.match"},
			},
			"spec.http2_route.match.query_parameter.match": {Attributes: []schema.Attribute{{Name: "regex", Optional: true}}},
		},
	}

	// Every match block has its own exact full-path heading.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `spec` - (Required) Spec.\n\n" +
		"### `spec.http2_route.match` Block\n\n" +
		"* `prefix` - (Optional) Prefix.\n\n" +
		"### `spec.http2_route.match.header.match` Block\n\n" +
		"* `exact` - (Optional) Exact.\n\n" +
		"### `spec.http2_route.match.query_parameter.match` Block\n\n" +
		"* `regex` - (Optional) Regex.\n"

	d, err := doc.ParseWithTemplates([]byte(markdown), "aws_test", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"}}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if sharedRe.MatchString(r.Message) {
			t.Errorf("full-path heading whose descendants have their own headings must not get a shared-section finding; got: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_IdenticalRepeatedBlocks_NotAmbiguous is a
// regression test for the order-dependent block-signature bug. Two
// blocks sharing a leaf ("config") have the identical attribute set but
// list the attributes in different declaration order. Because schema
// attributes are loaded from a Go map, declaration order is not stable;
// an unsorted signature made these compare unequal, falsely marking the
// leaf ambiguous (nondeterministically). A shared heading for two
// structurally identical blocks is unambiguous, so no warning is
// expected.
func TestSchemaDocsRule_IdenticalRepeatedBlocks_NotAmbiguous(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"config", "parent"},
			},
			// Same attribute set {auth_ttl, issuer}, different order.
			"config": {Attributes: []schema.Attribute{
				{Name: "auth_ttl", Optional: true},
				{Name: "issuer", Required: true},
			}},
			"parent": {ChildBlocks: []string{"parent.config"}},
			"parent.config": {Attributes: []schema.Attribute{
				{Name: "issuer", Required: true},
				{Name: "auth_ttl", Optional: true},
			}},
		},
	}

	// Only the bare "config" heading exists; parent.config has none, so
	// the bare key would resolve to both blocks. The blocks are
	// identical, so this is not a real ambiguity.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `config` - (Optional) Config.\n" +
		"* `parent` - (Optional) Parent.\n\n" +
		"### `config` Block\n\n" +
		"* `auth_ttl` - (Optional) TTL.\n" +
		"* `issuer` - (Required) Issuer.\n"

	d, err := doc.ParseWithTemplates([]byte(markdown), "aws_test", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"}}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	for _, r := range results {
		if sharedRe.MatchString(r.Message) {
			t.Errorf("structurally identical repeated blocks must not get a shared-section finding; got: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_GenuineAmbiguity_StillWarns guards against
// over-suppression. Two blocks share the leaf "config" but are
// structurally DISTINCT, and only the bare top-level heading exists —
// the nested block has no dedicated heading, so the bare key genuinely
// covers both. This is a real ambiguity and must still get a shared-section finding.
func TestSchemaDocsRule_GenuineAmbiguity_StillWarns(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"config", "parent"},
			},
			"config": {Attributes: []schema.Attribute{{Name: "issuer", Required: true}}},
			"parent": {ChildBlocks: []string{"parent.config"}},
			// Distinct from top-level config (extra attribute).
			"parent.config": {Attributes: []schema.Attribute{
				{Name: "issuer", Required: true},
				{Name: "extra", Optional: true},
			}},
		},
	}

	// Only the bare "config" heading; parent.config has none.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `config` - (Optional) Config.\n" +
		"* `parent` - (Optional) Parent.\n\n" +
		"### `config` Block\n\n" +
		"* `issuer` - (Required) Issuer.\n"

	d, err := doc.ParseWithTemplates([]byte(markdown), "aws_test", doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	rule := &check.SchemaDocsRule{Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"}}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	var found bool
	for _, r := range results {
		if sharedRe.MatchString(r.Message) && strings.HasPrefix(r.Message, `section "config"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("genuine ambiguity (distinct blocks, only bare heading) must still get a shared-section finding; got %d results", len(results))
		for _, r := range results {
			t.Logf("  result: %s", r.Message)
		}
	}
}

// TestSchemaDocsRule_PathKeyedHeadings_PartialPathStillAmbiguous
// covers the case Copilot flagged: a partial-path key like
// `header.match` (from a `{Parent}` template or similar) still
// resolves to multiple schema paths under the suffix-composite
// lookup. The ambiguity warning must fire in this case even though
// the doc key contains a dot.
func TestSchemaDocsRule_PathKeyedHeadings_PartialPathStillAmbiguous(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{
		Name: "aws_test",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"spec"},
			},
			"spec": {
				ChildBlocks: []string{"spec.http_route", "spec.http2_route"},
			},
			"spec.http_route": {
				ChildBlocks: []string{"spec.http_route.header"},
			},
			"spec.http_route.header": {
				ChildBlocks: []string{"spec.http_route.header.match"},
			},
			"spec.http_route.header.match": {
				Attributes: []schema.Attribute{{Name: "exact", Optional: true}},
			},
			"spec.http2_route": {
				ChildBlocks: []string{"spec.http2_route.header"},
			},
			"spec.http2_route.header": {
				ChildBlocks: []string{"spec.http2_route.header.match"},
			},
			"spec.http2_route.header.match": {
				Attributes: []schema.Attribute{{Name: "regex", Optional: true}},
			},
		},
	}

	// Heading uses a {Parent} template producing doc key
	// "header.match". This 2-segment composite resolves to BOTH
	// schema paths via the suffix-composite lookup, so the doc key
	// does not actually disambiguate.
	markdown := "## Argument Reference\n\n" +
		"* `name` - (Required) Name.\n" +
		"* `spec` - (Required) Spec.\n\n" +
		"### `header` `match` Block\n\n" +
		"* `exact` - (Optional) Exact.\n" +
		"* `regex` - (Optional) Regex.\n"

	// Use a parent template so the heading parses to "header.match".
	templates := doc.HeadingTemplates{
		"`{Parent}` `{Block}` Block",
		"`{Path}` Block",
		"`{Block}` Block",
	}
	d, err := doc.ParseWithTemplates([]byte(markdown), "aws_test", templates)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Confirm the doc actually keyed the block as "header.match" via
	// the {Parent} template — otherwise the test isn't exercising the
	// scenario.
	if _, ok := d.ArgumentBlocks["header.match"]; !ok {
		var keys []string
		for k := range d.ArgumentBlocks {
			keys = append(keys, k)
		}
		t.Fatalf("expected ArgumentBlocks[\"header.match\"] (got keys: %v)", keys)
	}

	rule := &check.SchemaDocsRule{
		Preferred: doc.HeadingTemplates{"`{Path}` Block", "`{Block}` Block"},
	}
	results := rule.Check(check.CheckContext{Resource: "aws_test", Schema: rs, Doc: d})

	var ambiguityFound bool
	for _, r := range results {
		if !sharedRe.MatchString(r.Message) {
			continue
		}
		if strings.HasPrefix(r.Message, `section "header.match"`) &&
			(strings.Contains(r.Message, "spec.http_route.header.match") || strings.Contains(r.Message, "spec.http2_route.header.match")) {
			ambiguityFound = true
		}
	}
	if !ambiguityFound {
		t.Errorf("expected shared-section finding on partial-path key 'header.match' citing both colliding schema paths; got %d results", len(results))
		for _, r := range results {
			t.Logf("  result: %s", r.Message)
		}
	}
}
