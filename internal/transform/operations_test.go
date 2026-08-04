// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package transform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
)

func TestHasOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "Path with an operation", src: "paths:\n  /a:\n    get: {}\n", want: true},
		{name: "Webhook with an operation", src: "webhooks:\n  a:\n    post: {}\n", want: true},
		{name: "Additional operation only", src: "paths:\n  /a:\n    additionalOperations: {COPY: {}}\n", want: true},
		{name: "Path item behind a ref", src: "paths:\n  /a: {$ref: '#/components/pathItems/A'}\ncomponents:\n  pathItems:\n    A: {get: {}}\n", want: true},
		{name: "Ref to nothing", src: "paths:\n  /a: {$ref: '#/components/pathItems/Missing'}\n"},
		{name: "Ref to another file", src: "paths:\n  /a: {$ref: './a.yaml'}\n"},
		{name: "Path items without operations", src: "paths:\n  /a: {parameters: []}\n"},
		{name: "Models only", src: "components:\n  schemas:\n    A: {type: string}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := oasdoc.Parse([]byte("openapi: 3.2.0\n"+tt.src), "spec.yaml")
			require.NoError(t, err)
			assert.Equal(t, tt.want, HasOperations(doc))
		})
	}
}

// runGolden applies fn to testdata/<dir>/in.yaml and compares the result with out.yaml.
func runGolden(t *testing.T, dir string, fn func(*oasdoc.Doc) (bool, []diag.Diagnostic)) {
	t.Helper()

	in := filepath.Join("testdata", dir, "in.yaml")
	data, err := os.ReadFile(in)
	require.NoError(t, err)
	doc, err := oasdoc.Parse(data, "in.yaml")
	require.NoError(t, err)

	isChanged, diags := fn(doc)
	out, err := doc.Marshal()
	require.NoError(t, err)

	var b strings.Builder
	b.Write(out)
	fmt.Fprintf(&b, "# changed: %t\n", isChanged)
	for _, d := range diags {
		fmt.Fprintf(&b, "# %s %s %s: %s\n", d.Severity, d.Code, d.Pointer, d.Message)
	}
	golden(t, filepath.Join("testdata", dir, "out.yaml"), b.String())
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
