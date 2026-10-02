// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"bufio"
	"bytes"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// DefaultImplicitAttributes are attribute names that are always implicitly
// present in provider docs and don't need explicit documentation entries.
var DefaultImplicitAttributes = []string{"id", "tags_all"}

// DefaultAllowPhantoms are attribute names that may appear in docs without
// a corresponding schema entry (provider-injected attributes).
var DefaultAllowPhantoms = []string{"tags", "tags_all"}

// DefaultSkipBlocks are block names that are always skipped.
var DefaultSkipBlocks = []string{"timeouts"}

// DefaultBadDescriptionPrefixes is the list of weak or redundant description starts.
var DefaultBadDescriptionPrefixes = []string{
	"A ", "An ", "The ", "This ", "It ",
	"Indicates ", "Specifies ", "Describes ", "Defines ",
	"Contains ", "Determines ", "Identifies ", "Represents ", "Denotes ", "Holds ", "Used ",
}

// SchemaDocsRule validates argument and attribute sections against the schema.
// Sub-checks (all enabled by default, disable with pointer-to-false):
//   - Coverage: every schema attr documented, every documented attr in schema
//   - Ordering: alphabetical within required/optional/unmarked groups
//   - Description: descriptions don't start with bad prefixes
//   - Heading: block headings match preferred style
//   - Format: no code blocks, single-line attrs, uninterrupted lists
//   - Labels: arguments have (Required)/(Optional), attributes do not
type SchemaDocsRule struct {
	// Coverage options
	IgnoreDeprecated   bool
	ImplicitAttributes []string
	AllowPhantoms      []string
	SkipBlocks         []string

	// AllowInlineReadOnly permits Read-Only (computed-only) attributes to
	// be documented inline in Argument Reference with a "(Read-Only)" label,
	// alongside Required and Optional siblings, instead of requiring them
	// in Attribute Reference. Default false: strict separation — Argument
	// Reference is for configurable attributes only.
	AllowInlineReadOnly *bool

	// Sub-check toggles (nil = enabled)
	Coverage    *bool
	Ordering    *bool
	Description *bool
	Heading     *bool
	Format      *bool
	Labels      *bool
	Byline      *bool
	Deprecated  *bool

	// Description options
	BadPrefixes []string

	// Heading options
	Preferred doc.HeadingTemplates

	// Format sub-options (nil = enabled)
	NoCodeBlocks              *bool
	SingleLineAttrs           *bool
	UninterruptedLists        *bool
	AllowAttributeIndentation *bool // nil/true = allow indented sub-attrs in Attribute Reference
}

func (r *SchemaDocsRule) Name() string { return "schema_docs" }

func (r *SchemaDocsRule) implicit() []string {
	if r.ImplicitAttributes != nil {
		return r.ImplicitAttributes
	}
	return DefaultImplicitAttributes
}

func (r *SchemaDocsRule) phantom() []string {
	if r.AllowPhantoms != nil {
		return r.AllowPhantoms
	}
	return DefaultAllowPhantoms
}

func (r *SchemaDocsRule) skipBlocks() []string {
	if r.SkipBlocks != nil {
		return r.SkipBlocks
	}
	return DefaultSkipBlocks
}

func (r *SchemaDocsRule) prefixes() []string {
	if r.BadPrefixes != nil {
		return r.BadPrefixes
	}
	return DefaultBadDescriptionPrefixes
}

// allowInlineReadOnly reports whether (Read-Only) labels are permitted in
// Argument Reference. Default false: strict separation between Argument
// Reference (configurable) and Attribute Reference (Read-Only).
func (r *SchemaDocsRule) allowInlineReadOnly() bool {
	return r.AllowInlineReadOnly != nil && *r.AllowInlineReadOnly
}

func enabled(b *bool) bool { return b == nil || *b }

func (r *SchemaDocsRule) Check(ctx CheckContext) []Result {
	var results []Result
	if ctx.Doc != nil {
		ctx.Doc = withProseSections(ctx.Schema, ctx.Doc)
	}

	var idx map[*doc.DocBlock][]string
	var shared []sharedSection
	if ctx.Schema != nil && ctx.Doc != nil && (enabled(r.Coverage) || enabled(r.Labels)) {
		idx = r.sectionIndex(ctx)
	}
	if idx != nil && enabled(r.Coverage) {
		shared = r.sharedSections(ctx, idx)
	}
	if enabled(r.Coverage) {
		results = append(results, r.checkCoverage(ctx, idx, shared)...)
	}
	if enabled(r.Ordering) {
		results = append(results, r.checkOrdering(ctx)...)
	}
	if enabled(r.Description) {
		results = append(results, r.checkDescriptions(ctx)...)
	}
	if enabled(r.Heading) {
		results = append(results, r.checkHeadings(ctx)...)
	}
	if enabled(r.Format) {
		results = append(results, r.checkFormat(ctx)...)
	}
	if enabled(r.Labels) {
		results = append(results, r.checkLabels(ctx, idx)...)
	}
	if enabled(r.Byline) {
		results = append(results, r.checkBylines(ctx)...)
	}
	if enabled(r.Deprecated) {
		results = append(results, r.checkDeprecated(ctx, shared)...)
	}

	return results
}

// --- Coverage ---

func (r *SchemaDocsRule) checkCoverage(ctx CheckContext, idx map[*doc.DocBlock][]string, shared []sharedSection) []Result {
	rs := ctx.Schema
	if rs == nil {
		return nil
	}

	var results []Result
	missing := r.undocumentedBlocks(ctx)

	// Findings are keyed by schema path, never by leaf name: same-named blocks
	// under different parents are different defects (#77). Sorted iteration
	// keeps output order deterministic (#65).
	for _, blockPath := range slices.Sorted(maps.Keys(rs.Blocks)) {
		schemaBlock := rs.Blocks[blockPath]
		if slices.Contains(r.skipBlocks(), blockPath) {
			continue
		}

		docBlocks := resolveSections(rs, ctx.Doc, blockPath)

		if len(docBlocks) == 0 {
			if msg, ok := missing.message(blockPath); ok {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
					Message: msg,
					Block:   blockPath,
				})
			}
			continue
		}

		// Aggregate attributes across all matching doc blocks so that
		// coverage works whether the schema block is documented under
		// its leaf name (e.g. `### \`probabilistic\`` Block), under its
		// full path (e.g. via a dot-notation reference like
		// `rule[*].probabilistic[*].x` routed during parsing), or both.
		//
		// A section whose key has several headings is judged by the fit rule
		// (checkDuplicateHeadings) instead: its fields aren't checked for
		// existence here, and fields whose only home is its reference section
		// aren't checked for absence.
		documented := make(map[string]bool)
		var malformed []doc.MalformedAttr
		for _, b := range docBlocks {
			for _, attr := range b.Attributes {
				documented[attr.Name] = true
			}
			malformed = append(malformed, b.MalformedAttributes...)
		}
		argSection := resolveSection(rs, ctx.Doc.ArgumentBlocks, blockPath)
		argDuplicated := duplicated(argSection)
		attrSection := resolveSection(rs, ctx.Doc.AttributeBlocks, blockPath)

		for _, attr := range schemaBlock.Attributes {
			if r.shouldSkipAttribute(attr) {
				continue
			}
			if argDuplicated && !schemaBlock.ConfigUnknown && !attr.Computed {
				continue
			}
			if !documented[attr.Name] {
				msg := fmt.Sprintf("attribute %q in block %q is not documented", attr.Name, displayPath(blockPath)) +
					missingPointer(rs, idx, argSection, blockPath, attr.Name)
				if m, ok := findMalformed(malformed, attr.Name); ok {
					msg = fmt.Sprintf("attribute %q in block %q is documented but missing the \" - \" separator (expected: * `%s` - (Required|Optional) ...)", attr.Name, displayPath(blockPath), attr.Name)
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: severity(attr), Message: msg, Block: blockPath, Line: m.Line,
					})
				} else {
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: severity(attr), Message: msg, Block: blockPath,
					})
				}
			} else if res, ok := r.outsideHome(ctx, blockPath, attr, argSection, attrSection); ok {
				results = append(results, res)
			} else if m, ok := findMalformed(malformed, attr.Name); ok {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("attribute %q in block %q is documented but missing the \" - \" separator (expected: * `%s` - (Required|Optional) ...)", attr.Name, displayPath(blockPath), attr.Name),
					Block:   blockPath, Line: m.Line,
				})
			}
		}

		// Doc → schema: documented attributes should exist in schema.
		if len(schemaBlock.Attributes) == 0 && len(schemaBlock.ChildBlocks) == 0 {
			continue
		}
		schemaAttrNames := make(map[string]bool, len(schemaBlock.Attributes))
		for _, attr := range schemaBlock.Attributes {
			schemaAttrNames[attr.Name] = true
		}
		for _, child := range schemaBlock.ChildBlocks {
			schemaAttrNames[leafName(child)] = true
		}

		for _, b := range docBlocks {
			if duplicated(b) {
				continue
			}
			kind := "attribute"
			if b == argSection {
				kind = "argument"
			}
			for _, docAttr := range b.Attributes {
				if schemaAttrNames[docAttr.Name] || slices.Contains(r.phantom(), docAttr.Name) {
					continue
				}
				msg := fmt.Sprintf("documented %s %q in block %q does not exist in schema", kind, docAttr.Name, displayPath(blockPath))
				var where []string
				for _, q := range idx[b] {
					if q != blockPath && blockHasField(rs.Blocks[q], docAttr.Name) {
						where = append(where, q)
					}
				}
				if len(where) > 0 {
					msg += fmt.Sprintf("; section %q also documents %s, where %q exists", b.Name, pointerList(where), docAttr.Name)
				}
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError, Message: msg, Block: blockPath, Line: docAttr.Line,
				})
			}
		}

		// Computed-only misplacement in arguments. Suppressed when
		// AllowInlineReadOnly is true: under the permissive convention,
		// inline (Read-Only) labels in Argument Reference are
		// intentional, not misplaced.
		if blockPath == "" && !r.allowInlineReadOnly() {
			if argBlock := ctx.Doc.ArgumentBlocks[""]; argBlock != nil {
				results = append(results, r.checkComputedMisplacement(ctx, schemaBlock, argBlock, ctx.Doc.AttributeBlocks[""])...)
			}
		}
	}

	// Attributes: Read-Only (computed-only) coverage across all schema blocks.
	results = append(results, r.checkAttributeCoverage(ctx)...)

	results = append(results, r.checkPhantomBlocks(ctx)...)
	results = append(results, r.checkOrphans(ctx)...)
	results = append(results, r.checkDuplicateHeadings(ctx)...)
	results = append(results, r.checkUnresolvedSections(ctx)...)
	results = append(results, r.sharedSectionResults(ctx, shared)...)

	return results
}

// outsideHome reports a pure-configurable argument at p that is documented
// only under Attribute Reference, with no label (§2 home section). The union
// of p's sections counts it as documented, and labels reads an unlabeled
// bullet there as a computed output, so nothing else reports it. A labeled
// bullet is left to labels' move finding, so the two never both fire.
// Severity follows that finding: a warning for a root scalar (#62).
func (r *SchemaDocsRule) outsideHome(ctx CheckContext, p string, attr schema.Attribute, argSection, attrSection *doc.DocBlock) (Result, bool) {
	if attrSection == nil || !configurableArgAtPath(ctx.Schema, p, attr.Name) {
		return Result{}, false
	}
	if argSection != nil && slices.ContainsFunc(argSection.Attributes, func(a doc.DocAttribute) bool { return a.Name == attr.Name }) {
		return Result{}, false
	}
	i := slices.IndexFunc(attrSection.Attributes, func(a doc.DocAttribute) bool { return a.Name == attr.Name })
	if i < 0 {
		return Result{}, false
	}
	da := attrSection.Attributes[i]
	if da.Required || da.Optional || da.ReadOnly {
		return Result{}, false
	}
	label := "(Optional)"
	if attr.Required {
		label = "(Required)"
	}
	where := ""
	sev := severity(attr)
	if p == "" {
		sev = SeverityWarning
	} else {
		where = fmt.Sprintf(" in block %q", displayPath(p))
	}
	return Result{
		Rule: r.Name(), Resource: ctx.Resource, Severity: sev, Block: p, Line: da.Line,
		Message: fmt.Sprintf("argument %q%s is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference and label it %s", attr.Name, where, label),
	}, true
}

// --- Shared sections ---

// sectionIndex maps each section to the schema paths it serves, sorted: the
// paths that resolve to it under resolveSection, excluding skip_blocks
// (docs/rules/coverage-path-resolution.md §2). Argument and Attribute
// Reference sections are distinct blocks, so one map holds both.
func (r *SchemaDocsRule) sectionIndex(ctx CheckContext) map[*doc.DocBlock][]string {
	idx := make(map[*doc.DocBlock][]string)
	for _, p := range slices.Sorted(maps.Keys(ctx.Schema.Blocks)) {
		if p == "" || slices.Contains(r.skipBlocks(), p) {
			continue
		}
		for _, blocks := range []map[string]*doc.DocBlock{ctx.Doc.ArgumentBlocks, ctx.Doc.AttributeBlocks} {
			if b := resolveSection(ctx.Schema, blocks, p); b != nil {
				idx[b] = append(idx[b], p)
			}
		}
	}
	return idx
}

// shareConflict is one way a shared section can't be right for all the paths
// it serves (§6): kind 1, field X must be listed for Q but doesn't exist at P;
// 2, X's label differs; 3, X's deprecation differs; 4, child block X differs
// below this level.
type shareConflict struct {
	kind        int
	field, q, p string
	qVal, pVal  string // labels, for kind 2
}

// sharedSection is a section serving several paths that has conflicts.
type sharedSection struct {
	inAttrs   bool
	key       string
	block     *doc.DocBlock
	served    []string
	conflicts []shareConflict
}

