// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// TestParse_BlockAnchorsAndLinks records heading anchors and per-bullet link
// targets, the raw material for shared-subsection resolution (issue #51).
func TestParse_BlockAnchorsAndLinks(t *testing.T) {
	t.Parallel()

	md := "## Attribute Reference\n\n" +
		"### `endpoints` Block\n\n" +
		"* `management` - Endpoint. See [Endpoint](#endpoint).\n" +
		"* `intercluster` - Endpoint. See [Endpoint](#endpoint).\n\n" +
		"#### Endpoint\n\n" +
		"* `dns_name` - DNS name.\n" +
		"* `ip_addresses` - IP addresses.\n"

	d, err := doc.ParseWithOptions([]byte(md), "t", doc.DefaultHeadingTemplates(), doc.ParseOptions{CaptureNestedAttributes: true})
	if err != nil {
		t.Fatal(err)
	}
	if d.BlockAnchors["endpoint"] != "endpoint" {
		t.Errorf(`BlockAnchors["endpoint"] = %q, want "endpoint"`, d.BlockAnchors["endpoint"])
	}
	eb := d.AttributeBlocks["endpoints"]
	if eb == nil {
		t.Fatal(`expected "endpoints" block`)
	}
	for _, a := range eb.Attributes {
		if a.Name == "management" && a.LinkAnchor != "endpoint" {
			t.Errorf(`management LinkAnchor = %q, want "endpoint"`, a.LinkAnchor)
		}
	}
}

// TestParse_HeadingAnchorsPreserveUnderscores guards that generated anchor
// slugs keep underscores (matching GitHub), so snake_case block headings like
// "`ec2_configuration` Block" produce "ec2_configuration-block" rather than
// dropping the underscore. Duplicate headings receive GitHub's -1/-2 suffixes.
func TestParse_HeadingAnchorsPreserveUnderscores(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `x` - (Optional) X.\n\n" +
		"### `ec2_configuration` Block\n\n" +
		"* `image_type` - (Optional) Type.\n\n" +
		"### `ec2_configuration` Block\n\n" +
		"* `image_id` - (Optional) ID.\n"

	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	if !d.HeadingAnchors["ec2_configuration-block"] {
		t.Errorf("HeadingAnchors missing underscore-preserving slug; got %v", d.HeadingAnchors)
	}
	if !d.HeadingAnchors["ec2_configuration-block-1"] {
		t.Errorf("HeadingAnchors missing duplicate-suffixed slug; got %v", d.HeadingAnchors)
	}
}

// TestParse_HeadingAnchorCollisionAvoidance guards GitHub's collision handling:
// a suffixed candidate that clashes with an explicit heading is bumped again.
// Headings "foo Block", "foo Block", "foo Block-1" slug to "foo-block",
// "foo-block", "foo-block-1"; the second duplicate becomes "foo-block-1", which
// then collides with the third heading's own slug, so the third must become
// "foo-block-1-1". A per-base counter would emit "foo-block-1" twice and never
// produce "foo-block-1-1", so this fixture fails without collision avoidance.
func TestParse_HeadingAnchorCollisionAvoidance(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `x` - (Optional) X.\n\n" +
		"### foo Block\n\n* `a` - (Optional) A.\n\n" +
		"### foo Block\n\n* `b` - (Optional) B.\n\n" +
		"### foo Block-1\n\n* `c` - (Optional) C.\n"

	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"foo-block", "foo-block-1", "foo-block-1-1"} {
		if !d.HeadingAnchors[want] {
			t.Errorf("HeadingAnchors missing %q; got %v", want, d.HeadingAnchors)
		}
	}
}

// TestParse_HeadingAnchorsUnicode guards that non-ASCII heading text keeps its
// letters in the slug (matching GitHub), so a link to "#über" resolves.
func TestParse_HeadingAnchorsUnicode(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n## Über Configuration\n\nbody.\n"
	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	if !d.HeadingAnchors["über-configuration"] {
		t.Errorf("expected Unicode-preserving slug %q; got %v", "über-configuration", d.HeadingAnchors)
	}
	if d.HeadingAnchors["ber-configuration"] {
		t.Errorf("ASCII-stripped slug must not be produced; got %v", d.HeadingAnchors)
	}
}

