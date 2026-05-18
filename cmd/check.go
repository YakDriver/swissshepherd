// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

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
	providerSource string
	providerDir    string
	target         string
	targetType     string
	prefix         string
	outputJSON     bool
	verbose        bool
	refreshSchema  bool
)

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				v += " (" + s.Value[:7] + ")"
			}
		}
		if v != "" {
			return v
		}
	}
	return "dev"
}

func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:          "swissshepherd",
	Short:        "Terraform provider documentation checker",
	Version:      version(),
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
		fs.Flags().StringVar(&providerSource, "provider-source", "", "provider source (e.g., registry.terraform.io/hashicorp/aws)")
		fs.Flags().StringVar(&providerDir, "provider-dir", "", "path to provider source directory (builds provider and generates schema automatically)")
		fs.Flags().StringVar(&target, "target", "", "check a single named target (e.g., aws_instance)")
		fs.Flags().StringVar(&targetType, "type", "", "target type for --target or --prefix (e.g., resource, data_source)")
		fs.Flags().StringVar(&prefix, "prefix", "", "check all targets whose name begins with this prefix (e.g., aws_dms_)")
		fs.Flags().BoolVar(&outputJSON, "json", false, "output results as JSON")
		fs.Flags().BoolVar(&refreshSchema, "refresh-schema", false, "regenerate cached schema even if schema_json file exists")
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
	if providerSource != "" {
		cfg.ProviderSource = providerSource
	}
	if providerDir != "" {
		cfg.ProviderDir = providerDir
	}

	// Schema resolution:
	// 1. If schema_json is set and file exists (and no --refresh-schema): use it
	// 2. If schema_json is set but missing (or --refresh-schema) and provider_dir is set: generate and cache
	// 3. If no schema_json and provider_dir is set: generate to temp dir, clean up after
	if cfg.SchemaJSON != "" && cfg.ProviderDir != "" {
		// Resolve relative to provider_dir
		schemaPath := cfg.SchemaJSON
		if !filepath.IsAbs(schemaPath) {
			schemaPath = filepath.Join(cfg.ProviderDir, schemaPath)
		}
		if refreshSchema || !fileExists(schemaPath) {
			if cfg.ProviderSource == "" {
				return fmt.Errorf("provider-source is required when generating schema")
			}
			fmt.Fprintf(os.Stderr, "Building schema (this may take a few minutes)...\n")
			fmt.Fprintf(os.Stderr, "Subsequent runs will use the cached schema at %s\n", schemaPath)
			fmt.Fprintf(os.Stderr, "Use --refresh-schema to regenerate after provider changes.\n")
			if err := os.MkdirAll(filepath.Dir(schemaPath), 0o755); err != nil {
				return fmt.Errorf("creating schema directory: %w", err)
			}
			if err := provider.GenerateSchemaTo(cfg.ProviderDir, cfg.ProviderSource, schemaPath); err != nil {
				return fmt.Errorf("generating schema: %w", err)
			}
		}
		cfg.SchemaJSON = schemaPath
	} else if cfg.ProviderDir != "" && cfg.SchemaJSON == "" {
		if cfg.ProviderSource == "" {
			return fmt.Errorf("provider-source is required when using provider_dir")
		}
		schemaPath, err := provider.GenerateSchema(cfg.ProviderDir, cfg.ProviderSource)
		if err != nil {
			return fmt.Errorf("generating schema: %w", err)
		}
		defer provider.CleanupSchema(schemaPath)
		cfg.SchemaJSON = schemaPath
	}

	// Validate required fields
	if cfg.SchemaJSON == "" {
		return fmt.Errorf("schema-json is required (via --schema-json or config file)")
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
		"ephemerals", len(ps.Ephemerals),
		"list_resources", len(ps.ListResources),
		"actions", len(ps.Actions),
		"functions", len(ps.Functions),
	)

	// Set up rules based on config (all enabled by default)
	var rules []check.Rule
	var fileRules []check.FileRule

	if cfg.IsCheckEnabled("schema_docs") {
		cc := cfg.GetCheck("schema_docs")
		rules = append(rules, &check.SchemaDocsRule{
			IgnoreDeprecated:          cc.IgnoreDeprecated == nil || *cc.IgnoreDeprecated,
			ImplicitAttributes:        cc.ImplicitAttributes,
			AllowPhantoms:             cc.AllowPhantoms,
			SkipBlocks:                cc.SkipBlocks,
			Coverage:                  cc.Coverage,
			Ordering:                  cc.Ordering,
			Description:               cc.Description,
			Heading:                   cc.Heading,
			Format:                    cc.Format,
			Labels:                    cc.Labels,
			Byline:                    cc.Byline,
			BadPrefixes:               cc.BadPrefixes,
			Preferred:                 preferredHeadingTemplates(cfg),
			NoCodeBlocks:              cc.NoCodeBlocks,
			SingleLineAttrs:           cc.SingleLineAttrs,
			UninterruptedLists:        cc.UninterruptedLists,
			AllowAttributeIndentation: cc.AllowAttributeIndentation,
		})
	}
	if cfg.IsCheckEnabled("title_section") {
		rules = append(rules, &check.TitleSectionRule{
			AllowPrefixes: cfg.GetCheck("title_section").AllowPrefixes,
		})
	}
	if cfg.IsCheckEnabled("section_presence") {
		rules = append(rules, &check.SectionPresenceRule{})
	}
	if cfg.IsCheckEnabled("timeouts_section") {
		rules = append(rules, &check.TimeoutsSectionRule{})
	}
	if cfg.IsCheckEnabled("import_section") {
		rules = append(rules, &check.ImportSectionRule{
			RequireIdentitySection: cfg.CheckBool("import_section", "require_identity_section", true),
		})
	}
	if cfg.IsCheckEnabled("example_section") {
		rules = append(rules, &check.ExampleSectionRule{
			AllowLanguages: cfg.GetCheck("example_section").AllowLanguages,
		})
	}
	if cfg.IsCheckEnabled("signature_section") {
		rules = append(rules, &check.SignatureSectionRule{})
	}
	if cfg.IsCheckEnabled("region_argument") {
		cc := cfg.GetCheck("region_argument")
		rules = append(rules, &check.RegionArgumentRule{
			IgnoreResources: cc.IgnoreResources,
		})
	}
	if cfg.IsCheckEnabled("frontmatter") {
		fileRules = append(fileRules, frontmatterRule(cfg))
	}
	if cfg.IsCheckEnabled("file_check") {
		cc := cfg.GetCheck("file_check")
		fileRules = append(fileRules, &check.FileCheckRule{
			MaxSize:                 cc.MaxFileSize,
			AllowExtensions:         cc.AllowExtensions,
			AllowRegistryExtensions: cc.AllowRegistryExtensions,
			InlineLinks:             cc.InlineLinks,
		})
	}

	runner := &check.Runner{
		Schema:                    ps,
		Config:                    cfg,
		Rules:                     rules,
		FileRules:                 fileRules,
		Logger:                    logger,
		HeadingTemplates:          headingTemplates(cfg),
		PreferredHeadingTemplates: preferredHeadingTemplates(cfg),
	}

	// Verbose: log enabled checks and their scoping
	if verbose {
		logEnabledChecks(logger, cfg, rules, fileRules)
	}

	// Dispatch: exactly one of (--target) / (--prefix) / (--type) / none.
	// --target selects a single named target; when --type is set it
	// disambiguates same-name targets across types. --prefix scopes by name
	// prefix; --type additionally scopes by type. Providing neither runs
	// every rule against every enumerable target.
	var results []check.Result

	// Global checks (run once, not per-target).
	if cfg.IsCheckEnabled("file_match") {
		cc := cfg.GetCheck("file_match")
		fmRule := &check.FileMatchRule{
			RequireDoc:    cc.RequireDoc,
			RequireSchema: cc.RequireSchema,
			MixedLayout:   cc.MixedLayout,
			IgnoreMissing: cc.IgnoreMissing,
			IgnoreExtra:   cc.IgnoreExtra,
		}
		results = append(results, fmRule.Check(cfg, ps)...)
	}

	switch {
	case target != "":
		res, runErr := runner.RunOne(target, targetType)
		if runErr != nil {
			return runErr
		}
		results = append(results, res...)
	case prefix != "" || targetType != "":
		results = append(results, runner.RunPrefix(prefix, targetType)...)
	default:
		results = append(results, runner.RunAll()...)
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
		loc := r.Resource
		if r.Path != "" && r.Line > 0 {
			loc = fmt.Sprintf("%s (%s:%d)", r.Resource, r.Path, r.Line)
		} else if r.Path != "" {
			loc = fmt.Sprintf("%s (%s)", r.Resource, r.Path)
		}
		fmt.Fprintf(os.Stdout, "%s  [%s] %s: %s\n", prefix, r.Rule, loc, r.Message)
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
	checkCfg := cfg.GetCheck("schema_docs")
	if len(checkCfg.BlockHeadingStyles) > 0 {
		return doc.HeadingTemplates(checkCfg.BlockHeadingStyles)
	}
	return doc.DefaultHeadingTemplates()
}

