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
	Description string // text after the (Required/Optional) marker
}

// DocBlock represents a documented block section (## Argument Reference or ### block_name Block).
type DocBlock struct {
	Name       string         // block name (empty string for root)
	Heading    string         // raw heading text
	Attributes []DocAttribute // attributes listed in this section
}

// Document represents a parsed documentation file.
type Document struct {
	ResourceName    string
	ArgumentBlocks  map[string]*DocBlock // blocks under ## Argument Reference
	AttributeBlocks map[string]*DocBlock // blocks under ## Attribute Reference
}

// Blocks returns a merged view of argument + attribute blocks (for backward compat).
func (d *Document) Blocks() map[string]*DocBlock {
	merged := make(map[string]*DocBlock, len(d.ArgumentBlocks)+len(d.AttributeBlocks))
	maps.Copy(merged, d.ArgumentBlocks)
	for k, v := range d.AttributeBlocks {
		if existing, ok := merged[k]; ok {
			// Merge attributes into existing block
			existing.Attributes = append(existing.Attributes, v.Attributes...)
		} else {
			merged[k] = v
		}
	}
	return merged
}

// ParseFile reads and parses a markdown documentation file.
func ParseFile(path string) (*Document, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading doc file: %w", err)
	}
	return Parse(source, path)
}

// Parse parses markdown source into a Document.
func Parse(source []byte, name string) (*Document, error) {
	md := goldmark.New()
	reader := text.NewReader(source)
	tree := md.Parser().Parse(reader)

	doc := &Document{
		ResourceName:    name,
		ArgumentBlocks:  make(map[string]*DocBlock),
		AttributeBlocks: make(map[string]*DocBlock),
	}

	extractBlocks(tree, source, doc)
	return doc, nil
}

// extractBlocks walks the AST and extracts argument/attribute sections.
func extractBlocks(tree ast.Node, source []byte, doc *Document) {
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

			if n.Level == 3 && (inArguments || inAttributes) {
				blockName := extractBlockName(headingText)
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

// extractBlockName extracts the block name from a heading like:
//   - "`rule` Block"
//   - "Credit Specification"
//   - "CPU Options"
//   - "Network Interfaces"
func extractBlockName(heading string) string {
	h := strings.TrimSpace(heading)

	// Remove common suffixes
	h = strings.TrimSuffix(h, " Block")
	h = strings.TrimSuffix(h, " block")

	// Remove backticks — if present, it's already snake_case
	if strings.Contains(h, "`") {
		h = strings.Trim(h, "`")
		if !strings.Contains(h, " ") {
			return h
		}
	}

	// If it contains spaces, convert title case to snake_case
	if strings.Contains(h, " ") {
		return titleToSnake(h)
	}

	return h
}

// titleToSnake converts "Credit Specification" → "credit_specification",
// "CPU Options" → "cpu_options", "EBS Block Device" → "ebs_block_device".
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

// parseListItem extracts an attribute from a list item like:
// `name` - (Required) Description...
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

	// Names with dots or brackets are dot-notation references (e.g., "block.*.attr"),
	// not actual attribute names in this block.
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
				// Description is everything after the closing paren + space
				attr.Description = strings.TrimSpace(desc[end+1:])
			}
		} else {
			attr.Description = strings.TrimSpace(desc)
		}
	}

	return attr
}
