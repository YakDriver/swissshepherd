// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// TestClassifyAttrPlacement pins the §3 classification table
// (docs/rules/argument-attribute-misplacement.md): a labeled attribute under
// Attribute Reference is misplaced iff it is a purely configurable argument at
// the resolved path; unlabeled attributes are fine; everything else keeps the
// legacy strip-label guidance. The resolved flag must prevent an unresolved
// subsection from ever producing a move, including the root-"" collision.
func TestClassifyAttrPlacement(t *testing.T) {
	t.Parallel()

	// Root scalars: instance_type (pure-config), arn (computed-only), mode
	// (Optional+Computed). A pure-config child block (network.subnet_id) and a
	// ConfigUnknown block (opaque) exercise the block and lossy-object paths.
	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"": {
			Attributes: []schema.Attribute{
				{Name: "instance_type", Optional: true},
				{Name: "arn", Computed: true},
				{Name: "mode", Optional: true, Computed: true},
			},
			ChildBlocks: []string{"network"},
		},
		"network": {Path: "network", Attributes: []schema.Attribute{
			{Name: "subnet_id", Required: true},
		}},
		"opaque": {Path: "opaque", ConfigUnknown: true, Attributes: []schema.Attribute{
			{Name: "x", Optional: true},
		}},
	}}

	cases := []struct {
		name     string
		path     string
		resolved bool
		attr     doc.DocAttribute
		want     placement
	}{
		{
			name:     "unlabeled computed output is fine",
			path:     "",
			resolved: true,
			attr:     doc.DocAttribute{Name: "arn"}, // no label
			want:     placementOK,
		},
		{
			name:     "labeled pure-config root scalar is misplaced (#62)",
			path:     "",
			resolved: true,
			attr:     doc.DocAttribute{Name: "instance_type", Optional: true},
			want:     placementMisplaced,
		},
		{
			name:     "labeled computed-only field strips label",
			path:     "",
			resolved: true,
			attr:     doc.DocAttribute{Name: "arn", Optional: true}, // erroneous label
			want:     placementStripLabel,
		},
		{
			name:     "labeled Optional+Computed strips label (never a move)",
			path:     "",
			resolved: true,
			attr:     doc.DocAttribute{Name: "mode", Optional: true},
			want:     placementStripLabel,
		},
		{
			name:     "unresolved subsection never moves even on a matching root name",
			path:     "", // caller passes "" for unresolved; resolved=false must guard it
			resolved: false,
			attr:     doc.DocAttribute{Name: "instance_type", Optional: true},
			want:     placementStripLabel,
		},
		{
			name:     "labeled attribute absent from schema strips label",
			path:     "",
			resolved: true,
			attr:     doc.DocAttribute{Name: "ghost", Optional: true},
			want:     placementStripLabel,
		},
		{
			name:     "labeled child block with pure-config subtree is misplaced",
			path:     "",
			resolved: true,
			attr:     doc.DocAttribute{Name: "network", Optional: true},
			want:     placementMisplaced,
		},
		{
			name:     "ConfigUnknown block strips label",
			path:     "opaque",
			resolved: true,
			attr:     doc.DocAttribute{Name: "x", Optional: true},
			want:     placementStripLabel,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyAttrPlacement(rs, tc.path, tc.resolved, tc.attr); got != tc.want {
				t.Errorf("classifyAttrPlacement = %d, want %d", got, tc.want)
			}
		})
	}
}
