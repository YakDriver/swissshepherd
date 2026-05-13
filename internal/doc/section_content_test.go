// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

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
