// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindModule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		gomod    string
		override string
		want     string
	}{
		{name: "Module line gives the path", gomod: "module example.com/pets\n\ngo 1.26.0\n", want: "example.com/pets"},
		{name: "Quoted path with a comment", gomod: "// pets\nmodule \"example.com/pets\" // main\n", want: "example.com/pets"},
		{name: "Tab after the keyword", gomod: "module\texample.com/pets\n", want: "example.com/pets"},
		{name: "Lines that only look like a module line are skipped", gomod: "modules x\nmodule\nmodule // none\nmodule example.com/pets\n", want: "example.com/pets"},
		{name: "Override replaces the path", gomod: "module example.com/pets\n", override: "example.com/other", want: "example.com/other"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(tc.gomod), 0o600))
			sub := filepath.Join(root, "api", "v1")
			require.NoError(t, os.MkdirAll(sub, 0o750))

			mod, err := FindModule(sub, tc.override)

			require.NoError(t, err)
			assert.Equal(t, Module{Path: tc.want, Dir: root}, mod)
		})
	}
}

func TestFindModuleWithoutGoMod(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	mod, err := FindModule(dir, "example.com/pets")

	require.NoError(t, err)
	assert.Equal(t, Module{Path: "example.com/pets", Dir: dir}, mod)
}

func TestFindModuleRelativeDir(t *testing.T) {
	t.Parallel()

	mod, err := FindModule(".", "")

	require.NoError(t, err)
	wd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, Module{Path: "github.com/mockzilla/codegen", Dir: filepath.Dir(filepath.Dir(wd))}, mod)
}

func TestFindModuleErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		wantMsg string
	}{
		{
			name: "go.mod without a module line",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.26.0\n"), 0o600))
			},
			wantMsg: "read go.mod: {dir}/go.mod: no module line",
		},
		{
			name: "go.mod that cannot be read",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.Mkdir(filepath.Join(dir, "go.mod"), 0o750))
			},
			wantMsg: "read go.mod: read {dir}/go.mod: is a directory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			tc.setup(t, dir)

			_, err := FindModule(dir, "")

			require.ErrorIs(t, err, ErrModule)
			assert.EqualError(t, err, strings.ReplaceAll(tc.wantMsg, "{dir}", dir))
		})
	}
}
