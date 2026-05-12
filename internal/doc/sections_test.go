// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// fullDoc is a complete, valid provider documentation file exercising every
// top-level section the Sections parser tracks. Individual tests mutate this
// to exercise failure paths.
const fullDoc = `---
subcategory: "Test"
---

# Resource: test_instance

Manages a Test Instance.

## Example Usage

### Basic Usage

` + "```terraform" + `
resource "test_instance" "example" {
  name = "example"
}
` + "```" + `

## Argument Reference

The following arguments are required:

* ` + "`name`" + ` - (Required) Name.

## Attribute Reference

This resource exports the following attributes in addition to the arguments above:

* ` + "`arn`" + ` - ARN.

## Timeouts

* ` + "`create`" + ` - (Default ` + "`30m`" + `)

## Import

In Terraform v1.5.0 and later, use an ` + "`import`" + ` block:

` + "```terraform" + `
import {
  to = test_instance.example
  id = "i-123"
}
` + "```" + `
`

func TestSections_FullDocument_AllSectionsDiscovered(t *testing.T) {
	t.Parallel()

	d, err := doc.Parse([]byte(fullDoc), "test_instance")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if d.Sections == nil {
		t.Fatal("Document.Sections is nil; parser should always initialize it")
	}

	tests := []struct {
		name    string
		section *doc.Section
		want    string
	}{
		{"Title", d.Sections.Title, "Resource: test_instance"},
		{"Example", d.Sections.Example, "Example Usage"},
		{"Arguments", d.Sections.Arguments, "Argument Reference"},
		{"Attributes", d.Sections.Attributes, "Attribute Reference"},
		{"Timeouts", d.Sections.Timeouts, "Timeouts"},
		{"Import", d.Sections.Import, "Import"},
	}
	for _, tt := range tests {
		if tt.section == nil {
			t.Errorf("Sections.%s is nil, want section with text %q", tt.name, tt.want)
			continue
		}
		if tt.section.Text != tt.want {
			t.Errorf("Sections.%s.Text = %q, want %q", tt.name, tt.section.Text, tt.want)
		}
		if tt.section.Heading == nil {
			t.Errorf("Sections.%s.Heading is nil", tt.name)
		}
	}

	// Functions-only section is absent from a resource page.
	if d.Sections.Signature != nil {
		t.Errorf("Sections.Signature should be nil for resource docs, got %+v", d.Sections.Signature)
	}
}

func TestSections_HeadingLevels(t *testing.T) {
	t.Parallel()

	d, err := doc.Parse([]byte(fullDoc), "test_instance")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if got := d.Sections.Title.Heading.Level; got != 1 {
		t.Errorf("Title heading level = %d, want 1", got)
	}
	for _, s := range []struct {
		name string
		sec  *doc.Section
	}{
		{"Example", d.Sections.Example},
		{"Arguments", d.Sections.Arguments},
		{"Attributes", d.Sections.Attributes},
		{"Timeouts", d.Sections.Timeouts},
		{"Import", d.Sections.Import},
	} {
		if got := s.sec.Heading.Level; got != 2 {
			t.Errorf("%s heading level = %d, want 2", s.name, got)
		}
	}
}

func TestSections_FencedCodeBlocksCollected(t *testing.T) {
	t.Parallel()

	d, err := doc.Parse([]byte(fullDoc), "test_instance")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Title section should never hold a code block in a well-formed doc.
	if got := len(d.Sections.Title.FencedCodeBlocks); got != 0 {
		t.Errorf("Title.FencedCodeBlocks = %d, want 0", got)
	}

	// Example Usage and Import each have one fenced code block.
	if got := len(d.Sections.Example.FencedCodeBlocks); got != 1 {
		t.Errorf("Example.FencedCodeBlocks = %d, want 1", got)
	}
	if got := len(d.Sections.Import.FencedCodeBlocks); got != 1 {
		t.Errorf("Import.FencedCodeBlocks = %d, want 1", got)
	}
}

