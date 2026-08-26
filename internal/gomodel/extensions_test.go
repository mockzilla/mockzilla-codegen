// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/extension"
)

func TestExtensionsInModel(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.ExtraTags = []string{"yaml"}

	checkGolden(t, "extensions", "extensions", opts)
}

func TestGoType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		typ  extension.Type
		want Type
	}{
		{name: "Builtin", typ: extension.Type{Name: "int64"}, want: Builtin{Name: "int64"}},
		{name: "Standard library type", typ: extension.Type{Name: "time.Duration"}, want: Qualified{Import: Import{Path: "time"}, Name: "Duration"}},
		{
			name: "Type of an imported package",
			typ:  extension.Type{Name: "dec.Decimal", Path: "github.com/shopspring/decimal", Alias: "dec"},
			want: Qualified{Import: Import{Path: "github.com/shopspring/decimal", Alias: "dec"}, Name: "Decimal"},
		},
		{name: "Composite type as written", typ: extension.Type{Name: "[]uuid.UUID"}, want: Builtin{Name: "[]uuid.UUID"}},
		{name: "Two dots as written", typ: extension.Type{Name: "a.b.C"}, want: Builtin{Name: "a.b.C"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, goType(&tc.typ))
		})
	}
}
