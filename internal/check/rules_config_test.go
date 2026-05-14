// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// --- SchemaDocsRule magic-value config tests ---------------------------

func TestSchemaDocsRule_DefaultImplicitAttributesSkipped(t *testing.T) {
	t.Parallel()

	// "id" and "tags_all" are in DefaultImplicitAttributes and must never
	// appear as "not documented" errors even when absent from the doc.
	rs := &schema.ResourceSchema{
		Name: "test_thing",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "id", Computed: true},
					{Name: "tags_all", Computed: true},
					{Name: "name", Required: true},
				},
			},
		},
	}
	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) Name.

## Attribute Reference

This resource exports no additional attributes.
`), "test_thing")

	rule := &check.SchemaDocsRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "test_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("unexpected error (id/tags_all should be implicit): %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_CustomImplicitAttributes(t *testing.T) {
	t.Parallel()

	// Override implicit list to include "region" — a provider-injected attr.
	rs := &schema.ResourceSchema{
		Name: "test_thing",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes: []schema.Attribute{
					{Name: "region", Optional: true},
					{Name: "name", Required: true},
				},
			},
		},
	}
	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) Name.

## Attribute Reference

This resource exports no additional attributes.
`), "test_thing")

	rule := &check.SchemaDocsRule{
		IgnoreDeprecated:   true,
		ImplicitAttributes: []string{"id", "tags_all", "region"},
	}
	results := rule.Check(check.CheckContext{Resource: "test_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("region should be implicit with custom list; got: %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_CustomSkipBlocks(t *testing.T) {
	t.Parallel()

	// Override skip_blocks to also skip "network" — useful for providers
	// that document network blocks in a separate guide.
	rs := &schema.ResourceSchema{
		Name: "test_thing",
		Blocks: map[string]*schema.Block{
			"": {
				Attributes:  []schema.Attribute{{Name: "name", Required: true}},
				ChildBlocks: []string{"network"},
			},
			"network": {
				Attributes: []schema.Attribute{{Name: "subnet_id", Required: true}},
			},
		},
	}
	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) Name.

## Attribute Reference

This resource exports no additional attributes.
`), "test_thing")

	rule := &check.SchemaDocsRule{
		IgnoreDeprecated: true,
		SkipBlocks:       []string{"timeouts", "network"},
	}
	results := rule.Check(check.CheckContext{Resource: "test_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("network block should be skipped; got: %s", r.Message)
		}
	}
}

func TestSchemaDocsRule_DefaultSkipBlocksContainsTimeouts(t *testing.T) {
	t.Parallel()

	if !slices.Contains(check.DefaultSkipBlocks, "timeouts") {
		t.Error("DefaultSkipBlocks must contain 'timeouts'")
	}
}

// --- SchemaDocsRule magic-value config tests -----------------------

func TestSchemaDocsRule_DefaultPrefixesFire(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) The name of the thing.
`), "test")

	rule := &check.SchemaDocsRule{}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'The ' prefix, got %d: %v", len(results), resultMessages(results))
	}
}

func TestSchemaDocsRule_CustomBadPrefixes(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) The name of the thing.
* `+"`mode`"+` - (Optional) FORBIDDEN start.
`), "test")

	// Replace the default list with a custom one that only flags "FORBIDDEN".
	rule := &check.SchemaDocsRule{BadPrefixes: []string{"FORBIDDEN "}}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 1 {
		t.Fatalf("expected 1 result for custom prefix, got %d: %v", len(results), resultMessages(results))
	}
	if results[0].Message == "" {
		t.Error("result should have message set")
	}
}

func TestSchemaDocsRule_EmptyBadPrefixesMatchesNothing(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) The name of the thing.
`), "test")

	rule := &check.SchemaDocsRule{BadPrefixes: []string{}}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 0 {
		t.Errorf("empty BadPrefixes should match nothing, got %d results", len(results))
	}
}

func TestSchemaDocsRule_DefaultPrefixesMatchesTfproviderdocs(t *testing.T) {
	t.Parallel()

	// Pin the default list so a future refactor can't silently drop a prefix
	// that AWS CI depends on.
	want := []string{"A ", "An ", "The ", "Indicates ", "Specifies ", "Describes ", "Defines "}
	if !slices.Equal(check.DefaultBadDescriptionPrefixes, want) {
		t.Errorf("DefaultBadDescriptionPrefixes = %v, want %v", check.DefaultBadDescriptionPrefixes, want)
	}
}

// --- SchemaDocsRule magic-value config tests ----------------------------

func TestSchemaDocsRule_DefaultsAllEnabled(t *testing.T) {
	t.Parallel()

	src := "# Resource: test\n\n## Argument Reference\n\n```\ncode block here\n```\n\n* `name` - (Required) Name.\n"

	d, err := doc.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}

	rule := &check.SchemaDocsRule{} // all nil → all enabled
	results := rule.Check(check.CheckContext{Resource: "test", Doc: d})

	if len(results) == 0 {
		t.Error("zero-value SchemaDocsRule should flag the code block (default enabled)")
	}
}

func TestSchemaDocsRule_DisableNoCodeBlocks(t *testing.T) {
	t.Parallel()

	f := false
	rule := &check.SchemaDocsRule{NoCodeBlocks: &f}

	src := "# Resource: test\n\n## Argument Reference\n\n```\ncode block here\n```\n\n* `name` - (Required) Name.\n"

	d, err := doc.Parse([]byte(src), "test")
	if err != nil {
		t.Fatal(err)
	}

	results := rule.Check(check.CheckContext{Resource: "test", Doc: d})
	for _, r := range results {
		if r.Rule == "schema_docs" && strings.Contains(r.Message, "code block") {
			t.Errorf("NoCodeBlocks=false should suppress code-block errors; got: %s", r.Message)
		}
	}
}