// sharedSections evaluates the shared-section invariant for every section
// that serves two or more paths: it is valid only when every schema-derived
// property swissshepherd compares is identical across those paths (§6). Each
// disjunct is evaluated over the whole served set, never against one
// representative path, because interchangeability isn't transitive.
// Duplicated keys are left to the fit rule.
func (r *SchemaDocsRule) sharedSections(ctx CheckContext, idx map[*doc.DocBlock][]string) []sharedSection {
	e := &shareEval{r: r, rs: ctx.Schema, memo: make(map[string][]shareConflict)}
	var out []sharedSection
	for _, inAttrs := range []bool{false, true} {
		blocks := ctx.Doc.ArgumentBlocks
		if inAttrs {
			blocks = ctx.Doc.AttributeBlocks
		}
		for _, key := range slices.Sorted(maps.Keys(blocks)) {
			b := blocks[key]
			served := idx[b]
			if len(served) < 2 || duplicated(b) {
				continue
			}
			listed := make(map[string]bool, len(b.Attributes))
			for _, a := range b.Attributes {
				listed[a.Name] = true
			}
			conflicts := e.eval(served, inAttrs, listed)
			// Disjunct 1 reports one representative per section: the
			// field-existence errors already enumerate the rest.
			var first *shareConflict
			var rest []shareConflict
			for i, c := range conflicts {
				if c.kind != 1 {
					rest = append(rest, c)
					continue
				}
				if first == nil || c.q < first.q || (c.q == first.q && (c.field < first.field || (c.field == first.field && c.p < first.p))) {
					first = &conflicts[i]
				}
			}
			if first != nil {
				rest = append([]shareConflict{*first}, rest...)
			}
			if len(rest) > 0 {
				out = append(out, sharedSection{inAttrs: inAttrs, key: key, block: b, served: served, conflicts: rest})
			}
		}
	}
	return out
}

type shareEval struct {
	r    *SchemaDocsRule
	rs   *schema.ResourceSchema
	memo map[string][]shareConflict
}

// eval returns the conflicts among paths for one reference section. listed
// holds the fields the section lists; nil at child level, where no section is
// involved: there every field that exists is compared for labels and
// deprecation, since the children's own sections would list it.
func (e *shareEval) eval(paths []string, inAttrs bool, listed map[string]bool) []shareConflict {
	fieldSet := make(map[string]bool)
	for _, p := range paths {
		b := e.rs.Blocks[p]
		for _, a := range b.Attributes {
			fieldSet[a.Name] = true
		}
		for _, c := range b.ChildBlocks {
			fieldSet[leafName(c)] = true
		}
	}
	for f := range listed {
		fieldSet[f] = true
	}

	var out []shareConflict
	for _, f := range slices.Sorted(maps.Keys(fieldSet)) {
		if slices.Contains(e.r.implicit(), f) || slices.Contains(e.r.phantom(), f) {
			continue
		}
		// exist is schema presence: a field coverage ignores (deprecated,
		// under ignore_deprecated) still exists, so listing it isn't wrong.
		// Ignored fields can't force a listing or a marker comparison.
		var exist, single, compared []string
		for _, p := range paths {
			b := e.rs.Blocks[p]
			if !blockHasField(b, f) {
				continue
			}
			exist = append(exist, p)
			if e.ignored(b, f) {
				continue
			}
			compared = append(compared, p)
			if e.r.homeOnlyIn(e.rs, p, f, inAttrs) {
				single = append(single, p)
			}
		}
		if len(single) > 0 && len(exist) < len(paths) {
			missing := slices.IndexFunc(paths, func(p string) bool { return !slices.Contains(exist, p) })
			out = append(out, shareConflict{kind: 1, field: f, q: single[0], p: paths[missing]})
		}
		if !(listed == nil || listed[f] || len(single) > 0) || len(compared) < 2 {
			continue
		}
		// A field can be an attribute at some paths and a child block at
		// others, and an object-typed attribute is both: with
		// nested_object_attributes its fields are expanded into a block at
		// p.f that ChildBlocks doesn't list. Labels and deprecation are
		// compared among the attributes, contents among the blocks.
		var asAttr, children []string
		for _, p := range compared {
			if _, ok := attrAt(e.rs.Blocks[p], f); ok {
				asAttr = append(asAttr, p)
			}
			if c := p + "." + f; !slices.Contains(e.r.skipBlocks(), c) {
				if _, ok := e.rs.Blocks[c]; ok {
					children = append(children, c)
				}
			}
		}
		// A ConfigUnknown block's labels are unknowable, so they're never
		// compared (never guess).
		labelsKnown := !slices.ContainsFunc(asAttr, func(p string) bool { return e.rs.Blocks[p].ConfigUnknown })
		if len(asAttr) > 1 {
			rep := asAttr[0]
			sa, _ := attrAt(e.rs.Blocks[rep], f)
			for _, p := range asAttr[1:] {
				pa, _ := attrAt(e.rs.Blocks[p], f)
				if !inAttrs && labelsKnown && labelFor(sa) != labelFor(pa) {
					out = append(out, shareConflict{kind: 2, field: f, q: rep, p: p, qVal: labelFor(sa), pVal: labelFor(pa)})
					break
				}
			}
			for _, p := range asAttr[1:] {
				pa, _ := attrAt(e.rs.Blocks[p], f)
				if sa.Deprecated != pa.Deprecated {
					q, other := rep, p
					if !sa.Deprecated {
						q, other = p, rep
					}
					out = append(out, shareConflict{kind: 3, field: f, q: q, p: other})
					break
				}
			}
		}
		if len(children) < 2 {
			continue
		}
		// Conflicts among the children name child paths (each is p+"."+f), so
		// the parents are recovered by dropping the leaf.
		if cs := e.below(children); len(cs) > 0 {
			out = append(out, shareConflict{kind: 4, field: f, q: strings.TrimSuffix(cs[0].q, "."+f), p: strings.TrimSuffix(cs[0].p, "."+f)})
		}
	}
	return out
}

// below returns the conflicts among a set of same-named child blocks in
// either reference section, memoized by the sorted set.
func (e *shareEval) below(children []string) []shareConflict {
	key := strings.Join(children, "\x00")
	if cs, ok := e.memo[key]; ok {
		return cs
	}
	e.memo[key] = nil // a tree has no cycles, but don't recurse into an entry being built
	cs := append(e.eval(children, false, nil), e.eval(children, true, nil)...)
	e.memo[key] = cs
	return cs
}

// ignored reports whether coverage's filters drop f at block b.
func (e *shareEval) ignored(b *schema.Block, f string) bool {
	a, ok := attrAt(b, f)
	return ok && e.r.IgnoreDeprecated && a.Deprecated
}

func attrAt(b *schema.Block, f string) (schema.Attribute, bool) {
	if i := slices.IndexFunc(b.Attributes, func(a schema.Attribute) bool { return a.Name == f }); i >= 0 {
		return b.Attributes[i], true
	}
	return schema.Attribute{}, false
}

// labelFor is the one schema-correct label for an attribute.
func labelFor(a schema.Attribute) string {
	switch {
	case a.Required:
		return "(Required)"
	case a.Optional:
		return "(Optional)"
	}
	return "(Read-Only)"
}

// homeOnlyIn reports whether field f at path p may be documented only in the
// given reference section (§2 field homes).
func (r *SchemaDocsRule) homeOnlyIn(rs *schema.ResourceSchema, p, f string, inAttrs bool) bool {
	b := rs.Blocks[p]
	if b.ConfigUnknown {
		return false
	}
	readOnlyHome := inAttrs && !r.allowInlineReadOnly()
	if a, ok := attrAt(b, f); ok {
		switch {
		case (a.Required || a.Optional) && !a.Computed:
			return !inAttrs
		case a.Computed && !a.Required && !a.Optional:
			return readOnlyHome
		}
		return false
	}
	c := childBlockPath(p, f)
	switch {
	case blockTreeHasPureConfigurable(rs, c, make(map[string]bool)):
		return !inAttrs
	case blockTreeHasConfigurable(rs, c):
		return false
	}
	return readOnlyHome
}

// blockTreeHasConfigurable reports whether any Required or Optional field
// exists in the subtree at path.
func blockTreeHasConfigurable(rs *schema.ResourceSchema, path string) bool {
	b, ok := rs.Blocks[path]
	if !ok {
		return false
	}
	if slices.ContainsFunc(b.Attributes, func(a schema.Attribute) bool { return a.Required || a.Optional }) {
		return true
	}
	return slices.ContainsFunc(b.ChildBlocks, func(c string) bool { return blockTreeHasConfigurable(rs, childBlockPath(path, c)) })
}

// sharedSectionResults renders the shared-section findings.
func (r *SchemaDocsRule) sharedSectionResults(ctx CheckContext, shared []sharedSection) []Result {
	var results []Result
	for _, sh := range shared {
		n := len(sh.served)
		for _, c := range sh.conflicts {
			head := fmt.Sprintf("section %q (line %d) in %s documents %d paths; ", sh.key, sh.block.HeadingLine, referenceName(sh.inAttrs), n)
			var what, cond string
			switch c.kind {
			case 1:
				what = fmt.Sprintf("%q must be listed for %q but doesn't exist at %q", c.field, displayPath(c.q), displayPath(c.p))
				cond = fmt.Sprintf("paths where %q exists", c.field)
			case 2:
				what = fmt.Sprintf("%q is %s at %q and %s at %q, so one section can't label it correctly", c.field, c.qVal, displayPath(c.q), c.pVal, displayPath(c.p))
				cond = fmt.Sprintf("paths where %q has the same label", c.field)
			case 3:
				what = fmt.Sprintf("%q is deprecated at %q but not at %q, so one section can't mark it correctly", c.field, displayPath(c.q), displayPath(c.p))
				cond = fmt.Sprintf("paths where %q has the same deprecation", c.field)
			case 4:
				what = fmt.Sprintf("%q differs below this level between %q and %q, so the section's %q bullet can't lead to the right %q for both", c.field, displayPath(c.q), displayPath(c.p), c.field, c.field)
				cond = fmt.Sprintf("paths whose %q blocks match at every depth", c.field)
			}
			// A label or deprecation conflict means the section's marker is
			// wrong for some path: an error, as under the fit rule and in
			// labels and deprecation. Existence and child-content conflicts
			// are already errors at each path; this finding only explains them.
			sev := SeverityWarning
			if c.kind == 2 || c.kind == 3 {
				sev = SeverityError
			}
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: sev, Line: sh.block.HeadingLine, Block: c.q,
				Message: head + what + fmt.Sprintf(". Qualifying means up to %d sections: give %q a heading that resolves only to %s, e.g. %q",
					n, displayPath(c.q), cond, doc.RenderHeading(r.pathTemplate(), c.q)),
			})
		}
	}
	return results
}

// missingPointer is appended to a missing-field finding when the section
// documenting p also serves paths where f doesn't exist: listing f there would
// make it wrong for them, so the fix is a qualified heading, not a bullet.
func missingPointer(rs *schema.ResourceSchema, idx map[*doc.DocBlock][]string, section *doc.DocBlock, p, f string) string {
	if section == nil {
		return ""
	}
	var without []string
	for _, q := range idx[section] {
		if q != p && !blockHasField(rs.Blocks[q], f) {
			without = append(without, q)
		}
	}
	if len(without) == 0 {
		return ""
	}
	return fmt.Sprintf("; the section documenting it (%q, line %d) also documents %s, where %q does not exist", section.Name, section.HeadingLine, pointerList(without), f)
}

// pointerList renders up to three paths, sorted, with a count of the rest.
func pointerList(paths []string) string {
	shown := make([]string, 0, 3)
	for _, p := range paths[:min(3, len(paths))] {
		shown = append(shown, strconv.Quote(displayPath(p)))
	}
	out := strings.Join(shown, ", ")
	if len(paths) > 3 {
		out += fmt.Sprintf(" (and %d more)", len(paths)-3)
	}
	return out
}

// checkUnresolvedSections reports parsed headings that no schema path
// resolves to, so their fields are compared against nothing
// (docs/rules/coverage-path-resolution.md §5). Excluded, to avoid reporting a
// heading twice: Argument Reference headings whose name is no schema block,
// which checkPhantomBlocks reports, and Attribute Reference headings naming an
// object-typed attribute, which checkPhantomBlocks deliberately allows.
func (r *SchemaDocsRule) checkUnresolvedSections(ctx CheckContext) []Result {
	rs := ctx.Schema
	if rs == nil {
		return nil
	}
	leaves := make(map[string]bool, len(rs.Blocks))
	for p := range rs.Blocks {
		if p != "" {
			leaves[leafName(p)] = true
		}
	}
	objectLeaves := nestedAttributeLeaves(rs)

	var results []Result
	for _, inAttrs := range []bool{false, true} {
		blocks := ctx.Doc.ArgumentBlocks
		if inAttrs {
			blocks = ctx.Doc.AttributeBlocks
		}
		served := make(map[*doc.DocBlock]bool)
		for p := range rs.Blocks {
			if b := resolveSection(rs, blocks, p); b != nil {
				served[b] = true
			}
		}
		for _, key := range slices.Sorted(maps.Keys(blocks)) {
			b := blocks[key]
			// A heading with no bullets hides no unchecked fields.
			if key == "" || b.Heading == "" || served[b] || (len(b.Attributes) == 0 && len(b.MalformedAttributes) == 0) {
				continue
			}
			leaf := leafName(key)
			if objectLeaves[leaf] || (!inAttrs && !leaves[leaf]) {
				continue
			}
			why := "no schema block has that name"
			if leaves[leaf] {
				why = fmt.Sprintf("every block named %q resolves to another heading", leaf)
				if strings.Contains(key, ".") && !keyMatchesSomePath(rs, key) {
					why = fmt.Sprintf("no block named %q sits under %q", leaf, key[:strings.LastIndex(key, ".")])
				}
			}
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning, Line: b.HeadingLine,
				Message: fmt.Sprintf("heading %q in %s documents no block (%s), so its fields aren't checked against the schema", b.Heading, referenceName(inAttrs), why),
			})
		}
	}
	return results
}

