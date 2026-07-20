// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package transform

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/pkg/config"
)

func TestSimplify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dir  string
		s    config.Simplify
	}{
		{name: "Unions collapse to one variant", dir: "simplify-unions", s: config.Simplify{Unions: true}},
		{name: "Extensions are stripped from every schema", dir: "simplify-extensions"},
		{
			name: "Fixed optional properties cap",
			dir:  "simplify-optional-fixed",
			s:    config.Simplify{OptionalProperties: &config.OptionalProperties{Min: 1, Max: 1}},
		},
		{
			name: "Seeded optional properties cap",
			dir:  "simplify-optional-seeded",
			s:    config.Simplify{OptionalProperties: &config.OptionalProperties{Min: 0, Max: 3, Seed: 7}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runGolden(t, tt.dir, func(doc *oasdoc.Doc) (bool, []diag.Diagnostic) { return Simplify(doc, tt.s), nil })
		})
	}
}

func TestSimplifySeedIsStable(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/simplify-optional-seeded/in.yaml")
	require.NoError(t, err)
	s := config.Simplify{OptionalProperties: &config.OptionalProperties{Min: 0, Max: 3, Seed: 7}}

	var outs []string
	for range 3 {
		doc, err := oasdoc.Parse(data, "in.yaml")
		require.NoError(t, err)
		Simplify(doc, s)
		out, err := doc.Marshal()
		require.NoError(t, err)
		outs = append(outs, string(out))
	}
	assert.Equal(t, outs[0], outs[1])
	assert.Equal(t, outs[0], outs[2])
}

func TestSimplifyWithNothingToDo(t *testing.T) {
	t.Parallel()

	doc, err := oasdoc.Parse([]byte("openapi: 3.1.0\ncomponents:\n  schemas:\n    A: {type: string}\n"), "spec.yaml")
	require.NoError(t, err)
	assert.False(t, Simplify(doc, config.Simplify{Unions: true}))
}
