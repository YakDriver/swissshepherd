// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

const DefaultConfigFile = ".swissshepherd.hcl"

//go:embed defaults.hcl
var defaultsHCL []byte

// Config is the top-level configuration for swissshepherd.
type Config struct {
	ProviderSource string `hcl:"provider_source,optional"`
	ProviderDir    string `hcl:"provider_dir,optional"`
	SchemaJSON     string `hcl:"schema_json,optional"`

	// IgnoreContentsCheck suppresses all schema+doc rule findings for the
	// listed target names. Useful for deprecated/removed resources whose docs
	// are intentionally minimal stubs.
	IgnoreContentsCheck     []string `hcl:"ignore_contents_check,optional"`
	IgnoreContentsCheckFile string   `hcl:"ignore_contents_check_file,optional"`

	// FileAliases maps a schema target name to the doc target name used for
	// path resolution. Keys can be plain names (apply to all types) or
	// type-qualified as "type/name" (e.g. "list_resource/aws_ebs_volume").
	FileAliases map[string]string `hcl:"file_aliases,optional"`

	// Types defines every documentation category swissshepherd knows about.
	// A set of standard types (resource, data_source, ephemeral, function,
	// list_resource, action, guide, index) ships embedded; user blocks with
	// the same name replace defaults wholesale, new names add new types.
	Types []Type `hcl:"type,block"`

	Checks []CheckConfig `hcl:"check,block"`
}

// CheckConfig represents a single check block in the config.
type CheckConfig struct {
	Name    string `hcl:"name,label"`
	Enabled bool   `hcl:"enabled,optional"`

	// BlockHeadingStyles defines templates for recognizing block headings.
	// Use {Block} as placeholder for the block name (snake_case).
	// Use {Title} as placeholder for title-case name (converted to snake_case).
	// Default: ["`{Block}` Block", "{Block} Block", "`{Block}`", "{Block}", "{Title}"]
	BlockHeadingStyles []string `hcl:"block_heading_styles,optional"`

	// PreferBlockHeadingStyles defines the target heading format(s).
	// When a block heading matches an accepted style but not a preferred style,
	// a warning is emitted suggesting the preferred format.
	PreferBlockHeadingStyles []string `hcl:"prefer_block_heading_styles,optional"`

	// Path scoping — control which targets this check applies to.
	// Empty lists mean "all". When multiple allowlists are set, a target
	// must satisfy each populated axis:
	//
	//   - Types: include only targets whose type name is in this list.
	//   - Prefixes / Targets: a target's name must have one of the listed
	//     prefixes OR be listed exactly in Targets. (The two name-axis
	//     lists are OR'd together; they're two ways of saying "include
	//     these names".)
	//
	// IgnoreTargets subtracts unconditionally: a name in IgnoreTargets is
	// never checked even when allowlists include it. IgnoreTargetsFile is
	// the file form of the same list (one name per line; '#' starts a
	// comment; relative paths resolve against CWD like every other _file
	// option).
	Types             []string `hcl:"types,optional"`
	Prefixes          []string `hcl:"prefixes,optional"`
	Targets           []string `hcl:"targets,optional"`
	IgnoreTargets     []string `hcl:"ignore_targets,optional"`
	IgnoreTargetsFile string   `hcl:"ignore_targets_file,optional"`
	IgnorePrefixes    []string `hcl:"ignore_prefixes,optional"`

	// Frontmatter rule options. Set require_* to fail when a field is absent;
	// set forbid_* to fail when it is present. A field covered by both require
	// and forbid is a configuration error — require wins but emits both.
	RequireSubcategory   bool `hcl:"require_subcategory,optional"`
	RequirePageTitle     bool `hcl:"require_page_title,optional"`
	RequireDescription   bool `hcl:"require_description,optional"`
	RequireLayout        bool `hcl:"require_layout,optional"`
	ForbidSubcategory    bool `hcl:"forbid_subcategory,optional"`
	ForbidPageTitle      bool `hcl:"forbid_page_title,optional"`
	ForbidDescription    bool `hcl:"forbid_description,optional"`
	ForbidLayout         bool `hcl:"forbid_layout,optional"`
	ForbidSidebarCurrent bool `hcl:"forbid_sidebar_current,optional"`

	// Subcategory allowlist for the frontmatter rule. When non-empty, a
	// frontmatter subcategory value outside this list is reported. The allowlist
	// only fires when subcategory is actually present in the file — pair with
	// require_subcategory if absence should also fail.
	AllowSubcategories           []string `hcl:"allow_subcategories,optional"`
	AllowSubcategoriesFile       string   `hcl:"allow_subcategories_file,optional"`
	AllowEmptySubcategoryTargets []string `hcl:"allow_empty_subcategory_targets,optional"`

	// TitleSection rule options.
	//
	// AllowPrefixes replaces the default set of permitted level-1 heading
	// prefixes ("Action", "Data Source", "Ephemeral", "Function",
	// "List Resource", "Resource"). Leave empty to use the default.
	AllowPrefixes []string `hcl:"allow_prefixes,optional"`

	// Completeness rule options.
	IgnoreDeprecated   *bool    `hcl:"ignore_deprecated,optional"`
	ImplicitAttributes []string `hcl:"implicit_attributes,optional"`
	AllowPhantoms      []string `hcl:"allow_phantoms,optional"`
	SkipBlocks         []string `hcl:"skip_blocks,optional"`

	// DescriptionStyle rule options.
	BadPrefixes []string `hcl:"bad_prefixes,optional"`

	// FormatStyle rule options. nil means enabled (default true).
	NoCodeBlocks       *bool `hcl:"no_code_blocks,optional"`
	SingleLineAttrs    *bool `hcl:"single_line_attrs,optional"`
	UninterruptedLists *bool `hcl:"uninterrupted_lists,optional"`

	// SchemaDocsRule sub-check toggles. nil means enabled (default true).
	Coverage    *bool `hcl:"coverage,optional"`
	Ordering    *bool `hcl:"ordering,optional"`
	Description *bool `hcl:"description,optional"`
	Heading     *bool `hcl:"heading,optional"`
	Format      *bool `hcl:"format,optional"`
	Labels      *bool `hcl:"labels,optional"`
	Byline      *bool `hcl:"byline,optional"`

	// ImportSection rule options.
	RequireIdentitySection *bool `hcl:"require_identity_section,optional"`

	// ExampleSection rule options.
	AllowLanguages []string `hcl:"allow_languages,optional"`

	// FileCheck rule options.
	MaxFileSize             int64    `hcl:"max_file_size,optional"`
	AllowExtensions         []string `hcl:"allow_extensions,optional"`
	AllowRegistryExtensions []string `hcl:"allow_registry_extensions,optional"`

	// DirectoryLayout rule options.
	IgnoreCdktf bool `hcl:"ignore_cdktf,optional"`

	// FileMatch rule options.
	RequireDoc        *bool    `hcl:"require_doc,optional"`
	RequireSchema     *bool    `hcl:"require_schema,optional"`
	MixedLayout       *bool    `hcl:"mixed_layout,optional"`
	IgnoreMissing     []string `hcl:"ignore_missing,optional"`
	IgnoreMissingFile string   `hcl:"ignore_missing_file,optional"`
	IgnoreExtra       []string `hcl:"ignore_extra,optional"`
	IgnoreExtraFile   string   `hcl:"ignore_extra_file,optional"`

	// RegionArgument rule options.
	IgnoreResources     []string `hcl:"ignore_resources,optional"`
	IgnoreResourcesFile string   `hcl:"ignore_resources_file,optional"`
}