// keyMatchesSomePath reports whether key is a candidate heading key for any
// schema path.
func keyMatchesSomePath(rs *schema.ResourceSchema, key string) bool {
	for p := range rs.Blocks {
		if p != "" && slices.Contains(sectionKeyCandidates(p), key) {
			return true
		}
	}
	return false
}

// duplicated reports whether a section's key has more than one heading.
func duplicated(b *doc.DocBlock) bool {
	return b != nil && len(b.Occurrences) > 1
}

// checkDuplicateHeadings applies the fit rule to keys with several headings in
// one reference section (docs/rules/coverage-path-resolution.md §5). Position
// doesn't say which heading documents which path, so instead it asks whether
// any assignment makes every block exact. An error when some served path has
// no heading that fits it, or some heading fits no served path: then a block
// lacks its exact fields whichever way they're matched. Otherwise a warning,
// because a reader still can't tell from either heading which block it
// documents. There is no passing case.
func (r *SchemaDocsRule) checkDuplicateHeadings(ctx CheckContext) []Result {
	rs := ctx.Schema
	if rs == nil {
		return nil
	}
	var results []Result
	for _, inAttrs := range []bool{false, true} {
		blocks := ctx.Doc.ArgumentBlocks
		if inAttrs {
			blocks = ctx.Doc.AttributeBlocks
		}
		section := referenceName(inAttrs)
		for _, key := range slices.Sorted(maps.Keys(blocks)) {
			b := blocks[key]
			if !duplicated(b) {
				continue
			}
			var served []string
			for _, p := range slices.Sorted(maps.Keys(rs.Blocks)) {
				if p != "" && !slices.Contains(r.skipBlocks(), p) && resolveSection(rs, blocks, p) == b {
					served = append(served, p)
				}
			}
			if len(served) == 0 {
				continue // resolves to nothing: the unresolved-section warning's job
			}
			var lines []string
			for _, o := range b.Occurrences {
				lines = append(lines, strconv.Itoa(o.Line))
			}
			lineList := strings.Join(lines, ", ")

			// Only headings with bullets are fitted. A heading with none ("See
			// `ebs_config` above.") documents nothing; flagging its fields as
			// missing would name the wrong defect. It still counts as a
			// duplicate below.
			var occs []doc.Occurrence
			for _, o := range b.Occurrences {
				if len(o.Attributes) > 0 {
					occs = append(occs, o)
				}
			}

			// mis[i][j] lists how occurrence i fails to fit served path j.
			mis := make([][][]string, len(occs))
			for i, o := range occs {
				mis[i] = make([][]string, len(served))
				for j, p := range served {
					mis[i][j] = r.fitMismatches(rs, o, p, inAttrs)
				}
			}
			before := len(results)
			// reported holds each path error's differences, so a heading whose
			// closest path already carries the same differences isn't
			// reported again: the path's message names every heading line.
			reported := make(map[int]string)
			for j, p := range served {
				best := 0
				for i := range mis {
					if closer(mis[i][j], mis[best][j]) {
						best = i
					}
				}
				if len(occs) == 0 {
					break
				}
				if d := mis[best][j]; len(d) > 0 {
					reported[j] = strings.Join(d, "\x00")
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError, Block: p, Line: occs[best].Line,
						Message: fmt.Sprintf("none of the %d headings for %q in %s (lines %s) documents block %q exactly; the closest, at line %d: %s",
							len(b.Occurrences), key, section, lineList, displayPath(p), occs[best].Line, summarize(d)),
					})
				}
			}
			for i, o := range occs {
				best := 0
				for j := range served {
					if closer(mis[i][j], mis[i][best]) {
						best = j
					}
				}
				if d := mis[i][best]; len(d) > 0 {
					if prev, ok := reported[best]; ok && prev == strings.Join(d, "\x00") {
						continue
					}
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError, Block: served[best], Line: o.Line,
						Message: fmt.Sprintf("heading %q (line %d) in %s documents no block exactly; the closest is %q: %s",
							o.Heading, o.Line, section, displayPath(served[best]), summarize(d)),
					})
				}
			}
			if len(results) > before {
				continue
			}
			shown := served
			more := ""
			if len(shown) > 3 {
				more = fmt.Sprintf(" and %d more", len(shown)-3)
				shown = shown[:3]
			}
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning, Line: b.Occurrences[0].Line,
				Message: fmt.Sprintf("%d headings in %s normalize to %q (lines %s), so a reader can't tell which block each documents; give each the heading of the path it documents (%s%s)",
					len(b.Occurrences), section, key, lineList, strings.Join(shown, ", "), more),
			})
		}
	}
	return results
}

// closer reports whether difference list a is a nearer fit than b: fewer
// content differences (fields that don't exist or aren't listed) first, then
// fewer differences overall. A heading whose fields are right but whose label
// is wrong is nearer than one listing a field that isn't there.
func closer(a, b []string) bool {
	content := func(d []string) int {
		n := 0
		for _, s := range d {
			if strings.HasSuffix(s, "doesn't exist there") || strings.HasSuffix(s, "isn't listed") {
				n++
			}
		}
		return n
	}
	if ca, cb := content(a), content(b); ca != cb {
		return ca < cb
	}
	return len(a) < len(b)
}

// summarize joins up to three differences, with a count of the rest.
func summarize(d []string) string {
	if len(d) <= 3 {
		return strings.Join(d, "; ")
	}
	return fmt.Sprintf("%s; and %d more", strings.Join(d[:3], "; "), len(d)-3)
}

// fitMismatches lists the ways occurrence o fails to document path p exactly
// in one reference section: listed fields that don't exist at p, fields whose
// only home is that section and that aren't listed, and listed fields whose
// label or deprecation marker is wrong for p. None means o fits p. Uses
// coverage's own filters, so a difference only in a skipped or allowed field
// never counts.
func (r *SchemaDocsRule) fitMismatches(rs *schema.ResourceSchema, o doc.Occurrence, p string, inAttrs bool) []string {
	b := rs.Blocks[p]
	var n []string
	listed := make(map[string]bool, len(o.Attributes))
	for _, a := range o.Attributes {
		listed[a.Name] = true
		if slices.Contains(r.phantom(), a.Name) {
			continue
		}
		if !blockHasField(b, a.Name) {
			n = append(n, fmt.Sprintf("%q doesn't exist there", a.Name))
			continue
		}
		i := slices.IndexFunc(b.Attributes, func(sa schema.Attribute) bool { return sa.Name == a.Name })
		if i < 0 {
			continue // a child block bullet carries no label or marker to compare
		}
		sa := b.Attributes[i]
		if a.Deprecated != sa.Deprecated && !(r.IgnoreDeprecated && sa.Deprecated) {
			n = append(n, fmt.Sprintf("%q deprecation marker is wrong", a.Name))
		}
		if !inAttrs && !b.ConfigUnknown && (a.Required || a.Optional || a.ReadOnly) {
			// The one schema-correct label, as labelCorrectness defines it.
			wantOptional := sa.Optional && !sa.Required
			wantReadOnly := !sa.Required && !sa.Optional
			if a.Required != sa.Required || a.Optional != wantOptional || a.ReadOnly != wantReadOnly {
				want := "(Read-Only)"
				switch {
				case sa.Required:
					want = "(Required)"
				case wantOptional:
					want = "(Optional)"
				}
				n = append(n, fmt.Sprintf("%q should be %s", a.Name, want))
			}
		}
	}
	if b.ConfigUnknown {
		slices.Sort(n)
		return n
	}
	for _, sa := range b.Attributes {
		if listed[sa.Name] || slices.Contains(r.implicit(), sa.Name) || (r.IgnoreDeprecated && sa.Deprecated) {
			continue
		}
		pure := (sa.Required || sa.Optional) && !sa.Computed
		computedOnly := sa.Computed && !sa.Required && !sa.Optional
		if (!inAttrs && pure) || (inAttrs && computedOnly && !r.allowInlineReadOnly()) {
			n = append(n, fmt.Sprintf("%q isn't listed", sa.Name))
		}
	}
	if !inAttrs {
		for _, c := range b.ChildBlocks {
			if !listed[leafName(c)] && blockTreeHasPureConfigurable(rs, childBlockPath(p, c), make(map[string]bool)) {
				n = append(n, fmt.Sprintf("%q isn't listed", leafName(c)))
			}
		}
	}
	// Schema attribute order comes from map iteration; sort for stable output.
	slices.Sort(n)
	return n
}

// checkOrphans reports headings whose bullets belong to no section, so their
// fields are compared against nothing (docs/rules/coverage-path-resolution.md
// §5), and prose lists that stand in for a heading. When an unparseable
// heading names a schema block, the finding suggests a heading for it. The
// suggestion is advisory: an unparseable heading still documents nothing, so
// this doesn't loosen headings-only resolution.
func (r *SchemaDocsRule) checkOrphans(ctx CheckContext) []Result {
	var results []Result
	add := func(line int, msg string, names []string) {
		if key := suggestHeadingKey(ctx.Schema, names); key != "" {
			msg += fmt.Sprintf("; use a block heading, e.g. %q", doc.RenderHeading(r.pathTemplate(), key))
		}
		results = append(results, Result{Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning, Message: msg, Line: line})
	}
	for _, o := range ctx.Doc.Orphans {
		add(o.Line, fmt.Sprintf("heading %q in %s isn't a recognized block heading, so its bullets aren't checked against the schema", o.Text, referenceName(o.InAttributes)), headingNames(o.Text))
	}
	for _, ps := range proseSections(ctx.Schema, ctx.Doc) {
		msg := fmt.Sprintf("list introduced by prose (%q) in %s documents %q without a block heading; use a block heading, e.g. %q",
			truncate(ps.Text, 80), referenceName(ps.InAttributes), ps.key, doc.RenderHeading(r.pathTemplate(), ps.key))
		if ps.resumeLine > 0 {
			msg += fmt.Sprintf(". The list runs on into fields of the enclosing section from line %d; end it before them", ps.resumeLine)
		}
		results = append(results, Result{Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning, Line: ps.Line, Message: msg})
	}
	slices.SortFunc(results, func(a, b Result) int { return a.Line - b.Line })
	return results
}

func referenceName(inAttributes bool) string {
	if inAttributes {
		return "Attribute Reference"
	}
	return "Argument Reference"
}

// proseSection is a prose lead-in that stands in for a block heading, with
// the section key its list documents. Bullets that resume the enclosing
// section (resumed, from line resumeLine) stay there.
type proseSection struct {
	doc.ProseLeadIn
	key        string
	resumeLine int
}

// proseSections returns the prose lead-ins that introduce another block's list
// ("The `cloudwatch_logs` object takes the following arguments:"): the prose
// names a schema block in backticks, and the list follows the section's own
// bullets. The prose is a heading in all but syntax, so its list documents the
// named block, keyed as suggestHeadingKey would key a heading for it (§5).
// Prose naming nothing in the schema ("…the same arguments as
// `aws_instance`, with the addition of:") continues its section, as does prose
// before any list under a reference section heading (the byline's place) or a
// heading that resolves to a schema path: that prose introduces the heading's
// own list. Under an unparseable or unresolved heading it opens the block.
//
// Markdown gives a prose list no end: a bullet after a blank line continues
// it, and authors resume the enclosing section that way
// (aws_codepipeline_custom_action_type). So a trailing run of bullets that
// are fields of the enclosing section and not of the named block stays in the
// section, and the warning says where the list should end. Crediting them to
// the named block would report correct fields as nonexistent.
func proseSections(rs *schema.ResourceSchema, d *doc.Document) []proseSection {
	if rs == nil {
		return nil
	}
	var out []proseSection
	for _, pl := range d.ProseLeadIns {
		key := suggestHeadingKey(rs, backtickedNames(pl.Text))
		if len(pl.Bullets) == 0 || key == "" || (!pl.Orphaned && key == pl.Section) {
			continue
		}
		if pl.AfterHeading && !pl.Orphaned && (pl.Section == "" || keyMatchesSomePath(rs, pl.Section)) {
			continue
		}
		ps := proseSection{ProseLeadIn: pl, key: key}
		if !pl.Orphaned {
			n := len(pl.Bullets)
			for n > 0 && sectionField(rs, pl.Section, pl.Bullets[n-1].Name) && !sectionField(rs, key, pl.Bullets[n-1].Name) {
				n--
			}
			if n == 0 {
				continue // the whole list is the section's own
			}
			if n < len(pl.Bullets) {
				ps.resumeLine = pl.Bullets[n].Line
				ps.Bullets = pl.Bullets[:n]
			}
		}
		out = append(out, ps)
	}
	return out
}

// sectionField reports whether name is a field at some schema path the
// section key could document.
func sectionField(rs *schema.ResourceSchema, key, name string) bool {
	for p, b := range rs.Blocks {
		if (p == key || (p != "" && slices.Contains(sectionKeyCandidates(p), key))) && blockHasField(b, name) {
			return true
		}
	}
	return false
}

func blockHasField(b *schema.Block, name string) bool {
	for _, a := range b.Attributes {
		if a.Name == name {
			return true
		}
	}
	return slices.ContainsFunc(b.ChildBlocks, func(c string) bool { return leafName(c) == name })
}

