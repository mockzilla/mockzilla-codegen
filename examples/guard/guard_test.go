// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package guard checks that generated code and the runtime refuse to build together when their
// generator API levels differ.
package guard

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const runtimePath = "github.com/mockzilla/codegen/pkg/runtime"

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) {
	return f(path)
}

func TestRuntimeGuard(t *testing.T) {
	t.Parallel()

	generated, err := os.ReadFile(filepath.Join("..", "models", "basic", "gen.go"))
	require.NoError(t, err)
	require.Contains(t, string(generated), "const _ = runtime.SupportsGeneratorV1")

	tests := []struct {
		name      string
		generated string
		runtime   func(src string) string
		wantErr   string
	}{
		{
			name:      "Matching levels build",
			generated: string(generated),
		},
		{
			name:      "Runtime too old for the code",
			generated: strings.ReplaceAll(string(generated), "SupportsGeneratorV1", "SupportsGeneratorV2"),
			wantErr:   "undefined: runtime.SupportsGeneratorV2",
		},
		{
			name:      "Runtime too new for the code",
			generated: string(generated),
			runtime:   func(src string) string { return strings.ReplaceAll(src, "SupportsGeneratorV1", "SupportsGeneratorV2") },
			wantErr:   "undefined: runtime.SupportsGeneratorV1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := typeCheck(t, tc.generated, tc.runtime)

			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// typeCheck checks a generated file against the runtime sources, changed by edit when set.
func typeCheck(t *testing.T, generated string, edit func(src string) string) error {
	t.Helper()

	fset := token.NewFileSet()
	std := importer.ForCompiler(fset, "source", nil)

	dir := filepath.Join("..", "..", "pkg", "runtime")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var files []*ast.File
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		text := string(src)
		if edit != nil {
			text = edit(text)
		}
		f, err := parser.ParseFile(fset, e.Name(), text, 0)
		require.NoError(t, err)
		files = append(files, f)
	}
	runtimePkg, err := (&types.Config{Importer: std}).Check(runtimePath, fset, files, nil)
	require.NoError(t, err)

	file, err := parser.ParseFile(fset, "gen.go", generated, 0)
	require.NoError(t, err)
	conf := types.Config{Importer: importerFunc(func(path string) (*types.Package, error) {
		if path == runtimePath {
			return runtimePkg, nil
		}
		return std.Import(path)
	})}
	_, err = conf.Check("basic", fset, []*ast.File{file}, nil)
	return err
}
