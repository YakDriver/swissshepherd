// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// DocAttribute represents a single documented attribute.
type DocAttribute struct {
	Name        string
	Required    bool
	Optional    bool
	Deprecated  bool
	Description string
	Line        int // 1-based line number in the source file
}

// MalformedAttr records an attribute name with a formatting issue and its location.
type MalformedAttr struct {
	Name string
	Line int
}

// DocBlock represents a documented block section.
type DocBlock struct {
	Name                string
	Heading             string
	Attributes          []DocAttribute
	MalformedAttributes []MalformedAttr // attributes found but with formatting issues
	SplitByLabel        bool            // true if the doc explicitly separates required/optional with distinct bylines
}

// HeadingTemplates defines patterns for recognizing block headings.
// Use {Block} for a snake_case block name placeholder.
// Use {Title} for a title-case name placeholder (converted to snake_case).
type HeadingTemplates []string

// Render produces a heading string from a template and block name.
// For {Block} templates, inserts the name directly.
// For {Title} templates, converts snake_case to Title Case.
// {Parent} is left as-is (caller must substitute if needed).
func RenderHeading(tmpl, blockName string) string {
	result := strings.Replace(tmpl, "{Block}", blockName, 1)
	result = strings.Replace(result, "{Title}", snakeToTitle(blockName), 1)
	return result
}

