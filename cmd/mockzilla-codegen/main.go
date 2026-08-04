// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Command mockzilla-codegen generates Go code from OpenAPI specs. Run mockzilla-codegen help for
// the commands.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/mockzilla/mockzilla-codegen/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
