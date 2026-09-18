// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build measure

package check

// TRANSIENT measurement (build-tagged `measure`, never compiled in normal runs
// or CI). Sizes the prose-heading anchor-bridge gap described in
// MISPLACEMENT_REDESIGN.md §12 against the real provider corpus.
//
// Run:
//   SS_PROVIDER_DIR=/abs/path/to/terraform-provider-aws2 \
//   go test -tags measure ./internal/check/ -run TestMeasureProseGap -v -count=1
//
// It reuses the exact runner pipeline (doc.ParseWithOptions with the configured
// heading templates + nested-object toggle, ProviderSchema.ResourceSchemaFor)
// and the redesign §4 resolution rules (dotted=exact, bare=exact|unique-leaf).

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// resolvesSchemaGrounded mirrors redesign §4 for a headed subsection: a dotted
// heading resolves only by exact schema path; a bare heading by exact path or a
// unique leaf. Prose headings resolve to neither.
// classifySubsection reports the schema path a headed subsection resolves to and
// the resolution class: "dotted-exact", "bare-exact", "unique-leaf", or
// "unresolved" (prose).
func classifySubsection(rs *schema.ResourceSchema, key string) (string, string) {
	if rs == nil || key == "" {
		return "", "unresolved"
	}
	if strings.Contains(key, ".") {
		if _, ok := rs.Blocks[key]; ok {
			return key, "dotted-exact"
		}
		return "", "unresolved"
	}
	if _, ok := rs.Blocks[key]; ok {
		return key, "bare-exact"
	}
	if p, ok := uniqueSchemaPathForLeaf(rs, leafName(key)); ok {
		return p, "unique-leaf"
	}
	return "", "unresolved"
}

// resolveKeyPath returns the schema path a headed subsection key resolves to
// under redesign §4 (dotted=exact, bare=exact|unique-leaf), and whether it
// resolved.
func resolveKeyPath(rs *schema.ResourceSchema, key string) (string, bool) {
	if rs == nil {
		return "", false
	}
	if key == "" {
		return "", true
	}
	if strings.Contains(key, ".") {
		if _, ok := rs.Blocks[key]; ok {
			return key, true
		}
		return "", false
	}
	if _, ok := rs.Blocks[key]; ok {
		return key, true
	}
	return uniqueSchemaPathForLeaf(rs, leafName(key))
}

func resolvesSchemaGrounded(rs *schema.ResourceSchema, key string) bool {
	if key == "" || rs == nil {
		return true
	}
	if strings.Contains(key, ".") {
		_, ok := rs.Blocks[key]
		return ok
	}
	if _, ok := rs.Blocks[key]; ok {
		return true
	}
	_, ok := uniqueSchemaPathForLeaf(rs, leafName(key))
	return ok
}

