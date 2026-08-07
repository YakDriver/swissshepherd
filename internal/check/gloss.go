// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// GlossRule flags banned glosses anywhere in a document. A gloss is an
// abbreviation expansion like "Amazon Resource Name (ARN)". The rule is
// entirely opt-in: it does nothing unless a provider configures a
// phrase→abbreviation map. For each configured pair it flags both:
//
//   - the glossed form   — "Amazon Resource Name (ARN)"  → use "ARN"
//   - the standalone phrase — "Amazon Resource Name"      → use "ARN"
//
// The bare abbreviation ("ARN") is always acceptable and never flagged. An
// optional leading "Amazon "/"AWS " on the phrase, and an optional
// "Amazon "/"AWS " inside the parenthetical, are recognized. Matching runs
// over the whole document body but skips fenced code blocks, inline code
// spans, and URLs so real configuration and identifiers are left alone.
type GlossRule struct {
	entries         []glossEntry
	skipFrontmatter bool
	severity        Severity
}

type glossEntry struct {
	re  *regexp.Regexp
	abb string
}

// NewGlossRule compiles a GlossRule from a phrase→abbreviation map. Entries
// with an empty phrase or abbreviation are skipped. A rule with no usable
// entries produces no findings. When skipFrontmatter is true, the leading
// YAML frontmatter block is excluded from the scan. severity sets the
// severity of the findings it emits.
func NewGlossRule(glosses map[string]string, skipFrontmatter bool, severity Severity) *GlossRule {
	r := &GlossRule{skipFrontmatter: skipFrontmatter, severity: severity}
	for _, phrase := range slices.Sorted(maps.Keys(glosses)) {
		abb := strings.TrimSpace(glosses[phrase])
		p := strings.TrimSpace(phrase)
		if p == "" || abb == "" {
			continue
		}
		r.entries = append(r.entries, glossEntry{re: glossRegexp(p, abb), abb: abb})
	}
	return r
}

func (r *GlossRule) Name() string { return "banned_glosses" }

// glossRegexp builds the matcher for one phrase→abbreviation pair. It matches
// the phrase (case-insensitive) with an optional leading "Amazon "/"AWS " and
// an optional trailing gloss parenthetical containing the abbreviation. A
// trailing plural "s" on both phrase and abbreviation is tolerated so
// "Amazon Resource Names (ARNs)" is caught alongside the singular form.
func glossRegexp(phrase, abb string) *regexp.Regexp {
	pat := `(?i)\b(?:amazon |aws )?` + regexp.QuoteMeta(phrase) + `s?\b` +
		`(?:\s*\((?:amazon |aws )?` + regexp.QuoteMeta(abb) + `s?\))?`
	return regexp.MustCompile(pat)
}

func (r *GlossRule) CheckFile(ctx FileCheckContext) []Result {
	if len(r.entries) == 0 {
		return nil
	}

	var results []Result
	lines := strings.Split(string(ctx.Content), "\n")
	fmEnd := -1
	if r.skipFrontmatter {
		fmEnd = frontmatterEnd(lines)
	}
	inFence := false
	for i, raw := range lines {
		if i <= fmEnd {
			continue
		}
		if isFenceDelimiter(raw) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		scannable := maskUnscannable(raw)
		for _, e := range r.entries {
			for _, loc := range e.re.FindAllStringIndex(scannable, -1) {
				matched := strings.TrimSpace(raw[loc[0]:loc[1]])
				results = append(results, Result{
					Rule:     r.Name(),
					Resource: ctx.Resource,
					Severity: r.severity,
					Line:     i + 1,
					Message:  fmt.Sprintf("avoid %q; use %q instead", matched, recommend(matched, e.abb)),
				})
			}
		}
	}
	return results
}

// recommend returns the abbreviation to suggest, pluralized when the matched
// text was plural (ended in "s" but the abbreviation did not).
func recommend(matched, abb string) string {
	if strings.HasSuffix(matched, "s") && !strings.HasSuffix(abb, "s") &&
		!strings.HasSuffix(matched, ")") {
		return abb + "s"
	}
	return abb
}

// isFenceDelimiter reports whether a line opens or closes a fenced code block
// (``` or ~~~, possibly indented, optionally with an info string).
func isFenceDelimiter(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// frontmatterEnd returns the index of the closing "---" of a leading YAML
// frontmatter block, or -1 when the document has none. Lines 0..return
// (inclusive) constitute the frontmatter.
func frontmatterEnd(lines []string) int {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return -1
	}
	for j := 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "---" {
			return j
		}
	}
	return -1
}

var (
	reInlineCode = regexp.MustCompile("`[^`\n]*`")
	reLinkURL    = regexp.MustCompile(`\]\([^)]*\)`)
	reAutolink   = regexp.MustCompile(`<[^>\n]*>`)
	reBareURL    = regexp.MustCompile(`https?://\S+`)
)

// maskUnscannable blanks out regions that should not be scanned — inline code
// spans, markdown link targets, autolinks, and bare URLs — replacing each with
// equal-length spaces so byte offsets (and thus any reported positions) stay
// aligned with the original line. Visible link text and gloss parentheses
// like "(ARN)" are preserved.
func maskUnscannable(line string) string {
	out := line
	for _, re := range []*regexp.Regexp{reInlineCode, reLinkURL, reAutolink, reBareURL} {
		out = re.ReplaceAllStringFunc(out, blankOut)
	}
	return out
}

func blankOut(s string) string { return strings.Repeat(" ", len(s)) }