func snakeToTitle(s string) string {
	words := strings.Split(s, "_")
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// DefaultHeadingTemplates accepts common formats.
func DefaultHeadingTemplates() HeadingTemplates {
	return HeadingTemplates{
		"`{Block}` Block",
		"{Block} Block",
		"`{Block}`",
		"{Block}",
		"{Title}",
	}
}

// Match tries each template against a heading and returns the extracted block name, or "".
func (t HeadingTemplates) Match(heading string) string {
	names := t.MatchAll(heading)
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

// MatchAll returns all block names from a heading. Combined headings like
// "`publish_auth_mode` and `subscribe_auth_mode`" return both names.
func (t HeadingTemplates) MatchAll(heading string) []string {
	h := strings.TrimSpace(heading)

	// Skip example/usage headings (but not snake_case block names that happen
	// to contain "usage", e.g. "usage_based_pricing_term Block").
	lower := strings.ToLower(h)
	if (strings.Contains(lower, "example") || strings.Contains(lower, "usage")) && !strings.Contains(h, "_") {
		return nil
	}

	// Try combined "X and Y" pattern first.
	if parts := splitCombinedHeading(h); len(parts) > 1 {
		var names []string
		for _, part := range parts {
			for _, tmpl := range t {
				if name := matchTemplate(tmpl, part); name != "" {
					names = append(names, name)
					break
				}
			}
		}
		if len(names) == len(parts) {
			return names
		}
	}

	for _, tmpl := range t {
		if name := matchTemplate(tmpl, h); name != "" {
			return []string{name}
		}
	}
	return nil
}

// splitCombinedHeading splits headings like "X and Y" or "X, Y, and Z" into parts.
func splitCombinedHeading(heading string) []string {
	if !strings.Contains(heading, " and ") {
		return nil
	}
	var result []string
	for p := range strings.SplitSeq(heading, " and ") {
		p = strings.TrimSpace(p)
		if strings.Contains(p, ", ") || strings.HasSuffix(p, ",") {
			for sub := range strings.SplitSeq(p, ", ") {
				sub = strings.TrimRight(strings.TrimSpace(sub), ",")
				if sub != "" {
					result = append(result, sub)
				}
			}
		} else if p != "" {
			result = append(result, p)
		}
	}
	if len(result) < 2 {
		return nil
	}
	return result
}

// matchTemplate tries to match a single template against a heading.
func matchTemplate(tmpl, heading string) string {
	// {Block} matches a snake_case name (no spaces, lowercase with underscores)
	// {Title} matches title-case words (converted to snake_case)
	// {Parent} matches a snake_case name (used as disambiguator, value is discarded)

	// Handle {Parent} by replacing it with a greedy snake_case match.
	if strings.Contains(tmpl, "{Parent}") {
		// Strip backticks from template, split on {Parent}
		clean := strings.ReplaceAll(tmpl, "`", "")
		before, after, _ := strings.Cut(clean, "{Parent}")
		before = strings.TrimSpace(before)

		if before != "" && !strings.HasPrefix(heading, before) {
			return ""
		}
		rest := heading[len(before):]
		rest = strings.TrimSpace(rest)

		// The after portion contains {Block} and possibly a suffix like "Block".
		// Extract the suffix after {Block}.
		afterClean := strings.TrimSpace(strings.ReplaceAll(after, "`", ""))
		_, blockSuffix, _ := strings.Cut(afterClean, "{Block}")
		blockSuffix = strings.TrimSpace(blockSuffix)

		// Strip the suffix from the heading remainder to isolate "parent... block_name"
		candidate := rest
		if blockSuffix != "" {
			if !strings.HasSuffix(candidate, blockSuffix) {
				return ""
			}
			candidate = strings.TrimSuffix(candidate, blockSuffix)
			candidate = strings.TrimSpace(candidate)
		}

		// The last word is the block name; everything before is the parent.
		lastSpace := strings.LastIndexByte(candidate, ' ')
		if lastSpace < 0 {
			return ""
		}
		parent := candidate[:lastSpace]
		blockName := candidate[lastSpace+1:]

		// Validate: parent must be all lowercase words, block must be snake_case
		if parent == "" || blockName == "" {
			return ""
		}
		if blockName != strings.ToLower(blockName) || strings.Contains(blockName, " ") {
			return ""
		}
		for word := range strings.FieldsSeq(parent) {
			if word != strings.ToLower(word) {
				return ""
			}
		}

		// Return composite key with enough parent context to disambiguate
		// For "a b c block_name", prefer "b.c.block_name" if multiple parents exist
		// to handle cases like "dest s3 format config" vs "dest upsolver format config"
		parentWords := strings.Fields(parent)
		if len(parentWords) >= 2 {
			// Use last two parent words for disambiguation
			return parentWords[len(parentWords)-2] + "." + parentWords[len(parentWords)-1] + "." + blockName
		}
		// Single parent: use it directly
		return parentWords[0] + "." + blockName
	}

	if strings.Contains(tmpl, "{Block}") {
		prefix, suffix, _ := strings.Cut(tmpl, "{Block}")
		// Goldmark strips backticks from inline code, so "`{Block}` Block" becomes "{Block} Block" in practice
		prefix = strings.ReplaceAll(prefix, "`", "")
		suffix = strings.ReplaceAll(suffix, "`", "")

		if !strings.HasPrefix(heading, prefix) || !strings.HasSuffix(heading, suffix) {
			return ""
		}
		name := heading[len(prefix) : len(heading)-len(suffix)]
		name = strings.TrimSpace(name)
		if name == "" || strings.Contains(name, " ") {
			return ""
		}
		// Must look like snake_case
		if name != strings.ToLower(name) {
			return ""
		}
		return name
	}

	if strings.Contains(tmpl, "{Title}") {
		prefix, suffix, _ := strings.Cut(tmpl, "{Title}")
		prefix = strings.ReplaceAll(prefix, "`", "")
		suffix = strings.ReplaceAll(suffix, "`", "")

		if !strings.HasPrefix(heading, prefix) || !strings.HasSuffix(heading, suffix) {
			return ""
		}
		title := heading[len(prefix) : len(heading)-len(suffix)]
		title = strings.TrimSpace(title)
		if title == "" {
			return ""
		}
		// Must start with uppercase (title case) to distinguish from bare snake_case
		if title[0] < 'A' || title[0] > 'Z' {
			return ""
		}
		return titleToSnake(title)
	}

	return ""
}

// Document represents a parsed documentation file.
type Document struct {
	ResourceName    string
	ArgumentBlocks  map[string]*DocBlock
	AttributeBlocks map[string]*DocBlock
	Sections        *Sections
	source          []byte
}

// Source returns the raw markdown source bytes.
func (d *Document) Source() []byte { return d.source }

// Blocks returns a merged view of argument + attribute blocks.
// The returned map is independent — it does not mutate the original blocks.
func (d *Document) Blocks() map[string]*DocBlock {
	merged := make(map[string]*DocBlock, len(d.ArgumentBlocks)+len(d.AttributeBlocks))
	for k, v := range d.ArgumentBlocks {
		clone := *v
		clone.Attributes = append([]DocAttribute(nil), v.Attributes...)
		merged[k] = &clone
	}
	for k, v := range d.AttributeBlocks {
		if existing, ok := merged[k]; ok {
			existing.Attributes = append(existing.Attributes, v.Attributes...)
		} else {
			merged[k] = v
		}
	}
	return merged
}

// ParseFile reads and parses a markdown documentation file (accepts all heading styles).
func ParseFile(path string) (*Document, error) {
	return ParseFileWithTemplates(path, DefaultHeadingTemplates())
}

// ParseFileWithTemplates reads and parses with specific heading templates.
func ParseFileWithTemplates(path string, templates HeadingTemplates) (*Document, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading doc file: %w", err)
	}
	return ParseWithTemplates(source, path, templates)
}

// Parse parses markdown source (accepts all heading styles).
func Parse(source []byte, name string) (*Document, error) {
	return ParseWithTemplates(source, name, DefaultHeadingTemplates())
}

// ParseWithTemplates parses markdown source with specific heading templates.
func ParseWithTemplates(source []byte, name string, templates HeadingTemplates) (*Document, error) {
	md := goldmark.New()
	reader := text.NewReader(source)
	tree := md.Parser().Parse(reader)

	doc := &Document{
		ResourceName:    name,
		ArgumentBlocks:  make(map[string]*DocBlock),
		AttributeBlocks: make(map[string]*DocBlock),
		Sections:        &Sections{},
		source:          source,
	}

	extractBlocks(tree, source, doc, templates)
	return doc, nil
}

func extractBlocks(tree ast.Node, source []byte, doc *Document, templates HeadingTemplates) {
	var currentBlockName string
	var currentBlockAliases []string
	var currentSection *Section
	var inArguments, inAttributes bool
	var sawRequiredByline bool // true between a "required:" byline and the next list

	// closeSection finalizes the current section's EndOffset.
	closeSection := func(endOffset int) {
		if currentSection != nil && currentSection.EndOffset == 0 {
			currentSection.EndOffset = endOffset
		}
	}

	// assignSection records a freshly discovered top-level section and makes it
	// the current accumulator target for paragraphs and code blocks that follow.
	// Only the first occurrence of each section is captured — duplicate headings
	// keep pointing at the first one so rules can still reason about "the"
	// section without the walker silently replacing it.
	assignSection := func(field **Section, heading *ast.Heading, text string) {
		startOff := 0
		if lines := heading.Lines(); lines.Len() > 0 {
			// Lines().At(0).Start is the content start (after "## ").
			// Walk backwards to find the actual line start (the # character).
			startOff = lines.At(0).Start
			for startOff > 0 && source[startOff-1] != '\n' {
				startOff--
			}
		}
		closeSection(startOff)
		if *field == nil {
			*field = &Section{Heading: heading, Text: text, StartOffset: startOff}
		}
		currentSection = *field
	}

	_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n := node.(type) {
		case *ast.Heading:
			headingText := string(n.Text(source))

			if n.Level == 1 {
				assignSection(&doc.Sections.Title, n, headingText)
				inArguments = false
				inAttributes = false
				sawRequiredByline = false
				return ast.WalkSkipChildren, nil
			}

			if n.Level == 2 {
				inArguments = strings.HasPrefix(headingText, "Argument")
				inAttributes = strings.HasPrefix(headingText, "Attribute")
				sawRequiredByline = false

				switch {
				case inArguments:
					currentBlockName = ""
					currentBlockAliases = nil
					ensureBlock(doc.ArgumentBlocks, "", headingText)
					assignSection(&doc.Sections.Arguments, n, headingText)
				case inAttributes:
					currentBlockName = ""
					currentBlockAliases = nil
					ensureBlock(doc.AttributeBlocks, "", headingText)
					assignSection(&doc.Sections.Attributes, n, headingText)
				case strings.HasPrefix(headingText, "Example"):
					assignSection(&doc.Sections.Example, n, headingText)
				case strings.HasPrefix(headingText, "Timeout"):
					assignSection(&doc.Sections.Timeouts, n, headingText)
				case strings.HasPrefix(headingText, "Import"):
					assignSection(&doc.Sections.Import, n, headingText)
				case strings.HasPrefix(headingText, "Signature"):
					assignSection(&doc.Sections.Signature, n, headingText)
				default:
					// Unknown level-2 section — stop accumulating into any
					// recognized section until the next known heading.
					currentSection = nil
				}
				return ast.WalkSkipChildren, nil
			}

			if n.Level >= 3 && (inArguments || inAttributes) {
				blockNames := templates.MatchAll(headingText)
				if len(blockNames) > 0 {
					currentBlockName = blockNames[0]
					sawRequiredByline = false
					for _, bn := range blockNames {
						if inArguments {
							ensureBlock(doc.ArgumentBlocks, bn, headingText)
						} else {
							ensureBlock(doc.AttributeBlocks, bn, headingText)
						}
					}
					currentBlockAliases = blockNames[1:]
				}
				if currentSection != nil {
					currentSection.ChildHeadings = append(currentSection.ChildHeadings, ChildHeading{Level: n.Level, Text: headingText})
				}
				return ast.WalkSkipChildren, nil
			}

			// Level >= 3 heading outside arguments/attributes — still record
			// as a child heading of the current section (e.g. ### Basic Usage
			// inside ## Example Usage).
			if n.Level >= 3 && currentSection != nil {
				currentSection.ChildHeadings = append(currentSection.ChildHeadings, ChildHeading{Level: n.Level, Text: headingText})
			}

			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock:
			if currentSection != nil {
				currentSection.FencedCodeBlocks = append(currentSection.FencedCodeBlocks, n)
			}
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			if currentSection != nil {
				currentSection.Paragraphs = append(currentSection.Paragraphs, n)
			}
			// Detect "required:" / "optional:" bylines preceding lists.
			// "required:" sets sawRequiredByline; the next list-bearing block
			// will be marked split.
			if inArguments {
				paraText := strings.ToLower(strings.TrimSpace(string(n.Text(source))))
				if strings.Contains(paraText, "arguments are required:") {
					sawRequiredByline = true
				}
			}
			return ast.WalkSkipChildren, nil

		case *ast.List:
			if !inArguments && !inAttributes {
				// Capture list items into the current section (e.g. Timeouts).
				if currentSection != nil {
					for child := n.FirstChild(); child != nil; child = child.NextSibling() {
						if li, ok := child.(*ast.ListItem); ok {
							if item := parseSectionListItem(li, source); item.Name != "" {
								item.Line = nodeLineNumber(li, source)
								currentSection.ListItems = append(currentSection.ListItems, item)
							}
						}
					}
				}
				return ast.WalkSkipChildren, nil
			}

			target := doc.ArgumentBlocks
			if inAttributes {
				target = doc.AttributeBlocks
			}

			block := target[currentBlockName]
			if block == nil {
				return ast.WalkSkipChildren, nil
			}

			// If we saw a "required:" byline before this list, mark the block as split.
			if inArguments && sawRequiredByline {
				block.SplitByLabel = true
				sawRequiredByline = false
			}

			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				if li, ok := child.(*ast.ListItem); ok {
					line := nodeLineNumber(li, source)
					attr := parseListItem(li, source)
					if attr.Name != "" {
						attr.Line = line
						block.Attributes = append(block.Attributes, attr)
						// Flag attributes with malformed separator (e.g. `mode`- instead of `mode` -).
						if hasMalformedSeparator(li, source, attr.Name) {
							block.MalformedAttributes = append(block.MalformedAttributes, MalformedAttr{Name: attr.Name, Line: line})
						}
						// Mirror to alias blocks (combined headings).
						for _, alias := range currentBlockAliases {
							if ab := target[alias]; ab != nil {
								ab.Attributes = append(ab.Attributes, attr)
							}
						}
					} else if name := malformedAttrName(li, source); name != "" {
						block.MalformedAttributes = append(block.MalformedAttributes, MalformedAttr{Name: name, Line: line})
					}
				}
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	// Finalize the last section's EndOffset.
	closeSection(len(source))
}

// parseSectionListItem extracts a name/value pair from a list item in a
// non-argument section (e.g. Timeouts: * `create` - (Default `60m`)).
func parseSectionListItem(li *ast.ListItem, source []byte) SectionListItem {
	var rawText string
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.TextBlock:
			rawText = string(c.Text(source))
		case *ast.Paragraph:
			rawText = string(c.Text(source))
		}
		if rawText != "" {
			break
		}
	}
	if rawText == "" {
		return SectionListItem{}
	}

	parts := strings.SplitN(rawText, " - ", 2)
	if len(parts) < 1 {
		return SectionListItem{}
	}
	name := strings.Trim(strings.TrimSpace(parts[0]), "`")
	if name == "" || strings.Contains(name, " ") {
		return SectionListItem{}
	}
	var value string
	if len(parts) == 2 {
		value = strings.TrimSpace(parts[1])
	}
	return SectionListItem{Name: name, Value: value}
}

