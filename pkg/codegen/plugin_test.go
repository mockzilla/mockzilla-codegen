// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTypeRefExpr(t *testing.T) {
	t.Parallel()

	pet := TypeRef{Name: "[]Pet", Package: "models", ImportPath: "example.com/work/models"}
	tests := []struct {
		name string
		typ  TypeRef
		from string
		want string
	}{
		{name: "Type that needs no import", typ: TypeRef{Name: "func() any"}, from: "example.com/work/api", want: "func() any"},
		{name: "Type of the package written", typ: pet, from: "example.com/work/models", want: "[]Pet"},
		{name: "Type of another package", typ: pet, from: "example.com/work/api", want: "[]models.Pet"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.typ.Expr(tc.from))
		})
	}
}

func TestScaffoldKindString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "service", ScaffoldService.String())
	assert.Equal(t, "middleware", ScaffoldMiddleware.String())
	assert.Equal(t, "main", ScaffoldMain.String())
	assert.Equal(t, "unknown", ScaffoldKind(9).String())
}
