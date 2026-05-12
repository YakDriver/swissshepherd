// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/YakDriver/swissshepherd/internal/check"
	"github.com/YakDriver/swissshepherd/internal/config"
	"github.com/YakDriver/swissshepherd/internal/doc"
	"github.com/YakDriver/swissshepherd/internal/provider"
	"github.com/YakDriver/swissshepherd/internal/schema"
	"github.com/spf13/cobra"
)

var (
	cfgFile        string
	schemaJSON     string
	docsPath       string
	providerSource string
	providerDir    string
	resource       string
	prefix         string
	outputJSON     bool
	verbose        bool
)

func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:          "swissshepherd",
	Short:        "Terraform provider documentation checker",
	SilenceUsage: true,
	// Default to check command when no subcommand is given
	RunE: runCheck,
}

var checkCmd = &cobra.Command{
	Use:          "check",
	Short:        "Run documentation checks against provider schema",
	SilenceUsage: true,
	RunE:         runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)

	// Config is a persistent flag available to all subcommands
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: .swissshepherd.hcl)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "verbose logging")

	// Check flags on both root (default) and check subcommand
	for _, fs := range []*cobra.Command{rootCmd, checkCmd} {
		fs.Flags().StringVar(&schemaJSON, "schema-json", "", "path to terraform providers schema -json output")
		fs.Flags().StringVar(&docsPath, "docs-path", "", "path to documentation directory")
		fs.Flags().StringVar(&providerSource, "provider-source", "", "provider source (e.g., registry.terraform.io/hashicorp/aws)")
		fs.Flags().StringVar(&providerDir, "provider-dir", "", "path to provider source directory (builds provider and generates schema automatically)")
		fs.Flags().StringVar(&resource, "resource", "", "check a single resource (e.g., aws_instance)")
		fs.Flags().StringVar(&prefix, "prefix", "", "check all resources matching a prefix (e.g., aws_dms_)")
		fs.Flags().BoolVar(&outputJSON, "json", false, "output results as JSON")
	}
}

func runCheck(cmd *cobra.Command, args []string) error {
	// Load config
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// CLI flags override config
	if schemaJSON != "" {
		cfg.SchemaJSON = schemaJSON
	}
	if docsPath != "" {
		cfg.DocsPath = docsPath
	}
	if providerSource != "" {
		cfg.ProviderSource = providerSource
	}
	if providerDir != "" {
		cfg.ProviderDir = providerDir
	}

	// Auto-generate schema from provider directory
	if cfg.ProviderDir != "" && cfg.SchemaJSON == "" {
		if cfg.ProviderSource == "" {
			return fmt.Errorf("provider-source is required when using provider_dir")
		}
		schemaPath, err := provider.GenerateSchema(cfg.ProviderDir, cfg.ProviderSource)
		if err != nil {
			return fmt.Errorf("generating schema: %w", err)
		}
		defer provider.CleanupSchema(schemaPath)
		cfg.SchemaJSON = schemaPath

		// Default docs-path to website/docs under the provider dir
		if cfg.DocsPath == "" {
			cfg.DocsPath = cfg.ProviderDir + "/website/docs"
		}
	}

	// Validate required fields
	if cfg.SchemaJSON == "" {
		return fmt.Errorf("schema-json is required (via --schema-json or config file)")
	}
	if cfg.DocsPath == "" {
		return fmt.Errorf("docs-path is required (via --docs-path or config file)")
	}
	if cfg.ProviderSource == "" {
		return fmt.Errorf("provider-source is required (via --provider-source or config file)")
	}

	// Set up logger
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// Load schema
	logger.Info("loading schema", "path", cfg.SchemaJSON)
	ps, err := schema.LoadFile(cfg.SchemaJSON, cfg.ProviderSource)
	if err != nil {
		return fmt.Errorf("loading schema: %w", err)
	}

	logger.Info("schema loaded",
		"resources", len(ps.Resources),
		"data_sources", len(ps.DataSources),
	)

	// Set up rules based on config (all enabled by default)
	var rules []check.Rule
	var fileRules []check.FileRule

	if cfg.IsCheckEnabled("completeness") {
		rules = append(rules, &check.CompletenessRule{IgnoreDeprecated: true})
	}
	if cfg.IsCheckEnabled("ordering") {
		rules = append(rules, &check.OrderingRule{})
	}
	if cfg.IsCheckEnabled("description_style") {
		rules = append(rules, &check.DescriptionStyleRule{})
	}
	if cfg.IsCheckEnabled("computed_attribute") {
		rules = append(rules, &check.ComputedAttributeRule{})
	}
	if cfg.IsCheckEnabled("title_section") {
		rules = append(rules, &check.TitleSectionRule{
			AllowedPrefixes: cfg.GetCheck("title_section").AllowedPrefixes,
		})
	}

	preferred := preferredHeadingTemplates(cfg)
	if cfg.IsCheckEnabled("heading_style") && len(preferred) > 0 {
		rules = append(rules, &check.HeadingStyleRule{Preferred: preferred})
	}
	if cfg.IsCheckEnabled("format_style") {
		fileRules = append(fileRules, &check.FormatStyleRule{
			NoCodeBlocks:       true,
			SingleLineAttrs:    true,
			UninterruptedLists: true,
		})
	}
	if cfg.IsCheckEnabled("frontmatter") {
		fileRules = append(fileRules, frontmatterRule(cfg))
	}

	runner := &check.Runner{
		Schema:                    ps,
		Config:                    cfg,
		Rules:                     rules,
		FileRules:                 fileRules,
		Logger:                    logger,
		HeadingTemplates:          headingTemplates(cfg),
		PreferredHeadingTemplates: preferred,
	}

	// Run checks
	var results []check.Result
	if resource != "" {
		results, err = runner.RunOne(resource)
		if err != nil {
			return err
		}
	} else if prefix != "" {
		results = runner.RunPrefix(prefix)
	} else {
		results = runner.RunAll()
	}

	// Output results
	if outputJSON {
		return outputResultsJSON(results)
	}
	return outputResultsText(results)
}