// withProseSections returns d with each prose-introduced list (proseSections)
// moved from the section the parser credited it to into a section for the
// block the prose names, as one occurrence of that key, so every sub-check
// reads it as it would a list under a heading. d itself is not modified.
func withProseSections(rs *schema.ResourceSchema, d *doc.Document) *doc.Document {
	sections := proseSections(rs, d)
	if len(sections) == 0 {
		return d
	}
	c := *d
	c.ArgumentBlocks = maps.Clone(d.ArgumentBlocks)
	c.AttributeBlocks = maps.Clone(d.AttributeBlocks)
	// Blocks are cloned on first write so d's are never modified.
	cloned := make(map[*doc.DocBlock]bool)
	own := func(blocks map[string]*doc.DocBlock, key string) *doc.DocBlock {
		b := blocks[key]
		if b == nil {
			b = &doc.DocBlock{Name: key}
		} else if !cloned[b] {
			cb := *b
			cb.Attributes = slices.Clone(b.Attributes)
			cb.MalformedAttributes = slices.Clone(b.MalformedAttributes)
			cb.Occurrences = slices.Clone(b.Occurrences)
			for i := range cb.Occurrences {
				cb.Occurrences[i].Attributes = slices.Clone(cb.Occurrences[i].Attributes)
			}
			b = &cb
		}
		cloned[b] = true
		blocks[key] = b
		return b
	}
	for _, ps := range sections {
		blocks := c.ArgumentBlocks
		if ps.InAttributes {
			blocks = c.AttributeBlocks
		}
		lines := make(map[int]bool, len(ps.Bullets))
		for _, a := range ps.Bullets {
			lines[a.Line] = true
		}
		// The parser credits the bullets to the section and mirrors them into
		// every alias of a combined heading ("`a` and `b`"), so they're
		// removed from every section that holds them, by line.
		var malformed []doc.MalformedAttr
		moved := func(a doc.DocAttribute) bool { return lines[a.Line] }
		for _, key := range slices.Sorted(maps.Keys(blocks)) {
			b := blocks[key]
			if key == ps.key || (!slices.ContainsFunc(b.Attributes, moved) && !slices.ContainsFunc(b.MalformedAttributes, func(m doc.MalformedAttr) bool { return lines[m.Line] })) {
				continue
			}
			from := own(blocks, key)
			from.Attributes = slices.DeleteFunc(from.Attributes, moved)
			for i := range from.Occurrences {
				from.Occurrences[i].Attributes = slices.DeleteFunc(from.Occurrences[i].Attributes, moved)
			}
			from.MalformedAttributes = slices.DeleteFunc(from.MalformedAttributes, func(m doc.MalformedAttr) bool {
				if lines[m.Line] {
					if key == ps.Section {
						malformed = append(malformed, m)
					}
					return true
				}
				return false
			})
		}
		to := own(blocks, ps.key)
		if to.HeadingLine == 0 {
			to.HeadingLine = ps.Line
		}
		to.Attributes = append(to.Attributes, ps.Bullets...)
		to.MalformedAttributes = append(to.MalformedAttributes, malformed...)
		to.Occurrences = append(to.Occurrences, doc.Occurrence{Heading: ps.Text, Line: ps.Line, Attributes: slices.Clone(ps.Bullets)})

		// Sub-bullets captured under a moved bullet document the named
		// block's fields too, so they move with it (#88). A trailing run the
		// section keeps (resumeLine) keeps its sub-bullets.
		for _, rel := range slices.Sorted(maps.Keys(ps.Nested)) {
			var sub []doc.DocAttribute
			for _, a := range ps.Nested[rel] {
				if ps.resumeLine == 0 || a.Line < ps.resumeLine {
					sub = append(sub, a)
				}
			}
			if len(sub) == 0 {
				continue
			}
			if !ps.Orphaned {
				fromKey := joinPath(ps.Section, rel)
				subLines := make(map[int]bool, len(sub))
				for _, a := range sub {
					subLines[a.Line] = true
				}
				if blocks[fromKey] != nil {
					from := own(blocks, fromKey)
					from.Attributes = slices.DeleteFunc(from.Attributes, func(a doc.DocAttribute) bool { return subLines[a.Line] })
					if len(from.Attributes) == 0 && from.Heading == "" && len(from.Occurrences) == 0 {
						delete(blocks, fromKey)
					}
				}
			}
			to := own(blocks, joinPath(ps.key, rel))
			to.Attributes = append(to.Attributes, sub...)
		}
	}
	return &c
}

// joinPath joins a section key and a dot-path relative to it; the root key
// is empty.
func joinPath(key, rel string) string {
	if key == "" {
		return rel
	}
	return key + "." + rel
}

// pathTemplate is the first preferred heading style that takes a path, as
// checkHeadings uses for its suggestions.
func (r *SchemaDocsRule) pathTemplate() string {
	for _, t := range r.Preferred {
		if strings.Contains(t, "{Path}") {
			return t
		}
	}
	return "`{Path}` Block"
}

var headingNameRe = regexp.MustCompile(`[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*`)

// headingNames returns the lower-case identifiers in a heading, backticked or
// not: "source: auth" -> [source auth].
func headingNames(text string) []string {
	return headingNameRe.FindAllString(strings.ToLower(text), -1)
}

// backtickedNames returns the identifiers quoted in backticks, with array
// indexers stripped.
func backtickedNames(text string) []string {
	var out []string
	for i, part := range strings.Split(text, "`") {
		if i%2 == 0 {
			continue
		}
		if name, ok := doc.NormalizeDotPath(part); ok {
			out = append(out, name)
		}
	}
	return out
}

// suggestHeadingKey picks the heading key for the block that names refer to,
// or "" when no name is a schema block. Candidates are schema paths ending in
// one of the names; the ones containing the most of the other names as
// segments win ("source: auth" prefers source.auth over
// secondary_sources.auth). A single winner is suggested by leaf when that
// leaf is unique in the schema, else by full path. Several winners sharing a
// leaf are suggested by that leaf: one shared section, which coverage then
// checks path by path.
func suggestHeadingKey(rs *schema.ResourceSchema, names []string) string {
	if rs == nil || len(names) == 0 {
		return ""
	}
	best, bestScore := []string(nil), -1
	for _, p := range slices.Sorted(maps.Keys(rs.Blocks)) {
		if p == "" {
			continue
		}
		segs := strings.Split(p, ".")
		for _, n := range names {
			if p != n && !strings.HasSuffix(p, "."+n) {
				continue
			}
			score := 0
			for _, other := range names {
				if other != n && slices.Contains(segs, other) {
					score++
				}
			}
			switch {
			case score > bestScore:
				best, bestScore = []string{p}, score
			case score == bestScore && !slices.Contains(best, p):
				best = append(best, p)
			}
		}
	}
	if len(best) == 0 {
		return ""
	}
	leaf := leafName(best[0])
	for _, p := range best[1:] {
		if leafName(p) != leaf {
			return ""
		}
	}
	if len(best) > 1 {
		return leaf
	}
	if _, unique := uniqueSchemaPathForLeaf(rs, leaf); unique {
		return leaf
	}
	return best[0]
}

// undocumented is the set of schema paths with configurable fields and no
// section in either reference section.
type undocumented map[string]bool

func (r *SchemaDocsRule) undocumentedBlocks(ctx CheckContext) undocumented {
	m := make(undocumented)
	for p, b := range ctx.Schema.Blocks {
		if p == "" || slices.Contains(r.skipBlocks(), p) || !hasConfigurableAttributes(b) {
			continue
		}
		if len(resolveSections(ctx.Schema, ctx.Doc, p)) == 0 {
			m[p] = true
		}
	}
	return m
}

// message returns the missing-block finding for p, or false when p is beneath
// another undocumented block. Only the shallowest undocumented block in a
// subtree is reported: its descendants can't be reached from the docs, so the
// real defect is the missing ancestor section, and once it exists the next run
// reports them (docs/rules/coverage-path-resolution.md §6). Ancestors without
// configurable fields are never reported, so they never absorb descendants.
// The message carries both counts so no work is hidden.
func (u undocumented) message(p string) (string, bool) {
	if !u[p] {
		return "", false
	}
	for a := p; strings.Contains(a, "."); {
		a = a[:strings.LastIndex(a, ".")]
		if u[a] {
			return "", false
		}
	}
	leaf := leafName(p)
	beneath, sameName := 0, 0
	for q := range u {
		if strings.HasPrefix(q, p+".") {
			beneath++
		}
		if q != p && leafName(q) == leaf {
			sameName++
		}
	}
	msg := fmt.Sprintf("block %q is not documented", displayPath(p))
	var notes []string
	if beneath > 0 {
		notes = append(notes, fmt.Sprintf("%d %s beneath it %s also undocumented", beneath, plural(beneath, "path", "paths"), plural(beneath, "is", "are")))
	}
	if sameName > 0 {
		notes = append(notes, fmt.Sprintf("%d other undocumented %s %s the name %q", sameName, plural(sameName, "path", "paths"), plural(sameName, "shares", "share"), leaf))
	}
	if len(notes) > 0 {
		msg += " (" + strings.Join(notes, "; ") + ")"
	}
	return msg, true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// checkPhantomBlocks reports doc blocks in the Argument Reference section
// whose name has no counterpart in the schema. This catches stray
// subheadings that get parsed as block names — e.g. a `### \`rules\“
// heading followed by a `#### Arguments` subheading creates a phantom
// "arguments" block in the doc model. The schema has no such block; the
// H4 should not be there.
//
// Limited to ArgumentBlocks because block headings in the Attribute
// Reference section commonly document the nested structure of computed
// attributes (e.g. `### Endpoint`, `### master_user_secret`) that have
// no Block representation in the schema. Flagging those would produce
// false positives on standard provider doc patterns.
//
// A doc block name matches the schema if any schema block path has the
// same leaf name. (resolveSection's candidate keys always end in the leaf,
// so a heading whose leaf matches no schema block can't document anything.)
func (r *SchemaDocsRule) checkPhantomBlocks(ctx CheckContext) []Result {
	if ctx.Schema == nil {
		return nil
	}

	schemaLeaves := make(map[string]bool, len(ctx.Schema.Blocks))
	for path := range ctx.Schema.Blocks {
		if path == "" {
			continue
		}
		schemaLeaves[leafName(path)] = true
	}
	// Object-typed nested attributes (list/set/single object attributes,
	// e.g. list(object({...}))) are documented with block-style headings
	// (### `application_settings`) even though they are attributes, not
	// blocks. They live as Attribute.Children in the schema model rather
	// than as rs.Blocks entries, so add their leaf names too — otherwise
	// a correctly-documented nested attribute is flagged as a phantom
	// block.
	for leaf := range nestedAttributeLeaves(ctx.Schema) {
		schemaLeaves[leaf] = true
	}

	reported := make(map[string]bool)
	var results []Result
	for blockName, block := range ctx.Doc.ArgumentBlocks {
		if blockName == "" || block.Heading == "" {
			continue
		}
		leaf := leafName(blockName)
		if schemaLeaves[leaf] {
			continue
		}
		// Dedupe by heading text — combined headings ("X and Y") create
		// multiple doc blocks but should produce one finding per heading.
		if reported[block.Heading] {
			continue
		}
		reported[block.Heading] = true
		results = append(results, Result{
			Rule:     r.Name(),
			Resource: ctx.Resource,
			Severity: SeverityError,
			Message:  fmt.Sprintf("block heading %q in Argument Reference has no matching block in schema", block.Heading),
			Block:    blockName,
		})
	}
	return results
}

// nestedAttributeLeaves collects the leaf names of object-typed nested
// attributes (those with children) at every depth of the schema. Such
// attributes — encoded as list(object({...})), set(object({...})), or a
// bare object, whether SDK cty types or Framework nested types — are
// conventionally documented with block-style headings and a list of
// their sub-attributes, exactly like a nested block. They are stored as
// Attribute.Children rather than as rs.Blocks entries, so callers that
// only consult rs.Blocks would otherwise treat these legitimate
// headings as phantom blocks.
func nestedAttributeLeaves(rs *schema.ResourceSchema) map[string]bool {
	leaves := make(map[string]bool)
	var walk func(attrs []schema.Attribute)
	walk = func(attrs []schema.Attribute) {
		for _, a := range attrs {
			if len(a.Children) == 0 {
				continue
			}
			leaves[a.Name] = true
			walk(a.Children)
		}
	}
	for _, b := range rs.Blocks {
		if b == nil {
			continue
		}
		walk(b.Attributes)
	}
	return leaves
}

// checkAttributeCoverage ensures every Read-Only (computed-only) schema
// attribute, at every depth of nesting, is documented somewhere reachable.
//
// The default expectation is that Read-Only attributes appear in
// ## Attribute Reference (under the appropriate block heading for nested
// ones). When AllowInlineReadOnly is true, the rule additionally accepts
// Read-Only attributes documented inline in ## Argument Reference with a
// (Read-Only) label, alongside Required and Optional siblings — the
// taxonomy used by tfplugindocs.
//
// This is the per-attribute presence rule. Misplacement (Read-Only in
// Argument Reference when the toggle is off) is handled separately by
// checkComputedMisplacement.
func (r *SchemaDocsRule) checkAttributeCoverage(ctx CheckContext) []Result {
	if ctx.Schema == nil {
		return nil
	}

	allowInline := r.allowInlineReadOnly()
	var results []Result
	reported := make(map[string]bool)

	for blockPath, schemaBlock := range ctx.Schema.Blocks {
		if slices.Contains(r.skipBlocks(), blockPath) {
			continue
		}
		if schemaBlock == nil {
			continue
		}

		argDoc := resolveSection(ctx.Schema, ctx.Doc.ArgumentBlocks, blockPath)
		attrDoc := resolveSection(ctx.Schema, ctx.Doc.AttributeBlocks, blockPath)

		for _, attr := range schemaBlock.Attributes {
			// Only Read-Only (computed-only) attributes are covered here.
			// Required / Optional coverage is handled in checkCoverage.
			if !(attr.Computed && !attr.Optional && !attr.Required) {
				continue
			}
			if slices.Contains(r.implicit(), attr.Name) {
				continue
			}
			if r.IgnoreDeprecated && attr.Deprecated {
				continue
			}

			if duplicated(attrDoc) && !allowInline && !schemaBlock.ConfigUnknown {
				continue // judged by the fit rule (checkDuplicateHeadings)
			}
			inAttrs := docBlockHasAttr(attrDoc, attr.Name)
			inArgs := docBlockHasAttr(argDoc, attr.Name)

			// Documented in Attribute Reference always satisfies the rule.
			// Documented inline in Argument Reference satisfies the rule
			// only when AllowInlineReadOnly is true.
			//
			// For ConfigUnknown blocks — object-typed attributes whose
			// configurable parent leaves per-field Required/Optional/Computed
			// unknowable — documentation in either section satisfies the rule,
			// since the schema gives us no basis to demand a specific one.
			if schemaBlock.ConfigUnknown {
				if inAttrs || inArgs {
					continue
				}
			} else if inAttrs || (allowInline && inArgs) {
				continue
			}

			key := blockPath + "." + attr.Name
			if reported[key] {
				continue
			}
			reported[key] = true

			var msg string
			switch {
			case schemaBlock.ConfigUnknown:
				// Section can't be dictated; report neutrally as undocumented.
				msg = fmt.Sprintf("attribute %q in block %q is not documented", attr.Name, displayPath(blockPath))
			case blockPath == "":
				msg = fmt.Sprintf("Read-Only attribute %q should be documented in Attribute Reference section", attr.Name)
			default:
				msg = fmt.Sprintf("Read-Only attribute %q in block %q should be documented in Attribute Reference section", attr.Name, displayPath(blockPath))
			}
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: ctx.Resource,
				Severity: SeverityError,
				Message:  msg,
				Block:    blockPath,
			})
		}
	}
	return results
}

