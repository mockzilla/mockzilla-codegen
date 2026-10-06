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
		{name: "Nullable", typ: gomodel.Nullable{Elem: gomodel.DeclRef{Decl: pet}}, want: "Nullable[Pet]"},
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
		{name: "Nullable", typ: gomodel.Nullable{Elem: pet}, want: pet},
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
		{name: "Map key is left as it is", expr: "map[Kind]Pet", pkg: "models", want: "map[Kind]models.Pet"},
		{name: "Array", expr: "[4]Pet", pkg: "models", want: "[4]models.Pet"},
		{name: "Channel", expr: "<-chan Pet", pkg: "models", want: "<-chan models.Pet"},
		{name: "Wide character before the identifier", expr: "\ufeffPet", pkg: "models", want: "\ufeffmodels.Pet"},
		{name: "No package", expr: "[]Pet", want: "[]Pet"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Qualify(tc.expr, tc.pkg))
		})
	}
}

func TestCanQualify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{name: "Identifier", expr: "Pet", want: true},
		{name: "Pointer to a pointer", expr: "**Pet", want: true},
		{name: "Slice of arrays", expr: "[][4]Pet", want: true},
		{name: "Map of maps, whatever the key", expr: "map[Kind]map[func() string]*Pet", want: true},
		{name: "Channels", expr: "chan<- <-chan Pet", want: true},
		{name: "Space before the identifier", expr: "[] Pet", want: true},
		{name: "Space after the identifier", expr: "Pet "},
		{name: "Comment after the identifier", expr: "Pet // Pet"},
		{name: "Line break before the type", expr: "\n*Pet"},
		{name: "Byte order mark before the type", expr: "\ufeffPet"},
		{name: "Array of unknown length", expr: "[...]Pet"},
		{name: "Map without a key", expr: "map[]Pet"},
		{name: "Generic type", expr: "Option[Pet]"},
		{name: "Function", expr: "func(Pet)"},
		{name: "Function that returns the identifier", expr: "func() Pet"},
		{name: "Qualified already", expr: "models.Pet"},
		{name: "Parentheses", expr: "*(Pet)"},
		{name: "Struct", expr: "struct{ Pet }"},
		{name: "No expression", expr: "[]"},
		{name: "Empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, CanQualify(tc.expr))
		})
	}
}