// hasMalformedSeparator checks if the raw source for a list item has a
// backtick-dash pattern (`name`- ) instead of the correct `name` - format.
// nodeLineNumber returns the 1-based line number of a block node by inspecting
// its first line segment or recursing into its first child.
func nodeLineNumber(n ast.Node, source []byte) int {
	if lines := n.Lines(); lines.Len() > 0 {
		offset := lines.At(0).Start
		return bytes.Count(source[:offset], []byte{'\n'}) + 1
	}
	// ListItem often has no direct lines; check first child.
	if fc := n.FirstChild(); fc != nil {
		if lines := fc.Lines(); lines.Len() > 0 {
			offset := lines.At(0).Start
			return bytes.Count(source[:offset], []byte{'\n'}) + 1
		}
	}
	return 0
}

func hasMalformedSeparator(li *ast.ListItem, source []byte, name string) bool {
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		var raw string
		switch c := child.(type) {
		case *ast.TextBlock:
			raw = string(c.Text(source))
		case *ast.Paragraph:
			raw = string(c.Text(source))
		}
		if raw != "" {
			// Look for `name`- (no space between closing backtick and dash)
			return strings.Contains(raw, "`"+name+"`-")
		}
	}
	return false
}

func ensureBlock(blocks map[string]*DocBlock, name, heading string) {
	if _, exists := blocks[name]; !exists {
		blocks[name] = &DocBlock{Name: name, Heading: heading}
	}
}

