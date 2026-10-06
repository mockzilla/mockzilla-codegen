// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        WriteOptions
		isOverwrite bool
		wantActions []WriteAction
		wantGen     string
		wantMain    string
	}{
		{
			name:        "Existing scaffold is kept",
			wantActions: []WriteAction{ActionWrite, ActionSkip, ActionWrite},
			wantGen:     "package api\n",
			wantMain:    "package main // mine\n",
		},
		{
			name:        "Existing scaffold is overwritten when it says so",
			isOverwrite: true,
			wantActions: []WriteAction{ActionWrite, ActionWrite, ActionWrite},
			wantGen:     "package api\n",
			wantMain:    "package main\n",
		},
		{
			name:        "Dry run writes nothing",
			opts:        WriteOptions{DryRun: true},
			wantActions: []WriteAction{ActionWrite, ActionSkip, ActionWrite},
			wantMain:    "package main // mine\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			main := filepath.Join(dir, "cmd", "main.go")
			require.NoError(t, os.MkdirAll(filepath.Dir(main), 0o750))
			require.NoError(t, os.WriteFile(main, []byte("package main // mine\n"), 0o600))
			res := &Result{Files: []File{
				{Path: filepath.Join(dir, "api", "gen.go"), Content: []byte("package api\n")},
				{Path: main, Kind: FileScaffold, IsOverwritten: tc.isOverwrite, Content: []byte("package main\n")},
				{Path: filepath.Join(dir, "api", "service.go"), Kind: FileScaffold, IsOverwritten: tc.isOverwrite, Content: []byte("package api\n")},
			}}

			reports, err := Write(res, tc.opts)

			require.NoError(t, err)
			var actions []WriteAction
			for i, r := range reports {
				assert.Equal(t, res.Files[i].Path, r.Path)
				actions = append(actions, r.Action)
			}
			assert.Equal(t, tc.wantActions, actions)
			assertFile(t, filepath.Join(dir, "api", "gen.go"), tc.wantGen)
			assertFile(t, main, tc.wantMain)
		})
	}
}

func TestWriteErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))

	tests := []struct {
		name string
		file File
	}{
		{name: "Folder that cannot be made", file: File{Path: filepath.Join(blocker, "gen.go")}},
		{name: "Scaffold that cannot be checked", file: File{Path: filepath.Join(blocker, "main.go"), Kind: FileScaffold}},
		{name: "File that cannot be written", file: File{Path: dir}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ok := File{Path: filepath.Join(t.TempDir(), "ok.go")}
			reports, err := Write(&Result{Files: []File{ok, tc.file}}, WriteOptions{})

			require.ErrorIs(t, err, ErrWrite)
			assert.Equal(t, []WriteReport{{Path: ok.Path, Action: ActionWrite}}, reports)
		})
	}
}

func TestWriteActionString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "write", ActionWrite.String())
	assert.Equal(t, "skip", ActionSkip.String())
	assert.Equal(t, "unknown", WriteAction(9).String())
}

// assertFile checks the content of path; empty want means the file must not exist.
func assertFile(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if want == "" {
		assert.ErrorIs(t, err, os.ErrNotExist)
		return
	}
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}
