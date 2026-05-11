// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// Prefixes that indicate weak or redundant description starts.
var badDescriptionPrefixes = []string{
	"A ",
	"An ",
	"The ",
	"Indicates ",
	"Specifies ",
	"Describes ",
	"Defines ",
}

// DescriptionStyleRule checks that attribute descriptions don't start with articles or fluff words.
type DescriptionStyleRule struct{}

func (r *DescriptionStyleRule) Name() string { return "description_style" }

func (r *DescriptionStyleRule) Check(resource string, _ *schema.ResourceSchema, d *doc.Document) []Result {
	var results []Result

	results = append(results, checkDescriptions(resource, r.Name(), d.ArgumentBlocks)...)
	results = append(results, checkDescriptions(resource, r.Name(), d.AttributeBlocks)...)

	return results
}

func checkDescriptions(resource, ruleName string, blocks map[string]*doc.DocBlock) []Result {
	var results []Result

	for blockName, block := range blocks {
		for _, attr := range block.Attributes {
			if attr.Description == "" {
				continue
			}
			for _, prefix := range badDescriptionPrefixes {
				if strings.HasPrefix(attr.Description, prefix) {
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
