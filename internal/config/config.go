// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

const DefaultConfigFile = ".swissshepherd.hcl"

// Config is the top-level configuration for swissshepherd.
type Config struct {
	ProviderSource string `hcl:"provider_source,optional"`
	SchemaJSON     string `hcl:"schema_json,optional"`
	DocsPath       string `hcl:"docs_path,optional"`

	AllowedSubcategories     []string `hcl:"allowed_subcategories,optional"`
	AllowedSubcategoriesFile string   `hcl:"allowed_subcategories_file,optional"`

	RequireResourceSubcategory bool `hcl:"require_resource_subcategory,optional"`
	IgnoreCdktfMissingFiles    bool `hcl:"ignore_cdktf_missing_files,optional"`

	Checks []CheckConfig `hcl:"check,block"`
}

// CheckConfig represents a single check block in the config.
type CheckConfig struct {
	Name    string `hcl:"name,label"`
	Enabled bool   `hcl:"enabled,optional"`

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

// ProviderName extracts the short provider name from the source (e.g., "aws" from "registry.terraform.io/hashicorp/aws").
func (c *Config) ProviderName() string {
	parts := strings.Split(c.ProviderSource, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// resolveFiles loads any _file references into their corresponding slice fields.
func (c *Config) resolveFiles() error {
	if c.AllowedSubcategoriesFile != "" {
		lines, err := readLines(c.AllowedSubcategoriesFile)
		if err != nil {
			return err
		}
		c.AllowedSubcategories = append(c.AllowedSubcategories, lines...)
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
