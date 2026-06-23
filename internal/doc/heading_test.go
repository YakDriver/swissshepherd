// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

func TestHeadingTemplates_Match(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		templates doc.HeadingTemplates
		heading   string
		want      string
	}{
		// {Block} Block — goldmark strips backticks so `network` Block → "network Block"
		{
			name:      "backtick_block style",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "network Block",
			want:      "network",
		},
		{
			name:      "backtick_block with underscores",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "credit_specification Block",
			want:      "credit_specification",
		},
		// {Block} alone — bare snake_case name
		{
			name:      "bare block name",
			templates: doc.HeadingTemplates{"{Block}"},
			heading:   "statement",
			want:      "statement",
		},
		{
			name:      "bare block name with underscores",
			templates: doc.HeadingTemplates{"{Block}"},
			heading:   "redis_settings",
			want:      "redis_settings",
		},
		// {Title} — title case converted to snake_case
		{
			name:      "title case",
			templates: doc.HeadingTemplates{"{Title}"},
			heading:   "Credit Specification",
			want:      "credit_specification",
		},
		{
			name:      "title case multi-word",
			templates: doc.HeadingTemplates{"{Title}"},
			heading:   "CPU Options",
			want:      "cpu_options",
		},
		// {Block} Block without backticks
		{
			name:      "block suffix no backticks",
			templates: doc.HeadingTemplates{"{Block} Block"},
			heading:   "network Block",
			want:      "network",
		},
		// Multiple templates — first match wins
		{
			name:      "multiple templates first match",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "network Block",
			want:      "network",
		},
		{
			name:      "multiple templates title case fallback",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Network Interfaces",
			want:      "network_interfaces",
		},
		{
			name:      "multiple templates bare name",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "condition",
			want:      "condition",
		},
		// Title-case headings match {Title} template (the parser only invokes
		// MatchAll inside Argument/Attribute Reference, where Example Usage
		// subheadings cannot appear).
		{
			name:      "title template snake_cases multi-word headings",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Basic Usage",
			want:      "basic_usage",
		},
		{
			name:      "title template snake_cases mixed case",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Network example",
			want:      "network_example",
		},
		// Non-matches
		{
			name:      "strict rejects bare name",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "network",
			want:      "",
		},
		{
			name:      "strict rejects title case",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "Network Interfaces",
			want:      "",
		},
		{
			name:      "strict accepts correct format",
			templates: doc.HeadingTemplates{"`{Block}` Block"},
			heading:   "network_interface Block",
			want:      "network_interface",
		},
		// Title case rejected when not in templates
		{
			name:      "no title template rejects title case",
			templates: doc.HeadingTemplates{"`{Block}` Block", "{Block}"},
			heading:   "CPU Options",
			want:      "",
		},
		// Upper case rejected as {Block} (must be lowercase)
		{
			name:      "block rejects uppercase",
			templates: doc.HeadingTemplates{"{Block}"},
			heading:   "Network",
			want:      "",
		},
		// {Parent} template — single parent
		{
			name:      "parent single word",
			templates: doc.HeadingTemplates{"`{Parent}` `{Block}` Block"},
			heading:   "custom_key header Block",
			want:      "custom_key.header",
		},
		// {Parent} template — multi-word parent (uses last two parent words for disambiguation)
		{
			name:      "parent multi-word",
			templates: doc.HeadingTemplates{"`{Parent}` `{Block}` Block"},
			heading:   "customized_metric_specification metrics metric_stat Block",
			want:      "customized_metric_specification.metrics.metric_stat",
		},
		{
			name:      "parent three words",
			templates: doc.HeadingTemplates{"`{Parent}` `{Block}` Block"},
			heading:   "metric_data_query metric_stat metric Block",
			want:      "metric_data_query.metric_stat.metric",
		},
		// {Parent} template — no match without suffix
		{
			name:      "parent rejects single word",
			templates: doc.HeadingTemplates{"`{Parent}` `{Block}` Block"},
			heading:   "header Block",
			want:      "",
		},
		// {Path} template — single segment (acts like {Block})
		{
			name:      "path single segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "network Block",
			want:      "network",
		},
		// {Path} template — two segments dot-notation
		{
			name:      "path two segments",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "partition_spec.fields Block",
			want:      "partition_spec.fields",
		},
		// {Path} template — three segments dot-notation
		{
			name:      "path three segments",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "analyzer_configuration.internal_access_configuration.internal_access_analysis_rule Block",
			want:      "analyzer_configuration.internal_access_configuration.internal_access_analysis_rule",
		},
		// tfplugindocs canonical heading
		{
			name:      "tfplugindocs nested schema",
			templates: doc.HeadingTemplates{"Nested Schema for `{Path}`"},
			heading:   "Nested Schema for partition_spec.fields",
			want:      "partition_spec.fields",
		},
		{
			name:      "tfplugindocs nested schema single segment",
			templates: doc.HeadingTemplates{"Nested Schema for `{Path}`"},
			heading:   "Nested Schema for analyzer_configuration",
			want:      "analyzer_configuration",
		},
		{
			name:      "tfplugindocs nested schema deep",
			templates: doc.HeadingTemplates{"Nested Schema for `{Path}`"},
			heading:   "Nested Schema for analyzer_configuration.unused_access_configuration.analysis_rule.exclusions.resource_tags",
			want:      "analyzer_configuration.unused_access_configuration.analysis_rule.exclusions.resource_tags",
		},
		// {Path} rejects malformed dot-notation
		{
			name:      "path rejects leading dot",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   ".fields Block",
			want:      "",
		},
		{
			name:      "path rejects trailing dot",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "fields. Block",
			want:      "",
		},
		{
			name:      "path rejects double dot",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "a..b Block",
			want:      "",
		},
		{
			name:      "path rejects uppercase segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "Foo.bar Block",
			want:      "",
		},
		{
			name:      "path rejects space in segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "partition spec.fields Block",
			want:      "",
		},
		{
			name:      "path rejects hyphen in segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "foo-bar.fields Block",
			want:      "",
		},
		{
			name:      "path rejects slash in segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "a/b.fields Block",
			want:      "",
		},
		{
			name:      "path rejects punctuation in segment",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "foo+bar.fields Block",
			want:      "",
		},
		{
			name:      "path accepts digits",
			templates: doc.HeadingTemplates{"`{Path}` Block"},
			heading:   "config_v2.field_1 Block",
			want:      "config_v2.field_1",
		},
		// Default templates accept the path forms
		{
			name:      "defaults accept dot-notation block",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "partition_spec.fields Block",
			want:      "partition_spec.fields",
		},
		{
			name:      "defaults accept tfplugindocs",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Nested Schema for partition_spec.fields",
			want:      "partition_spec.fields",
		},
		{
			name:      "defaults accept bare dot-notation",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "partition_spec.fields",
			want:      "partition_spec.fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.templates.Match(tt.heading)
			if got != tt.want {
				t.Errorf("Match(%q) = %q, want %q", tt.heading, got, tt.want)
			}
		})
	}
}

