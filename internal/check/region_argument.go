// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"slices"
)

// RegionArgumentRule validates that region-aware resources document the
// "region" argument. This is provider-specific (primarily AWS) and only
// fires for types with RegionAware = true.
type RegionArgumentRule struct {
	// IgnoreResources lists resources that should not be checked for region.
	IgnoreResources []string
}

func (r *RegionArgumentRule) Name() string { return "region_argument" }

func (r *RegionArgumentRule) Check(ctx CheckContext) []Result {
	if ctx.Type == nil || !ctx.Type.RegionAware {
		return nil
	}
	if ctx.Schema == nil || ctx.Doc == nil {
		return nil
	}
	if slices.Contains(r.IgnoreResources, ctx.Resource) {
		return nil
	}

	// Check if schema has a "region" attribute.
	rootBlock := ctx.Schema.Blocks[""]
	if rootBlock == nil {
		return nil
	}
	hasRegionInSchema := false
	for _, attr := range rootBlock.Attributes {
		if attr.Name == "region" && (attr.Optional || attr.Required) {
			hasRegionInSchema = true
			break
		}
	}
	if !hasRegionInSchema {
		return nil
	}

	// Check if doc has "region" in argument blocks.
	for _, block := range ctx.Doc.ArgumentBlocks {
		for _, attr := range block.Attributes {
			if attr.Name == "region" {
				return nil
			}
		}
	}

	return []Result{{
		Rule: r.Name(), Resource: ctx.Resource, Severity: SeverityError,
		Message: fmt.Sprintf("schema has \"region\" attribute but it is not documented in Argument Reference"),
	}}
}
