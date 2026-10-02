// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"slices"
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

// TestParse_CanonicalSectionsExactMatch confirms canonical level-2 section
// classification uses exact heading text. Variants like "Importing" or
// "Examples" must NOT be absorbed into the Import / Example fields; they
// belong in UnknownHeadings so section_presence can report them as
// unknown sections (or accept them as custom ones if the Type spec
// declares them).
func TestParse_CanonicalSectionsExactMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		heading     string
		wantUnknown bool
	}{
		// Exact canonical match: classified as the named section.
		{"exact import", "Import", false},
		{"exact signature", "Signature", false},
		{"exact timeouts", "Timeouts", false},
		{"exact example usage", "Example Usage", false},

		// Non-canonical variants: must be unknown headings.
		{"variant importing", "Importing", true},
		{"variant import notes", "Import Notes", true},
		{"variant examples", "Examples", true},
		{"variant timeout", "Timeout", true},
		{"variant signatures", "Signatures", true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := "# Resource: aws_test\n\n## " + tt.heading + "\n\nbody.\n"
			d, err := doc.Parse([]byte(source), "test")
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			var unknownTexts []string
			for _, h := range d.Sections.UnknownHeadings {
				unknownTexts = append(unknownTexts, h.Text)
			}

			isUnknown := slices.Contains(unknownTexts, tt.heading)

			if tt.wantUnknown != isUnknown {
				t.Errorf("heading %q: wantUnknown=%v, isUnknown=%v (unknowns: %v)",
					tt.heading, tt.wantUnknown, isUnknown, unknownTexts)
			}
		})
	}
}

// TestParse_UnknownHeadingClosesPreviousSection confirms that when an
// unknown level-2 heading appears between two canonical sections, the
// previous section's EndOffset is finalized at the unknown heading
// rather than bleeding past it. Without this, slicing Sections.Example.
// Source(...) would include all subsequent body content up to EOF.
func TestParse_UnknownHeadingClosesPreviousSection(t *testing.T) {
	t.Parallel()

	source := "# Resource: aws_test\n\n" +
		"## Example Usage\n\n" +
		"example body line one.\n\n" +
		"## Notes\n\n" +
		"unknown body line.\n\n" +
		"## Argument Reference\n\n" +
		"args body line.\n"

	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if d.Sections.Example == nil {
		t.Fatal("Example section should be parsed")
	}
	if d.Sections.Example.EndOffset == 0 {
		t.Fatal("Example.EndOffset should be set, not zero")
	}
	end := d.Sections.Example.EndOffset
	body := source[d.Sections.Example.StartOffset:end]
	if strings.Contains(body, "## Notes") || strings.Contains(body, "unknown body line") {
		t.Errorf("Example section bled past the unknown heading. Body:\n%s", body)
	}
}

func TestSection_ChildHeadings(t *testing.T) {
	t.Parallel()

	source := `# Resource: test

## Example Usage

### Basic Usage

Some text.

### Advanced Usage

More text.

## Argument Reference

### ` + "`config`" + ` Block

* ` + "`name`" + ` - (Required) Name.
`

	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Example section should have two child headings.
	ex := d.Sections.Example
	if ex == nil {
		t.Fatal("expected Example section")
	}
	if got := len(ex.ChildHeadings); got != 2 {
		t.Fatalf("Example.ChildHeadings: got %d, want 2", got)
	}
	if ex.ChildHeadings[0].Text != "Basic Usage" {
		t.Errorf("ChildHeadings[0].Text = %q, want %q", ex.ChildHeadings[0].Text, "Basic Usage")
	}
	if ex.ChildHeadings[0].Level != 3 {
		t.Errorf("ChildHeadings[0].Level = %d, want 3", ex.ChildHeadings[0].Level)
	}
	if ex.ChildHeadings[1].Text != "Advanced Usage" {
		t.Errorf("ChildHeadings[1].Text = %q, want %q", ex.ChildHeadings[1].Text, "Advanced Usage")
	}

	// Arguments section should have the config block as a child heading.
	args := d.Sections.Arguments
	if args == nil {
		t.Fatal("expected Arguments section")
	}
	if got := len(args.ChildHeadings); got != 1 {
		t.Fatalf("Arguments.ChildHeadings: got %d, want 1", got)
	}
	if args.ChildHeadings[0].Text != "config Block" {
		t.Errorf("ChildHeadings[0].Text = %q, want %q", args.ChildHeadings[0].Text, "config Block")
	}
}

