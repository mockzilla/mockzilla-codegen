// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouterMethod(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Get", routerMethod("GET"))
	assert.Equal(t, "Options", routerMethod("OPTIONS"))
}

func TestWrites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "The field before a space", text: "{{.Packages.http}} x", want: true},
		{name: "The field at the end", text: "x .Packages.http", want: true},
		{name: "A longer field only", text: "{{.Packages.httpx}}", want: false},
		{name: "A longer field, then the field", text: "{{.Packages.httpx}} {{.Packages.http}}", want: true},
		{name: "No field", text: "{{.Framework}}", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, writes(tc.text, ".Packages.http"))
		})
	}
}

func TestRouterViewOverride(t *testing.T) {
	t.Parallel()

	m := petModel()
	opts := allOptions()
	opts.RouterExtra = "_ = {{.Runtime}}.ErrPanic"
	g, _ := New(m, opts)
	plain, _ := New(m, allOptions())

	got := routerView(g, fixture{m: m, g: g, cfg: scaffoldConfig}.scope(t, PartRouter))
	without := routerView(plain, fixture{m: m, g: plain, cfg: scaffoldConfig}.scope(t, PartRouter))

	assert.Equal(t, "runtime", got.Runtime, "the router imports what the override writes")
	assert.Empty(t, without.Runtime)
}