// titleToSnake converts "Credit Specification" → "credit_specification".
func titleToSnake(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "_")
}

// parseListItem extracts an attribute from a list item.
func parseListItem(li *ast.ListItem, source []byte) DocAttribute {
	var rawText string
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.TextBlock:
			rawText = string(c.Text(source))
		case *ast.Paragraph:
			rawText = string(c.Text(source))
		}
		if rawText != "" {
			break
		}
	}

	if rawText == "" {
		return DocAttribute{}
	}

	// Primary separator is " - " (with spaces). Also accept "`- " which appears
	// when authors omit the space before the dash: `mode`- (Required) ...
	sep := " - "
	if !strings.Contains(rawText, sep) && strings.Contains(rawText, "`- ") {
		sep = "`- "
		// Re-attach the backtick to the name side so trimming works correctly.
		rawText = strings.Replace(rawText, "`- ", "` - ", 1)
	}
	parts := strings.SplitN(rawText, " - ", 2)
	if len(parts) < 1 {
		return DocAttribute{}
	}

	name := strings.Trim(strings.TrimSpace(parts[0]), "`")
	name = strings.TrimRight(name, "*")

	if name == "" || strings.Contains(name, " ") {
		return DocAttribute{}
	}

	// Names with dots or brackets are dot-notation references, not attribute names.
	if strings.ContainsAny(name, ".[]*") {
		return DocAttribute{}
	}

	attr := DocAttribute{Name: name}

	if len(parts) == 2 {
		desc := parts[1]
		if strings.HasPrefix(desc, "(") {
			end := strings.IndexByte(desc, ')')
			if end > 0 {
				traits := desc[1:end]
				for trait := range strings.SplitSeq(traits, ", ") {
					switch strings.TrimSpace(trait) {
					case "Required":
						attr.Required = true
					case "Optional":
						attr.Optional = true
					case "Deprecated", "**Deprecated**":
						attr.Deprecated = true
					default:
						if strings.Contains(trait, "Deprecated") {
							attr.Deprecated = true
						}
					}
				}
				attr.Description = strings.TrimSpace(desc[end+1:])
			}
		} else {
			attr.Description = strings.TrimSpace(desc)
		}
	}

	return attr
}

