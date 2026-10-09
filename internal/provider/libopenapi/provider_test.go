// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/pb33f/libopenapi/datamodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
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
		name     string
		src      string
		wantErr  error
		wantText string
	}{
		{name: "Invalid YAML", src: "a: [b\n", wantErr: provider.ErrParse},
		{
			name:     "JSON syntax error at its line and column",
			src:      "{\n  \"openapi\": \"3.1.0\",\n  \"info\": {\"title\": \"t\", \"version\": \"1\"},\n  \"paths\": {\"a\": 1,}\n}\n",
			wantErr:  provider.ErrParse,
			wantText: "spec.yaml:4:20: failed to unmarshal JSON",
		},
		{name: "Not an OpenAPI document", src: "a: b\n", wantErr: provider.ErrParse},
		{name: "Swagger 2.0", src: "swagger: '2.0'\ninfo: {title: t, version: '1'}\npaths: {}\n", wantErr: provider.ErrUnsupportedVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := New().Parse(context.Background(), []byte(tt.src), provider.ParseOptions{File: "spec.yaml"})
			require.ErrorIs(t, err, tt.wantErr)
			assert.ErrorContains(t, err, cmp.Or(tt.wantText, "spec.yaml"))
		})
	}
}

func TestParseUnresolvedRefs(t *testing.T) {
	t.Parallel()

	file := diag.Origin{File: "spec.yaml"}
	tests := []struct {
		name      string
		src       string
		wantDiags []diag.Diagnostic
	}{
		{
			name: "Ref inside an example value",
			src: "openapi: 3.1.0\ninfo: {title: t, version: '1'}\n" +
				"paths:\n  /a:\n    get:\n      responses:\n        '200':\n          description: ok\n" +
				"          content:\n            application/json:\n" +
				"              examples:\n                many: {$ref: '#/components/examples/Many'}\n" +
				"components:\n  examples:\n    One:\n      value: {id: 1}\n" +
				"    Many:\n      value: {$ref: '#/components/examples/One/value'}\n",
			wantDiags: []diag.Diagnostic{{
				Severity: diag.Warning,
				Code:     diag.CodeBuildIssue,
				Origin:   file,
				Message:  "cannot resolve reference `#/components/examples/One/value`, it's missing: $.components.examples['One'].value [18:15]",
			}},
		},
		{
			name: "Ref to nothing",
			src:  minimalSpec + "components:\n  schemas:\n    A: {$ref: '#/components/schemas/Missing'}\n",
			wantDiags: []diag.Diagnostic{
				{
					Severity: diag.Warning,
					Code:     diag.CodeBuildIssue,
					Origin:   file,
					Message:  "cannot resolve reference `#/components/schemas/Missing`, it's missing: $.components.schemas['Missing'] [6:9]",
				},
				{
					Severity: diag.Warning,
					Code:     diag.CodeBuildIssue,
					Origin:   file,
					Message:  "component `#/components/schemas/Missing` does not exist in the specification",
				},
				{
					Severity: diag.Error,
					Code:     diag.CodeSchemaBuild,
					Pointer:  "/components/schemas/Missing",
					Origin:   diag.Origin{File: "spec.yaml", Line: 6, Col: 8},
					Message:  "schema cannot be built: build schema failed: reference cannot be found: '#/components/schemas/Missing'",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, diags, err := New().Parse(context.Background(), []byte(tt.src), provider.ParseOptions{File: "spec.yaml"})
			require.NoError(t, err)
			assert.Equal(t, tt.wantDiags, diags)
		})
	}
}

func TestSyntaxAt(t *testing.T) {
	t.Parallel()

	syntaxError := func(data string) error { return json.Unmarshal([]byte(data), new(any)) }
	tests := []struct {
		name string
		data string
		err  error
		want string
	}{
		{name: "Bad byte on a later line", data: "{\n  \"a\": 1,}", err: syntaxError("{\n  \"a\": 1,}"), want: "spec.json:2:10"},
		{name: "Bad byte on the first line", data: "{,}", err: syntaxError("{,}"), want: "spec.json:1:2"},
		{name: "Error of another kind", data: "{}", err: errors.New("other"), want: "spec.json"},
		{name: "Offset outside the data", data: "{}", err: &json.SyntaxError{Offset: 9}, want: "spec.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, syntaxAt("spec.json", []byte(tt.data), tt.err))
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
