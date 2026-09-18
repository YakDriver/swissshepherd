// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// TestParse_HeadingSlugRenderedText guards that heading anchor slugs are built
// from the heading's rendered visible text, not goldmark's raw source segments.
// Raw Node.Text would fold HTML entity spellings and inline raw-HTML tag bytes
// into the slug, so a valid link to the rendered anchor would be reported dead.
func TestParse_HeadingSlugRenderedText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		heading string
		want    string
		absent  string
	}{
		{
			// "&amp;" renders as "&", which slugs to a dropped char: "A & B"
			// -> "a--b", not the raw-entity "a-amp-b".
			name:    "named entity",
			heading: "A &amp; B",
			want:    "a--b",
			absent:  "a-amp-b",
		},
		{
			// Numeric reference for "_" renders as an underscore.
			name:    "numeric reference",
			heading: "x&#95;y",
			want:    "x_y",
			absent:  "xy",
		},
		{
			// Inline HTML tags are not part of the rendered text: "A B".
			name:    "inline raw html",
			heading: "A <em>B</em>",
			want:    "a-b",
			absent:  "a-emb-em",
		},
		{
			// Code spans are visible text and keep their content verbatim.
			name:    "code span preserved",
			heading: "`ec2_configuration` Block",
			want:    "ec2_configuration-block",
			absent:  "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			md := "# Resource: aws_thing\n\n## " + c.heading + "\n\nbody.\n"
			d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
			if err != nil {
				t.Fatal(err)
			}
			if !d.HeadingAnchors[c.want] {
				t.Errorf("expected slug %q; got %v", c.want, d.HeadingAnchors)
			}
			if c.absent != "" && d.HeadingAnchors[c.absent] {
				t.Errorf("raw-source slug %q must not be produced; got %v", c.absent, d.HeadingAnchors)
			}
		})
	}
}
