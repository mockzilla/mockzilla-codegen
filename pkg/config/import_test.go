// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImportName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		imp  Import
		want string
	}{
		{name: "Path gives the name", imp: Import{Package: "github.com/google/uuid"}, want: "uuid"},
		{name: "Path with a major version", imp: Import{Package: "github.com/go-chi/chi/v5"}, want: "chi"},
		{name: "Alias wins over the path", imp: Import{Package: "example.com/shop/tenant", Alias: "tn"}, want: "tn"},
		{name: "Alias names a path that gives no name", imp: Import{Package: "example.com/9lives", Alias: "lives"}, want: "lives"},
		{name: "Import under _ has no name", imp: Import{Package: "embed", Alias: "_"}, want: ""},
		{name: "Path that gives no name", imp: Import{Package: "example.com/9lives"}, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.imp.Name())
		})
	}
}
