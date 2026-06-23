<!-- Copyright IBM Corp. 2019, 2026 -->
<!-- SPDX-License-Identifier: MPL-2.0 -->

# Copilot review instructions

## What this is

swissshepherd is a documentation linter for Terraform providers. It compares a provider's schema against its Markdown documentation and reports missing, phantom, misordered, mislabeled, or misformatted docs — covering Read-Only attribute coverage, section presence, frontmatter, byline, heading style, and more.

## Coding style

Write modern Go (Go 1.25+). Prefer:

- `slices` and `maps` for collection operations.
- `cmp` for ordering and comparisons.
- `iter` and `range`-over-int / `range`-over-func where it improves clarity.
- `errors.Is` / `errors.As` over `==` and type assertions on errors.
- Return early instead of deeply nested conditionals.
