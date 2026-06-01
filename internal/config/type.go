// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"fmt"
	"slices"
	"strings"
)

// SectionName names a recognized doc section. Each value maps to a field on
// doc.Sections.
type SectionName string

// Recognized section names. The order of these constants matches the
// canonical AWS provider doc structure but is not load-bearing — the Type's
// section blocks determine order.
const (
	SectionTitle      SectionName = "title"
	SectionSignature  SectionName = "signature"
	SectionExample    SectionName = "example"
	SectionArguments  SectionName = "arguments"
	SectionAttributes SectionName = "attributes"
	SectionTimeouts   SectionName = "timeouts"
	SectionImport     SectionName = "import"
)

// AllSectionNames lists every recognized section. Used for config validation
// and for the section_presence rule's "unknown sections" check.
var AllSectionNames = []SectionName{
	SectionTitle,
	SectionSignature,
	SectionExample,
	SectionArguments,
	SectionAttributes,
	SectionTimeouts,
	SectionImport,
}

// IsValid reports whether n is one of the recognized section names.
func (n SectionName) IsValid() bool {
	return slices.Contains(AllSectionNames, n)
}

// HeadingText returns the canonical "## <text>" heading for the section.
// Used in error messages.
func (n SectionName) HeadingText() string {
	switch n {
	case SectionTitle:
		return "<title>"
	case SectionSignature:
		return "Signature"
	case SectionExample:
		return "Example Usage"
	case SectionArguments:
		return "Argument Reference"
	case SectionAttributes:
		return "Attribute Reference"
	case SectionTimeouts:
		return "Timeouts"
	case SectionImport:
		return "Import"
	}
	return string(n)
}

// SectionSpec declares one section's place in a Type's canonical doc
// structure. The order of SectionSpec entries on a Type IS the expected
// section order; section_presence enforces that order when its
// enforce_order option is set.
//
// Required = true means the section must be present.
// Forbidden = true means the section must be absent.
// Both true is a config error; both false means "optional" (allowed but
// not required).
type SectionSpec struct {
	Name      string `hcl:"name,label"`
	Required  bool   `hcl:"required,optional"`
	Forbidden bool   `hcl:"forbidden,optional"`
}

// SectionName returns Name typed as a SectionName for use in checks.
func (s SectionSpec) SectionName() SectionName {
	return SectionName(s.Name)
}

// Type describes one category of provider documentation — resource, data
// source, ephemeral, function, list resource, action, guide, index, or a
// provider-specific extension.
//
// The Type block in HCL captures everything swissshepherd needs to know about
// a category *except* the check logic itself: where docs live on disk, where
// the category's schema lives in the provider schema JSON, and what
// conventions rules should apply. Defaults for the Terraform-standard
// categories are embedded; users can add new types or override defaults by
// name.
type Type struct {
	Name string `hcl:"name,label"`

	// SchemaKind ties this Type to a schema-accessor registered in
	// internal/schema. The built-in accessors are: "resource", "data_source",
	// "ephemeral", "function", "list_resource", "action", and "none" (for
	// content-only categories like guides and the index).
	SchemaKind string `hcl:"schema_kind"`

	// WebsitePaths is a list of templates for resolving a target's doc file.
	// Each template uses "{name}" as the placeholder for the
	// provider-prefix-stripped target name (e.g., "instance" for
	// "aws_instance"). Templates are tried in order; the first existing file
	// wins. Supporting multiple templates lets a single type handle both
	// registry ("docs/resources/{name}.md") and legacy
	// ("website/docs/r/{name}.html.markdown") layouts simultaneously.
	WebsitePaths []string `hcl:"website_paths"`

	// TitlePrefix is the allowed "<Kind>: " prefix for the level-1 heading of
	// this type's doc. Empty means no title constraint (used for guides).
	TitlePrefix string `hcl:"title_prefix,optional"`

	// ArgumentsBylines is the set of paragraph texts allowed immediately
	// under "## Argument Reference". Empty means no byline constraint.
	ArgumentsBylines []string `hcl:"arguments_bylines,optional"`

	// AttributesBylines is the analogous set for "## Attribute Reference".
	AttributesBylines []string `hcl:"attributes_bylines,optional"`

	// ArgumentsHeading overrides the expected "## Argument Reference" text.
	// Functions use "Arguments" instead, for example.
	ArgumentsHeading string `hcl:"arguments_heading,optional"`

	// AllowMissingArgumentsByline relaxes the byline requirement — useful
	// for categories where the section header alone is enough.
	AllowMissingArgumentsByline bool `hcl:"allow_missing_arguments_byline,optional"`

	// Sections declares the canonical doc structure for this type. The order
	// of section blocks here IS the expected order in the doc file. Each
	// section block accepts:
	//   - required = true   → section must be present
	//   - forbidden = true  → section must be absent
	//   - both unset        → section is optional (allowed, not required)
	//   - both set          → config error
	//
	// section_presence reads this to enforce presence and (when its
	// enforce_order option is true) order.
	Sections []SectionSpec `hcl:"section,block"`

	// Frontmatter field requirements. FrontmatterRequire fields must be
	// present; FrontmatterForbid fields must be absent. The overlap is a
	// config error.
	FrontmatterRequire []string `hcl:"frontmatter_require,optional"`
	FrontmatterForbid  []string `hcl:"frontmatter_forbid,optional"`

	// RegionAware declares whether the enhanced-region argument check
	// applies to this type. Functions, actions, and guides are typically
	// not region-aware.
	RegionAware bool `hcl:"region_aware,optional"`
}

