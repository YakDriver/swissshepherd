// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

func TestParseListItem_Formats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		wantName      string
		wantRequired  bool
		wantOptional  bool
		wantReadOnly  bool
		wantMalformed bool // should appear in MalformedAttributes instead
	}{
		// Valid formats
		{
			name:         "standard required",
			input:        "* `name` - (Required) The name.",
			wantName:     "name",
			wantRequired: true,
		},
		{
			name:         "standard optional",
			input:        "* `description` - (Optional) A description.",
			wantName:     "description",
			wantOptional: true,
		},
		{
			name:         "optional with forces new",
			input:        "* `name` - (Optional, Forces new resource) The name.",
			wantName:     "name",
			wantOptional: true,
		},
		{
			name:         "required with forces new",
			input:        "* `cidr_block` - (Required, Forces new resource) The CIDR block.",
			wantName:     "cidr_block",
			wantRequired: true,
		},
		{
			name:         "standard read-only",
			input:        "* `arn` - (Read-Only) ARN of the resource.",
			wantName:     "arn",
			wantReadOnly: true,
		},
		{
			name:         "read-only with deprecated",
			input:        "* `legacy_id` - (Read-Only, Deprecated) Old identifier.",
			wantName:     "legacy_id",
			wantReadOnly: true,
		},
		{
			name:     "attribute without required/optional (attribute reference style)",
			input:    "* `arn` - ARN of the resource.",
			wantName: "arn",
		},
		// Malformed - missing dash
		{
			name:          "missing dash before required",
			input:         "* `name` (Required) The name.",
			wantMalformed: true,
		},
		{
			name:          "missing dash before optional",
			input:         "* `description` (Optional) A description.",
			wantMalformed: true,
		},
		// Malformed - em-dash instead of regular dash
		{
			name:          "em-dash instead of dash",
			input:         "* `name` \u2013 (Required) The name.",
			wantMalformed: true,
		},
		{
			name:          "em-dash long instead of dash",
			input:         "* `name` \u2014 (Optional) The name.",
			wantMalformed: true,
		},
		{
			name:          "missing dash before read-only",
			input:         "* `arn` (Read-Only) ARN.",
			wantMalformed: true,
		},
		// Malformed - (Optional) in attribute reference (should not have it)
		// This is actually valid for arguments, but we want to detect it in attribute sections
		// For now, this parses fine as an argument
		{
			name:         "optional in what might be attribute ref",
			input:        "* `id` - (Optional) The ID.",
			wantName:     "id",
			wantOptional: true,
		},
		// Invalid - not an attribute at all
		{
			name:  "plain text list item",
			input: "* Some random text.",
		},
		{
			name:  "category header",
			input: "* Creating an Amazon issued certificate",
		},
		// Edge cases
		{
			name:     "dash with no parenthetical",
			input:    "* `status` - Current status of the thing.",
			wantName: "status",
		},
		{
			name:          "no space before dash is malformed",
			input:         "* `name`- (Optional) The name.",
			wantName:      "name",
			wantOptional:  true,
			wantMalformed: true,
		},
		{
			name:          "no space before dash required is malformed",
			input:         "* `mode`- (Required) The mode.",
			wantName:      "mode",
			wantRequired:  true,
			wantMalformed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := "## Argument Reference\n\n" + tt.input + "\n"
			templates := doc.DefaultHeadingTemplates()
			d, err := doc.ParseWithTemplates([]byte(source), "test", templates)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			root := d.ArgumentBlocks[""]
			if root == nil {
				t.Fatal("no root block found")
			}

			if tt.wantMalformed {
				if len(root.MalformedAttributes) == 0 {
					t.Errorf("expected malformed attribute, got none (attrs: %v)", attrNames(root.Attributes))
				}
				if tt.wantName == "" {
					return
				}
				// Fall through to also check the attribute was parsed.
			}

			if tt.wantName == "" {
				if len(root.Attributes) != 0 {
					t.Errorf("expected no attributes, got %v", attrNames(root.Attributes))
				}
				return
			}

			if len(root.Attributes) == 0 {
				t.Fatalf("expected attribute %q, got none (malformed: %v)", tt.wantName, root.MalformedAttributes)
			}

			attr := root.Attributes[0]
			if attr.Name != tt.wantName {
				t.Errorf("name = %q, want %q", attr.Name, tt.wantName)
			}
			if attr.Required != tt.wantRequired {
				t.Errorf("required = %v, want %v", attr.Required, tt.wantRequired)
			}
			if attr.Optional != tt.wantOptional {
				t.Errorf("optional = %v, want %v", attr.Optional, tt.wantOptional)
			}
			if attr.ReadOnly != tt.wantReadOnly {
				t.Errorf("read-only = %v, want %v", attr.ReadOnly, tt.wantReadOnly)
			}
		})
	}
}

func attrNames(attrs []doc.DocAttribute) []string {
	var names []string
	for _, a := range attrs {
		names = append(names, a.Name)
	}
	return names
}
