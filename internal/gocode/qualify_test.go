// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestText(t *testing.T) {
	t.Parallel()

	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes}
	tests := []struct {
		name string
		typ  gomodel.Type
		want string
	}{
		{name: "Builtin", typ: gomodel.Builtin{Name: "string"}, want: "string"},
		{name: "Declaration is not qualified", typ: gomodel.DeclRef{Decl: pet}, want: "Pet"},
		{name: "Qualified type is not qualified either", typ: gomodel.Qualified{Import: gomodel.Import{Path: "time"}, Name: "Time"}, want: "Time"},
		{name: "Pointer, slice and map", typ: gomodel.Map{Key: gomodel.Builtin{Name: "string"}, Elem: gomodel.Slice{Elem: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: pet}}}}, want: "map[string][]*Pet"},
		{name: "Nil type is any", want: "any"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Text(tc.typ))
		})
	}
}

func TestLeaf(t *testing.T) {
	t.Parallel()

	pet := gomodel.DeclRef{Decl: &gomodel.Decl{Name: "Pet"}}
	tests := []struct {
		name string
		typ  gomodel.Type
		want gomodel.Type
	}{
		{name: "Named type is its own leaf", typ: pet, want: pet},
		{name: "Map of slices of pointers", typ: gomodel.Map{Key: gomodel.Builtin{Name: "string"}, Elem: gomodel.Slice{Elem: gomodel.Pointer{Elem: pet}}}, want: pet},
		{name: "Nil type"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Leaf(tc.typ))
		})
	}
}

func TestQualify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
		pkg  string
		want string
	}{
		{name: "Identifier", expr: "Pet", pkg: "models", want: "models.Pet"},
		{name: "Slice of pointers", expr: "[]*Pet", pkg: "models", want: "[]*models.Pet"},
		{name: "Map", expr: "map[string]Pet_2", pkg: "models", want: "map[string]models.Pet_2"},
		{name: "No package", expr: "[]Pet", want: "[]Pet"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Qualify(tc.expr, tc.pkg))
		})
	}
}
