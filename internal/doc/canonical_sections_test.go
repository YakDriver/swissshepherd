// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

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
