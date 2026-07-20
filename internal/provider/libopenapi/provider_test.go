// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"context"
	"testing"

	"github.com/pb33f/libopenapi/datamodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/codegen/internal/diag"
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
	_, _, err = p.Parse(ctx, nil, provider.ParseOptions{})
	require.ErrorIs(t, err, context.Canceled)
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
