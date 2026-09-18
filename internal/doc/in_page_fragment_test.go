// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc_test

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

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
