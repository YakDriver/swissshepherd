// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// ProseCasing flags a word in doc prose whose casing should be the
// configured canonical form but isn't — "arn" should be "ARN", "dynamodb"
// should be "DynamoDB" — but never upcases a word that is already correct,
// and never touches the word inside a code span, fence, URL, or compound
// identifier. See docs/rules/prose-casing.md for the design.
type ProseCasing struct {
	re               *regexp.Regexp    // single combined alternation; nil means no words configured
	canon            map[string]string // lowercase form -> canonical form
	headingTemplates doc.HeadingTemplates
	ignoreGlobal     map[string]bool
	ignoreTargets    map[string]map[string]bool // lowercase word -> set of target specifiers
	skipFrontmatter  bool
	severity         Severity
}

// NewProseCasingRule builds a ProseCasing rule from the built-in default
// word list merged with enforceCasing (provider additions/overrides),
// minus ignoreWords (bare entries remove a word everywhere; "target/word"
// entries remove it for one target only). headingTemplates is the
// provider's accepted block-heading styles, used to withhold the
// capitalization fix when it would stop a heading resolving against the
// schema. Returns an error if an enforceCasing entry is already lowercase
// (it would suggest itself).
func NewProseCasingRule(enforceCasing, ignoreWords []string, headingTemplates doc.HeadingTemplates, skipFrontmatter bool, severity Severity) (*ProseCasing, error) {
	merged := make(map[string]string, len(defaultProseCasingWords)+len(enforceCasing))
	maps.Copy(merged, defaultProseCasingWords)
	for _, want := range enforceCasing {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}
		lower := strings.ToLower(want)
		if want == lower {
			return nil, fmt.Errorf("prose_casing: enforce_casing entry %q is already lowercase; it would suggest itself", want)
		}
		merged[lower] = want
	}

	r := &ProseCasing{
		headingTemplates: headingTemplates,
		skipFrontmatter:  skipFrontmatter,
		severity:         severity,
		ignoreGlobal:     make(map[string]bool),
		ignoreTargets:    make(map[string]map[string]bool),
	}
	for _, w := range ignoreWords {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		// "word" ignores everywhere; "target/word" (where target is a bare
		// resource name or "type/name") ignores for one target only, the
		// word named last so a bare entry is never mistaken for a target.
		target, word, ok := cutLastSlash(w)
		if !ok {
			r.ignoreGlobal[strings.ToLower(w)] = true
			continue
		}
		lower := strings.ToLower(word)
		if r.ignoreTargets[lower] == nil {
			r.ignoreTargets[lower] = make(map[string]bool)
		}
		r.ignoreTargets[lower][target] = true
	}

	var words []string
	for _, lower := range slices.Sorted(maps.Keys(merged)) {
		if r.ignoreGlobal[lower] {
			continue
		}
		words = append(words, lower)
	}
	if len(words) == 0 {
		return r, nil
	}
	// Longest first so "https" wins over "http" at the same position: Go's
	// RE2 engine tries alternation branches in order and takes the first
	// that matches, so a shorter prefix listed first would shadow the
	// longer word.
	slices.SortFunc(words, func(a, b string) int { return len(b) - len(a) })

	r.canon = merged
	quoted := make([]string, len(words))
	for i, lower := range words {
		quoted[i] = regexp.QuoteMeta(lower)
	}
	// One regex for the whole rule, not one per word: running separate
	// regexes against the same text would let a shorter and a longer entry
	// (e.g. "cpu" and "cpu's") both independently match overlapping text,
	// double-reporting the same token.
	r.re = regexp.MustCompile(`(?i)\b(?:` + strings.Join(quoted, "|") + `)\b`)
	return r, nil
}

func (r *ProseCasing) Name() string { return "prose_casing" }

func (r *ProseCasing) CheckFile(ctx FileCheckContext) []Result {
	if r.re == nil {
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
		// An unterminated inline code span can't be told apart from real
		// text by maskUnscannable (it only matches balanced pairs), so a
		// line with an odd number of backticks is skipped outright rather
		// than scanned with the span unmasked.
		if strings.Count(raw, "`")%2 != 0 {
			continue
		}
		if !r.re.MatchString(raw) {
			continue
		}

		scannable := maskProseCasingLiterals(maskUnscannable(raw))
		isHeading := strings.HasPrefix(strings.TrimSpace(raw), "#")

		for _, loc := range r.re.FindAllStringIndex(scannable, -1) {
			got := raw[loc[0]:loc[1]]
			want := r.canon[strings.ToLower(got)]
			if got == want || r.isIgnored(strings.ToLower(got), ctx) {
				continue
			}
			results = append(results, Result{
				Rule:     r.Name(),
				Resource: ctx.Resource,
				Severity: r.severity,
				Line:     i + 1,
				Message:  r.message(raw, got, want, isHeading),
			})
		}
	}
	return results
}