func TestParseWithTemplates_Strict(t *testing.T) {
	t.Parallel()

	// The test fixture uses "### `network` Block" which goldmark renders as "network Block"
	// With strict backtick_block template, this should match
	strict := doc.HeadingTemplates{"`{Block}` Block"}

	d, err := doc.ParseFileWithTemplates("../../testdata/docs/r/instance.html.markdown", strict)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if _, ok := d.ArgumentBlocks["network"]; !ok {
		t.Error("strict template should match 'network Block' heading")
	}
}

func TestParseWithTemplates_StrictRejectsBareName(t *testing.T) {
	t.Parallel()

	// Create a doc with bare name heading style (### statement)
	source := []byte(`# Resource: test_thing

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

### statement

* ` + "`sid`" + ` - (Optional) Statement ID.
`)

	// Strict backtick_block should NOT match "statement" (bare name)
	strict := doc.HeadingTemplates{"`{Block}` Block"}
	d, err := doc.ParseWithTemplates(source, "test", strict)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if _, ok := d.ArgumentBlocks["statement"]; ok {
		t.Error("strict backtick_block template should NOT match bare 'statement' heading")
	}

	// But default templates should match it
	d2, err := doc.ParseWithTemplates(source, "test", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if _, ok := d2.ArgumentBlocks["statement"]; !ok {
		t.Error("default templates should match bare 'statement' heading")
	}
}
