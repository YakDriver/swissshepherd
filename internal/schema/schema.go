// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// Schema-kind identifiers used to select the right target map on
// ProviderSchema. They match the schema_kind values declared in
// internal/config/defaults.hcl.
const (
	KindResource     = "resource"
	KindDataSource   = "data_source"
	KindEphemeral    = "ephemeral"
	KindListResource = "list_resource"
	KindAction       = "action"
	KindFunction     = "function"
	KindNone         = "none" // content-only categories (guides, index)
)

// BlockKinds are the schema kinds whose targets carry a SchemaBlock and can
// therefore be represented as *ResourceSchema. KindFunction is intentionally
// excluded — functions have signatures, not blocks.
var BlockKinds = []string{
	KindResource,
	KindDataSource,
	KindEphemeral,
	KindListResource,
	KindAction,
}

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

// ResourceSchema holds the flattened block map for a single block-based
// target (resource, data source, ephemeral, list resource, or action).
type ResourceSchema struct {
	Name   string
	Blocks map[string]*Block // keyed by dot-path (e.g., "", "rule", "rule.action")
}

// FunctionSchema holds the minimal representation of a provider function.
// Full signature data (return type, variadic parameters) is captured on
// demand; we record only what rules consume today — the function's
// positional parameter names and descriptions. Expand when a function-aware
// rule needs more.
type FunctionSchema struct {
	Name              string
	Description       string
	ParameterNames    []string
	VariadicParameter string // empty when none
}

// ProviderSchema holds every target the provider exposes, grouped by
// schema kind. Kinds with no entries in the provider have empty (but
// non-nil) maps so callers can iterate safely.
type ProviderSchema struct {
	Resources       map[string]*ResourceSchema
	DataSources     map[string]*ResourceSchema
	Ephemerals      map[string]*ResourceSchema
	ListResources   map[string]*ResourceSchema
	Actions         map[string]*ResourceSchema
	Functions       map[string]*FunctionSchema
	IdentitySchemas map[string]*IdentitySchema
}

// IdentityAttribute describes a single attribute in a resource identity schema.
type IdentityAttribute struct {
	Name     string
	Type     string
	Required bool // required_for_import
}

// IdentitySchema holds the identity attributes for a resource that supports
// import-by-identity (Terraform v1.12+).
type IdentitySchema struct {
	Attributes []IdentityAttribute
}

// TargetNames returns the sorted set of target names for the given schema
// kind. Unknown kinds (including KindNone) yield an empty slice. Output is
// sorted so test assertions and log output are stable.
func (ps *ProviderSchema) TargetNames(kind string) []string {
	switch kind {
	case KindResource:
		return sortedKeys(ps.Resources)
	case KindDataSource:
		return sortedKeys(ps.DataSources)
	case KindEphemeral:
		return sortedKeys(ps.Ephemerals)
	case KindListResource:
		return sortedKeys(ps.ListResources)
	case KindAction:
		return sortedKeys(ps.Actions)
	case KindFunction:
		return sortedFuncKeys(ps.Functions)
	}
	return nil
}

// ResourceSchemaFor returns the flattened block schema for a named target
// of a block-kind category, or nil when: the kind is unknown, the kind is
// KindFunction or KindNone (no block schema to return), or the target name
// does not exist in that kind.
func (ps *ProviderSchema) ResourceSchemaFor(kind, name string) *ResourceSchema {
	var m map[string]*ResourceSchema
	switch kind {
	case KindResource:
		m = ps.Resources
	case KindDataSource:
		m = ps.DataSources
	case KindEphemeral:
		m = ps.Ephemerals
	case KindListResource:
		m = ps.ListResources
	case KindAction:
		m = ps.Actions
	default:
		return nil
	}
	return m[name]
}

// Function returns the function with the given name, or nil when absent.
func (ps *ProviderSchema) Function(name string) *FunctionSchema {
	return ps.Functions[name]
}

// LoadFile reads a `terraform providers schema -json` file and returns the
// parsed schemas for every target category Terraform currently exposes.
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
		Resources:       flattenBlockSchemas(provider.ResourceSchemas),
		DataSources:     flattenBlockSchemas(provider.DataSourceSchemas),
		Ephemerals:      flattenBlockSchemas(provider.EphemeralResourceSchemas),
		ListResources:   flattenBlockSchemas(provider.ListResourceSchemas),
		Actions:         flattenActionSchemas(provider.ActionSchemas),
		Functions:       flattenFunctions(provider.Functions),
		IdentitySchemas: flattenIdentitySchemas(provider.ResourceIdentitySchemas),
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

// flattenBlockSchemas processes any map of *tfjson.Schema (resources, data
// sources, ephemerals, list resources) through a single path. Returns an
// empty map when the provider has none of this kind.
func flattenBlockSchemas(in map[string]*tfjson.Schema) map[string]*ResourceSchema {
	out := make(map[string]*ResourceSchema, len(in))
	for name, s := range in {
		out[name] = flattenSchema(name, s)
	}
	return out
}

// flattenActionSchemas adapts tfjson's ActionSchema (which wraps a
// SchemaBlock without version) into the common ResourceSchema shape. Actions
// live in their own tfjson type but are structurally block-based, so it
// makes sense for rules to treat them uniformly.
func flattenActionSchemas(in map[string]*tfjson.ActionSchema) map[string]*ResourceSchema {
	out := make(map[string]*ResourceSchema, len(in))
	for name, a := range in {
		rs := &ResourceSchema{
			Name:   name,
			Blocks: make(map[string]*Block),
		}
		if a != nil && a.Block != nil {
			flattenBlock(rs, "", a.Block)
		}
		out[name] = rs
	}
	return out
}

// flattenFunctions produces the minimal FunctionSchema records. Full
// signature info is recoverable from the tfjson source if a future rule
// needs it; this slice just captures parameter names for argument-order
// style checks.
func flattenFunctions(in map[string]*tfjson.FunctionSignature) map[string]*FunctionSchema {
	out := make(map[string]*FunctionSchema, len(in))
	for name, sig := range in {
		fs := &FunctionSchema{Name: name}
		if sig != nil {
			fs.Description = sig.Description
			fs.ParameterNames = make([]string, 0, len(sig.Parameters))
			for _, p := range sig.Parameters {
				fs.ParameterNames = append(fs.ParameterNames, p.Name)
			}
			if sig.VariadicParameter != nil {
				fs.VariadicParameter = sig.VariadicParameter.Name
			}
		}
		out[name] = fs
	}
	return out
}

func flattenIdentitySchemas(in map[string]*tfjson.IdentitySchema) map[string]*IdentitySchema {
	out := make(map[string]*IdentitySchema, len(in))
	for name, is := range in {
		if is == nil {
			continue
		}
		s := &IdentitySchema{}
		for attrName, attr := range is.Attributes {
			s.Attributes = append(s.Attributes, IdentityAttribute{
				Name:     attrName,
				Type:     attr.IdentityType.FriendlyName(),
				Required: attr.RequiredForImport,
			})
		}
		slices.SortFunc(s.Attributes, func(a, b IdentityAttribute) int {
			return strings.Compare(a.Name, b.Name)
		})
		out[name] = s
	}
	return out
}

func flattenSchema(name string, s *tfjson.Schema) *ResourceSchema {
	rs := &ResourceSchema{
		Name:   name,
		Blocks: make(map[string]*Block),
	}
	if s != nil && s.Block != nil {
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

func sortedKeys(m map[string]*ResourceSchema) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func sortedFuncKeys(m map[string]*FunctionSchema) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
