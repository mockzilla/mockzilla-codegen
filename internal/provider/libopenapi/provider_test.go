// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/pb33f/libopenapi/datamodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/provider"
)

const minimalSpec = "openapi: 3.1.0\ninfo: {title: t, version: '1'}\npaths: {}\n"

func TestApplyOverlay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		overlay   string
		want      string
		wantDiags []diag.Diagnostic
		wantErr   error
	}{
		{
			name:    "Update lands in the spec",
			overlay: "overlay: 1.0.0\ninfo: {title: o, version: '1'}\nactions:\n  - target: $.info\n    update: {title: new}\n",
			want:    "title: new",
		},
		{
			name:    "Target that matches nothing is a warning",
			overlay: "overlay: 1.0.0\ninfo: {title: o, version: '1'}\nactions:\n  - target: $.nope\n    update: {x: 1}\n",
			want:    "title: t",
			wantDiags: []diag.Diagnostic{{
				Severity: diag.Warning,
				Code:     diag.CodeOverlayTarget,
				Message:  "overlay target $.nope: target matched zero nodes",
			}},
		},
		{name: "Invalid overlay", overlay: "not: an overlay\n", wantErr: provider.ErrOverlay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, diags, err := New().ApplyOverlay(context.Background(), []byte(minimalSpec), []byte(tt.overlay))
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, string(out), tt.want)
			assert.Equal(t, tt.wantDiags, diags)
		})
	}
}

func TestBundle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	root := "openapi: 3.1.0\ninfo: {title: t, version: '1'}\npaths:\n  /pets:\n    get:\n      responses:\n        '200':\n          description: ok\n          content:\n            application/json:\n              schema: {$ref: 'models.yaml#/Pet'}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.yaml"), []byte(root), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "models.yaml"), []byte("Pet:\n  type: object\n"), 0o644))
	local := "openapi: 3.1.0\ninfo: {title: t, version: '1'}\npaths: {}\ncomponents:\n  schemas:\n    A: {$ref: '#/components/schemas/B'}\n    B: {type: string}\n"

	tests := []struct {
		name        string
		src         provider.Source
		wantSame    bool
		wantOrigins map[string]diag.Origin
		wantErr     error
	}{
		{name: "Local refs only pass through untouched", src: provider.Source{Data: []byte(local)}, wantSame: true},
		{
			name: "External file is lifted into components",
			src:  provider.Source{Data: []byte(root), Path: filepath.Join(dir, "root.yaml")},
			wantOrigins: map[string]diag.Origin{
				"/components/schemas/Pet": {File: filepath.Join(dir, "models.yaml"), Line: 2, Col: 3},
			},
		},
		{
			name: "Base dir resolves refs of in-memory bytes",
			src:  provider.Source{Data: []byte(root), BaseDir: dir},
			wantOrigins: map[string]diag.Origin{
				"/components/schemas/Pet": {File: filepath.Join(dir, "models.yaml"), Line: 2, Col: 3},
			},
		},
		{name: "Missing external file", src: provider.Source{Data: []byte(root), Path: filepath.Join(t.TempDir(), "root.yaml")}, wantErr: provider.ErrBundle},
		{name: "Invalid YAML", src: provider.Source{Data: []byte("a: [b\n")}, wantErr: oasdoc.ErrParse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := New().Bundle(context.Background(), tt.src)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.ErrorIs(t, err, provider.ErrBundle)
				return
			}
			require.NoError(t, err)
			if tt.wantSame {
				assert.Equal(t, &provider.Bundled{Data: tt.src.Data}, got)
				return
			}
			assert.Equal(t, tt.wantOrigins, got.Origins)

			doc, _, err := New().Parse(context.Background(), got.Data, provider.ParseOptions{})
			require.NoError(t, err)
			assert.Equal(t, "Pet", doc.Operations[0].Responses[0].Contents[0].Schema.Ref.Name)
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantErr error
	}{
		{name: "Invalid YAML", src: "a: [b\n", wantErr: provider.ErrParse},
		{name: "Not an OpenAPI document", src: "a: b\n", wantErr: provider.ErrParse},
		{name: "Swagger 2.0", src: "swagger: '2.0'\ninfo: {title: t, version: '1'}\npaths: {}\n", wantErr: provider.ErrUnsupportedVersion},
		{
			name:    "Ref to nothing",
			src:     minimalSpec + "components:\n  schemas:\n    A: {$ref: '#/components/schemas/Missing'}\n",
			wantErr: provider.ErrParse,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := New().Parse(context.Background(), []byte(tt.src), provider.ParseOptions{File: "spec.yaml"})
			require.ErrorIs(t, err, tt.wantErr)
			assert.ErrorContains(t, err, "spec.yaml")
		})
	}
}

func TestCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := New()

	_, _, err := p.ApplyOverlay(ctx, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	_, err = p.Bundle(ctx, provider.Source{})
	require.ErrorIs(t, err, context.Canceled)
	_, _, err = p.Parse(ctx, nil, provider.ParseOptions{})
	require.ErrorIs(t, err, context.Canceled)
}

func TestBundleConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		src          provider.Source
		wantBasePath string
		wantSpecFile string
		wantBaseURL  *url.URL
	}{
		{name: "File path sets the base path", src: provider.Source{Path: "/specs/api.yaml"}, wantBasePath: "/specs", wantSpecFile: "/specs/api.yaml"},
		{name: "Base dir wins over the file's folder", src: provider.Source{Path: "/specs/api.yaml", BaseDir: "/shared"}, wantBasePath: "/shared", wantSpecFile: "/specs/api.yaml"},
		{
			name:        "URL sets the base URL",
			src:         provider.Source{Path: "https://example.com/specs/api.yaml", AllowRemote: true},
			wantBaseURL: &url.URL{Scheme: "https", Host: "example.com", Path: "/specs"},
		},
		{name: "Bytes with nothing else", src: provider.Source{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := New()
			assert.Equal(t, &datamodel.DocumentConfiguration{
				Logger:                  p.logger,
				TransformSiblingRefs:    true,
				AllowFileReferences:     true,
				AllowRemoteReferences:   tt.src.AllowRemote,
				ExtractRefsSequentially: true,
				BasePath:                tt.wantBasePath,
				SpecFilePath:            tt.wantSpecFile,
				BaseURL:                 tt.wantBaseURL,
			}, p.bundleConfig(tt.src))
		})
	}
}

func TestSpecVersion(t *testing.T) {
	t.Parallel()

	_, err := specVersion(&datamodel.SpecInfo{SpecFormat: datamodel.OAS2, Version: "2.0"})
	assert.ErrorIs(t, err, provider.ErrUnsupportedVersion)
}

func TestRecoverPanic(t *testing.T) {
	t.Parallel()

	err := func() (err error) {
		defer recoverPanic(&err, "spec.yaml")
		panic("boom")
	}()
	require.ErrorIs(t, err, provider.ErrProviderPanic)
	assert.EqualError(t, err, "provider panic: spec.yaml: boom")
}
