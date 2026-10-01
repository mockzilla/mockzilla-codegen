// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

func TestWalk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "Known keys at every level pass",
			src:  "spec: {filter: {include: {tags: [a]}}}\nserver: {scaffold: {port: 1}}\nimports: [{package: a}]\n",
		},
		{
			name: "Unknown top-level key",
			src:  "specs: {}\n",
			want: []string{"specs"},
		},
		{
			name: "Unknown key in a nested block",
			src:  "spec: {filter: {exclude: {tag: [a]}}}\n",
			want: []string{"spec.filter.exclude.tag"},
		},
		{
			name: "Unknown key in a pointer block",
			src:  "server: {scaffold: {prt: 1}}\n",
			want: []string{"server.scaffold.prt"},
		},
		{
			name: "Unknown key in a list item",
			src:  "imports: [{package: a}, {package: b, alais: c}]\n",
			want: []string{"imports[1].alais"},
		},
		{
			name: "Several unknown keys come back in document order",
			src:  "zeta: 1\nmodels: {int_type: int}\nalpha: 2\n",
			want: []string{"zeta", "models.int_type", "alpha"},
		},
		{
			name: "Keys inside free-form maps are not checked",
			src:  "user-context: {any: {thing: 1}}\ntemplates: {header: x}\noutput: {files: {./a.go: [models]}}\n",
		},
		{
			name: "Unknown key in a block inside a map",
			src:  "templates: {server.service-header: {fiel: a}, server.router-extra: {file: b}}\n",
			want: []string{"templates.server.service-header.fiel"},
		},
		{
			name: "Merge key in a map of blocks is left to the decoder",
			src:  "user-context: {base: &b {server.service-header: {fiel: a}}}\ntemplates: {<<: *b, server.router-extra: {file: b, nope: 1}}\n",
			want: []string{"templates.server.router-extra.nope"},
		},
		{
			name: "Alias is checked where it is used",
			src:  "user-context: {base: &s {framework: chi, nope: 1}}\nserver: *s\n",
			want: []string{"server.nope"},
		},
		{
			name: "Merge key is left to the decoder",
			src:  "user-context: {base: &b {package: a}}\nimports: [{<<: *b, alias: x}]\n",
		},
		{
			name: "Scalar where a block belongs is left to the decoder",
			src:  "server: chi\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, walk(rootNode(t, tc.src), reflect.TypeFor[Config](), ""))
		})
	}
}

func TestWalkOpensEmptyBlocks(t *testing.T) {
	t.Parallel()

	n := rootNode(t, "server:\nclient: ~\nspec: {simplify: }\nmodels: {int-type: }\n")
	require.Empty(t, walk(n, reflect.TypeFor[Config](), ""))

	var cfg Config
	require.NoError(t, n.Load(&cfg))
	assert.Equal(t, Config{
		Spec:   Spec{Simplify: &Simplify{}},
		Models: &Models{},
		Server: &Server{},
		Client: &Client{},
	}, cfg)
}

func rootNode(t *testing.T, src string) *yaml.Node {
	t.Helper()

	var doc yaml.Node
	require.NoError(t, yaml.Load([]byte(src), &doc))
	return doc.Content[0]
}
