// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

const DefaultConfigFile = ".swissshepherd.hcl"

// Config is the top-level configuration for swissshepherd.
type Config struct {
	ProviderSource string `hcl:"provider_source,optional"`
	ProviderDir    string `hcl:"provider_dir,optional"`
	SchemaJSON     string `hcl:"schema_json,optional"`
	DocsPath       string `hcl:"docs_path,optional"`

	IgnoreCdktfMissingFiles bool `hcl:"ignore_cdktf_missing_files,optional"`

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

	// Ignore lists (inline)
	IgnoreResources   []string `hcl:"ignore_resources,optional"`
	IgnoreDataSources []string `hcl:"ignore_data_sources,optional"`

	// Ignore lists (from file)
	IgnoreResourcesFile        string   `hcl:"ignore_resources_file,optional"`
	IgnoreDataSourcesFile      string   `hcl:"ignore_data_sources_file,optional"`
	IgnoreSubcategoriesFile    string   `hcl:"ignore_subcategories_file,optional"`
	IgnoreMissingResources     []string `hcl:"ignore_missing_resources,optional"`
	IgnoreMissingDataSources   []string `hcl:"ignore_missing_data_sources,optional"`
	IgnoreMissingResourcesFile string   `hcl:"ignore_missing_resources_file,optional"`

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
}

// Load reads and parses an HCL config file. Returns a zero-value Config if the file doesn't exist.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigFile
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Config{}, nil
	}

	var cfg Config
	if err := hclsimple.DecodeFile(path, nil, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	// Resolve relative paths from the config file's directory
	cfgDir := filepath.Dir(path)
	if abs, err := filepath.Abs(cfgDir); err == nil {
		cfgDir = abs
	}
	cfg.resolvePaths(cfgDir)

	if err := cfg.resolveFiles(); err != nil {
		return nil, err
	}

	return &cfg, nil
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

// resolvePaths makes relative paths absolute relative to the config file's directory.
func (c *Config) resolvePaths(baseDir string) {
	resolve := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(baseDir, p)
	}

	c.ProviderDir = resolve(c.ProviderDir)
	c.SchemaJSON = resolve(c.SchemaJSON)
	c.DocsPath = resolve(c.DocsPath)

	for i := range c.Checks {
		c.Checks[i].IgnoreResourcesFile = resolve(c.Checks[i].IgnoreResourcesFile)
		c.Checks[i].IgnoreDataSourcesFile = resolve(c.Checks[i].IgnoreDataSourcesFile)
		c.Checks[i].IgnoreSubcategoriesFile = resolve(c.Checks[i].IgnoreSubcategoriesFile)
		c.Checks[i].IgnoreMissingResourcesFile = resolve(c.Checks[i].IgnoreMissingResourcesFile)
		c.Checks[i].AllowedSubcategoriesFile = resolve(c.Checks[i].AllowedSubcategoriesFile)
	}
}

// resolveFiles loads any _file references into their corresponding slice fields.
func (c *Config) resolveFiles() error {
	for i := range c.Checks {
		ch := &c.Checks[i]
		if ch.IgnoreResourcesFile != "" {
			lines, err := readLines(ch.IgnoreResourcesFile)
			if err != nil {
				return err
			}
			ch.IgnoreResources = append(ch.IgnoreResources, lines...)
		}
		if ch.IgnoreDataSourcesFile != "" {
			lines, err := readLines(ch.IgnoreDataSourcesFile)
			if err != nil {
				return err
			}
			ch.IgnoreDataSources = append(ch.IgnoreDataSources, lines...)
		}
		if ch.IgnoreSubcategoriesFile != "" {
			lines, err := readLines(ch.IgnoreSubcategoriesFile)
			if err != nil {
				return err
			}
			// Store in IgnoreResources as a general-purpose list for now
			ch.IgnoreResources = append(ch.IgnoreResources, lines...)
		}
		if ch.IgnoreMissingResourcesFile != "" {
			lines, err := readLines(ch.IgnoreMissingResourcesFile)
			if err != nil {
				return err
			}
			ch.IgnoreMissingResources = append(ch.IgnoreMissingResources, lines...)
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