func docBlockHasAttr(b *doc.DocBlock, name string) bool {
	if b == nil {
		return false
	}
	for _, a := range b.Attributes {
		if a.Name == name {
			return true
		}
	}
	return false
}

func (r *SchemaDocsRule) checkComputedMisplacement(ctx CheckContext, schemaBlock *schema.Block, argBlock *doc.DocBlock, attrBlock *doc.DocBlock) []Result {
	var results []Result
	documented := make(map[string]bool, len(argBlock.Attributes))
	docLines := make(map[string]int, len(argBlock.Attributes))
	for _, attr := range argBlock.Attributes {
		documented[attr.Name] = true
		docLines[attr.Name] = attr.Line
	}

	// Build set of attrs in the attribute section to avoid false positives
	// when broad heading templates cause attribute-section items to also
	// appear in ArgumentBlocks.
	inAttrSection := make(map[string]bool)
	if attrBlock != nil {
		for _, attr := range attrBlock.Attributes {
			inAttrSection[attr.Name] = true
		}
	}

	for _, attr := range schemaBlock.Attributes {
		if attr.Computed && !attr.Optional && !attr.Required {
			if slices.Contains(r.implicit(), attr.Name) {
				continue
			}
			if documented[attr.Name] && !inAttrSection[attr.Name] {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("computed-only attribute %q should not appear in Argument Reference section", attr.Name),
					Line:    docLines[attr.Name],
				})
			}
		}
	}
	return results
}

func (r *SchemaDocsRule) shouldSkipAttribute(attr schema.Attribute) bool {
	if slices.Contains(r.implicit(), attr.Name) {
		return true
	}
	if r.IgnoreDeprecated && attr.Deprecated {
		return true
	}
	if attr.Computed && !attr.Optional && !attr.Required {
		return true
	}
	return false
}

// --- Ordering ---

func (r *SchemaDocsRule) checkOrdering(ctx CheckContext) []Result {
	var results []Result
	for blockPath, block := range ctx.Doc.ArgumentBlocks {
		results = append(results, checkBlockOrdering(ctx.Resource, r.Name(), "argument", blockPath, block)...)
	}
	for blockPath, block := range ctx.Doc.AttributeBlocks {
		results = append(results, checkBlockOrdering(ctx.Resource, r.Name(), "attribute", blockPath, block)...)
	}
	return results
}

// --- Description style ---

func (r *SchemaDocsRule) checkDescriptions(ctx CheckContext) []Result {
	seen := make(map[bulletKey]bool)
	var results []Result
	results = append(results, checkDescriptionBlocks(ctx.Resource, r.Name(), r.prefixes(), ctx.Doc.ArgumentBlocks, seen)...)
	results = append(results, checkDescriptionBlocks(ctx.Resource, r.Name(), r.prefixes(), ctx.Doc.AttributeBlocks, seen)...)

	// Bullets that belong to no section still get the description check: it
	// doesn't depend on which block they document. The finding names the
	// heading instead of a block.
	check := func(attrs []doc.DocAttribute, where string) {
		for _, attr := range attrs {
			for _, prefix := range r.prefixes() {
				if attr.Description != "" && strings.HasPrefix(attr.Description, prefix) {
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
						Message: fmt.Sprintf("attribute %q description should not start with %q (%s)", attr.Name, strings.TrimSpace(prefix), where),
						Line:    attr.Line,
					})
					break
				}
			}
		}
	}
	for _, o := range ctx.Doc.Orphans {
		check(o.Bullets, fmt.Sprintf("under heading %q", o.Text))
	}
	return results
}

// bulletKey identifies one bullet. Each bullet is one defect: two bullets
// with the same name, in either reference section or under the same key, are
// two. The parser credits one bullet to several sections (the aliases of a
// combined heading, or both sections under broad heading templates), so the
// key is the bullet's line, never its section or name alone (#86).
type bulletKey struct {
	line int
	name string
}

// checkDescriptionBlocks reports each bullet once, under the first section
// in sorted order that holds it.
func checkDescriptionBlocks(resource, ruleName string, prefixes []string, blocks map[string]*doc.DocBlock, seen map[bulletKey]bool) []Result {
	var results []Result
	for _, blockName := range slices.Sorted(maps.Keys(blocks)) {
		for _, attr := range blocks[blockName].Attributes {
			if attr.Description == "" {
				continue
			}
			key := bulletKey{attr.Line, attr.Name}
			if seen[key] {
				continue
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(attr.Description, prefix) {
					seen[key] = true
					results = append(results, Result{
						Rule: ruleName, Resource: resource, Severity: SeverityError,
						Message: fmt.Sprintf("attribute %q description should not start with %q (block %q)", attr.Name, strings.TrimSpace(prefix), displayPath(blockName)),
						Block:   blockName,
						Line:    attr.Line,
					})
					break
				}
			}
		}
	}
	return results
}

// --- Heading style ---

func (r *SchemaDocsRule) checkHeadings(ctx CheckContext) []Result {
	rs := ctx.Schema
	if rs == nil || len(r.Preferred) == 0 {
		return nil
	}

	schemaLeaves := make(map[string]bool)
	for path := range rs.Blocks {
		schemaLeaves[leafName(path)] = true
	}

	var results []Result
	for _, blocks := range []map[string]*doc.DocBlock{ctx.Doc.ArgumentBlocks, ctx.Doc.AttributeBlocks} {
		for _, block := range blocks {
			if block.Name == "" || block.Heading == "" {
				continue
			}
			// block.Name may be a leaf ("match") or a full dot-path
			// ("spec.grpc_route.match"). Look up by leaf so path-keyed
			// blocks still participate in the preferred-style check.
			blockLeaf := leafName(block.Name)
			if !schemaLeaves[blockLeaf] {
				continue
			}

			// Preferred-style check: does the heading match one of the
			// preferred templates? Whether a heading serves several blocks
			// is coverage's shared-section finding (#77).
			if r.Preferred.Match(block.Heading) != "" {
				continue
			}

			// Suggestion strategy depends on whether the block is
			// already path-keyed:
			//
			//  - path-keyed (block.Name contains a dot): pick the first
			//    {Path} template so the suggestion preserves the full
			//    dot-path disambiguator. Falling back to a leaf-only
			//    template here would reintroduce the ambiguity this
			//    rule is designed to catch.
			//
			//  - leaf-keyed: pick the first non-{Parent}, non-{Path}
			//    template since a single segment can't fill {Path} in
			//    a useful way and {Parent} requires words we don't
			//    have.
			var suggested string
			if strings.Contains(block.Name, ".") {
				for _, tmpl := range r.Preferred {
					if strings.Contains(tmpl, "{Path}") {
						suggested = doc.RenderHeading(tmpl, block.Name)
						break
					}
				}
				if suggested == "" {
					suggested = doc.RenderHeading("`{Path}` Block", block.Name)
				}
			} else {
				for _, tmpl := range r.Preferred {
					if !strings.Contains(tmpl, "{Parent}") && !strings.Contains(tmpl, "{Path}") {
						suggested = doc.RenderHeading(tmpl, blockLeaf)
						break
					}
				}
				if suggested == "" {
					suggested = doc.RenderHeading("`{Block}` Block", blockLeaf)
				}
			}
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Message: fmt.Sprintf("block %q heading %q should be %q", block.Name, block.Heading, suggested),
				Block:   block.Name,
			})
		}
	}
	return results
}

// --- Format (raw-line checks) ---

func (r *SchemaDocsRule) checkFormat(ctx CheckContext) []Result {
	source := ctx.Doc.Source()
	if len(source) == 0 {
		return nil
	}

	noCode := enabled(r.NoCodeBlocks)
	singleLine := enabled(r.SingleLineAttrs)
	uninterrupted := enabled(r.UninterruptedLists)

	var results []Result
	var inSection bool
	var inAttributes bool
	var inCodeBlock bool
	var inList bool
	var prevWasAttr bool
	// attrStack tracks attribute names at each indentation level for nesting validation.
	// Index 0 = top-level (0 spaces), 1 = first indent (4 spaces), etc.
	var attrStack []string
	scanner := bufio.NewScanner(bytes.NewReader(source))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.HasPrefix(line, "## Argument Reference") || strings.HasPrefix(line, "## Attribute Reference") {
			inSection = true
			inAttributes = strings.HasPrefix(line, "## Attribute")
			inList = false
			prevWasAttr = false
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") {
			inSection = false
			inCodeBlock = false
			inList = false
		}
		if !inSection {
			continue
		}

		if strings.HasPrefix(line, "```") {
			if noCode && !inCodeBlock {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
					Message: fmt.Sprintf("code block in argument/attribute section (line %d)", lineNum),
				})
			}
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		isAttrLine := strings.HasPrefix(line, "* `")
		isHeading := strings.HasPrefix(line, "#")
		isBlank := line == ""

		if singleLine && prevWasAttr && !isAttrLine && !isHeading && !isBlank && strings.HasPrefix(line, "  ") {
			trimmed := strings.TrimLeft(line, " ")
			isIndentedAttr := strings.HasPrefix(trimmed, "* `")
			if isIndentedAttr {
				if inAttributes && enabled(r.AllowAttributeIndentation) {
					indent := len(line) - len(trimmed)
					level := indent / 4 // 4 spaces per level
					name := extractAttrName(trimmed)
					// Update stack for this level.
					if level < len(attrStack) {
						attrStack = attrStack[:level]
					}
					attrStack = append(attrStack, name)
					// Validate against schema if available.
					if ctx.Schema != nil && level >= 1 {
						results = append(results, r.validateIndentedAttr(ctx, attrStack, lineNum)...)
					}
					continue
				}
				section := "Argument Reference"
				if inAttributes {
					section = "Attribute Reference"
				}
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("indented sub-attribute in %s (line %d); use a subsection heading instead", section, lineNum),
				})
			} else {
				results = append(results, Result{
					Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
					Message: fmt.Sprintf("multi-line attribute description (line %d); each attribute should be on one line", lineNum),
				})
			}
		}

		if uninterrupted && inList && !isAttrLine && !isHeading && !isBlank && !strings.HasPrefix(line, "  ") && !isListProse(line) {
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Message: fmt.Sprintf("attribute list interrupted (line %d): %q", lineNum, truncate(line, 60)),
			})
			inList = false
		}

		if isAttrLine {
			inList = true
			name := extractAttrName(line)
			attrStack = []string{name}
		}
		if isHeading {
			inList = false
			attrStack = nil
		}
		prevWasAttr = isAttrLine || (prevWasAttr && strings.HasPrefix(line, "  ") && strings.HasPrefix(strings.TrimLeft(line, " "), "* `"))
	}

	return results
}

// extractAttrName pulls the backticked name from a list item like "* `name` - ...".
func extractAttrName(line string) string {
	after, ok := strings.CutPrefix(line, "* `")
	if !ok {
		return ""
	}
	if i := strings.IndexByte(after, '`'); i > 0 {
		return after[:i]
	}
	return ""
}

// validateIndentedAttr checks that an indented sub-attribute exists in the schema
// at the correct nesting level. attrStack contains the attribute name chain from
// root to the current indented item.
func (r *SchemaDocsRule) validateIndentedAttr(ctx CheckContext, attrStack []string, lineNum int) []Result {
	if len(attrStack) < 2 {
		return nil
	}

	// Walk the schema attribute tree following the stack.
	// attrStack[0] is the root attribute, attrStack[1] is its child, etc.
	rootBlock := ctx.Schema.Blocks[""]
	if rootBlock == nil {
		return nil
	}

	// Find the root attribute.
	var children []schema.Attribute
	for _, a := range rootBlock.Attributes {
		if a.Name == attrStack[0] {
			children = a.Children
			break
		}
	}

	// Walk intermediate levels.
	for i := 1; i < len(attrStack)-1; i++ {
		found := false
		for _, a := range children {
			if a.Name == attrStack[i] {
				children = a.Children
				found = true
				break
			}
		}
		if !found {
			return nil // can't validate deeper if intermediate is unknown
		}
	}

	// Check the leaf (last element in stack).
	leaf := attrStack[len(attrStack)-1]

	if len(children) == 0 {
		// Parent has no known children in schema — the indentation is invalid.
		return []Result{{
			Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
			Message: fmt.Sprintf("indented attribute %q (line %d) under %q but schema has no nested attributes there", leaf, lineNum, attrStack[len(attrStack)-2]),
		}}
	}

	for _, a := range children {
		if a.Name == leaf {
			return nil // valid
		}
	}

	return []Result{{
		Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
		Message: fmt.Sprintf("indented attribute %q (line %d) not found in schema under %q", leaf, lineNum, attrStack[len(attrStack)-2]),
	}}
}

// --- Labels ---

