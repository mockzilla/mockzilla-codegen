// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sha256 of "ab".
const hashAB = "fb8e20fc2e4c3f248c60c39bd652f3c1347298bb977b8b4d5903b85055620603"

func TestLoadCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    string
		isDir   bool
		want    *Cache
		wantErr error
	}{
		{name: "Missing file gives an empty cache", want: &Cache{Tool: "t1", Passed: map[string]bool{}}},
		{
			name: "Same tool keeps the entries",
			file: `{"tool": "t1", "passed": {"k1": true}}`,
			want: &Cache{Tool: "t1", Passed: map[string]bool{"k1": true}},
		},
		{
			name: "Other tool drops the entries",
			file: `{"tool": "t0", "passed": {"k1": true}}`,
			want: &Cache{Tool: "t1", Passed: map[string]bool{}},
		},
		{name: "Broken file", file: "{", wantErr: ErrCache},
		{name: "Unreadable file", isDir: true, wantErr: syscall.EISDIR},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := filepath.Join(t.TempDir(), "cache.json")
			if tc.file != "" {
				require.NoError(t, os.WriteFile(p, []byte(tc.file), 0o644))
			}
			if tc.isDir {
				require.NoError(t, os.Mkdir(p, 0o755))
			}

			got, err := LoadCache(p, "t1")

			require.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCacheSave(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "cache.json")
	c := &Cache{Tool: "t1", Passed: map[string]bool{"k2": true, "k1": true}}

	require.NoError(t, c.Save(p))
	require.Error(t, c.Save(dir))

	data, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tool": "t1", "passed": {"k1": true, "k2": true}}`, string(data))
	loaded, err := LoadCache(p, "t1")
	require.NoError(t, err)
	assert.Equal(t, c, loaded)
}

func TestKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yml")
	require.NoError(t, os.WriteFile(p, []byte("ab"), 0o644))

	job := Job{Spec: Spec{Path: p}, Variant: Variant{Name: "chi", Config: "server: {}\n"}, Package: "specs/chi/spec"}
	got, err := Key(job)
	require.NoError(t, err)
	assert.Regexp(t, "^"+hashAB+" chi [0-9a-f]{16}$", got)

	for _, edit := range []func(v *Variant){
		func(v *Variant) { v.Config = "server: {framework: echo}\n" },
		func(v *Variant) { v.Files = map[string][]string{"./models/gen.go": {"models"}} },
		func(v *Variant) { v.Init = "%s.NewRouter(nil)" },
		func(v *Variant) { v.Imports = []string{"net/http"} },
	} {
		changed := job
		edit(&changed.Variant)
		other, keyErr := Key(changed)
		require.NoError(t, keyErr)
		assert.NotEqual(t, got, other)
	}

	_, err = Key(Job{Spec: Spec{Path: filepath.Join(dir, "nope.yml")}})
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHashFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b"), []byte("b"), 0o644))

	tests := []struct {
		name    string
		paths   []string
		want    string
		wantErr error
	}{
		{name: "Contents in order", paths: []string{filepath.Join(dir, "a"), filepath.Join(dir, "b")}, want: hashAB},
		{name: "No files", want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{name: "Missing file", paths: []string{filepath.Join(dir, "c")}, wantErr: os.ErrNotExist},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := HashFiles(tc.paths...)

			require.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, tc.want, got)
		})
	}
}
