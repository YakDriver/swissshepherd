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
	"github.com/YakDriver/swissshepherd/internal/schema"
	"github.com/spf13/cobra"
)

var (
	cfgFile        string
	schemaJSON     string
	docsPath       string
	providerSource string
	resource       string
	prefix         string
	outputJSON     bool
	verbose        bool
)

func Execute() error {
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:   "swissshepherd",
	Short: "Terraform provider documentation checker",
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run documentation checks against provider schema",
	RunE:  runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)

	checkCmd.Flags().StringVar(&cfgFile, "config", "", "config file (default: .swissshepherd.hcl)")
	checkCmd.Flags().StringVar(&schemaJSON, "schema-json", "", "path to terraform providers schema -json output")
	checkCmd.Flags().StringVar(&docsPath, "docs-path", "", "path to documentation directory")
	checkCmd.Flags().StringVar(&providerSource, "provider-source", "", "provider source (e.g., registry.terraform.io/hashicorp/aws)")
	checkCmd.Flags().StringVar(&resource, "resource", "", "check a single resource (e.g., aws_instance)")
	checkCmd.Flags().StringVar(&prefix, "prefix", "", "check all resources matching a prefix (e.g., aws_dms_)")
	checkCmd.Flags().BoolVar(&outputJSON, "json", false, "output results as JSON")
	checkCmd.Flags().BoolVar(&verbose, "verbose", false, "verbose logging")
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

	// Set up rules
	completenessCheck := &check.CompletenessRule{
		IgnoreDeprecated: true,
	}

	runner := &check.Runner{
		Schema: ps,
		Config: cfg,
		Rules:  []check.Rule{completenessCheck},
		Logger: logger,
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
