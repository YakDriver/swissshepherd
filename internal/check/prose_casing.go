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

// ProseCasing flags a word or multi-word name in doc prose whose casing or
// spacing should be the configured canonical form but isn't — "arn" should
// be "ARN", "apigateway" should be "API Gateway" — but never touches text
// that is already correct, or text inside a code span, fence, URL, or
// compound identifier. See docs/rules/prose-casing.md for the design.
type ProseCasing struct {
	entries          map[string]casingEntry // match key -> entry; empty means no words configured
	prefixes         map[string]bool        // every key prefix a match may have reached at a word break
	maxWords         int
	headingTemplates doc.HeadingTemplates
	ignoreTargets    map[string]map[string]bool // match key -> set of target specifiers
	skipFrontmatter  bool
	severity         Severity
}

// casingEntry is one canonical form. Its match key is the canonical form
// lowercased with spaces removed ("API Gateway" -> "apigateway"); breaks
// are the key offsets where the canonical form has a space ([3]). Text
// matches when its words, lowercased and joined, spell the key and every
// break in the text is also one of breaks: text may run the canonical's
// words together but never split one.
type casingEntry struct {
	canonical string
	key       string
	breaks    []int
}

// parseCasingEntry normalizes a configured canonical form (trimmed, inner
// whitespace collapsed to single spaces) and derives its match key. Each
// word is a run of [0-9A-Za-z_], the characters Go's \b treats as word
// characters, and the last may end in a possessive "'s". Anything else
// could never match: masking blanks text glued to "-", ".", "/", and "@"
// before matching.
func parseCasingEntry(s string) (casingEntry, error) {
	words := strings.Fields(s)
	if len(words) == 0 {
		return casingEntry{}, nil
	}
	e := casingEntry{canonical: strings.Join(words, " ")}
	var key []byte
	possessive := false
	for i, w := range words {
		if i == len(words)-1 {
			w, possessive = strings.CutSuffix(w, "'s")
		}
		if w == "" || strings.ContainsFunc(w, func(c rune) bool { return c > 0x7f || !isWordByte(byte(c)) }) {
			return casingEntry{}, fmt.Errorf("prose_casing: enforce_casing entry %q: words may contain only letters, digits, and underscores, with an optional trailing 's", s)
		}
		if i > 0 {
			e.breaks = append(e.breaks, len(key))
		}
		key = appendLower(key, w)
	}
	if possessive {
		key = append(key, "'s"...)
	}
	e.key = string(key)
	return e, nil
}

// casingKey returns the match key for s, the form ignore_words and config
// validation compare by. It ignores entry syntax errors, which
// parseCasingEntry reports.
func casingKey(s string) string {
	e, _ := parseCasingEntry(s)
	return e.key
}

// NewProseCasingRule builds a ProseCasing rule from the built-in default
// word list merged with enforceCasing (provider additions/overrides, which
// may be multi-word names such as "API Gateway"), minus ignoreWords (bare
// entries remove an entry everywhere; "target/word" entries remove it for
// one target only). Entries are compared by match key, so "Auto Scaling"
// overrides or ignores "autoscaling" too. headingTemplates is the
// provider's accepted block-heading styles, used to withhold the
// capitalization fix when it would stop a heading resolving against the
// schema. Returns an error if an enforceCasing entry is already lowercase
// (it would suggest itself) or contains characters that can never match.
func NewProseCasingRule(enforceCasing, ignoreWords []string, headingTemplates doc.HeadingTemplates, skipFrontmatter bool, severity Severity) (*ProseCasing, error) {
	merged := make(map[string]casingEntry, len(defaultProseCasingWords)+len(enforceCasing))
	for _, want := range slices.Sorted(maps.Values(defaultProseCasingWords)) {
		e, err := parseCasingEntry(want)
		if err != nil {
			return nil, err
		}
		merged[e.key] = e
	}
	for _, want := range enforceCasing {
		e, err := parseCasingEntry(want)
		if err != nil {
			return nil, err
		}
		if e.key == "" {
			continue
		}
		if e.canonical == strings.ToLower(e.canonical) {
			return nil, fmt.Errorf("prose_casing: enforce_casing entry %q is already lowercase; it would suggest itself", e.canonical)
		}
		merged[e.key] = e
	}

	r := &ProseCasing{
		headingTemplates: headingTemplates,
		skipFrontmatter:  skipFrontmatter,
		severity:         severity,
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
			delete(merged, casingKey(w))
			continue
		}
		key := casingKey(word)
		if r.ignoreTargets[key] == nil {
			r.ignoreTargets[key] = make(map[string]bool)
		}
		r.ignoreTargets[key][target] = true
	}

	r.entries = merged
	r.prefixes = make(map[string]bool, len(merged))
	for _, e := range merged {
		base := strings.TrimSuffix(e.key, "'s")
		for _, b := range e.breaks {
			r.prefixes[base[:b]] = true
		}
		r.prefixes[base] = true
		r.maxWords = max(r.maxWords, len(e.breaks)+1)
	}
	return r, nil
}

