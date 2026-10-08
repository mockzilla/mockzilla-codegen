// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package prepare

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/bundle"
	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/provider/libopenapi"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

func TestRunSplitSpecWithOverlay(t *testing.T) {
	t.Parallel()

	cfg := parseConfig(t, "spec:\n  path: openapi.yaml\n  overlays: [overlay.yaml]\n", filepath.Join("testdata", "split"))
	out, err := Run(context.Background(), libopenapi.New(), Input{Config: cfg})
	require.NoError(t, err)

	golden(t, filepath.Join("testdata", "split", "out.golden"), string(out.Bytes))
	assert.Equal(t, filepath.Join("testdata", "split", "openapi.yaml"), out.File)
	assert.Empty(t, out.Diagnostics)
	assert.Equal(t, diag.Origin{File: filepath.Join("testdata", "split", "pet.yaml"), Line: 1, Col: 1}, out.Positions["/components/schemas/pet"])
}

func TestRunPassesUnchangedBytesThrough(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("testdata", "used.json"))
	require.NoError(t, err)

	out, err := Run(context.Background(), libopenapi.New(), Input{Path: filepath.Join("testdata", "used.json"), Config: parseConfig(t, "{}", "")})
	require.NoError(t, err)
	assert.Equal(t, data, out.Bytes)
	assert.Equal(t, filepath.Join("testdata", "used.json"), out.File)
	assert.Empty(t, out.Diagnostics)
}