func preferredHeadingTemplates(cfg *config.Config) doc.HeadingTemplates {
	checkCfg := cfg.GetCheck("schema_docs")
	if len(checkCfg.PreferBlockHeadingStyles) > 0 {
		return doc.HeadingTemplates(checkCfg.PreferBlockHeadingStyles)
	}
	return nil
}

// frontmatterRule constructs a FrontmatterRule from the check "frontmatter"
// block of the HCL config.
func frontmatterRule(cfg *config.Config) *check.FrontmatterRule {
	cc := cfg.GetCheck("frontmatter")
	return &check.FrontmatterRule{
		RequireSubcategory:           cc.RequireSubcategory,
		RequirePageTitle:             cc.RequirePageTitle,
		RequireDescription:           cc.RequireDescription,
		RequireLayout:                cc.RequireLayout,
		ForbidSubcategory:            cc.ForbidSubcategory,
		ForbidPageTitle:              cc.ForbidPageTitle,
		ForbidDescription:            cc.ForbidDescription,
		ForbidLayout:                 cc.ForbidLayout,
		ForbidSidebarCurrent:         cc.ForbidSidebarCurrent,
		AllowSubcategories:           cc.AllowSubcategories,
		AllowEmptySubcategoryTargets: cc.AllowEmptySubcategoryTargets,
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func logEnabledChecks(logger *slog.Logger, cfg *config.Config, rules []check.Rule, fileRules []check.FileRule) {
	logger.Info("enabled checks", "schema_rules", len(rules), "file_rules", len(fileRules))

	for _, r := range rules {
		cc := cfg.GetCheck(r.Name())
		attrs := []any{"rule", r.Name()}
		if len(cc.Types) > 0 {
			attrs = append(attrs, "types", cc.Types)
		}
		if len(cc.Prefixes) > 0 {
			attrs = append(attrs, "prefixes", fmt.Sprintf("%d entries", len(cc.Prefixes)))
		}
		if len(cc.Targets) > 0 {
			attrs = append(attrs, "targets", fmt.Sprintf("%d entries", len(cc.Targets)))
		}
		if len(cc.IgnoreTargets) > 0 {
			attrs = append(attrs, "ignored", fmt.Sprintf("%d entries", len(cc.IgnoreTargets)))
		}
		logger.Info("  check", attrs...)
	}
	for _, r := range fileRules {
		cc := cfg.GetCheck(r.Name())
		attrs := []any{"rule", r.Name()}
		if len(cc.Types) > 0 {
			attrs = append(attrs, "types", cc.Types)
		}
		if len(cc.Prefixes) > 0 {
			attrs = append(attrs, "prefixes", fmt.Sprintf("%d entries", len(cc.Prefixes)))
		}
		logger.Info("  check", attrs...)
	}

	// Log disabled checks
	allChecks := []string{"schema_docs", "title_section",
		"section_presence", "timeouts_section", "import_section",
		"example_section", "signature_section",
		"format_style", "frontmatter"}
	for _, name := range allChecks {
		if !cfg.IsCheckEnabled(name) {
			logger.Info("  check (disabled)", "rule", name)
		}
	}

	// Log ignore lists
	if fm := cfg.FileMatchIgnoreMissing(); len(fm) > 0 {
		logger.Info("file_match.ignore_missing", "count", len(fm))
	}
	if len(cfg.IgnoreContentsCheck) > 0 {
		logger.Info("ignore_contents_check", "entries", cfg.IgnoreContentsCheck)
	}
	if len(cfg.FileAliases) > 0 {
		logger.Info("file_aliases", "count", len(cfg.FileAliases))
	}
}