// Validate enforces the non-HCL constraints on a single Type: schema_kind
// must be non-empty, at least one website_path must be set, sections must
// reference known names with no duplicates and no required+forbidden
// conflict, and FrontmatterRequire and FrontmatterForbid must not list the
// same field.
func (t *Type) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("type block has no name label")
	}
	if t.SchemaKind == "" {
		return fmt.Errorf("type %q: schema_kind is required", t.Name)
	}
	if len(t.WebsitePaths) == 0 {
		return fmt.Errorf("type %q: website_paths must list at least one template", t.Name)
	}
	// {name} is not required: types like "index" resolve to a single fixed
	// file ("docs/index.md") rather than one file per schema entry.

	seenSection := make(map[string]bool, len(t.Sections))
	for _, s := range t.Sections {
		if !s.SectionName().IsValid() {
			return fmt.Errorf("type %q: section %q is not a recognized section name; expected one of %v",
				t.Name, s.Name, AllSectionNames)
		}
		if seenSection[s.Name] {
			return fmt.Errorf("type %q: section %q declared more than once", t.Name, s.Name)
		}
		seenSection[s.Name] = true
		if s.Required && s.Forbidden {
			return fmt.Errorf("type %q: section %q has both required and forbidden set", t.Name, s.Name)
		}
	}

	for _, f := range t.FrontmatterRequire {
		if slices.Contains(t.FrontmatterForbid, f) {
			return fmt.Errorf("type %q: %q appears in both frontmatter_require and frontmatter_forbid",
				t.Name, f)
		}
	}

	return nil
}

// ResolveDocPath expands a type's website_paths templates for the given
// target name, returning every candidate path in order. Callers typically
// try each until one exists. `providerPrefix` is stripped from `target` so
// "aws_instance" with providerPrefix "aws" yields the candidates for
// "instance".
func (t *Type) ResolveDocPath(target, providerPrefix string) []string {
	short := strings.TrimPrefix(target, providerPrefix+"_")
	out := make([]string, len(t.WebsitePaths))
	for i, tmpl := range t.WebsitePaths {
		out[i] = strings.ReplaceAll(tmpl, "{name}", short)
	}
	return out
}

// mergeTypes returns the result of folding user-provided types on top of
// defaults: user types with the same name as a default replace the default
// wholesale, and new user-named types are appended. The returned slice
// preserves the order (defaults first, then new user types by original
// input order) so diagnostics stay predictable.
func mergeTypes(defaults, user []Type) []Type {
	out := make([]Type, 0, len(defaults)+len(user))
	userByName := make(map[string]Type, len(user))
	userOrder := make([]string, 0, len(user))
	for _, t := range user {
		if _, seen := userByName[t.Name]; !seen {
			userOrder = append(userOrder, t.Name)
		}
		userByName[t.Name] = t
	}

	seen := make(map[string]bool, len(defaults))
	for _, d := range defaults {
		if override, ok := userByName[d.Name]; ok {
			out = append(out, override)
		} else {
			out = append(out, d)
		}
		seen[d.Name] = true
	}
	for _, name := range userOrder {
		if !seen[name] {
			out = append(out, userByName[name])
		}
	}
	return out
}

// GetType returns the Type with the given name, or nil when no such Type is
// defined. The lookup is linear — the slice is small (a handful of entries
// in practice), and linear keeps the return ordering stable.
func (c *Config) GetType(name string) *Type {
	for i := range c.Types {
		if c.Types[i].Name == name {
			return &c.Types[i]
		}
	}
	return nil
}

// TypeNames returns the names of all loaded types, in config order.
func (c *Config) TypeNames() []string {
	names := make([]string, len(c.Types))
	for i, t := range c.Types {
		names[i] = t.Name
	}
	return names
}