func (r *SchemaDocsRule) checkLabels(ctx CheckContext, idx map[*doc.DocBlock][]string) []Result {
	var results []Result

	// Build set of attrs in attribute section to avoid false positives from
	// template bleed (broad heading templates can cause attribute-section
	// items to also appear in ArgumentBlocks).
	attrSectionNames := make(map[string]map[string]bool)
	for blockName, block := range ctx.Doc.AttributeBlocks {
		names := make(map[string]bool, len(block.Attributes))
		for _, attr := range block.Attributes {
			names[attr.Name] = true
		}
		attrSectionNames[blockName] = names
	}

	// A label is right only when it is both present and correct. Arguments must
	// carry (Required), (Optional), or (Read-Only); and when a (Required) or
	// (Optional) label is present, its value must match the schema (see
	// labelCorrectness). Presence and correctness are one check: "is the label
	// right?" — not two, because the schema_docs `ignore_targets` scope is shared
	// across sub-checks, so a separate correctness toggle would only add a global
	// on/off, never per-target granularity.
	allowReadOnly := r.allowInlineReadOnly()
	for blockName, block := range ctx.Doc.ArgumentBlocks {
		for _, attr := range block.Attributes {
			if attr.Required || attr.Optional {
				// A present (Required)/(Optional) label is schema-checkable
				// regardless of template bleed, and correctness runs BEFORE the
				// bleed guard below on purpose: a bleed artifact is an *unlabeled*
				// attribute-section bullet mirrored into ArgumentBlocks, so a
				// labeled bullet is a genuine argument. Guarding on a same-named
				// entry in the attribute section here would let an incorrect label
				// (e.g. (Required) on an Optional+Computed field listed in both
				// sections) slip through unreported (issue #68 review).
				if res := r.labelCorrectness(ctx, idx, blockName, attr); res != nil {
					results = append(results, *res)
				}
				continue
			}
			if attr.ReadOnly && allowReadOnly {
				// Inline Read-Only is permitted here, so the label is present.
				// Still verify it names a genuinely read-only attribute: a
				// (Read-Only) label on a configurable (Required/Optional) field is
				// wrong and must name the real requiredness (Gap A).
				if res := r.labelCorrectness(ctx, idx, blockName, attr); res != nil {
					results = append(results, *res)
				}
				continue
			}
			// Strict mode: (Read-Only) isn't an Argument Reference label. On a
			// computed-only field the fix is a move, not a label, so the
			// missing-label warning below would name the wrong fix.
			if paths, ok := r.labelPaths(ctx, idx, blockName); attr.ReadOnly && ok && r.servedLabel(ctx.Schema, paths, attr.Name) == "(Read-Only)" {
				if res := r.labelCorrectness(ctx, idx, blockName, attr); res != nil {
					results = append(results, *res)
				}
				continue
			}
			// Skip if this attr is also in the attribute section (template bleed:
			// broad heading templates can mirror an *unlabeled* attribute-section
			// item into ArgumentBlocks). This guard is scoped to the missing-label
			// path only — a labeled argument is handled above and must not be
			// silenced by a same-named attribute-section entry.
			if ns, ok := attrSectionNames[blockName]; ok && ns[attr.Name] {
				continue
			}
			label := "(Required) or (Optional)"
			if allowReadOnly {
				label = "(Required), (Optional), or (Read-Only)"
			}
			results = append(results, Result{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Message: fmt.Sprintf("argument %q in block %q is missing %s label", attr.Name, displayPath(blockName), label),
				Block:   blockName,
				Line:    attr.Line,
			})
		}
	}

	// Attribute Reference: flag labeled configurable arguments as misplaced
	// (move them to Argument Reference) instead of pushing the author to strip
	// accurate labels. Detection is per attribute; the message collapses to a
	// single subsection move when an entire block is configurable. See
	// docs/rules/argument-attribute-misplacement.md.
	results = append(results, r.attributeMisplacementFindings(ctx)...)

	return results
}

// labelCorrectness verifies that a documented argument's label matches the
// schema. For (Required)/(Optional) the invariant is single-valued: the label
// must be (Required) when the schema attribute is Required, (Optional) when it
// is configurable but not required (pure Optional or Optional+Computed), and
// (Read-Only) when it is computed-only. Only (Required) is wrong for an
// Optional+Computed field. A (Read-Only) label is valid only for a genuinely
// read-only attribute; on a configurable field it is wrong and must name the
// real requiredness (Gap A). A computed-only field mislabeled (Required)/
// (Optional) is a finding only under allow_inline_read_only = true, where inline
// documentation is permitted and (Read-Only) is the right label; in strict mode
// the field must move to Attribute Reference — reported by
// checkComputedMisplacement when the coverage sub-check is enabled (labels defers
// to avoid a double finding) and by labels itself when coverage is disabled.
//
// The section is checked against every path it serves (idx, the #77
// resolver), so a shared or partly qualified section is checked like any
// other (#80). It returns nil (no finding) whenever the label can't be settled
// so a finding never fires on a guess:
//   - the section serves no path (skip_blocks paths are never served),
//   - the field is a scalar attribute at none of the served paths whose
//     labels are known (a ConfigUnknown block's per-field flags aren't),
//   - the served paths disagree about the label: that's the shared-section
//     finding's to report once, since per-path findings would contradict
//     each other, or
//   - the key has several headings and coverage is on: the fit rule compares
//     each heading's labels against the paths it could document.
//
// Label additions such as "Forces new resource" or "Deprecated" do not matter:
// the parser sets attr.Required/attr.Optional from the leading token, so the
// booleans compared here are already correct regardless of trailing traits.
func (r *SchemaDocsRule) labelCorrectness(ctx CheckContext, idx map[*doc.DocBlock][]string, blockName string, attr doc.DocAttribute) *Result {
	paths, ok := r.labelPaths(ctx, idx, blockName)
	if !ok {
		return nil
	}
	want := r.servedLabel(ctx.Schema, paths, attr.Name)
	if want == "" {
		return nil
	}
	state := map[string]string{"(Required)": "required", "(Optional)": "optional", "(Read-Only)": "read-only"}[want]

	// The documented label(s). A well-formed argument bullet carries exactly one
	// category label; the parser sets a boolean per recognized trait, so a
	// contradictory bullet like "(Read-Only, Optional)" sets several. Build have
	// from every category present so a contradictory label can never coincide
	// with the single-valued want and slip through.
	var docLabels []string
	if attr.Required {
		docLabels = append(docLabels, "(Required)")
	}
	if attr.Optional {
		docLabels = append(docLabels, "(Optional)")
	}
	if attr.ReadOnly {
		docLabels = append(docLabels, "(Read-Only)")
	}
	if len(docLabels) == 0 {
		return nil // unlabeled — not reached from the argument loop
	}
	have := strings.Join(docLabels, ", ")
	// A computed-only attribute in Argument Reference, under any label, needs
	// care:
	//   - permissive mode (allow_inline_read_only): inline documentation is
	//     allowed and (Read-Only) is the correct label — reported below with
	//     "use (Read-Only)".
	//   - strict mode: the field does not belong in Argument Reference at all,
	//     even labeled (Read-Only). Coverage reports that move
	//     (checkComputedMisplacement at the root, Read-Only coverage below it),
	//     but only where Attribute Reference doesn't document the field, and
	//     only when coverage is enabled. Labels defers exactly then, avoiding a
	//     double finding, and otherwise reports it itself.
	if want == "(Read-Only)" && !r.allowInlineReadOnly() {
		inAttrs := !slices.ContainsFunc(paths, func(p string) bool {
			if b := ctx.Schema.Blocks[p]; b == nil || b.ConfigUnknown {
				return false
			} else if _, ok := attrAt(b, attr.Name); !ok {
				return false
			}
			s := resolveSection(ctx.Schema, ctx.Doc.AttributeBlocks, p)
			return s == nil || !slices.ContainsFunc(s.Attributes, func(a doc.DocAttribute) bool { return a.Name == attr.Name })
		})
		if enabled(r.Coverage) && !inAttrs {
			return nil // checkComputedMisplacement or Read-Only coverage reports the move
		}
		fix := "move it to Attribute Reference and remove the label"
		if inAttrs {
			fix = "Attribute Reference already documents it, so remove it from Argument Reference"
		}
		msg := fmt.Sprintf("argument %q is labeled %s but is computed-only in the schema; %s", attr.Name, have, fix)
		if blockName != "" {
			msg = fmt.Sprintf("argument %q in block %q is labeled %s but is computed-only in the schema; %s", attr.Name, displayPath(blockName), have, fix)
		}
		return &Result{
			Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
			Message: msg,
			Block:   blockName,
			Line:    attr.Line,
		}
	}

	if have == want {
		return nil // exactly one category label, and it matches the schema
	}

	msg := fmt.Sprintf("argument %q is labeled %s but is %s in the schema; use %s", attr.Name, have, state, want)
	if blockName != "" {
		msg = fmt.Sprintf("argument %q in block %q is labeled %s but is %s in the schema; use %s", attr.Name, displayPath(blockName), have, state, want)
	}
	return &Result{
		Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
		Message: msg,
		Block:   blockName,
		Line:    attr.Line,
	}
}

// labelPaths returns the schema paths an Argument Reference section's labels
// are checked against, or false when labels leaves the section to the fit
// rule (a duplicated key with coverage on) or there's no schema.
func (r *SchemaDocsRule) labelPaths(ctx CheckContext, idx map[*doc.DocBlock][]string, blockName string) ([]string, bool) {
	if ctx.Schema == nil {
		return nil, false
	}
	if blockName == "" {
		return []string{""}, true
	}
	b := ctx.Doc.ArgumentBlocks[blockName]
	if duplicated(b) && enabled(r.Coverage) {
		return nil, false
	}
	return idx[b], true
}

// servedLabel returns the one schema-correct label for field f across paths,
// or "" when it can't be settled: f is a scalar with known labels at none of
// them, or they disagree. A path where coverage ignores f (deprecated, under
// ignore_deprecated) counts only when no other path does, as in the
// shared-section comparison.
func (r *SchemaDocsRule) servedLabel(rs *schema.ResourceSchema, paths []string, f string) string {
	var labels, ignored []string
	for _, p := range paths {
		b := rs.Blocks[p]
		if b == nil || b.ConfigUnknown {
			continue
		}
		a, ok := attrAt(b, f)
		if !ok {
			continue
		}
		if r.IgnoreDeprecated && a.Deprecated {
			ignored = append(ignored, labelFor(a))
			continue
		}
		labels = append(labels, labelFor(a))
	}
	if len(labels) == 0 {
		labels = ignored
	}
	if len(slices.Compact(slices.Sorted(slices.Values(labels)))) != 1 {
		return ""
	}
	return labels[0]
}

// stripLabelResult builds the "attribute should not have <label(s)>" warning for
// a labeled attribute documented under Attribute Reference. Every parsed category
// label is listed so a contradictory bullet like "(Required, Read-Only)" is fully
// resolved by one fix rather than surfacing again on a second pass.
func stripLabelResult(r *SchemaDocsRule, ctx CheckContext, blockName string, attr doc.DocAttribute) Result {
	var labels []string
	if attr.Required {
		labels = append(labels, "(Required)")
	}
	if attr.Optional {
		labels = append(labels, "(Optional)")
	}
	if attr.ReadOnly {
		labels = append(labels, "(Read-Only)")
	}
	noun := "label"
	if len(labels) > 1 {
		noun = "labels"
	}
	return Result{
		Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
		Message: fmt.Sprintf("attribute %q in block %q should not have %s %s", attr.Name, displayPath(blockName), strings.Join(labels, ", "), noun),
		Block:   blockName,
		Line:    attr.Line,
	}
}

// --- Shared helpers ---

// attributeMisplacementFindings implements the attribute-granular
// Argument/Attribute-Reference misplacement rule in a single pass over
// AttributeBlocks (docs/rules/argument-attribute-misplacement.md §3–§5):
// resolve each subsection to a schema path, classify every labeled attribute,
// then choose the message granularity per subsection and deduplicate.
//
// A configurable argument (Required/Optional and not Computed) documented under
// Attribute Reference is *misplaced* — the author should move it to Argument
// Reference, not strip its (correct) label. When an entire subsection is
// configurable (documents no computed-only field) the moves collapse into one
// "move this subsection" finding; a mixed subsection emits per-attribute moves
// and leaves its computed-only fields where they are.
// collapseGroupKey identifies the physical subsection a block belongs to.
// Combined-heading aliases share a HeadingLine, so they map to one key; blocks
// without a heading line (e.g. manually constructed) key on their own name.
func collapseGroupKey(name string, line int) string {
	if line > 0 {
		return fmt.Sprintf("line:%d", line)
	}
	return "name:" + name
}