func TestSections_TitleCaptursCodeBlocks_WhenMisplaced(t *testing.T) {
	t.Parallel()

	// A code block between the H1 and the first H2 is exactly the misuse the
	// title rule will flag. Confirm the parser attributes it to Title.
	source := `# Resource: test_instance

Manages a Test Instance.

` + "```terraform" + `
resource "test_instance" "example" {}
` + "```" + `

## Example Usage

More content here.
`

	d, err := doc.Parse([]byte(source), "test_instance")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if d.Sections.Title == nil {
		t.Fatal("Title section missing")
	}
	if got := len(d.Sections.Title.FencedCodeBlocks); got != 1 {
		t.Fatalf("Title.FencedCodeBlocks = %d, want 1 (code block before first H2)", got)
	}
	if got := len(d.Sections.Example.FencedCodeBlocks); got != 0 {
		t.Errorf("Example.FencedCodeBlocks = %d, want 0 (code block should belong to Title)", got)
	}
}

func TestSections_NoTitle(t *testing.T) {
	t.Parallel()

	source := `## Example Usage

Something.

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
`
	d, err := doc.Parse([]byte(source), "test_instance")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if d.Sections.Title != nil {
		t.Errorf("Title should be nil when # heading absent; got text %q", d.Sections.Title.Text)
	}
	if d.Sections.Example == nil || d.Sections.Arguments == nil {
		t.Error("Example and Arguments should still parse normally")
	}
}

func TestSections_UnknownSectionIsIgnored(t *testing.T) {
	t.Parallel()

	source := `# Resource: test

## Notes

Some free-form section not tracked by Sections.

` + "```" + `
code here
` + "```" + `

## Argument Reference

* ` + "`name`" + ` - (Required) Name.
`
	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Notes is not one of the tracked sections, so the code block inside it
	// must not leak back into Title.
	if got := len(d.Sections.Title.FencedCodeBlocks); got != 0 {
		t.Errorf("Title.FencedCodeBlocks = %d, want 0; unknown section must reset accumulator", got)
	}
	// Arguments is still recognized after the unknown section.
	if d.Sections.Arguments == nil {
		t.Error("Arguments should still be discovered after an unknown section")
	}
}

func TestSections_DuplicateHeadingKeepsFirst(t *testing.T) {
	t.Parallel()

	// A misauthored doc has two ## Import sections. The parser keeps the
	// first; subsequent duplicates do not overwrite.
	source := `# Resource: test

## Import

First import prose.

` + "```terraform" + `
import { to = test.example, id = "a" }
` + "```" + `

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Import

Second import prose.
`
	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if d.Sections.Import == nil {
		t.Fatal("Import section missing")
	}
	// Both prose paragraphs should land under the single Import section because
	// the second heading re-selects it as currentSection.
	wantParagraphs := 2
	if got := len(d.Sections.Import.Paragraphs); got != wantParagraphs {
		t.Errorf("Import.Paragraphs = %d, want %d (both Import sections feed one record)", got, wantParagraphs)
	}
}

// TestSections_DoesNotBreakExistingBlocks is an integration guard: the section
// walker and the existing block walker run from the same ast.Walk, so this
// confirms adding Sections did not regress ArgumentBlocks / AttributeBlocks.
func TestSections_DoesNotBreakExistingBlocks(t *testing.T) {
	t.Parallel()

	d, err := doc.ParseFile("../../testdata/docs/r/instance.html.markdown")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if root := d.ArgumentBlocks[""]; root == nil || len(root.Attributes) == 0 {
		t.Error("root argument block should still have attributes after sections refactor")
	}
	if network := d.ArgumentBlocks["network"]; network == nil {
		t.Error("nested network argument block should still exist after sections refactor")
	}

	// And Sections should be populated on the fixture.
	if d.Sections == nil || d.Sections.Title == nil || !strings.Contains(d.Sections.Title.Text, "test_instance") {
		t.Errorf("fixture should have Title section containing resource name; got %+v", d.Sections)
	}
}
