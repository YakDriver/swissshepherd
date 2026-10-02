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

// TestResolveSection pins the coverage resolver
// (docs/rules/coverage-path-resolution.md §4): one section per path per
// reference section, chosen by heading key alone.
func TestResolveSection(t *testing.T) {
	t.Parallel()

	rs := &schema.ResourceSchema{Blocks: map[string]*schema.Block{
		"":                                     {},
		"visibility_config":                    {},
		"rule":                                 {},
		"rule.visibility_config":               {},
		"spec":                                 {},
		"spec.grpc_route":                      {},
		"spec.grpc_route.match":                {},
		"spec.grpc_route.match.metadata":       {},
		"spec.grpc_route.match.metadata.match": {},
		"a":                                    {},
		"a.b":                                  {},
		"a.x":                                  {},
		"a.x.b":                                {},
	}}

	testCases := map[string]struct {
		keys    []string // doc section keys present in one reference section
		path    string
		wantKey string // "" means unresolved
	}{
		"root": {
			keys: []string{""}, path: "", wantKey: "",
		},
		"exact path wins over bare leaf": {
			keys: []string{"spec.grpc_route.match.metadata.match", "match"}, path: "spec.grpc_route.match.metadata.match", wantKey: "spec.grpc_route.match.metadata.match",
		},
		"exact path wins over composite that is another path": {
			keys: []string{"spec.grpc_route.match", "spec.grpc_route.match.metadata.match"}, path: "spec.grpc_route.match.metadata.match", wantKey: "spec.grpc_route.match.metadata.match",
		},
		"dotted key naming another path is skipped": {
			keys: []string{"a.b"}, path: "a.x.b", wantKey: "",
		},
		"dotted key naming another path is skipped, falls through to leaf": {
			keys: []string{"a.b", "b"}, path: "a.x.b", wantKey: "b",
		},
		"bare leaf that is also a root path still serves nested paths": {
			keys: []string{"visibility_config"}, path: "rule.visibility_config", wantKey: "visibility_config",
		},
		"nearest ancestor composite before farther": {
			keys: []string{"spec.match", "metadata.match"}, path: "spec.grpc_route.match.metadata.match", wantKey: "metadata.match",
		},
		"three-segment composite before two-segment": {
			keys: []string{"metadata.match", "match.metadata.match"}, path: "spec.grpc_route.match.metadata.match", wantKey: "match.metadata.match",
		},
		"no matching key": {
			keys: []string{"other"}, path: "a.b", wantKey: "",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			blocks := make(map[string]*doc.DocBlock, len(tc.keys))
			for _, k := range tc.keys {
				blocks[k] = &doc.DocBlock{Name: k}
			}
			got := resolveSection(rs, blocks, tc.path)
			if tc.path == "" {
				if got != blocks[""] {
					t.Errorf("root: got %v, want the root block", got)
				}
				return
			}
			gotKey := ""
			if got != nil {
				gotKey = got.Name
			}
			if gotKey != tc.wantKey {
				t.Errorf("resolveSection(%q) = %q, want %q", tc.path, gotKey, tc.wantKey)
			}
		})
	}
}