func (r *SchemaDocsRule) attributeMisplacementFindings(ctx CheckContext) []Result {
	skip := r.skipBlocks()
	names := slices.Sorted(maps.Keys(ctx.Doc.AttributeBlocks))

	type subMeta struct {
		path      string
		resolved  bool
		heading   string
		line      int
		spans     bool // entry spans multiple physical subsections; never collapse
		hasMis    bool // documents at least one misplaced (labeled pure-config) attribute
		hasComp   bool // documents a field that must stay under Attribute Reference (blocks collapse)
		eligible  bool // this subsection is collapse-eligible on its own
		collapses bool // will actually collapse (self eligible AND whole alias group eligible)
	}
	subs := make(map[string]*subMeta, len(names))
	// pathHasMis[P] is true when a real-heading subsection resolving to P
	// documents at least one misplaced attribute (i.e. emits a move). Used to
	// dedup a child-block reference bullet only against a child subsection that
	// actually produces a finding.
	pathHasMis := make(map[string]bool)
	// groupMalformed[groupKey] collects the malformed-bullet names of every block
	// in a physical subsection (combined-heading aliases share a group key), so
	// each alias can be pinned by a malformed computed field even though the
	// parser records the bullet only on the primary alias.
	groupMalformed := make(map[string][]string)

	type misplacedAttr struct {
		block  string
		path   string
		attr   doc.DocAttribute
		target string // child block path if the attr is a child-block reference; else path
	}
	var misplaced []misplacedAttr
	type stripEntry struct {
		block string
		attr  doc.DocAttribute
	}
	var strips []stripEntry

	// Collect: resolve each subsection and classify its attributes.
	for _, name := range names {
		block := ctx.Doc.AttributeBlocks[name]
		path, _, ok := resolveSubsectionPath(ctx.Schema, ctx.Doc.AttributeBlocks, name)
		// skip_blocks: a block opted out of checks (applied to its resolved
		// schema path) must not produce a move. Treating it as unresolved keeps
		// its legacy strip-label guidance without emitting the new error.
		resolved := ok && !slices.Contains(skip, path)

		sm := &subMeta{path: path, resolved: resolved, heading: block.Heading, line: block.HeadingLine, spans: block.SpansSubsections}
		subs[name] = sm

		for _, attr := range block.Attributes {
			switch classifyAttrPlacement(ctx.Schema, path, resolved, attr) {
			case placementOK:
				// Unlabeled computed output — correctly placed.
			case placementStripLabel:
				strips = append(strips, stripEntry{name, attr})
			case placementMisplaced:
				target := path
				if b, okb := ctx.Schema.Blocks[path]; okb {
					if cp, isChild := childPathForAttr(path, b, attr.Name); isChild {
						target = cp
					}
				}
				if slices.Contains(skip, target) {
					// The move would relocate a reference to a skip_blocks target
					// (e.g. a root bullet pointing at "timeouts"); skip_blocks is
					// exempt from the new move, so fall back to the legacy
					// strip-label, exactly as a skipped subsection path does.
					strips = append(strips, stripEntry{name, attr})
					continue
				}
				misplaced = append(misplaced, misplacedAttr{name, path, attr, target})
				sm.hasMis = true
				if sm.line == 0 {
					sm.line = attr.Line
				}
			}
			if resolved && fieldRequiresAttributeReference(ctx.Schema, path, attr.Name) {
				sm.hasComp = true
			}
		}
		// Malformed bullets (bad separator / unparseable) live only in
		// MalformedAttributes and — for a combined heading — only on the primary
		// alias, while valid attributes are mirrored to every alias. Collect the
		// group's malformed names now and evaluate them against each alias's own
		// path after the loop, so a malformed computed-only field pins whichever
		// alias it is computed under.
		for _, ma := range block.MalformedAttributes {
			gk := collapseGroupKey(name, block.HeadingLine)
			groupMalformed[gk] = append(groupMalformed[gk], ma.Name)
		}
		if resolved && block.Heading != "" && sm.hasMis {
			pathHasMis[path] = true
		}
	}

	// Pin any subsection whose physical group documents a malformed computed-only
	// field at that subsection's path (see collection above): without this a
	// collapse could drag that computed output into Argument Reference, and a
	// malformed bullet on the primary alias would otherwise never be evaluated
	// against a non-primary alias's schema path.
	for _, name := range names {
		sm := subs[name]
		if !sm.resolved {
			continue
		}
		for _, mn := range groupMalformed[collapseGroupKey(name, sm.line)] {
			if fieldRequiresAttributeReference(ctx.Schema, sm.path, mn) {
				sm.hasComp = true
				break
			}
		}
	}

	// Collapse eligibility is per physical subsection (§5): a resolved,
	// real-heading, non-root subsection that documents at least one misplaced
	// attribute and nothing that must stay under Attribute Reference. Judged per
	// subsection — not per schema path — so a subsection is never silenced by a
	// *different* subsection that happens to resolve to the same path.
	for _, name := range names {
		sm := subs[name]
		sm.eligible = sm.resolved && sm.heading != "" && sm.path != "" && sm.hasMis && !sm.hasComp && !sm.spans
	}

	// Combined headings (`### `foo` and `bar``) mirror one physical subsection to
	// several alias blocks that share a HeadingLine but resolve to independent
	// schema paths. "move this subsection" relocates the whole physical block, so
	// collapse only when every alias in the group is eligible; if any alias must
	// keep content under Attribute Reference, suppress the group collapse and let
	// per-attribute moves handle the eligible aliases.
	groupEligible := make(map[string]bool)
	for _, name := range names {
		sm := subs[name]
		key := collapseGroupKey(name, sm.line)
		if _, seen := groupEligible[key]; !seen {
			groupEligible[key] = true
		}
		if !sm.eligible {
			groupEligible[key] = false
		}
	}
	for _, name := range names {
		sm := subs[name]
		sm.collapses = sm.eligible && groupEligible[collapseGroupKey(name, sm.line)]
	}

	// A collapse "move this subsection" covers exactly the fields documented in
	// that subsection. Mark those fields covered so an alternate subsection does
	// not re-report the same field — while its *distinct* fields still surface.
	covered := make(map[string]bool)
	for _, m := range misplaced {
		if subs[m.block].collapses {
			covered[m.target+"\x00"+m.attr.Name] = true
		}
	}

	var out []Result

	// Collapse findings: one per physical heading. Parser aliases share a
	// heading line, so dedup on it to avoid re-reporting one subsection under
	// each alias key; genuinely distinct headings have distinct lines.
	emittedCollapse := make(map[string]bool)
	for _, name := range names {
		sm := subs[name]
		if !sm.collapses {
			continue
		}
		phys := collapseGroupKey(name, sm.line)
		if emittedCollapse[phys] {
			continue
		}
		emittedCollapse[phys] = true
		out = append(out, Result{
			Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
			Message: fmt.Sprintf("block %q is documented under Attribute Reference but is a configurable argument block in the schema; move this subsection to Argument Reference", displayPath(name)),
			Block:   name,
			Line:    sm.line,
		})
	}

	// Per-attribute moves for misplaced attributes not covered by a collapse. A
	// child-block reference bullet is deduped only against a child subsection
	// that actually emits a move (pathHasMis), never against the mere existence
	// of a child heading. Severity is WARN only for a genuine root scalar (path
	// and target both root, #62); a child-block reference — even one documented
	// at the root — is a nested-block move at ERROR.
	seen := make(map[string]bool)
	for _, m := range misplaced {
		if subs[m.block].collapses {
			continue // covered by this subsection's own collapse
		}
		fk := m.target + "\x00" + m.attr.Name
		if covered[fk] || seen[fk] {
			continue
		}
		if m.target != m.path && pathHasMis[m.target] {
			continue // the child subsection carries the finding
		}
		seen[fk] = true

		sev := SeverityError
		var msg string
		if m.path == "" {
			if m.target == "" {
				sev = SeverityWarning // genuine root scalar (#62)
			}
			msg = fmt.Sprintf("argument %q is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference", m.attr.Name)
		} else {
			msg = fmt.Sprintf("argument %q in block %q is documented under Attribute Reference but is a configurable argument in the schema; move it to Argument Reference", m.attr.Name, displayPath(m.block))
		}
		out = append(out, Result{Rule: r.Name(), Resource: ctx.Resource, Severity: sev, Message: msg, Block: m.block, Line: m.attr.Line})
	}

	// Strip-label findings: labeled attributes that are not configurable
	// arguments (computed-only, Optional+Computed, ConfigUnknown, or unresolved)
	// keep the legacy "should not have label" guidance.
	for _, s := range strips {
		// A collapsing subsection moves wholesale to Argument Reference, where a
		// labeled Optional+Computed field is a valid argument that must keep its
		// label. Emitting a strip here would contradict the collapse and create a
		// new labels error, so skip strips the collapse already relocates.
		if subs[s.block].collapses {
			continue
		}
		out = append(out, stripLabelResult(r, ctx, s.block, s.attr))
	}

	return out
}

// fieldRequiresAttributeReference reports whether a documented attribute must
// remain under Attribute Reference, which prevents its subsection from
// collapsing into a wholesale "move this subsection" finding (§5). That holds
// for a computed-only scalar (Computed and neither Required nor Optional), and
// for a documented child block that is entirely read-only (no configurable
// field anywhere in its subtree) or whose per-field configurability is
// unknowable (ConfigUnknown, handled conservatively so read-only child docs are
// never dragged into Argument Reference).
func fieldRequiresAttributeReference(rs *schema.ResourceSchema, path, attrName string) bool {
	if rs == nil {
		return false
	}
	b, ok := rs.Blocks[path]
	if !ok {
		return false
	}
	for _, a := range b.Attributes {
		if a.Name == attrName {
			return a.Computed && !a.Required && !a.Optional
		}
	}
	if cp, isChild := childPathForAttr(path, b, attrName); isChild {
		if cb, okc := rs.Blocks[cp]; okc && cb.ConfigUnknown {
			return true
		}
		return !blockTreeHasPureConfigurable(rs, cp, make(map[string]bool))
	}
	return false
}

func hasConfigurableAttributes(block *schema.Block) bool {
	for _, attr := range block.Attributes {
		if attr.Required || attr.Optional {
			return true
		}
	}
	return false
}

// resolutionClass records how an Attribute-Reference subsection heading resolved
// to a schema path. It is returned by resolveSubsectionPath and pinned by the
// classifier test to validate §4 resolution, but it is NOT stored on a finding:
// severity is derived from the resolved (path, target) — a genuine root scalar
// is WARN, every nested move/collapse is ERROR — so no per-class provenance
// travels into Result (see docs/rules/argument-attribute-misplacement.md §9,
// §10 step 2b).
type resolutionClass int

const (
	// resolveUnresolved: no schema path could be assigned; the caller falls
	// back to the legacy strip-label behavior (never a move).
	resolveUnresolved resolutionClass = iota
	// resolveRoot: the root block (top-level scalars, #62).
	resolveRoot
	// resolveDottedExact: a dotted heading matched an exact schema block.
	resolveDottedExact
	// resolveBareExact: a bare heading matched an exact (root-level) schema block.
	resolveBareExact
	// resolveUniqueLeaf: a bare heading was inferred to the sole schema path
	// carrying that leaf. This is the ONLY inference step (see §4).
	resolveUniqueLeaf
)

// resolveSubsectionPath maps an Attribute-Reference subsection's doc-block key to
// a single schema path for misplacement classification, using the strict,
// ownership-free rules in docs/rules/argument-attribute-misplacement.md §4:
//
//   - root (""):   resolves to "" (resolveRoot).
//   - dotted key:  the exact schema block, else unresolved. A dotted heading
//     claims an exact path and is never remapped by leaf (fixes 2/3).
//   - bare key:    the exact root-level block (resolveBareExact); else the sole
//     schema path carrying that leaf (resolveUniqueLeaf); else unresolved
//     (fixes 1/3).
//
// Unlike coverage's ownership resolver (schemaPathsResolvedByDocKey), this never
// uses most-specific-owner logic: ownership exists to avoid double-counting
// coverage and is the wrong tool for "does this documented argument correspond
// to a configurable schema field." The unique-leaf branch is the only step that
// infers a path; it fires for real headings only (a heading-less synthetic
// block is resolved by exact path alone, never inferred) and is double-gated
// downstream by configurableArgAtPath, so it can never fabricate a move on a
// non-configurable field.
func resolveSubsectionPath(rs *schema.ResourceSchema, docBlocks map[string]*doc.DocBlock, key string) (string, resolutionClass, bool) {
	if rs == nil {
		return "", resolveUnresolved, false
	}
	if key == "" {
		return "", resolveRoot, true
	}
	if strings.Contains(key, ".") {
		if _, ok := rs.Blocks[key]; ok {
			return key, resolveDottedExact, true
		}
		return "", resolveUnresolved, false
	}
	// Bare key: exact root-level block first.
	if _, ok := rs.Blocks[key]; ok {
		return key, resolveBareExact, true
	}
	// Unique-leaf inference, gated to headings and prose lead-ins, which are
	// headings in all but syntax (coverage-path-resolution.md §5): both record
	// occurrences. Routed content (dot-path reference bullets) records none and
	// resolves only by the exact-path branch above.
	if b := docBlocks[key]; b != nil && (b.Heading != "" || len(b.Occurrences) > 0) {
		if p, ok := uniqueSchemaPathForLeaf(rs, key); ok {
			return p, resolveUniqueLeaf, true
		}
	}
	return "", resolveUnresolved, false
}

// uniqueSchemaPathForLeaf returns the single non-root schema path whose leaf
// name equals leaf, reporting false when zero or more than one path carries it.
// It underpins the alternate-heading suppression: an alternate subsection can be
// associated with a moved schema path only when the leaf is unambiguous.
func uniqueSchemaPathForLeaf(rs *schema.ResourceSchema, leaf string) (string, bool) {
	if rs == nil || leaf == "" {
		return "", false
	}
	var found string
	n := 0
	for path := range rs.Blocks {
		if path != "" && leafName(path) == leaf {
			found = path
			n++
		}
	}
	if n == 1 {
		return found, true
	}
	return "", false
}

// placement is the per-attribute verdict for an attribute documented under
// Attribute Reference (docs/rules/argument-attribute-misplacement.md §3).
type placement int

const (
	// placementOK: no finding. The attribute carries no label at all, so it is a
	// proper computed output living under Attribute Reference.
	placementOK placement = iota
	// placementStripLabel: the attribute is labeled but is not a purely
	// configurable argument at the resolved path (computed-only, Optional+
	// Computed, ConfigUnknown, or the subsection did not resolve), or it carries a
	// (Read-Only) label — which is never permitted under Attribute Reference. The
	// legacy strip-label guidance, never a move.
	placementStripLabel
	// placementMisplaced: the attribute is labeled AND a purely configurable
	// argument at the resolved path — the section is wrong; move it to Argument
	// Reference rather than stripping its (correct) label (#60, #62).
	placementMisplaced
)