// TestParse_HeadingAnchorsInsideList guards that a heading nested inside a list
// item is still collected (heading slugs are gathered by a full-tree walk, not
// the block extractor which skips list subtrees). Otherwise a link to such a
// heading would be a false dead anchor.
func TestParse_HeadingAnchorsInsideList(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* top\n\n    ### Nested Heading\n\n    detail\n"
	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	if !d.HeadingAnchors["nested-heading"] {
		t.Errorf("heading nested in a list item must be collected; got %v", d.HeadingAnchors)
	}
}

// TestParse_InPageLinksSkipCode confirms that link-looking text inside inline
// code spans and fenced code blocks is NOT collected as an in-page link. The
// anchors rule is enabled by default, so a regression here (e.g. a Goldmark or
// extension change) would turn literal example content into false dead-anchor
// errors — this test locks in the current behavior.
func TestParse_InPageLinksSkipCode(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `a` - (Optional) Real link [ok](#argument-reference) and inline `[x](#missing-inline)`.\n\n" +
		"```\n[y](#missing-fenced)\n```\n\n" +
		"~~~markdown\n[z](#missing-tilde)\n~~~\n"

	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, l := range d.InPageLinks {
		got[l.Fragment] = true
	}
	if !got["argument-reference"] {
		t.Errorf("expected the real prose link to be collected; got %v", d.InPageLinks)
	}
	for _, bad := range []string{"missing-inline", "missing-fenced", "missing-tilde"} {
		if got[bad] {
			t.Errorf("link inside code must not be collected: %q present in %v", bad, d.InPageLinks)
		}
	}
}

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
		{
			// Autolink label text is visible in the rendered heading, but
			// goldmark keeps it off the child list, so it must be pulled from
			// the AutoLink node explicitly.
			name:    "autolink label included",
			heading: "See <https://example.com>",
			want:    "see-httpsexamplecom",
			absent:  "see-",
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

// TestParse_LinkFragmentDecodingOrder guards that in-page link fragments are
// decoded in Goldmark's rendered-href order (UnescapePunctuations,
// ResolveNumericReferences, ResolveEntityNames) rather than resolving named
// entities first. For "#x&amp;#95;y" Goldmark renders the literal target
// "x&#95;y"; resolving the named entity first would instead recursively decode
// the exposed "&#95;" to "_", yielding "x_y" and wrongly accepting a dead link.
func TestParse_LinkFragmentDecodingOrder(t *testing.T) {
	t.Parallel()

	md := "# Resource: aws_thing\n\n" +
		"## Argument Reference\n\n" +
		"* `a` - (Optional) See [x](#x&amp;#95;y).\n"

	d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
	if err != nil {
		t.Fatal(err)
	}

	var got string
	for _, l := range d.InPageLinks {
		got = l.Fragment
	}
	if got != "x&#95;y" {
		t.Errorf("fragment = %q, want %q (must not over-decode to \"x_y\")", got, "x&#95;y")
	}
	if got == "x_y" {
		t.Error("named entities resolved before numeric references: fragment over-decoded to \"x_y\"")
	}
}

// TestParse_InPageFragmentDestinationDecoding guards that the in-page link
// check runs against the destination goldmark actually renders as the href, not
// the raw source. A backslash-escaped ("\#") or entity-encoded ("&#35;")
// opener renders with an href beginning "#", so those links must be collected
// (otherwise a dead anchor evades the rule), while a percent-encoded "#"
// ("%23") renders as a non-fragment path and must be ignored.
func TestParse_InPageFragmentDestinationDecoding(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dest string
		want string // expected fragment, or "" if the link must be ignored
	}{
		{name: "plain hash", dest: "#sec", want: "sec"},
		{name: "backslash-escaped hash", dest: "\\#sec", want: "sec"},
		{name: "numeric-entity hash", dest: "&#35;sec", want: "sec"},
		{name: "percent-encoded hash ignored", dest: "%23sec", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			md := "# Resource: aws_thing\n\n" +
				"## Argument Reference\n\n" +
				"* `a` - (Optional) See [x](" + c.dest + ").\n"

			d, err := doc.ParseWithTemplates([]byte(md), "aws_thing", doc.DefaultHeadingTemplates())
			if err != nil {
				t.Fatal(err)
			}

			var frags []string
			for _, l := range d.InPageLinks {
				frags = append(frags, l.Fragment)
			}
			switch c.want {
			case "":
				if len(frags) != 0 {
					t.Errorf("destination %q must not be an in-page link; got fragments %v", c.dest, frags)
				}
			default:
				if len(frags) != 1 || frags[0] != c.want {
					t.Errorf("destination %q: fragments = %v, want [%q]", c.dest, frags, c.want)
				}
			}
		})
	}
}
