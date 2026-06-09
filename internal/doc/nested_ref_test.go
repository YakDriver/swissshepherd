// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// TestParseNestedRef_DotNotationForms exercises the dot-notation parser
// across the forms the AWS provider and tfplugindocs-style schemas
// produce: single-level, single-level with array indexers, and
// multi-level paths.
func TestParseNestedRef_DotNotationForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		wantParent    string
		wantAttribute string
		wantReadOnly  bool
		wantRequired  bool
		wantOptional  bool
	}{
		{
			name:          "single level dot",
			input:         "* `network.private_ip` - Private IP.",
			wantParent:    "network",
			wantAttribute: "private_ip",
		},
		{
			name:          "single level with star indexer",
			input:         "* `network[*].private_ip` - Private IP.",
			wantParent:    "network",
			wantAttribute: "private_ip",
		},
		{
			name:          "single level with numeric indexer",
			input:         "* `tags[0].key` - Tag key.",
			wantParent:    "tags",
			wantAttribute: "key",
		},
		{
			name:          "two level path",
			input:         "* `parent.child.attr` - Some attr.",
			wantParent:    "parent.child",
			wantAttribute: "attr",
		},
		{
			name:          "three level path",
			input:         "* `analyzer_configuration.unused_access_configuration.unused_access_age` - Days for unused access.",
			wantParent:    "analyzer_configuration.unused_access_configuration",
			wantAttribute: "unused_access_age",
		},
		{
			name:          "multi level with mixed indexers",
			input:         "* `parent[*].child[0].attr` - Some attr.",
			wantParent:    "parent.child",
			wantAttribute: "attr",
		},
		{
			name:          "read-only label",
			input:         "* `network[*].private_ip` - (Read-Only) Private IP.",
			wantParent:    "network",
			wantAttribute: "private_ip",
			wantReadOnly:  true,
		},
		{
			name:          "required label",
			input:         "* `parent.child.attr` - (Required) Some attr.",
			wantParent:    "parent.child",
			wantAttribute: "attr",
			wantRequired:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := "## Attribute Reference\n\n" + tt.input + "\n"
			d, err := doc.Parse([]byte(source), "test")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			parentBlock := d.AttributeBlocks[tt.wantParent]
			if parentBlock == nil {
				t.Fatalf("expected attribute block %q to exist; have keys: %v",
					tt.wantParent, attrBlockKeys(d))
			}

			var found *doc.DocAttribute
			for i := range parentBlock.Attributes {
				if parentBlock.Attributes[i].Name == tt.wantAttribute {
					found = &parentBlock.Attributes[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("expected attribute %q in block %q, got: %v",
					tt.wantAttribute, tt.wantParent, attrNames(parentBlock.Attributes))
			}
			if found.ReadOnly != tt.wantReadOnly {
				t.Errorf("read-only = %v, want %v", found.ReadOnly, tt.wantReadOnly)
			}
			if found.Required != tt.wantRequired {
				t.Errorf("required = %v, want %v", found.Required, tt.wantRequired)
			}
			if found.Optional != tt.wantOptional {
				t.Errorf("optional = %v, want %v", found.Optional, tt.wantOptional)
			}
		})
	}
}

// TestParseNestedRef_NotADotNotation confirms the parser ignores items
// that look superficially similar but aren't valid dot-notation
// references (no dot, leading/trailing dot, or whitespace in path).
func TestParseNestedRef_NotADotNotation(t *testing.T) {
	t.Parallel()

	cases := []string{
		"* `plain_attr` - A plain attribute.",
		"* `.leading_dot` - Bad.",
		"* `trailing_dot.` - Bad.",
		"* `multiple..dots` - Bad.",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			source := "## Attribute Reference\n\n" + in + "\n"
			d, err := doc.Parse([]byte(source), "test")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			// Confirm no synthetic parent block was created.
			for k := range d.AttributeBlocks {
				if k != "" {
					t.Errorf("unexpected attribute block %q created from %q", k, in)
				}
			}
		})
	}
}

func attrBlockKeys(d *doc.Document) []string {
	keys := make([]string, 0, len(d.AttributeBlocks))
	for k := range d.AttributeBlocks {
		keys = append(keys, k)
	}
	return keys
}
