// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package naming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{name: "Plain folder name is kept", dir: "/work/api", want: "api"},
		{name: "Upper case is lowered", dir: "/work/PetStore", want: "petstore"},
		{name: "Separators are dropped", dir: "/work/pet-store_v2.go", want: "petstorev2go"},
		{name: "Leading digits are dropped", dir: "/work/2fa", want: "fa"},
		{name: "Only digits falls back", dir: "/work/2026", want: "api"},
		{name: "Non-ASCII letters are dropped", dir: "/work/café", want: "caf"},
		{name: "Keyword falls back", dir: "/work/type", want: "api"},
		{name: "Current dir falls back", dir: ".", want: "api"},
		{name: "Root dir falls back", dir: "/", want: "api"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Package(tc.dir))
		})
	}
}

func TestImportName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "Last element of a module path", path: "github.com/google/uuid", want: "uuid"},
		{name: "Last element of a standard library path", path: "encoding/json", want: "json"},
		{name: "Path of one element", path: "time", want: "time"},
		{name: "Major version is not part of the name", path: "github.com/go-chi/chi/v5", want: "chi"},
		{name: "v1 is a name", path: "example.com/v1", want: "v1"},
		{name: "Lone version element is a name", path: "v2", want: "v2"},
		{name: "go- prefix is dropped", path: "example.com/go-redis", want: "redis"},
		{name: "Anything after a dot is dropped", path: "gopkg.in/yaml.v3", want: "yaml"},
		{name: "Other separators are dropped", path: "example.com/shop-models", want: "shopmodels"},
		{name: "Underscore and upper case stay", path: "example.com/Shop_models", want: "Shop_models"},
		{name: "Keyword gives no name", path: "example.com/type", want: ""},
		{name: "Leading digit gives no name", path: "example.com/9lives", want: ""},
		{name: "Only separators give no name", path: "example.com/---", want: ""},
		{name: "Blank gives no name", path: "example.com/_", want: ""},
		{name: "Empty path gives no name", path: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, ImportName(tc.path))
		})
	}
}