func TestSection_ListItems(t *testing.T) {
	t.Parallel()

	source := `# Resource: test

## Timeouts

[Configuration options](https://developer.hashicorp.com/terraform/language/resources/syntax#operation-timeouts):

* ` + "`create`" + ` - (Default ` + "`60m`" + `)
* ` + "`update`" + ` - (Default ` + "`180m`" + `)
* ` + "`delete`" + ` - (Default ` + "`90m`" + `)

## Import

Import using the ID.
`

	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	tm := d.Sections.Timeouts
	if tm == nil {
		t.Fatal("expected Timeouts section")
	}
	if got := len(tm.ListItems); got != 3 {
		t.Fatalf("Timeouts.ListItems: got %d, want 3", got)
	}

	want := []struct {
		name  string
		value string
	}{
		{"create", "(Default `60m`)"},
		{"update", "(Default `180m`)"},
		{"delete", "(Default `90m`)"},
	}
	for i, w := range want {
		if tm.ListItems[i].Name != w.name {
			t.Errorf("ListItems[%d].Name = %q, want %q", i, tm.ListItems[i].Name, w.name)
		}
		if tm.ListItems[i].Value != w.value {
			t.Errorf("ListItems[%d].Value = %q, want %q", i, tm.ListItems[i].Value, w.value)
		}
		if tm.ListItems[i].Line == 0 {
			t.Errorf("ListItems[%d].Line should be non-zero", i)
		}
	}

	// Import section should have no list items (just prose).
	imp := d.Sections.Import
	if imp == nil {
		t.Fatal("expected Import section")
	}
	if got := len(imp.ListItems); got != 0 {
		t.Errorf("Import.ListItems: got %d, want 0", got)
	}
}

func TestSection_SourceRange(t *testing.T) {
	t.Parallel()

	source := `# Resource: test

## Example Usage

Example content here.

## Argument Reference

* ` + "`name`" + ` - (Required) Name.

## Import

Import using the ID.
`

	d, err := doc.Parse([]byte(source), "test")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Title section starts at 0.
	title := d.Sections.Title
	if title == nil {
		t.Fatal("expected Title section")
	}
	if title.StartOffset != 0 {
		t.Errorf("Title.StartOffset = %d, want 0", title.StartOffset)
	}

	// Example section starts after the title.
	ex := d.Sections.Example
	if ex == nil {
		t.Fatal("expected Example section")
	}
	if ex.StartOffset == 0 {
		t.Error("Example.StartOffset should be > 0")
	}
	if ex.EndOffset <= ex.StartOffset {
		t.Errorf("Example.EndOffset (%d) should be > StartOffset (%d)", ex.EndOffset, ex.StartOffset)
	}

	// The Example section's source should contain "Example content here."
	slice := string([]byte(source)[ex.StartOffset:ex.EndOffset])
	if !contains(slice, "Example content here.") {
		t.Errorf("Example source range does not contain expected text:\n%s", slice)
	}
	// But should NOT contain "Argument Reference"
	if contains(slice, "Argument Reference") {
		t.Error("Example source range should not contain Argument Reference")
	}

	// Import section ends at EOF.
	imp := d.Sections.Import
	if imp == nil {
		t.Fatal("expected Import section")
	}
	if imp.EndOffset != len(source) {
		t.Errorf("Import.EndOffset = %d, want %d (EOF)", imp.EndOffset, len(source))
	}
	impSlice := string([]byte(source)[imp.StartOffset:imp.EndOffset])
	if !contains(impSlice, "Import using the ID.") {
		t.Errorf("Import source range does not contain expected text:\n%s", impSlice)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
