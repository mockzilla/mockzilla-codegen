// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	sandboxModule = "sandbox"
	specsFolder   = "specs"
	toolName      = "mockzilla-codegen"
	toolPackage   = "./cmd/mockzilla-codegen"
	codegenModule = "github.com/mockzilla/mockzilla-codegen"
)

// Sandbox is the Go module in Dir that generated packages are built in. It replaces the codegen
// module with Repo, so generated code imports the runtime of the tree under test.
type Sandbox struct {
	Dir  string
	Repo string
}

// BuildTool builds the codegen CLI from Repo into the sandbox and returns its path. VCS stamping
// is off so the binary, and the cache keyed by its hash, change only with the code.
func (s Sandbox) BuildTool(ctx context.Context, run Command) (string, error) {
	tool := filepath.Join(s.Dir, "bin", toolName)
	if out, err := run(ctx, s.Repo, "go", "build", "-buildvcs=false", "-o", tool, toolPackage); err != nil {
		return "", fmt.Errorf("%w: build %s: %w\n%s", ErrCommand, toolName, err, out)
	}
	return tool, nil
}

// Setup removes the generated packages of an earlier run, writes go.mod and a file importing deps,
// the packages generated code may import, and tidies the module.
func (s Sandbox) Setup(ctx context.Context, run Command, deps []string) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.Dir, specsFolder)); err != nil {
		return err
	}

	var imports strings.Builder
	for _, d := range deps {
		imports.WriteString("import _ " + strconv.Quote(d) + "\n")
	}
	files := []struct{ name, content string }{
		{name: "go.mod", content: "module " + sandboxModule + "\n\nreplace " + codegenModule + " => " + strconv.Quote(s.Repo) + "\n"},
		{name: "deps.go", content: "package " + sandboxModule + "\n\n" + imports.String()},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(s.Dir, f.name), []byte(f.content), 0o644); err != nil {
			return err
		}
	}

	if out, err := run(ctx, s.Dir, "go", "mod", "tidy"); err != nil {
		return fmt.Errorf("%w: go mod tidy: %w\n%s", ErrCommand, err, out)
	}
	return nil
}
