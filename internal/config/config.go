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

	IgnoreCdktfMissingFiles bool `hcl:"ignore_cdktf_missing_files,optional"`

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

	// PreferredBlockHeadingStyles defines the target heading format(s).
	// When a block heading matches an accepted style but not a preferred style,
	// a warning is emitted suggesting the preferred format.
	PreferredBlockHeadingStyles []string `hcl:"preferred_block_heading_styles,optional"`

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
	// IgnoredTargets subtracts unconditionally: a name in IgnoredTargets is
	// never checked even when allowlists include it. IgnoredTargetsFile is
	// the file form of the same list (one name per line; '#' starts a
	// comment; relative paths resolve against CWD like every other _file
	// option).
	Types              []string `hcl:"types,optional"`
	Prefixes           []string `hcl:"prefixes,optional"`
	Targets            []string `hcl:"targets,optional"`
	IgnoredTargets     []string `hcl:"ignored_targets,optional"`
	IgnoredTargetsFile string   `hcl:"ignored_targets_file,optional"`

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
	AllowedSubcategories     []string `hcl:"allowed_subcategories,optional"`
	AllowedSubcategoriesFile string   `hcl:"allowed_subcategories_file,optional"`

	// TitleSection rule options.
	//
	// AllowedPrefixes replaces the default set of permitted level-1 heading
	// prefixes ("Action", "Data Source", "Ephemeral", "Function",
	// "List Resource", "Resource"). Leave empty to use the default.
	AllowedPrefixes []string `hcl:"allowed_prefixes,optional"`
}

// AppliesTo reports whether this check's path-scoping admits the given
// (name, typeName) target.
//
// Semantics:
//
//  1. IgnoredTargets wins unconditionally — a listed name is always
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
	if slices.Contains(c.IgnoredTargets, name) {
		return false
	}
	if len(c.Types) > 0 && !slices.Contains(c.Types, typeName) {
		return false
	}
	if len(c.Prefixes) == 0 && len(c.Targets) == 0 {
		return true
	}
	if slices.Contains(c.Targets, name) {
		return true
	}
	for _, p := range c.Prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
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

// ProviderName extracts the short provider name from the source.
func (c *Config) ProviderName() string {
	parts := strings.Split(c.ProviderSource, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// resolveFiles loads any _file references into their corresponding slice fields.
func (c *Config) resolveFiles() error {
	for i := range c.Checks {
		ch := &c.Checks[i]
		if ch.IgnoredTargetsFile != "" {
			lines, err := readLines(ch.IgnoredTargetsFile)
			if err != nil {
				return err
			}
			ch.IgnoredTargets = append(ch.IgnoredTargets, lines...)
		}
		if ch.AllowedSubcategoriesFile != "" {
			lines, err := readLines(ch.AllowedSubcategoriesFile)
			if err != nil {
				return err
			}
			ch.AllowedSubcategories = append(ch.AllowedSubcategories, lines...)
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
