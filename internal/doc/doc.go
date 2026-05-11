// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package doc

import (
	"fmt"
	"maps"
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
	Description string
}

// DocBlock represents a documented block section.
type DocBlock struct {
	Name       string
	Heading    string
	Attributes []DocAttribute
}

// HeadingTemplates defines patterns for recognizing block headings.
// Use {Block} for a snake_case block name placeholder.
// Use {Title} for a title-case name placeholder (converted to snake_case).
type HeadingTemplates []string

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
	h := strings.TrimSpace(heading)

	// Skip example/usage headings
	lower := strings.ToLower(h)
	if strings.Contains(lower, "example") || strings.Contains(lower, "usage") {
		return ""
	}

	for _, tmpl := range t {
		if name := matchTemplate(tmpl, h); name != "" {
			return name
		}
	}
	return ""
}

// matchTemplate tries to match a single template against a heading.
func matchTemplate(tmpl, heading string) string {
	// {Block} matches a snake_case name (no spaces, lowercase with underscores)
	// {Title} matches title-case words (converted to snake_case)

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
		// For bare {Title} (no suffix/prefix), require multiple words to avoid
		// matching single capitalized words that may not be block names.
		if suffix == "" && prefix == "" && !strings.Contains(title, " ") {
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
}

// Blocks returns a merged view of argument + attribute blocks.
func (d *Document) Blocks() map[string]*DocBlock {
	merged := make(map[string]*DocBlock, len(d.ArgumentBlocks)+len(d.AttributeBlocks))
	maps.Copy(merged, d.ArgumentBlocks)
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
	}

	extractBlocks(tree, source, doc, templates)
	return doc, nil
}

func extractBlocks(tree ast.Node, source []byte, doc *Document, templates HeadingTemplates) {
	var currentBlockName string
	var inArguments, inAttributes bool

	_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch n := node.(type) {
		case *ast.Heading:
			headingText := string(n.Text(source))

			if n.Level == 2 {
				inArguments = strings.HasPrefix(headingText, "Argument")
				inAttributes = strings.HasPrefix(headingText, "Attribute")

				if inArguments {
					currentBlockName = ""
					ensureBlock(doc.ArgumentBlocks, "", headingText)
				} else if inAttributes {
					currentBlockName = ""
					ensureBlock(doc.AttributeBlocks, "", headingText)
				} else {
					inArguments = false
					inAttributes = false
				}
				return ast.WalkSkipChildren, nil
			}

			if n.Level >= 3 && (inArguments || inAttributes) {
				blockName := templates.Match(headingText)
				if blockName != "" {
					currentBlockName = blockName
					if inArguments {
						ensureBlock(doc.ArgumentBlocks, blockName, headingText)
					} else {
						ensureBlock(doc.AttributeBlocks, blockName, headingText)
					}
				}
				return ast.WalkSkipChildren, nil
			}

			return ast.WalkSkipChildren, nil

		case *ast.List:
			if !inArguments && !inAttributes {
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

			for child := n.FirstChild(); child != nil; child = child.NextSibling() {
				if li, ok := child.(*ast.ListItem); ok {
					attr := parseListItem(li, source)
					if attr.Name != "" {
						block.Attributes = append(block.Attributes, attr)
					}
				}
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})
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
