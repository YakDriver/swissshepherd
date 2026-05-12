// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package check

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/schema"
)

// Runner orchestrates running checks across all resources.
//
// Rules operate on the parsed doc.Document plus the schema. FileRules operate
// on the raw file bytes. Runner reads each documentation file once and feeds
// the content to both kinds of checks.
type Runner struct {
	Schema                    *schema.ProviderSchema
	Config                    *config.Config
	Rules                     []Rule
	FileRules                 []FileRule
	Logger                    *slog.Logger
	HeadingTemplates          doc.HeadingTemplates
	PreferredHeadingTemplates doc.HeadingTemplates
}

// RunAll runs all checks against all resources and data sources.
func (r *Runner) RunAll() []Result {
	var results []Result

	checkCfg := r.Config.GetCheck("completeness")

	for name, rs := range r.Schema.Resources {
		if slices.Contains(checkCfg.IgnoreResources, name) {
			r.Logger.Debug("skipping ignored resource", "name", name)
			continue
		}
		results = append(results, r.checkTarget(name, rs, "r")...)
	}

	for name, rs := range r.Schema.DataSources {
		if slices.Contains(checkCfg.IgnoreDataSources, name) {
			r.Logger.Debug("skipping ignored data source", "name", name)
			continue
		}
		results = append(results, r.checkTarget(name, rs, "d")...)
	}

	return results
}

// RunOne runs checks against a single named resource or data source.
func (r *Runner) RunOne(name string) ([]Result, error) {
	rs, docType := r.findResource(name)
	if rs == nil {
		return nil, fmt.Errorf("resource %q not found in schema", name)
	}

	// For RunOne, surface load errors to the caller rather than logging and
	// continuing, so a bad --resource invocation fails loudly.
	docPath := resourceDocPath(r.Config.DocsPath, r.Config.ProviderName(), name, docType)
	content, err := os.ReadFile(docPath)
	if err != nil {
		return nil, fmt.Errorf("reading doc for %s: %w", name, err)
	}
	d, err := doc.ParseWithTemplates(content, docPath, r.HeadingTemplates)
	if err != nil {
		return nil, fmt.Errorf("parsing doc for %s: %w", name, err)
	}

	var results []Result
	for _, rule := range r.Rules {
		results = append(results, rule.Check(name, rs, d)...)
	}
	for _, rule := range r.FileRules {
		results = append(results, rule.CheckFile(name, docPath, content)...)
	}
	return results, nil
}

// RunPrefix runs checks against all resources and data sources matching a name prefix.
func (r *Runner) RunPrefix(prefix string) []Result {
	var results []Result

	checkCfg := r.Config.GetCheck("completeness")

	for name, rs := range r.Schema.Resources {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if slices.Contains(checkCfg.IgnoreResources, name) {
			continue
		}
		results = append(results, r.checkTarget(name, rs, "r")...)
	}

	for name, rs := range r.Schema.DataSources {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if slices.Contains(checkCfg.IgnoreDataSources, name) {
			continue
		}
		results = append(results, r.checkTarget(name, rs, "d")...)
	}

	return results
}

// checkTarget resolves the doc path, reads content once, parses it, and runs
// every configured Rule and FileRule. Load failures are logged and skipped
// rather than returned — RunAll and RunPrefix should not bail on one bad file.
func (r *Runner) checkTarget(name string, rs *schema.ResourceSchema, docType string) []Result {
	docPath := resourceDocPath(r.Config.DocsPath, r.Config.ProviderName(), name, docType)

	content, err := os.ReadFile(docPath)
	if err != nil {
		r.Logger.Warn("cannot read doc", "resource", name, "path", docPath, "error", err)
		return nil
	}

	d, err := doc.ParseWithTemplates(content, docPath, r.HeadingTemplates)
	if err != nil {
		r.Logger.Warn("cannot parse doc", "resource", name, "path", docPath, "error", err)
		return nil
	}

	var results []Result
	for _, rule := range r.Rules {
		results = append(results, rule.Check(name, rs, d)...)
	}
	for _, rule := range r.FileRules {
		results = append(results, rule.CheckFile(name, docPath, content)...)
	}
	return results
}

func (r *Runner) findResource(name string) (*schema.ResourceSchema, string) {
	if rs, ok := r.Schema.Resources[name]; ok {
		return rs, "r"
	}
	if rs, ok := r.Schema.DataSources[name]; ok {
		return rs, "d"
	}
	return nil, ""
}

func resourceDocPath(docsPath, providerName, resourceName, docType string) string {
	// Resource name like "aws_instance" → file "instance.html.markdown"
	suffix := strings.TrimPrefix(resourceName, providerName+"_")

	// Try registry-style first (docs/resources/instance.md), then legacy (website/docs/r/instance.html.markdown)
	registryDir := "resources"
	if docType == "d" {
		registryDir = "data-sources"
	}

	registryPath := filepath.Join(docsPath, registryDir, suffix+".md")
	if _, err := os.Stat(registryPath); err == nil {
		return registryPath
	}

	legacyPath := filepath.Join(docsPath, docType, suffix+".html.markdown")
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}

	// Return legacy path as default (will error on load)
	return legacyPath
}
