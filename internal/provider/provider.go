// Copyright IBM Corp. 2019, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// GenerateSchema builds the provider and generates the schema JSON.
// Returns the path to the generated schema.json file.
func GenerateSchema(providerDir, providerSource string) (string, error) {
	if err := requireTerraform(); err != nil {
		return "", err
	}

	// Create a temp working directory for terraform init/schema
	workDir, err := os.MkdirTemp("", "swissshepherd-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	// Build the provider
	binaryName := "terraform-provider"
	binaryPath := filepath.Join(workDir, binaryName)

	fmt.Fprintf(os.Stderr, "Building provider in %s...\n", providerDir)
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = providerDir
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("building provider: %w", err)
	}

	// Set up plugin directory structure
	parts := strings.Split(providerSource, "/")
	if len(parts) < 3 {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("provider-source must be in format registry.terraform.io/namespace/name, got %q", providerSource)
	}
	providerName := parts[len(parts)-1]
	platform := runtime.GOOS + "_" + runtime.GOARCH
	pluginDir := filepath.Join(workDir, "plugin-dir", providerSource, "99.99.99", platform)

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("creating plugin dir: %w", err)
	}

	destBinary := filepath.Join(pluginDir, fmt.Sprintf("terraform-provider-%s_v99.99.99", providerName))
	if err := os.Rename(binaryPath, destBinary); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("moving provider binary: %w", err)
	}

	// Write a minimal .tf file
	tfFile := filepath.Join(workDir, "main.tf")
	tfContent := fmt.Sprintf("data \"%s_partition\" \"example\" {}\n", providerName)
	// Fallback: use a terraform block with required_providers if partition data source doesn't exist
	if providerName != "aws" {
		tfContent = fmt.Sprintf(`terraform {
  required_providers {
    %s = {
      source = "%s"
    }
  }
}
`, providerName, providerSource)
	}
	if err := os.WriteFile(tfFile, []byte(tfContent), 0o644); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("writing tf file: %w", err)
	}

	// terraform init
	fmt.Fprintf(os.Stderr, "Running terraform init...\n")
	initCmd := exec.Command("terraform", "init", "-plugin-dir", filepath.Join(workDir, "plugin-dir"))
	initCmd.Dir = workDir
	initCmd.Stderr = os.Stderr
	if err := initCmd.Run(); err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("terraform init: %w", err)
	}

	// terraform providers schema -json
	fmt.Fprintf(os.Stderr, "Generating provider schema...\n")
	schemaPath := filepath.Join(workDir, "schema.json")
	schemaFile, err := os.Create(schemaPath)
	if err != nil {
		os.RemoveAll(workDir)
		return "", fmt.Errorf("creating schema file: %w", err)
	}

	schemaCmd := exec.Command("terraform", "providers", "schema", "-json")
	schemaCmd.Dir = workDir
	schemaCmd.Stdout = schemaFile
	schemaCmd.Stderr = os.Stderr
	if err := schemaCmd.Run(); err != nil {
		schemaFile.Close()
		os.RemoveAll(workDir)
		return "", fmt.Errorf("terraform providers schema: %w", err)
	}
	schemaFile.Close()

	fmt.Fprintf(os.Stderr, "Schema generated at %s\n", schemaPath)
	return schemaPath, nil
}

// CleanupSchema removes the temp directory created by GenerateSchema.
func CleanupSchema(schemaPath string) {
	if schemaPath != "" {
		os.RemoveAll(filepath.Dir(schemaPath))
	}
}

// GenerateSchemaTo builds the provider, generates the schema, and writes it
// to the specified destination path. The temp working directory is cleaned up.
func GenerateSchemaTo(providerDir, providerSource, destPath string) error {
	schemaPath, err := GenerateSchema(providerDir, providerSource)
	if err != nil {
		return err
	}
	defer CleanupSchema(schemaPath)

	src, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("reading generated schema: %w", err)
	}
	if err := os.WriteFile(destPath, src, 0o644); err != nil {
		return fmt.Errorf("writing schema to %s: %w", destPath, err)
	}
	return nil
}

func requireTerraform() error {
	_, err := exec.LookPath("terraform")
	if err != nil {
		return fmt.Errorf("terraform not found in PATH: install Terraform to use --provider-dir")
	}
	return nil
}