// isIgnored reports whether word (already lowercased) is excluded for this
// target, by a bare resource name or a "type/name"-qualified one.
func (r *ProseCasing) isIgnored(word string, ctx FileCheckContext) bool {
	targets, ok := r.ignoreTargets[word]
	if !ok {
		return false
	}
	if targets[ctx.Resource] {
		return true
	}
	if ctx.Type != nil && targets[ctx.Type.Name+"/"+ctx.Resource] {
		return true
	}
	return false
}

// cutLastSlash splits "target/word" at the final "/", so a target itself
// qualified as "type/name" (two slashes total) still separates correctly
// from the trailing word. Returns ok=false for an entry with no slash at
// all (a bare word, ignored everywhere).
func cutLastSlash(s string) (target, word string, ok bool) {
	i := strings.LastIndex(s, "/")
	if i < 0 {
		return "", s, false
	}
	return s[:i], s[i+1:], true
}

// message builds the finding text. A word that is the entire resolvable
// content of a heading only gets the capitalization fix when capitalizing
// would not change whether the heading resolves against the schema (a
// {Title}-style heading, or one that doesn't resolve to a block either
// way); otherwise only the backticks fix is offered, since capitalizing a
// {Block}/{Path}-style heading would make it stop resolving.
func (r *ProseCasing) message(raw, got, want string, isHeading bool) string {
	if isHeading && r.headingTemplates != nil {
		text := headingText(raw)
		before := r.headingTemplates.Match(text)
		after := r.headingTemplates.Match(strings.Replace(text, got, want, 1))
		if before != "" && after != before {
			return fmt.Sprintf("avoid %q; add backticks around it instead of capitalizing — capitalizing would stop this heading resolving to its block", got)
		}
	}
	return fmt.Sprintf("avoid %q; use %q instead, or add backticks around it if this is a correct, lowercase technical reference", got, want)
}

// headingText strips a Markdown heading's leading "#" markers and
// surrounding whitespace, mirroring the text the parser hands to
// HeadingTemplates.Match (goldmark's heading text, with the "#"s already
// removed).
func headingText(raw string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), "#"))
}

var (
	reColonLiteral = regexp.MustCompile(`\b\w+(?::\S+)+`)
	reGlued        = regexp.MustCompile(`[\w.]*[-_/@][\w.\-_/@]*|[\w-]+\.[\w-]+(?:\.[\w-]+)*|"[^"\n]*"`)
)

// maskProseCasingLiterals blanks out literal forms masking shared with
// GlossRule doesn't catch: colon-delimited literals (arn:aws:ec2:…,
// s3://bucket/prefix), tokens glued to a hyphen, underscore, slash, or "@"
// (execute-api, ~/.ssh/authorized_keys), a dotted compound where both sides
// are word characters (index.html, but not a sentence-final "rest api."),
// and a quoted literal ("only \"url\" can be used"). Each replacement is
// equal-length so byte offsets stay aligned with the original line.
func maskProseCasingLiterals(line string) string {
	out := reColonLiteral.ReplaceAllStringFunc(line, blankOut)
	out = reGlued.ReplaceAllStringFunc(out, blankOut)
	return out
}

// defaultProseCasingWords is the built-in, vendor-neutral word list
// (docs/rules/prose-casing.md "Default list"), keyed by lowercase form.
// AWS-specific vocabulary (ARN, VPC, DynamoDB, ...) is deliberately absent;
// a provider adds it via enforce_casing. Plural and possessive forms are
// listed only for words that occur in practice (docs/rules/prose-casing.md
// "Sizing"); enforce_casing lets a provider add others.
var defaultProseCasingWords = map[string]string{
	"id": "ID", "ids": "IDs", "id's": "ID's",
	"api":   "API",
	"url":   "URL",
	"uri":   "URI",
	"http":  "HTTP",
	"https": "HTTPS",
	"tls":   "TLS",
	"ssl":   "SSL",
	"ssh":   "SSH",
	"json":  "JSON",
	"xml":   "XML",
	"html":  "HTML",
	"sql":   "SQL",
	"dns":   "DNS",
	"ip":    "IP",
	"cpu":   "CPU", "cpus": "CPUs", "cpu's": "CPU's",
	"gpu":  "GPU",
	"uuid": "UUID",
	"guid": "GUID",
	"cli":  "CLI",
	"sdk":  "SDK",
	"ui":   "UI",
	"css":  "CSS",
	"jwt":  "JWT",
}
