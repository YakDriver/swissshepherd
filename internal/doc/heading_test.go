// Copyright (c) YakDriver, 2026
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
		// Non-matches
		{
			name:      "example heading rejected",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Basic Usage",
			want:      "",
		},
		{
			name:      "example heading rejected 2",
			templates: doc.DefaultHeadingTemplates(),
			heading:   "Network example",
			want:      "",
		},
		// Strict mode — only backtick_block accepted
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
