// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollect(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	dir := filepath.Join(repo, "testdata", "specs")
	writeFiles(t, dir, map[string]int{
		"3.0/a.yml":     10,
		"3.0/same.yml":  10,
		"3.0/b.json":    30,
		"3.1/c.yaml":    20,
		"3.0/-skip.yml": 5,
		"-hidden/x.yml": 5,
		"stash/y.yml":   5,
		"README.md":     5,
	})
	writeFiles(t, repo, map[string]int{"other/outside.yml": 7})
	elsewhere := filepath.Join(t.TempDir(), "far.yml")
	writeFiles(t, filepath.Dir(elsewhere), map[string]int{"far.yml": 3})
	spec := func(name string, size int64) Spec {
		return Spec{Name: name, Path: filepath.Join(dir, filepath.FromSlash(name)), Size: size}
	}

	tests := []struct {
		name    string
		dir     string
		named   []string
		want    []Spec
		wantErr error
	}{
		{
			name: "Walk finds specs largest first and skips stash and dash names",
			want: []Spec{spec("3.0/b.json", 30), spec("3.1/c.yaml", 20), spec("3.0/a.yml", 10), spec("3.0/same.yml", 10)},
		},
		{name: "Named relative to the specs folder", named: []string{"3.0/-skip.yml"}, want: []Spec{spec("3.0/-skip.yml", 5)}},
		{name: "Named relative to the repo", named: []string{"testdata/specs/3.1/c.yaml"}, want: []Spec{spec("3.1/c.yaml", 20)}},
		{
			name:  "Named outside the specs folder is named from the repo",
			named: []string{"other/outside.yml"},
			want:  []Spec{{Name: "other/outside.yml", Path: filepath.Join(repo, "other", "outside.yml"), Size: 7}},
		},
		{name: "Absolute name", named: []string{filepath.Join(dir, "3.0", "a.yml")}, want: []Spec{spec("3.0/a.yml", 10)}},
		{
			name:  "Absolute name outside the repo keeps its path",
			named: []string{elsewhere},
			want:  []Spec{{Name: filepath.ToSlash(elsewhere), Path: elsewhere, Size: 3}},
		},
		{
			name:  "Named twice gives one spec",
			named: []string{"3.0/a.yml", "testdata/specs/3.0/a.yml", "3.1/c.yaml"},
			want:  []Spec{spec("3.1/c.yaml", 20), spec("3.0/a.yml", 10)},
		},
		{name: "Missing spec", named: []string{"3.0/nope.yml"}, wantErr: ErrSpecNotFound},
		{name: "A folder is not a spec", named: []string{"3.0"}, wantErr: ErrSpecNotFound},
		{name: "Missing specs folder gives no specs", dir: filepath.Join(repo, "nope")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			specsDir := dir
			if tc.dir != "" {
				specsDir = tc.dir
			}

			got, err := Collect(repo, specsDir, tc.named)

			require.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCollectUnreadableFolder(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root reads any folder")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "3.0")
	require.NoError(t, os.Mkdir(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	got, err := Collect(dir, dir, nil)

	require.ErrorIs(t, err, os.ErrPermission)
	assert.Nil(t, got)
}

func TestSafeNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		names []string
		want  []string
	}{
		{name: "Path becomes a package name", names: []string{"3.0/misc/stripe.yml"}, want: []string{"s3_0_misc_stripe"}},
		{name: "Case and separators", names: []string{"Pet-Store.v2.yaml"}, want: []string{"pet_store_v2"}},
		{name: "Leading dash is dropped", names: []string{"-gladly.yml"}, want: []string{"gladly"}},
		{name: "Non-ASCII letters are dropped", names: []string{"café.yml"}, want: []string{"caf"}},
		{name: "Keyword gets an underscore", names: []string{"type.yml"}, want: []string{"type_"}},
		{name: "Main and init get an underscore", names: []string{"main.yml", "init.json"}, want: []string{"main_", "init_"}},
		{name: "No letters or digits", names: []string{"---.yml"}, want: []string{"s"}},
		{
			name:  "Clashes are numbered in sorted order",
			names: []string{"x_y.yml", "x-y.yml", "X.Y.yml"},
			want:  []string{"x_y_3", "x_y_2", "x_y"},
		},
		{name: "Same name twice keeps one package", names: []string{"a.yml", "a.yml"}, want: []string{"a", "a"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, SafeNames(tc.names))
		})
	}
}

// writeFiles writes each file under dir with content of its size in bytes.
func writeFiles(t *testing.T, dir string, sizes map[string]int) {
	t.Helper()

	for name, size := range sizes {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(strings.Repeat("x", size)), 0o644))
	}
}
