// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

// Command covercheck fails when any package in a coverage profile is below a minimum.
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