// AppliesTo reports whether this check's path-scoping admits the given
// (name, typeName) target.
//
// Semantics:
//
//  1. IgnoreTargets wins unconditionally — a listed name is always
//     excluded.
//  2. When Types is non-empty, typeName must be in it.
//  3. When either Prefixes or Targets is non-empty, the name must satisfy
//     at least one: name has a listed prefix, OR name equals a listed
//     exact target. Both lists empty means "any name is fine".
//
// Intended usage: the Runner calls AppliesTo per-rule before invoking it,
// so each check can roll out independently across a large provider (the
// service-by-service migration the phase-3 design was built for).
func (c CheckConfig) AppliesTo(name, typeName string) bool {
	qualified := typeName + "/" + name

	if slices.Contains(c.IgnoreTargets, name) || slices.Contains(c.IgnoreTargets, qualified) {
		return false
	}
	for _, p := range c.IgnorePrefixes {
		if matchesQualifiedPrefix(name, typeName, p) {
			return false
		}
	}
	if len(c.Types) > 0 && !slices.Contains(c.Types, typeName) {
		return false
	}
	if len(c.Prefixes) == 0 && len(c.Targets) == 0 {
		return true
	}
	if slices.Contains(c.Targets, name) || slices.Contains(c.Targets, qualified) {
		return true
	}
	for _, p := range c.Prefixes {
		if matchesQualifiedPrefix(name, typeName, p) {
			return true
		}
	}
	return false
}

// matchesQualifiedPrefix checks if a prefix matches, supporting type/prefix
// notation. "data_source/aws_s3" matches only when typeName is "data_source"
// and name starts with "aws_s3". A plain prefix "aws_s3" matches any type.
func matchesQualifiedPrefix(name, typeName, prefix string) bool {
	if t, p, ok := strings.Cut(prefix, "/"); ok {
		return typeName == t && strings.HasPrefix(name, p)
	}
	return strings.HasPrefix(name, prefix)
}

