// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/doc"
)

// DefaultBadDescriptionPrefixes is the list of weak or redundant description
// starts. Overridable via check "description_style" { bad_prefixes = [...] }.
var DefaultBadDescriptionPrefixes = []string{
	"A ",
	"An ",
	"The ",
	"Indicates ",
	"Specifies ",
	"Describes ",
	"Defines ",
}

// DescriptionStyleRule checks that attribute descriptions don't start with articles or fluff words.
type DescriptionStyleRule struct {
	// BadPrefixes overrides DefaultBadDescriptionPrefixes when non-nil.
	BadPrefixes []string
}

func (r *DescriptionStyleRule) prefixes() []string {
	if r.BadPrefixes != nil {
		return r.BadPrefixes
	}
	return DefaultBadDescriptionPrefixes
}

func (r *DescriptionStyleRule) Name() string { return "description_style" }

func (r *DescriptionStyleRule) Check(ctx CheckContext) []Result {
	resource, d := ctx.Resource, ctx.Doc
	seen := make(map[string]bool)
	var results []Result

	results = append(results, checkDescriptions(resource, r.Name(), r.prefixes(), d.ArgumentBlocks, seen)...)
	results = append(results, checkDescriptions(resource, r.Name(), r.prefixes(), d.AttributeBlocks, seen)...)

	return results
}

func checkDescriptions(resource, ruleName string, prefixes []string, blocks map[string]*doc.DocBlock, seen map[string]bool) []Result {
	var results []Result

	for blockName, block := range blocks {
		for _, attr := range block.Attributes {
			if attr.Description == "" {
				continue
			}
			key := blockName + "." + attr.Name
			if seen[key] {
				continue
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(attr.Description, prefix) {
					seen[key] = true
					results = append(results, Result{
						Rule:     ruleName,
						Resource: resource,
						Severity: SeverityError,
						Message:  fmt.Sprintf("attribute %q description should not start with %q (block %q)", attr.Name, strings.TrimSpace(prefix), displayPath(blockName)),
						Block:    blockName,
					})
					break
				}
			}
		}
	}

	return results
}