// malformedAttrName detects list items that look like attributes but are missing
// the standard " - " separator (e.g., using no dash, em-dash, or missing space).
func malformedAttrName(li *ast.ListItem, source []byte) string {
	var rawText string
	for child := li.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *ast.TextBlock:
			rawText = string(c.Text(source))
		case *ast.Paragraph:
			rawText = string(c.Text(source))
		}
		if rawText != "" {
			break
		}
	}
	if rawText == "" {
		return ""
	}

	// Already has " - " — parseListItem should have handled it
	if strings.Contains(rawText, " - ") {
		return ""
	}

	// Look for pattern: name (Required|Optional) or name – (with em-dash)
	// Extract potential name (first word, no spaces, looks like snake_case)
	parts := strings.Fields(rawText)
	if len(parts) < 2 {
		return ""
	}
	// Strip surrounding backticks and a trailing dash that appears when the
	// author writes `name`- with no space before the dash.
	name := strings.Trim(parts[0], "`")
	name = strings.TrimRight(name, "-")
	name = strings.TrimRight(name, "`")
	if name == "" || strings.Contains(name, " ") || name != strings.ToLower(name) {
		return ""
	}
	if strings.ContainsAny(name, ".[]*`") {
		return ""
	}

	// Check if what follows looks like (Required), (Optional), or a dash variant
	rest := rawText[len(parts[0]):]
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "(") || strings.HasPrefix(rest, "\u2013") || strings.HasPrefix(rest, "\u2014") {
		return name
	}
	// "- (" with missing leading space
	if strings.HasPrefix(rest, "-(") || strings.HasPrefix(rest, "- (") {
		return name
	}

	return ""
}