// classifyAttrPlacement judges a single attribute documented under Attribute
// Reference in a subsection resolved to schema path P, implementing the §3
// classification table by composing label state with configurableArgAtPath:
//
//   - unlabeled                          -> placementOK (proper computed output)
//   - (Read-Only), pure-config arg at P   -> placementMisplaced (move it; the
//     label is fixed once in Argument Reference)
//   - (Read-Only), not pure-config        -> placementStripLabel (no label allowed
//     under Attribute Reference, regardless of allow_inline_read_only) (Gap B)
//   - labeled, pure-config arg at P       -> placementMisplaced (move it)
//   - labeled, not pure-config / unresolved -> placementStripLabel (legacy)
//
// resolved is the ok result from resolveSubsectionPath and must gate the
// misplacement branch: it distinguishes a genuine root subsection (path == ""
// with resolved == true, for #62 top-level scalars) from an *unresolved*
// subsection that also carries an empty path — without it, an attribute whose
// name happened to match a configurable root argument would be spuriously
// flagged as misplaced. Optional+Computed and ConfigUnknown are non-misplacement
// by construction (configurableArgAtPath returns false), honoring the #62 guard.
func classifyAttrPlacement(rs *schema.ResourceSchema, path string, resolved bool, attr doc.DocAttribute) placement {
	if !attr.Required && !attr.Optional {
		if !attr.ReadOnly {
			return placementOK // truly unlabeled — a proper computed output
		}
		// A (Read-Only) label is never allowed under Attribute Reference. If the
		// field is actually a configurable argument it is *misplaced*: direct the
		// move to Argument Reference (where labelCorrectness then fixes the label),
		// not a bare label strip that would leave a configurable field silently
		// accepted here. Computed, unresolved, and unknown fields keep the strip.
		if resolved && configurableArgAtPath(rs, path, attr.Name) {
			return placementMisplaced
		}
		return placementStripLabel
	}
	if resolved && configurableArgAtPath(rs, path, attr.Name) {
		return placementMisplaced
	}
	return placementStripLabel
}

// configurableArgAtPath reports whether attrName is a purely configurable
// argument ((Required || Optional) && !Computed) of the schema block at the
// given canonical map-key path — either as a scalar attribute, or as an
// immediate child block that (itself or via a descendant) carries such an
// attribute. ConfigUnknown blocks and unresolved paths return false so an
// ERROR never fires on a guess. Optional+Computed scalars are excluded because
// they may legitimately appear under either section.
func configurableArgAtPath(rs *schema.ResourceSchema, path, attrName string) bool {
	if rs == nil {
		return false
	}
	b, ok := rs.Blocks[path]
	if !ok || b.ConfigUnknown {
		return false
	}
	for _, a := range b.Attributes {
		if a.Name == attrName {
			return (a.Required || a.Optional) && !a.Computed
		}
	}
	if childPath, found := childPathForAttr(path, b, attrName); found {
		return blockTreeHasPureConfigurable(rs, childPath, make(map[string]bool))
	}
	return false
}

// childBlockPath returns the canonical ResourceSchema.Blocks key for a
// ChildBlocks entry of the block at parentPath. Entries appear in two forms in
// this codebase — a bare leaf name (from the provider loader) or a full
// dot-path (common in fixtures and normalized elsewhere via leafName). A
// full-path entry is used as-is; a bare leaf is joined to the parent path.
func childBlockPath(parentPath, child string) string {
	if strings.Contains(child, ".") {
		return child
	}
	if parentPath == "" {
		return child
	}
	return parentPath + "." + child
}

// childPathForAttr returns the canonical schema path of the immediate child
// block of pb (at parentPath) whose leaf name is attrName, matching by leaf so
// both ChildBlocks representations resolve. The second result reports whether
// such a child exists.
func childPathForAttr(parentPath string, pb *schema.Block, attrName string) (string, bool) {
	for _, child := range pb.ChildBlocks {
		if leafName(child) == attrName {
			return childBlockPath(parentPath, child), true
		}
	}
	return "", false
}

// blockTreeHasPureConfigurable reports whether the schema block at path, or any
// descendant block, has a purely configurable (Required or Optional and NOT
// Computed) attribute. Blocks are looked up by exact full path (unambiguous),
// ConfigUnknown blocks are skipped, and visited guards against pathological
// cycles.
func blockTreeHasPureConfigurable(rs *schema.ResourceSchema, path string, visited map[string]bool) bool {
	if rs == nil || visited[path] {
		return false
	}
	visited[path] = true
	b, ok := rs.Blocks[path]
	if !ok || b.ConfigUnknown {
		return false
	}
	for _, a := range b.Attributes {
		if (a.Required || a.Optional) && !a.Computed {
			return true
		}
	}
	for _, child := range b.ChildBlocks {
		if blockTreeHasPureConfigurable(rs, childBlockPath(path, child), visited) {
			return true
		}
	}
	return false
}

func severity(attr schema.Attribute) Severity {
	if attr.Deprecated {
		return SeverityWarning
	}
	return SeverityError
}

// resolveSection returns the one section in blocks (a single reference
// section: Argument or Attribute Reference) that documents schema path p, or
// nil. Resolution uses heading keys only, never links or position
// (docs/rules/coverage-path-resolution.md §4). The first key that names a
// section wins, in this order: the exact path; three-segment composites
// a.b.leaf, nearest adjacent ancestors first; two-segment composites a.leaf,
// nearest ancestor first; the bare leaf. A dotted partial key that is itself a
// different schema path claims only that path, so it is skipped: a heading
// naming a.b documents a.b, not an undocumented a.x.b. The bare leaf is never
// skipped, even when it is also a root-level path: it is a leaf-name heading
// and serves every path with that leaf, and whether that is correct is decided
// per path by the field checks.
func resolveSection(rs *schema.ResourceSchema, blocks map[string]*doc.DocBlock, p string) *doc.DocBlock {
	if p == "" {
		return blocks[""]
	}
	for _, key := range sectionKeyCandidates(p) {
		b := blocks[key]
		if b == nil {
			continue
		}
		if key != p && strings.Contains(key, ".") && rs != nil {
			if _, isPath := rs.Blocks[key]; isPath {
				continue
			}
		}
		return b
	}
	return nil
}

// sectionKeyCandidates lists the heading keys that can document path p, in
// resolution order.
func sectionKeyCandidates(p string) []string {
	parts := strings.Split(p, ".")
	leaf := parts[len(parts)-1]
	keys := []string{p}
	for i := len(parts) - 3; i >= 0; i-- {
		keys = append(keys, parts[i]+"."+parts[i+1]+"."+leaf)
	}
	for i := len(parts) - 2; i >= 0; i-- {
		keys = append(keys, parts[i]+"."+leaf)
	}
	return append(keys, leaf)
}

// resolveSections returns the sections documenting p in Argument and
// Attribute Reference, skipping either that doesn't resolve.
func resolveSections(rs *schema.ResourceSchema, d *doc.Document, p string) []*doc.DocBlock {
	var out []*doc.DocBlock
	if b := resolveSection(rs, d.ArgumentBlocks, p); b != nil {
		out = append(out, b)
	}
	if b := resolveSection(rs, d.AttributeBlocks, p); b != nil {
		out = append(out, b)
	}
	return out
}

func leafName(path string) string {
	if path == "" {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i+1:]
		}
	}
	return path
}

func displayPath(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}

func findMalformed(malformed []doc.MalformedAttr, name string) (doc.MalformedAttr, bool) {
	for _, m := range malformed {
		if m.Name == name {
			return m, true
		}
	}
	return doc.MalformedAttr{}, false
}

func checkBlockOrdering(resource, ruleName, section, blockPath string, block *doc.DocBlock) []Result {
	var results []Result
	var required, optional, unmarked []string
	for _, attr := range block.Attributes {
		switch {
		case attr.Required:
			required = append(required, attr.Name)
		case attr.Optional:
			optional = append(optional, attr.Name)
		default:
			unmarked = append(unmarked, attr.Name)
		}
	}

	// If the doc explicitly split required/optional with separate bylines
	// ("The following arguments are required:" / "The following arguments
	// are optional:"), check each group independently. Otherwise check
	// all labeled attributes as a single alphabetical list.
	if block.SplitByLabel {
		for _, group := range [][]string{required, optional, unmarked} {
			if r := checkSliceOrdering(group, resource, ruleName, section, blockPath); r != nil {
				results = append(results, *r)
			}
		}
	} else {
		all := make([]string, 0, len(block.Attributes))
		for _, attr := range block.Attributes {
			all = append(all, attr.Name)
		}
		if r := checkSliceOrdering(all, resource, ruleName, section, blockPath); r != nil {
			results = append(results, *r)
		}
	}
	return results
}

func checkSliceOrdering(names []string, resource, ruleName, section, blockName string) *Result {
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			return &Result{
				Rule: ruleName, Resource: resource, Severity: SeverityError,
				Message: fmt.Sprintf("%s %q should come before %q in %s block %q", section, names[i], names[i-1], section, displayPath(blockName)),
				Block:   blockName,
			}
		}
	}
	return nil
}

func isListProse(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "the following arguments") || strings.Contains(lower, "the following attributes") {
		return true
	}
	if strings.Contains(lower, "this resource supports") || strings.Contains(lower, "this data source supports") {
		return true
	}
	if strings.Contains(lower, "this resource exports") || strings.Contains(lower, "this data source exports") {
		return true
	}
	if strings.HasPrefix(line, "~>") || strings.HasPrefix(line, "->") || strings.HasPrefix(line, "!>") {
		return true
	}
	// A legacy nested-block lead-in ("The `x[0].y[0]` block also exports:")
	// introduces a block, like a heading — not an interruption.
	if _, ok := doc.NestedBlockLeadIn(line); ok {
		return true
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// --- Byline ---

// checkBylines validates that the first paragraph after ## Argument Reference
// and ## Attribute Reference matches one of the expected byline texts defined
// in the type block.
func (r *SchemaDocsRule) checkBylines(ctx CheckContext) []Result {
	if ctx.Type == nil || ctx.Doc == nil {
		return nil
	}

	var results []Result

	// Arguments byline.
	if len(ctx.Type.ArgumentsBylines) > 0 && ctx.Doc.Sections.Arguments != nil {
		results = append(results, r.checkSectionByline(ctx, ctx.Doc.Sections.Arguments, ctx.Type.ArgumentsBylines, ctx.Type.AllowMissingArgumentsByline, "Argument Reference")...)
	}

	// Attributes byline.
	if len(ctx.Type.AttributesBylines) > 0 && ctx.Doc.Sections.Attributes != nil {
		results = append(results, r.checkSectionByline(ctx, ctx.Doc.Sections.Attributes, ctx.Type.AttributesBylines, false, "Attribute Reference")...)
	}

	return results
}

func (r *SchemaDocsRule) checkSectionByline(ctx CheckContext, section *doc.Section, expected []string, allowMissing bool, sectionName string) []Result {
	if len(section.Paragraphs) == 0 {
		if !allowMissing {
			return []Result{{
				Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
				Message: fmt.Sprintf("%s section is missing a byline paragraph", sectionName),
			}}
		}
		return nil
	}

	// Get the text of the first paragraph.
	source := ctx.Doc.Source()
	firstPara := section.Paragraphs[0]
	paraText := strings.TrimSpace(string(firstPara.Text(source)))

	if slices.Contains(expected, paraText) {
		return nil
	}

	return []Result{{
		Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityWarning,
		Message: fmt.Sprintf("%s byline %q does not match expected texts", sectionName, truncate(paraText, 80)),
	}}
}

// --- Deprecated ---

// checkDeprecated verifies that attributes marked deprecated in the schema
// are also marked as deprecated in the documentation.
//
// A field whose deprecation differs across the paths a shared section serves
// is reported once by the shared-section finding (disjunct 3); per path it
// would produce contradictory findings, so it's skipped here.
func (r *SchemaDocsRule) checkDeprecated(ctx CheckContext, shared []sharedSection) []Result {
	if ctx.Schema == nil {
		return nil
	}
	conflicted := make(map[*doc.DocBlock]map[string]bool)
	for _, sh := range shared {
		for _, c := range sh.conflicts {
			if c.kind == 3 {
				if conflicted[sh.block] == nil {
					conflicted[sh.block] = make(map[string]bool)
				}
				conflicted[sh.block][c.field] = true
			}
		}
	}

	var results []Result
	for _, blockPath := range slices.Sorted(maps.Keys(ctx.Schema.Blocks)) {
		schemaBlock := ctx.Schema.Blocks[blockPath]
		sections := resolveSections(ctx.Schema, ctx.Doc, blockPath)
		if len(sections) == 0 {
			continue
		}

		schemaAttrs := make(map[string]schema.Attribute, len(schemaBlock.Attributes))
		for _, a := range schemaBlock.Attributes {
			schemaAttrs[a.Name] = a
		}

		for _, docBlock := range sections {
			// Merged bullets of a duplicated key would be compared against
			// every path; the fit rule judges each heading's markers instead.
			if duplicated(docBlock) && enabled(r.Coverage) {
				continue
			}
			skip := conflicted[docBlock]
			docAttrs := make(map[string]*doc.DocAttribute, len(docBlock.Attributes))
			for i := range docBlock.Attributes {
				docAttrs[docBlock.Attributes[i].Name] = &docBlock.Attributes[i]
			}

			// Schema deprecated but doc not marked.
			for _, attr := range schemaBlock.Attributes {
				if !attr.Deprecated || skip[attr.Name] {
					continue
				}
				da, ok := docAttrs[attr.Name]
				if !ok {
					continue // not documented here — coverage handles absence
				}
				if !da.Deprecated {
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
						Message: fmt.Sprintf("attribute %q in block %q is deprecated in schema but not marked as deprecated in docs", attr.Name, displayPath(blockPath)),
						Block:   blockPath,
						Line:    da.Line,
					})
				}
			}

			// Doc marked deprecated but schema is not.
			for _, da := range docBlock.Attributes {
				if !da.Deprecated || skip[da.Name] {
					continue
				}
				sa, ok := schemaAttrs[da.Name]
				if !ok {
					continue // phantom — coverage check handles this
				}
				if !sa.Deprecated {
					results = append(results, Result{
						Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
						Message: fmt.Sprintf("attribute %q in block %q is marked deprecated in docs but not in schema; either mark as deprecated in schema or remove the deprecation notice", da.Name, displayPath(blockPath)),
						Block:   blockPath,
						Line:    da.Line,
					})
				}
			}
		}
	}
	return results
}
