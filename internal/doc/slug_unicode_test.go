// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// TestParse_HeadingAnchorsPreserveConnectors guards that all Unicode connector
// punctuation (\p{Pc}) survives the slug, matching GitHub's \p{Word} set — not
// just the ASCII underscore U+005F but also characters like U+203F (‿).
func TestParse_HeadingAnchorsPreserveConnectors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		heading string
		want    string
	}{
		{name: "ascii underscore", heading: "a_b", want: "a_b"},
		{name: "undertie U+203F", heading: "a\u203fb", want: "a\u203fb"},
		{name: "fullwidth low line U+FF3F", heading: "a\uff3fb", want: "a\uff3fb"},
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
				t.Errorf("expected connector-preserving slug %q; got %v", c.want, d.HeadingAnchors)
			}
		})
	}
}

// TestParse_HeadingAnchorsPreserveMarks guards that combining marks survive the
// slug, matching GitHub (which strips only characters outside Onigmo's \p{Word}
// class, and \p{Word} includes \p{M}). A decomposed "Café" ("Cafe"+U+0301)
// must slug to "café" (still decomposed), not the mark-stripped "cafe".
func TestParse_HeadingAnchorsPreserveMarks(t *testing.T) {
	t.Parallel()

	// "Cafe" + U+0301 COMBINING ACUTE ACCENT + " Configuration".
	md := "# Resource: aws_thing\n\n## Cafe\u0301 Configuration\n\nbody.\n"
	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	if !d.HeadingAnchors["cafe\u0301-configuration"] {
		t.Errorf("expected mark-preserving slug %q; got %v", "cafe\u0301-configuration", d.HeadingAnchors)
	}
	if d.HeadingAnchors["cafe-configuration"] {
		t.Errorf("combining mark must not be stripped; got %v", d.HeadingAnchors)
	}
}

// TestParse_HeadingAnchorsMarksKeptJoinersStripped covers Indic text (whose
// clusters rely on combining marks such as the virama, which are retained) and
// the zero-width joiner/non-joiner (U+200C/U+200D), which GitHub strips (they
// fall in github-slugger's removal range). Getting either wrong slugs the
// heading differently than GitHub and flags valid links as dead anchors.
func TestParse_HeadingAnchorsMarksKeptJoinersStripped(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		heading string
		want    string
		absent  string
	}{
		{
			// Devanagari "क्ष" = KA + U+094D VIRAMA (a mark) + SSA: mark kept.
			name:    "indic virama kept",
			heading: "\u0915\u094d\u0937",
			want:    "\u0915\u094d\u0937",
			absent:  "\u0915\u0937", // virama wrongly stripped
		},
		{
			name:    "zero-width joiner stripped",
			heading: "a\u200db",
			want:    "ab",
			absent:  "a\u200db",
		},
		{
			name:    "zero-width non-joiner stripped",
			heading: "a\u200cb",
			want:    "ab",
			absent:  "a\u200cb",
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
			if d.HeadingAnchors[c.absent] {
				t.Errorf("slug %q must not be produced; got %v", c.absent, d.HeadingAnchors)
			}
		})
	}
}