func (r *ProseCasing) Name() string { return "prose_casing" }

func (r *ProseCasing) CheckFile(ctx FileCheckContext) []Result {
	if len(r.entries) == 0 {
		return nil
	}

	var results []Result
	var buf []byte
	lines := strings.Split(string(ctx.Content), "\n")
	fmEnd := -1
	if r.skipFrontmatter {
		fmEnd = frontmatterEnd(lines)
	}
	var fenceChar byte
	fenceLen := 0
	for i, raw := range lines {
		if i <= fmEnd {
			continue
		}
		if char, length, closer := fenceDelimiter(raw); length > 0 {
			switch {
			case fenceChar == 0:
				fenceChar, fenceLen = char, length
			case closer && char == fenceChar && length >= fenceLen:
				fenceChar, fenceLen = 0, 0
			}
			continue
		}
		if fenceChar != 0 {
			continue
		}
		// An unterminated inline code span can't be told apart from real
		// text by maskUnscannable (it only matches balanced pairs), so a
		// line with an odd number of backticks is skipped outright rather
		// than scanned with the span unmasked.
		if strings.Count(raw, "`")%2 != 0 {
			continue
		}
		// Gate on the unmasked line first, as the regex this scan replaced
		// did: masking is the expensive step, and most lines have no
		// candidate word at all.
		var spans []casingSpan
		if spans, buf = r.find(raw, raw, buf, spans, 1); len(spans) == 0 {
			continue
		}

		scannable := maskProseCasingLiterals(maskUnscannable(raw))
		isHeading := strings.HasPrefix(strings.TrimSpace(raw), "#")

		spans, buf = r.find(scannable, raw, buf, spans[:0], -1)
		for _, sp := range spans {
			got := raw[sp.start:sp.end]
			want := sp.entry.canonical
			// Extra spaces or a tab inside a correctly cased name render
			// the same as one space, so they are not a defect.
			if got == want || strings.Join(strings.Fields(got), " ") == want || r.isIgnored(sp.entry.key, ctx) {
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

// casingSpan is one match: raw[start:end] spells entry's key.
type casingSpan struct {
	start, end int
	entry      casingEntry
}

// find appends to spans the non-overlapping, leftmost matches in text,
// stopping after limit matches (-1 for all). text is the line to tokenize,
// possibly masked; raw is the unmasked line, used so two words separated
// by a masked region (a code span, a URL) are never read as adjacent:
// masking blanks with spaces, but only raw spaces and tabs join words.
// buf is scratch space, returned for reuse so the scan doesn't allocate.
//
// At each word, the match is extended one adjacent word at a time while
// the joined lowercase text is still a prefix some entry can reach at a
// word break (r.prefixes), and the longest extension that spells a whole
// key wins, a possessive "'s" preferred over the bare form. A word not in
// r.prefixes, the common case, costs one map lookup.
func (r *ProseCasing) find(text, raw string, buf []byte, spans []casingSpan, limit int) ([]casingSpan, []byte) {
	var breaks []int
	pos := 0
	for limit < 0 || len(spans) < limit {
		start, end := nextWord(text, pos)
		if start < 0 {
			break
		}
		pos = end
		buf = appendLower(buf[:0], text[start:end])
		if !r.prefixes[string(buf)] {
			continue
		}
		breaks = breaks[:0]
		var best casingSpan
		found := false
		for n := 1; ; n++ {
			if possessiveAt(text, end) {
				buf = append(buf, "'s"...)
				e, ok := r.entries[string(buf)]
				buf = buf[:len(buf)-2]
				if ok && breaksAllowed(breaks, e.breaks) {
					best, found = casingSpan{start, end + 2, e}, true
					break
				}
			}
			if e, ok := r.entries[string(buf)]; ok && breaksAllowed(breaks, e.breaks) {
				best, found = casingSpan{start, end, e}, true
			}
			if n == r.maxWords {
				break
			}
			next := end
			for next < len(raw) && (raw[next] == ' ' || raw[next] == '\t') {
				next++
			}
			if next == end || next >= len(text) || !isWordByte(text[next]) {
				break
			}
			_, nextEnd := nextWord(text, next)
			breaks = append(breaks, len(buf))
			buf = appendLower(buf, text[next:nextEnd])
			if !r.prefixes[string(buf)] {
				break
			}
			end = nextEnd
		}
		if found {
			spans = append(spans, best)
			pos = best.end
		}
	}
	return spans, buf
}

// nextWord returns the bounds of the first run of word bytes in s at or
// after from, or (-1, -1) if there is none.
func nextWord(s string, from int) (start, end int) {
	start = from
	for start < len(s) && !isWordByte(s[start]) {
		start++
	}
	if start == len(s) {
		return -1, -1
	}
	end = start
	for end < len(s) && isWordByte(s[end]) {
		end++
	}
	return start, end
}

// isWordByte reports whether c is in [0-9A-Za-z_], the set Go's regexp \b
// is defined against, so word bounds here match the \b-based regex this
// scan replaced.
func isWordByte(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c|0x20 && c|0x20 <= 'z'
}

// appendLower appends s to b with ASCII letters lowercased. Words are ASCII
// by construction (isWordByte), so this never changes byte length.
func appendLower(b []byte, s string) []byte {
	for i := range len(s) {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b = append(b, c)
	}
	return b
}

// possessiveAt reports whether s has "'s" (either case) at i, ending a word.
func possessiveAt(s string, i int) bool {
	return i+1 < len(s) && s[i] == '\'' && s[i+1]|0x20 == 's' && (i+2 == len(s) || !isWordByte(s[i+2]))
}

// breaksAllowed reports whether every word break in the text is also a
// break in the canonical form, so "apigateway" matches "API Gateway" but
// "data sync" does not match "DataSync".
func breaksAllowed(text, canonical []int) bool {
	for _, b := range text {
		if !slices.Contains(canonical, b) {
			return false
		}
	}
	return true
}

// isIgnored reports whether the entry with match key key is excluded for
// this target, by a bare resource name or a "type/name"-qualified one.
func (r *ProseCasing) isIgnored(key string, ctx FileCheckContext) bool {
	targets, ok := r.ignoreTargets[key]
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
		text := doc.HeadingText(raw)
		before := r.headingTemplates.Match(text)
		after := r.headingTemplates.Match(strings.Replace(text, got, want, 1))
		if before != "" && after != before {
			return fmt.Sprintf("avoid %q; add backticks around it instead of capitalizing — capitalizing would stop this heading resolving to its block", got)
		}
	}
	return fmt.Sprintf("avoid %q; use %q instead, or add backticks around it if this is a correct, lowercase technical reference", got, want)
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
