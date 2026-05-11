// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// Attribute represents a single schema attribute with its properties.
type Attribute struct {
	Name       string
	Required   bool
	Optional   bool
	Computed   bool
	Deprecated bool
	Sensitive  bool
}

// Block represents a flattened schema block with its attributes and child block names.
type Block struct {
	Path        string
	Attributes  []Attribute
	ChildBlocks []string // names of immediate child blocks
}

// ResourceSchema holds the flattened block map for a single resource.
type ResourceSchema struct {
	Name   string
	Blocks map[string]*Block // keyed by dot-path (e.g., "", "rule", "rule.action")
}

// ProviderSchema holds all resource and data source schemas.
type ProviderSchema struct {
	Resources   map[string]*ResourceSchema
	DataSources map[string]*ResourceSchema
}

// LoadFile reads a terraform providers schema -json file and returns the parsed schemas.
func LoadFile(path string, providerSource string) (*ProviderSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schema file: %w", err)
	}

	var ps tfjson.ProviderSchemas
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("parsing schema JSON: %w", err)
	}

	provider := findProvider(&ps, providerSource)
	if provider == nil {
		return nil, fmt.Errorf("provider %q not found in schema", providerSource)
	}

	result := &ProviderSchema{
		Resources:   make(map[string]*ResourceSchema, len(provider.ResourceSchemas)),
		DataSources: make(map[string]*ResourceSchema, len(provider.DataSourceSchemas)),
	}

	for name, s := range provider.ResourceSchemas {
		result.Resources[name] = flattenSchema(name, s)
	}
	for name, s := range provider.DataSourceSchemas {
		result.DataSources[name] = flattenSchema(name, s)
	}

	return result, nil
}

func findProvider(ps *tfjson.ProviderSchemas, source string) *tfjson.ProviderSchema {
	if ps.Schemas == nil {
		return nil
	}
	if p, ok := ps.Schemas[source]; ok {
		return p
	}
	// Try short name (last segment)
	parts := strings.Split(source, "/")
	short := parts[len(parts)-1]
	if p, ok := ps.Schemas[short]; ok {
		return p
	}
	return nil
}

func flattenSchema(name string, s *tfjson.Schema) *ResourceSchema {
	rs := &ResourceSchema{
		Name:   name,
		Blocks: make(map[string]*Block),
	}
	if s.Block != nil {
		flattenBlock(rs, "", s.Block)
	}
	return rs
}

func flattenBlock(rs *ResourceSchema, path string, block *tfjson.SchemaBlock) {
	b := &Block{
		Path:       path,
		Attributes: make([]Attribute, 0, len(block.Attributes)),
	}

	for attrName, attr := range block.Attributes {
		b.Attributes = append(b.Attributes, Attribute{
			Name:       attrName,
			Required:   attr.Required,
			Optional:   attr.Optional,
			Computed:   attr.Computed,
			Deprecated: attr.Deprecated,
			Sensitive:  attr.Sensitive,
		})
	}

	for childName := range block.NestedBlocks {
		b.ChildBlocks = append(b.ChildBlocks, childName)
	}

	rs.Blocks[path] = b

	for childName, childBlock := range block.NestedBlocks {
		childPath := childName
		if path != "" {
			childPath = path + "." + childName
		}
		if childBlock.Block != nil {
			flattenBlock(rs, childPath, childBlock.Block)
		}
	}
}
