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
type Runner struct {
	Schema                    *schema.ProviderSchema
	Config                    *config.Config
	Rules                     []Rule
	Logger                    *slog.Logger
	HeadingTemplates          doc.HeadingTemplates
	PreferredHeadingTemplates doc.HeadingTemplates
}

// RunAll runs all checks against all resources and data sources.
func (r *Runner) RunAll() []Result {
	var results []Result

	checkCfg := r.Config.GetCheck("completeness")
	providerName := r.Config.ProviderName()
	docsPath := r.Config.DocsPath

	// Check resources
	for name, rs := range r.Schema.Resources {
		if slices.Contains(checkCfg.IgnoreResources, name) {
			r.Logger.Debug("skipping ignored resource", "name", name)
			continue
		}

		docPath := resourceDocPath(docsPath, providerName, name, "r")
		d, err := loadDoc(docPath, r.HeadingTemplates)
		if err != nil {
			r.Logger.Warn("cannot load doc", "resource", name, "path", docPath, "error", err)
			continue
		}

		for _, rule := range r.Rules {
			results = append(results, rule.Check(name, rs, d)...)
		}
	}

	// Check data sources
	for name, rs := range r.Schema.DataSources {
		if slices.Contains(checkCfg.IgnoreDataSources, name) {
			r.Logger.Debug("skipping ignored data source", "name", name)
			continue
		}

		docPath := resourceDocPath(docsPath, providerName, name, "d")
		d, err := loadDoc(docPath, r.HeadingTemplates)
		if err != nil {
			r.Logger.Warn("cannot load doc", "data_source", name, "path", docPath, "error", err)
			continue
		}

		for _, rule := range r.Rules {
			results = append(results, rule.Check(name, rs, d)...)
		}
	}

	return results
}

// RunOne runs checks against a single named resource or data source.
func (r *Runner) RunOne(name string) ([]Result, error) {
	providerName := r.Config.ProviderName()
	docsPath := r.Config.DocsPath

	rs, docType := r.findResource(name)
	if rs == nil {
		return nil, fmt.Errorf("resource %q not found in schema", name)
	}

	docPath := resourceDocPath(docsPath, providerName, name, docType)
	d, err := loadDoc(docPath, r.HeadingTemplates)
	if err != nil {
		return nil, fmt.Errorf("loading doc for %s: %w", name, err)
	}

	var results []Result
	for _, rule := range r.Rules {
		results = append(results, rule.Check(name, rs, d)...)
	}
	return results, nil
}

// RunPrefix runs checks against all resources and data sources matching a name prefix.
func (r *Runner) RunPrefix(prefix string) []Result {
	var results []Result

	checkCfg := r.Config.GetCheck("completeness")
	providerName := r.Config.ProviderName()
	docsPath := r.Config.DocsPath

	for name, rs := range r.Schema.Resources {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if slices.Contains(checkCfg.IgnoreResources, name) {
			continue
		}

		docPath := resourceDocPath(docsPath, providerName, name, "r")
		d, err := loadDoc(docPath, r.HeadingTemplates)
		if err != nil {
			r.Logger.Warn("cannot load doc", "resource", name, "path", docPath, "error", err)
			continue
		}

		for _, rule := range r.Rules {
			results = append(results, rule.Check(name, rs, d)...)
		}
	}

	for name, rs := range r.Schema.DataSources {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if slices.Contains(checkCfg.IgnoreDataSources, name) {
			continue
		}

		docPath := resourceDocPath(docsPath, providerName, name, "d")
		d, err := loadDoc(docPath, r.HeadingTemplates)
		if err != nil {
			r.Logger.Warn("cannot load doc", "data_source", name, "path", docPath, "error", err)
			continue
		}

		for _, rule := range r.Rules {
			results = append(results, rule.Check(name, rs, d)...)
		}
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

func loadDoc(path string, templates doc.HeadingTemplates) (*doc.Document, error) {
	return doc.ParseFileWithTemplates(path, templates)
}