func outputResultsJSON(results []check.Result) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func outputResultsText(results []check.Result) error {
	errors := 0
	warnings := 0

	for _, r := range results {
		prefix := "ERROR"
		if r.Severity == check.SeverityWarning {
			prefix = "WARN "
			warnings++
		} else {
			errors++
		}
		fmt.Fprintf(os.Stdout, "%s  %s: %s\n", prefix, r.Resource, r.Message)
	}

	if len(results) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d error(s), %d warning(s)\n", errors, warnings)
	} else {
		fmt.Fprintf(os.Stderr, "All checks passed.\n")
	}

	if errors > 0 {
		return fmt.Errorf("%d check(s) failed", errors)
	}
	return nil
}

func headingTemplates(cfg *config.Config) doc.HeadingTemplates {
	checkCfg := cfg.GetCheck("completeness")
	if len(checkCfg.BlockHeadingStyles) > 0 {
		return doc.HeadingTemplates(checkCfg.BlockHeadingStyles)
	}
	return doc.DefaultHeadingTemplates()
}

func preferredHeadingTemplates(cfg *config.Config) doc.HeadingTemplates {
	checkCfg := cfg.GetCheck("completeness")
	if len(checkCfg.PreferredBlockHeadingStyles) > 0 {
		return doc.HeadingTemplates(checkCfg.PreferredBlockHeadingStyles)
	}
	return nil
}

// frontmatterRule constructs a FrontmatterRule from the check "frontmatter"
// block of the HCL config.
func frontmatterRule(cfg *config.Config) *check.FrontmatterRule {
	cc := cfg.GetCheck("frontmatter")
	return &check.FrontmatterRule{
		RequireSubcategory:   cc.RequireSubcategory,
		RequirePageTitle:     cc.RequirePageTitle,
		RequireDescription:   cc.RequireDescription,
		RequireLayout:        cc.RequireLayout,
		ForbidSubcategory:    cc.ForbidSubcategory,
		ForbidPageTitle:      cc.ForbidPageTitle,
		ForbidDescription:    cc.ForbidDescription,
		ForbidLayout:         cc.ForbidLayout,
		ForbidSidebarCurrent: cc.ForbidSidebarCurrent,
		AllowedSubcategories: cc.AllowedSubcategories,
	}
}
