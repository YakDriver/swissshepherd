// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check_test

import (
	"slices"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// --- ArgumentsSectionRule magic-value config tests ---------------------------

func TestArgumentsSectionRule_DefaultImplicitAttributesSkipped(t *testing.T) {
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

	rule := &check.ArgumentsSectionRule{IgnoreDeprecated: true}
	results := rule.Check(check.CheckContext{Resource: "test_thing", Schema: rs, Doc: d})

	for _, r := range results {
		if r.Severity == check.SeverityError {
			t.Errorf("unexpected error (id/tags_all should be implicit): %s", r.Message)
		}
	}
}

func TestArgumentsSectionRule_CustomImplicitAttributes(t *testing.T) {
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

	rule := &check.ArgumentsSectionRule{
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

func TestArgumentsSectionRule_CustomSkipBlocks(t *testing.T) {
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

	rule := &check.ArgumentsSectionRule{
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

func TestArgumentsSectionRule_DefaultSkipBlocksContainsTimeouts(t *testing.T) {
	t.Parallel()

	if !slices.Contains(check.DefaultSkipBlocks, "timeouts") {
		t.Error("DefaultSkipBlocks must contain 'timeouts'")
	}
}

// --- DescriptionStyleRule magic-value config tests -----------------------

func TestDescriptionStyleRule_DefaultPrefixesFire(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) The name of the thing.
`), "test")

	rule := &check.DescriptionStyleRule{}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'The ' prefix, got %d: %v", len(results), resultMessages(results))
	}
}

func TestDescriptionStyleRule_CustomBadPrefixes(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) The name of the thing.
* `+"`mode`"+` - (Optional) FORBIDDEN start.
`), "test")

	// Replace the default list with a custom one that only flags "FORBIDDEN".
	rule := &check.DescriptionStyleRule{BadPrefixes: []string{"FORBIDDEN "}}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 1 {
		t.Fatalf("expected 1 result for custom prefix, got %d: %v", len(results), resultMessages(results))
	}
	if results[0].Message == "" {
		t.Error("result should have message set")
	}
}

func TestDescriptionStyleRule_EmptyBadPrefixesMatchesNothing(t *testing.T) {
	t.Parallel()

	d, _ := doc.Parse([]byte(`## Argument Reference

* `+"`name`"+` - (Required) The name of the thing.
`), "test")

	rule := &check.DescriptionStyleRule{BadPrefixes: []string{}}
	results := rule.Check(check.CheckContext{Resource: "test", Schema: nil, Doc: d})

	if len(results) != 0 {
		t.Errorf("empty BadPrefixes should match nothing, got %d results", len(results))
	}
}

func TestDescriptionStyleRule_DefaultPrefixesMatchesTfproviderdocs(t *testing.T) {
	t.Parallel()

	// Pin the default list so a future refactor can't silently drop a prefix
	// that AWS CI depends on.
	want := []string{"A ", "An ", "The ", "Indicates ", "Specifies ", "Describes ", "Defines "}
	if !slices.Equal(check.DefaultBadDescriptionPrefixes, want) {
		t.Errorf("DefaultBadDescriptionPrefixes = %v, want %v", check.DefaultBadDescriptionPrefixes, want)
	}
}

// --- FormatStyleRule magic-value config tests ----------------------------

func TestFormatStyleRule_DefaultsAllEnabled(t *testing.T) {
	t.Parallel()

	// A zero-value FormatStyleRule should behave identically to the old
	// hard-coded {NoCodeBlocks: true, SingleLineAttrs: true, UninterruptedLists: true}.
	content := []byte(`## Argument Reference

` + "```" + `
code block here
` + "```" + `

* ` + "`name`" + ` - (Required) Name.
`)

	rule := &check.FormatStyleRule{} // all nil → all enabled
	results := rule.CheckFile(check.FileCheckContext{Resource: "test", Path: "test.md", Content: content})

	if len(results) == 0 {
		t.Error("zero-value FormatStyleRule should flag the code block (default enabled)")
	}
}

func TestFormatStyleRule_DisableNoCodeBlocks(t *testing.T) {
	t.Parallel()

	f := false
	rule := &check.FormatStyleRule{NoCodeBlocks: &f}

	content := []byte(`## Argument Reference

` + "```" + `
code block here
` + "```" + `

* ` + "`name`" + ` - (Required) Name.
`)

	results := rule.CheckFile(check.FileCheckContext{Resource: "test", Path: "test.md", Content: content})
	for _, r := range results {
		if r.Rule == "format_style" && r.Severity == check.SeverityError {
			t.Errorf("NoCodeBlocks=false should suppress code-block errors; got: %s", r.Message)
		}
	}
}