func TestMeasureProseGap(t *testing.T) {
	providerDir := os.Getenv("SS_PROVIDER_DIR")
	if providerDir == "" {
		t.Skip("set SS_PROVIDER_DIR to the terraform-provider-aws worktree")
	}
	cfgPath := filepath.Join(providerDir, ".ci", "swissshepherd-weak.hcl")
	// The tool runs with cwd at the provider root (list files like
	// website/allowed-subcategories.txt resolve relative to it).
	prevWD, _ := os.Getwd()
	if err := os.Chdir(providerDir); err != nil {
		t.Fatalf("chdir provider: %v", err)
	}
	defer os.Chdir(prevWD)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.ProviderDir = providerDir
	schemaPath := cfg.SchemaJSON
	if !filepath.IsAbs(schemaPath) {
		schemaPath = filepath.Join(providerDir, schemaPath)
	}
	ps, err := schema.LoadFile(schemaPath, cfg.ProviderSource)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	// Match cmd/check.go: nested-object expansion + capture toggle.
	nestedCfg := cfg.GetCheck("schema_docs").NestedObjectAttributes
	nested := cfg.IsCheckEnabled("schema_docs") && nestedCfg != nil && *nestedCfg
	if nested {
		schema.ExpandObjectAttributes(ps)
	}
	tmpl := doc.DefaultHeadingTemplates()
	if styles := cfg.GetCheck("schema_docs").BlockHeadingStyles; len(styles) > 0 {
		tmpl = doc.HeadingTemplates(styles)
	}
	r := &Runner{Schema: ps, Config: cfg, HeadingTemplates: tmpl, CaptureNestedAttrs: nested}

	var (
		docsSeen        int
		proseSubsecs    int // prose (unresolved) subsections under Attribute Reference
		proseWithLabels int // ...of those, carrying >=1 labeled attr
		bridgeable      int // ...reachable via a configurable root anchor bullet
		bridgeAttrs     int // labeled configurable attrs under bridgeable subsections
		residual        int // prose+labels but NO config anchor bridge
		affected        = map[string]bool{}
		examples        []string

		// Resolution-class buckets for would-be move findings (§9 gate).
		exactDotted       int
		exactBare         int
		inferred          int // unique-leaf-inferred subsections
		inferredAttrs     int
		inferredContested int // ...where the path is also documented elsewhere
		inferredRes       = map[string]bool{}
		inferredEx        []string
	)

	for i := range cfg.Types {
		typ := &cfg.Types[i]
		if typ.SchemaKind == schema.KindNone {
			continue
		}
		for _, name := range ps.TargetNames(typ.SchemaKind) {
			rs := ps.ResourceSchemaFor(typ.SchemaKind, name)
			if rs == nil {
				continue
			}
			docPath, _, derr := r.resolveDocPath(typ, name)
			if derr != nil {
				continue
			}
			content, rerr := os.ReadFile(docPath)
			if rerr != nil {
				continue
			}
			d, perr := doc.ParseWithOptions(content, docPath, tmpl, doc.ParseOptions{CaptureNestedAttributes: nested})
			if perr != nil {
				continue
			}
			docsSeen++

			// Anchor bridge: any resolvable parent block's anchored bullet whose
			// name is a configurable CHILD block. Maps anchor slug -> the child
			// schema block path. Covers root and nested parents alike.
			bridge := map[string]string{}
			addBridges := func(blocks map[string]*doc.DocBlock) {
				for pkey, pb := range blocks {
					if pb == nil {
						continue
					}
					ppath, ok := resolveKeyPath(rs, pkey)
					if !ok {
						continue
					}
					sb, ok := rs.Blocks[ppath]
					if !ok {
						continue
					}
					for _, a := range pb.Attributes {
						if a.LinkAnchor == "" || !configurableArgAtPath(rs, ppath, a.Name) {
							continue
						}
						if cp, found := childPathForAttr(ppath, sb, a.Name); found {
							bridge[a.LinkAnchor] = cp
						}
					}
				}
			}
			addBridges(d.ArgumentBlocks)
			addBridges(d.AttributeBlocks)
			// Reverse BlockAnchors: block name -> anchor slug(s).
			blockToAnchors := map[string][]string{}
			for slug, blk := range d.BlockAnchors {
				blockToAnchors[blk] = append(blockToAnchors[blk], slug)
			}

			// Per-doc: how many headed doc blocks (Argument or Attribute
			// Reference) resolve to each schema path. >=2 means the path is
			// documented by competing headings — the split/alternate case where
			// PR #61's ownership resolver could differ from unique-leaf.
			pathCount := map[string]int{}
			for _, blocks := range []map[string]*doc.DocBlock{d.AttributeBlocks, d.ArgumentBlocks} {
				for k, b := range blocks {
					if b == nil || k == "" || b.Heading == "" {
						continue
					}
					if p, cls := classifySubsection(rs, k); cls != "unresolved" {
						pathCount[p]++
					}
				}
			}

			for key, block := range d.AttributeBlocks {
				if key == "" || block.Heading == "" {
					continue
				}
				p, cls := classifySubsection(rs, key)
				if cls != "unresolved" {
					// Would-be move finding? Needs >=1 configurable labeled attr.
					cfg := 0
					for _, a := range block.Attributes {
						if (a.Required || a.Optional) && configurableArgAtPath(rs, p, a.Name) {
							cfg++
						}
					}
					if cfg == 0 {
						continue
					}
					switch cls {
					case "dotted-exact":
						exactDotted++
					case "bare-exact":
						exactBare++
					case "unique-leaf":
						inferred++
						inferredAttrs += cfg
						inferredRes[name] = true
						if pathCount[p] >= 2 {
							inferredContested++
							if len(inferredEx) < 30 {
								inferredEx = append(inferredEx, "CONTESTED "+filepath.Base(docPath)+"  heading="+block.Heading+"  ->"+p+"  cfg="+itoa(cfg)+"  docBlocksForPath="+itoa(pathCount[p]))
							}
						} else if len(inferredEx) < 30 {
							inferredEx = append(inferredEx, "uniq "+filepath.Base(docPath)+"  heading="+block.Heading+"  ->"+p+"  cfg="+itoa(cfg))
						}
					}
					continue
				}
				proseSubsecs++
				labeled := 0
				for _, a := range block.Attributes {
					if a.Required || a.Optional {
						labeled++
					}
				}
				if labeled == 0 {
					continue
				}
				proseWithLabels++

				// Anchor bridge: some root configurable-block bullet links here.
				bridgedPath := ""
				for _, slug := range blockToAnchors[key] {
					if p, ok := bridge[slug]; ok {
						bridgedPath = p
						break
					}
				}
				if bridgedPath == "" {
					residual++
					if len(examples) < 30 {
						hasAnchor := len(blockToAnchors[key]) > 0
						rootLinks := 0
						for _, slug := range blockToAnchors[key] {
							_ = slug
							rootLinks++
						}
						examples = append(examples, "RESIDUAL "+filepath.Base(docPath)+"  heading="+block.Heading+"  key="+key+"  labeled="+itoa(labeled)+"  hasAnchor="+btoa(hasAnchor)+"  anchorsOnBlock="+itoa(rootLinks))
					}
					continue
				}
				cfgAttrs := 0
				for _, a := range block.Attributes {
					if (a.Required || a.Optional) && configurableArgAtPath(rs, bridgedPath, a.Name) {
						cfgAttrs++
					}
				}
				if cfgAttrs == 0 {
					residual++
					continue
				}
				bridgeable++
				bridgeAttrs += cfgAttrs
				affected[name] = true
				if len(examples) < 30 {
					examples = append(examples, filepath.Base(docPath)+"  heading="+block.Heading+"  ->schema="+bridgedPath+"  cfgAttrs="+itoa(cfgAttrs))
				}
			}
		}
	}

	sort.Strings(examples)
	t.Logf("\n=== prose-heading anchor-bridge measurement ===")
	t.Logf("docs parsed:                         %d", docsSeen)
	t.Logf("prose subsections (unresolved):      %d", proseSubsecs)
	t.Logf("  ...with >=1 labeled attribute:     %d", proseWithLabels)
	t.Logf("  ...bridgeable via root anchor:     %d   (across %d resources)", bridgeable, len(affected))
	t.Logf("  ...labeled configurable attrs:     %d", bridgeAttrs)
	t.Logf("  ...prose+labels, NO bridge:        %d", residual)
	t.Logf("--- resolution-class buckets (would-be move findings) ---")
	t.Logf("dotted-exact subsections:            %d", exactDotted)
	t.Logf("bare-exact subsections:              %d", exactBare)
	t.Logf("unique-leaf-inferred subsections:    %d   (%d cfg attrs, %d resources)", inferred, inferredAttrs, len(inferredRes))
	t.Logf("  ...of which CONTESTED (new vs #61): %d", inferredContested)
	t.Logf("--- inferred examples (<=30) ---")
	for _, e := range inferredEx {
		t.Logf("  %s", e)
	}
	t.Logf("--- bridgeable prose examples (<=30) ---")
	for _, e := range examples {
		t.Logf("  %s", e)
	}
}

func btoa(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
