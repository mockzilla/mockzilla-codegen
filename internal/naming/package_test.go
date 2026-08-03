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
