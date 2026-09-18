// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"testing"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// TestResolveSubsectionPath pins the §4 resolution classifier
// (docs/rules/argument-attribute-misplacement.md). It is deliberately a
// synthetic, in-CI unit test: the corpus counts in §9/§12 may drift, but the
// classification logic (dotted-exact / bare-exact / unique-leaf / unresolved /
// root) must not silently change when the parser or slug subsystem changes.
func TestResolveSubsectionPath(t *testing.T) {
	t.Parallel()

	// blocks builds a schema whose blocks are keyed by the given dot-paths.
	blocks := func(paths ...string) map[string]*schema.Block {
		m := make(map[string]*schema.Block, len(paths))
		for _, p := range paths {
			m[p] = &schema.Block{Path: p}
		}
		return m
	}

	cases := []struct {
		name      string
		rs        *schema.ResourceSchema
		heading   string // heading of docBlocks[key]; "" means heading-less synthetic
		key       string
		wantPath  string
		wantClass resolutionClass
		wantOK    bool
	}{
		{
			name:      "root",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "cluster.endpoint")},
			key:       "",
			wantPath:  "",
			wantClass: resolveRoot,
			wantOK:    true,
		},
		{
			name:      "dotted exact",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "cluster.endpoint")},
			heading:   "`cluster.endpoint` Block",
			key:       "cluster.endpoint",
			wantPath:  "cluster.endpoint",
			wantClass: resolveDottedExact,
			wantOK:    true,
		},
		{
			name:      "dotted miss never remaps by leaf",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "cluster.endpoint")},
			heading:   "`other.endpoint` Block",
			key:       "other.endpoint",
			wantClass: resolveUnresolved,
			wantOK:    false,
		},
		{
			name:      "bare exact root-level block",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "notification")},
			heading:   "`notification` Block",
			key:       "notification",
			wantPath:  "notification",
			wantClass: resolveBareExact,
			wantOK:    true,
		},
		{
			name:      "bare unique-leaf inference (fixes 1/3)",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "cluster.endpoint")},
			heading:   "endpoint",
			key:       "endpoint",
			wantPath:  "cluster.endpoint",
			wantClass: resolveUniqueLeaf,
			wantOK:    true,
		},
		{
			name:      "bare ambiguous leaf stays unresolved",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "filter.dimensions", "filter.and.dimensions")},
			heading:   "dimensions",
			key:       "dimensions",
			wantClass: resolveUnresolved,
			wantOK:    false,
		},
		{
			name:      "bare no schema match",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "cluster.endpoint")},
			heading:   "nonexistent",
			key:       "nonexistent",
			wantClass: resolveUnresolved,
			wantOK:    false,
		},
		{
			name:      "heading-less bare never inferred by leaf",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "cluster.endpoint")},
			heading:   "", // synthetic reference bullet / prose lead-in
			key:       "endpoint",
			wantClass: resolveUnresolved,
			wantOK:    false,
		},
		{
			name:      "heading-less dotted still resolves by exact path",
			rs:        &schema.ResourceSchema{Blocks: blocks("", "cluster.endpoint")},
			heading:   "",
			key:       "cluster.endpoint",
			wantPath:  "cluster.endpoint",
			wantClass: resolveDottedExact,
			wantOK:    true,
		},
		{
			name:      "nil schema",
			rs:        nil,
			key:       "notification",
			wantClass: resolveUnresolved,
			wantOK:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			docBlocks := map[string]*doc.DocBlock{}
			if tc.key != "" {
				docBlocks[tc.key] = &doc.DocBlock{Name: tc.key, Heading: tc.heading}
			}

			gotPath, gotClass, gotOK := resolveSubsectionPath(tc.rs, docBlocks, tc.key)
			if gotOK != tc.wantOK {
				t.Fatalf("ok = %v, want %v (path=%q class=%d)", gotOK, tc.wantOK, gotPath, gotClass)
			}
			if gotClass != tc.wantClass {
				t.Errorf("class = %d, want %d", gotClass, tc.wantClass)
			}
			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
		})
	}
}