// Load reads and parses an HCL config file. Returns a zero-value Config if
// the file doesn't exist (with default types already merged in).
//
// Relative paths in the config (provider_dir, docs_path, schema_json, and any
// *_file option) are interpreted by the OS relative to the current working
// directory at the time swissshepherd runs — the same convention as terraform,
// docker, go, make, and other tools. The location of the config file itself
// does not anchor paths; run swissshepherd from the provider repo root (or
// wherever your paths are written relative to) and point --config at the
// config file.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigFile
	}

	defaults, err := loadDefaultTypes()
	if err != nil {
		return nil, fmt.Errorf("loading embedded default types: %w", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := &Config{Types: defaults}
		return cfg, nil
	}

	var cfg Config
	if err := hclsimple.DecodeFile(path, nil, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	cfg.Types = mergeTypes(defaults, cfg.Types)
	for i := range cfg.Types {
		if err := cfg.Types[i].Validate(); err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	}

	if err := cfg.resolveFiles(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// loadDefaultTypes parses the embedded defaults.hcl and returns the set of
// default type blocks. Failures here indicate a bug in the embedded file —
// they should never fire in a shipped binary.
func loadDefaultTypes() ([]Type, error) {
	var wrapper struct {
		Types []Type `hcl:"type,block"`
	}
	if err := hclsimple.Decode("defaults.hcl", defaultsHCL, nil, &wrapper); err != nil {
		return nil, err
	}
	for i := range wrapper.Types {
		if err := wrapper.Types[i].Validate(); err != nil {
			return nil, fmt.Errorf("defaults.hcl: %w", err)
		}
	}
	return wrapper.Types, nil
}

// GetCheck returns the CheckConfig for a named check, or a disabled default.
func (c *Config) GetCheck(name string) CheckConfig {
	for _, ch := range c.Checks {
		if ch.Name == name {
			return ch
		}
	}
	return CheckConfig{Name: name}
}

// IsCheckEnabled returns true if a check is enabled.
// Checks are enabled by default unless explicitly disabled in config.
func (c *Config) IsCheckEnabled(name string) bool {
	for _, ch := range c.Checks {
		if ch.Name == name {
			return ch.Enabled
		}
	}
	return true // enabled by default if not mentioned in config
}

// ShouldIgnoreContents reports whether schema+doc rules should be skipped for
// the given resource name (deprecated stubs, etc.). Entries can be bare names
// ("aws_kms_secret") matching any type, or type-qualified ("data_source/aws_kms_secret").
func (c *Config) ShouldIgnoreContents(resource, typeName string) bool {
	for _, entry := range c.IgnoreContentsCheck {
		if entry == resource || entry == typeName+"/"+resource {
			return true
		}
	}
	return false
}

// CheckBool returns a named bool option from a check block, or the given
// default when the check block doesn't exist or the field is nil.
func (c *Config) CheckBool(checkName, field string, defaultVal bool) bool {
	for _, ch := range c.Checks {
		if ch.Name != checkName {
			continue
		}
		switch field {
		case "require_identity_section":
			if ch.RequireIdentitySection != nil {
				return *ch.RequireIdentitySection
			}
		}
	}
	return defaultVal
}

// ProviderName extracts the short provider name from the source.
func (c *Config) ProviderName() string {
	parts := strings.Split(c.ProviderSource, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// FileMatchIgnoreMissing returns the ignore_missing list from the file_match
// check block. Used by the Runner to suppress "doc not found" log warnings.
func (c *Config) FileMatchIgnoreMissing() []string {
	return c.GetCheck("file_match").IgnoreMissing
}

// resolveFiles loads any _file references into their corresponding slice fields.
func (c *Config) resolveFiles() error {
	for i := range c.Checks {
		ch := &c.Checks[i]
		if ch.IgnoreTargetsFile != "" {
			lines, err := readLines(ch.IgnoreTargetsFile)
			if err != nil {
				return err
			}
			ch.IgnoreTargets = append(ch.IgnoreTargets, lines...)
		}
		if ch.AllowSubcategoriesFile != "" {
			lines, err := readLines(ch.AllowSubcategoriesFile)
			if err != nil {
				return err
			}
			ch.AllowSubcategories = append(ch.AllowSubcategories, lines...)
		}
	}

	if c.IgnoreContentsCheckFile != "" {
		lines, err := readLines(c.IgnoreContentsCheckFile)
		if err != nil {
			return err
		}
		c.IgnoreContentsCheck = append(c.IgnoreContentsCheck, lines...)
	}

	for i := range c.Checks {
		ch := &c.Checks[i]
		if ch.IgnoreResourcesFile != "" {
			lines, err := readLines(ch.IgnoreResourcesFile)
			if err != nil {
				return err
			}
			ch.IgnoreResources = append(ch.IgnoreResources, lines...)
		}
		if ch.IgnoreMissingFile != "" {
			lines, err := readLines(ch.IgnoreMissingFile)
			if err != nil {
				return err
			}
			ch.IgnoreMissing = append(ch.IgnoreMissing, lines...)
		}
		if ch.IgnoreExtraFile != "" {
			lines, err := readLines(ch.IgnoreExtraFile)
			if err != nil {
				return err
			}
			ch.IgnoreExtra = append(ch.IgnoreExtra, lines...)
		}
	}

	return nil
}

// readLines reads a file and returns non-empty, trimmed lines.
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading list file %s: %w", path, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}
