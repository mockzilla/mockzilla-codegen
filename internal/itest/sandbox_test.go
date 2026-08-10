// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxBuildTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		wantErr string
	}{
		{name: "Tool is built into the sandbox"},
		{name: "Build failure", err: errors.New("exit status 1"), wantErr: "command failed: build mockzilla-codegen: exit status 1\nbroken"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeExec{respond: func(context.Context, call) ([]byte, error) { return []byte("broken"), tc.err }}
			s := Sandbox{Dir: "/sandbox", Repo: "/repo"}

			tool, err := s.BuildTool(t.Context(), f.run)

			assert.Equal(t, []call{{
				Dir:  "/repo",
				Name: "go",
				Args: []string{"build", "-buildvcs=false", "-o", filepath.Join("/sandbox", "bin", "mockzilla-codegen"), "./cmd/mockzilla-codegen"},
			}}, f.calls)
			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrCommand)
				assert.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Join("/sandbox", "bin", "mockzilla-codegen"), tool)
		})
	}
}

func TestSandboxSetup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prepare    func(t *testing.T, dir string) string
		tidyErr    error
		wantErr    error
		wantErrMsg string
	}{
		{name: "Module and deps are written and tidied"},
		{
			name: "Generated packages of an earlier run are removed",
			prepare: func(t *testing.T, dir string) string {
				t.Helper()
				writeFiles(t, dir, map[string]int{"specs/models/old/gen.go": 1})
				return dir
			},
		},
		{
			name:       "Tidy failure",
			tidyErr:    errors.New("exit status 1"),
			wantErr:    ErrCommand,
			wantErrMsg: "command failed: go mod tidy: exit status 1\nno network",
		},
		{
			name: "Sandbox under a file",
			prepare: func(t *testing.T, dir string) string {
				t.Helper()
				writeFiles(t, dir, map[string]int{"file": 1})
				return filepath.Join(dir, "file", "sandbox")
			},
			wantErr: syscall.ENOTDIR,
		},
		{
			name: "Go.mod is a folder",
			prepare: func(t *testing.T, dir string) string {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "go.mod"), 0o755))
				return dir
			},
			wantErr: syscall.EISDIR,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if tc.prepare != nil {
				dir = tc.prepare(t, dir)
			}
			f := &fakeExec{respond: func(context.Context, call) ([]byte, error) { return []byte("no network"), tc.tidyErr }}
			s := Sandbox{Dir: dir, Repo: "/repo"}

			err := s.Setup(t.Context(), f.run, []string{"github.com/mockzilla/mockzilla-codegen/pkg/runtime", "example.com/chi"})

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				if tc.wantErrMsg != "" {
					require.EqualError(t, err, tc.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []call{{Dir: dir, Name: "go", Args: []string{"mod", "tidy"}}}, f.calls)
			assert.Equal(t, map[string]string{
				"go.mod":  "module sandbox\n\nreplace github.com/mockzilla/mockzilla-codegen => \"/repo\"\n",
				"deps.go": "package sandbox\n\nimport _ \"github.com/mockzilla/mockzilla-codegen/pkg/runtime\"\nimport _ \"example.com/chi\"\n",
			}, readFiles(t, dir))
		})
	}
}

func TestSandboxSetupRemoveError(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root removes any file")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]int{"specs/models/old/gen.go": 1})
	locked := filepath.Join(dir, "specs", "models", "old")
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	f := &fakeExec{respond: func(context.Context, call) ([]byte, error) { return nil, nil }}

	err := Sandbox{Dir: dir}.Setup(t.Context(), f.run, nil)

	require.ErrorIs(t, err, os.ErrPermission)
	assert.Empty(t, f.calls)
}

// readFiles returns the content of every regular file in dir, by name.
func readFiles(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	files := map[string]string{}
	for _, e := range entries {
		if e.Type().IsRegular() {
			data, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
			require.NoError(t, readErr)
			files[e.Name()] = string(data)
		}
	}
	return files
}
