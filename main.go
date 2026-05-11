// Copyright (c) YakDriver, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"

	"github.com/YakDriver/swissshepherd/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