func TestRunReadsSlashEscapesInJSON(t *testing.T) {
	t.Parallel()

	src := `{"openapi": "3.0.3", "info": {"title": "a\/b", "version": "1"}, "paths": {}}`
	out, err := Run(context.Background(), libopenapi.New(), Input{Spec: []byte(src), Config: parseConfig(t, "{}", "")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"openapi": "3.0.3", "info": {"title": "a/b", "version": "1"}, "paths": {}}`, string(out.Bytes))
}

func TestRunWritesYAMLForChangedJSON(t *testing.T) {
	t.Parallel()

	cfg := parseConfig(t, "spec:\n  filter:\n    exclude: {paths: [/a]}\n", "")
	out, err := Run(context.Background(), libopenapi.New(), Input{Path: filepath.Join("testdata", "used.json"), Config: cfg})
	require.NoError(t, err)
	assert.Equal(t, "openapi: 3.0.3\ninfo:\n  title: t\n  version: \"1\"\npaths: {}\n", string(out.Bytes))
	assert.Equal(t, []diag.Diagnostic{{
		Severity: diag.Warning,
		Code:     diag.CodeFilterEmpty,
		Origin:   diag.Origin{File: filepath.Join("testdata", "used.json"), Line: 1, Col: 1},
		Message:  "the filter removed every operation",
	}}, out.Diagnostics)
}

func TestRunModelsOnlySpecIsNotPruned(t *testing.T) {
	t.Parallel()

	cfg := parseConfig(t, "spec:\n  path: models.yaml\n  simplify: {unions: true}\n", "testdata")
	out, err := Run(context.Background(), libopenapi.New(), Input{Config: cfg})
	require.NoError(t, err)
	assert.Equal(t, "openapi: 3.1.0\ninfo: {title: models, version: '1'}\ncomponents:\n  schemas:\n    A: {type: string}\n    B:\n      type: string\n", string(out.Bytes))
	assert.Equal(t, []diag.Diagnostic{{
		Severity: diag.Info,
		Code:     diag.CodePruneSkipped,
		Origin:   diag.Origin{File: filepath.Join("testdata", "models.yaml"), Line: 1, Col: 1},
		Message:  "the spec has no operations, so nothing is pruned",
	}}, out.Diagnostics)
}

func TestRunWithPruneOff(t *testing.T) {
	t.Parallel()

	cfg := parseConfig(t, "spec:\n  path: openapi.yaml\n  prune: false\n", filepath.Join("testdata", "split"))
	out, err := Run(context.Background(), libopenapi.New(), Input{Config: cfg})
	require.NoError(t, err)
	assert.Contains(t, string(out.Bytes), "Unused: {type: string}")
	assert.Contains(t, string(out.Bytes), "pet:\n")
}

func TestRunInMemorySpec(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join("testdata", "split", "openapi.yaml"))
	require.NoError(t, err)

	tests := []struct {
		name  string
		input Input
		want  diag.Origin
	}{
		{
			name:  "Refs resolve against the config's folder",
			input: Input{Spec: data, Config: parseConfig(t, "{}", filepath.Join("testdata", "split"))},
			want:  diag.Origin{File: filepath.Join("testdata", "split", "pet.yaml"), Line: 1, Col: 1},
		},
		{
			name:  "Path names the in-memory spec",
			input: Input{Spec: data, Path: filepath.Join("testdata", "split", "openapi.yaml"), Config: parseConfig(t, "{}", "")},
			want:  diag.Origin{File: filepath.Join("testdata", "split", "pet.yaml"), Line: 1, Col: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, runErr := Run(context.Background(), libopenapi.New(), tt.input)
			require.NoError(t, runErr)
			assert.Equal(t, tt.want, out.Positions["/components/schemas/pet"])
		})
	}
}

func TestRunRemoteSpecAndOverlay(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.FileServer(http.Dir(filepath.Join("testdata", "split"))))
	t.Cleanup(srv.Close)

	cfg := parseConfig(t, fmt.Sprintf("spec:\n  path: %s/openapi.yaml\n  overlays: [%s/overlay.yaml]\n", srv.URL, srv.URL), "")
	out, err := Run(context.Background(), libopenapi.New(), Input{Config: cfg})
	require.NoError(t, err)
	assert.Contains(t, string(out.Bytes), "x-go-name: Animal")
	assert.Equal(t, diag.Origin{File: srv.URL + "/pet.yaml", Line: 1, Col: 1}, out.Positions["/components/schemas/pet"])
}

func TestRunErrors(t *testing.T) {
	t.Parallel()

	split := filepath.Join("testdata", "split")
	tests := []struct {
		name    string
		input   Input
		wantErr error
	}{
		{name: "No spec", input: Input{Config: parseConfig(t, "{}", "")}, wantErr: ErrNoSpec},
		{name: "Missing file", input: Input{Config: parseConfig(t, "spec: {path: missing.yaml}\n", "testdata")}, wantErr: ErrRead},
		{name: "Invalid YAML", input: Input{Spec: []byte("a: [b\n"), Config: parseConfig(t, "{}", "")}, wantErr: oasdoc.ErrParse},
		{name: "Swagger 2.0", input: Input{Spec: []byte("swagger: '2.0'\n"), Config: parseConfig(t, "{}", "")}, wantErr: oasdoc.ErrUnsupportedVersion},
		{
			name:    "Ref to a missing file",
			input:   Input{Spec: []byte("openapi: 3.1.0\ncomponents:\n  schemas:\n    A: {$ref: ./missing.yaml}\n"), Config: parseConfig(t, "{}", "")},
			wantErr: bundle.ErrLoad,
		},
		{name: "Missing overlay", input: Input{Config: parseConfig(t, "spec: {path: openapi.yaml, overlays: [missing.yaml]}\n", split)}, wantErr: ErrRead},
		{name: "Invalid overlay", input: Input{Config: parseConfig(t, "spec: {path: openapi.yaml, overlays: [pet.yaml]}\n", split)}, wantErr: provider.ErrOverlay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Run(context.Background(), libopenapi.New(), tt.input)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestPipelineMarshalErrors(t *testing.T) {
	t.Parallel()

	broken := func(t *testing.T) *job {
		t.Helper()
		doc, err := oasdoc.Parse([]byte("openapi: 3.1.0\n"), "spec.yaml")
		require.NoError(t, err)
		require.NoError(t, doc.Set("/bad", &yaml.Node{Kind: 99}))
		return &job{p: libopenapi.New(), cfg: parseConfig(t, "spec: {overlays: [o.yaml]}\n", ""), doc: doc, isChanged: true}
	}

	_, err := broken(t).result(nil, nil)
	require.ErrorIs(t, err, oasdoc.ErrMarshal)
	require.ErrorIs(t, broken(t).overlays(context.Background()), oasdoc.ErrMarshal)
}

func TestWithOrigins(t *testing.T) {
	t.Parallel()

	positions := map[string]diag.Origin{
		"":         {File: "spec.yaml", Line: 1, Col: 1},
		"/a":       {File: "spec.yaml", Line: 2, Col: 1},
		"/a/b":     {File: "b.yaml", Line: 1, Col: 1},
		"/unknown": {},
	}
	own := diag.Origin{File: "own.yaml", Line: 9, Col: 9}
	got := withOrigins([]diag.Diagnostic{
		{Code: "exact", Pointer: "/a/b"},
		{Code: "ancestor", Pointer: "/a/c/d"},
		{Code: "own", Pointer: "/a", Origin: own},
		{Code: "root", Pointer: ""},
	}, positions)
	assert.Equal(t, []diag.Diagnostic{
		{Code: "exact", Pointer: "/a/b", Origin: positions["/a/b"]},
		{Code: "own", Pointer: "/a", Origin: own},
		{Code: "root", Pointer: "", Origin: positions[""]},
		{Code: "ancestor", Pointer: "/a/c/d", Origin: positions["/a"]},
	}, got)
	assert.Equal(t, []diag.Diagnostic{{Code: "none", Pointer: "/x"}}, withOrigins([]diag.Diagnostic{{Code: "none", Pointer: "/x"}}, nil))
}

func parseConfig(t *testing.T, src, dir string) *config.Config {
	t.Helper()

	cfg, err := config.Parse([]byte(src), dir)
	require.NoError(t, err)
	return cfg
}

// golden compares got with name; UPDATE=1 rewrites the file instead.
func golden(t *testing.T, name, got string) {
	t.Helper()

	if os.Getenv("UPDATE") != "" {
		require.NoError(t, os.WriteFile(name, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(name)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}
