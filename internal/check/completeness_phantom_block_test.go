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
