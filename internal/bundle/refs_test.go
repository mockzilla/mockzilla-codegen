// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/codegen/internal/oasdoc"
)

func TestSectionEntries(t *testing.T) {
	t.Parallel()

	doc, err := oasdoc.Parse([]byte("schemas: {A: {}, B: {}}\nx-ext: {C: {}}\nresponses: []\nparameters: {P: {}}\n"), "spec.yaml")
	require.NoError(t, err)

	got := sectionEntries(doc.Root())
	assert.Equal(t, []entry{
		{section: "schemas", name: "A", value: doc.Get("/schemas/A")},
		{section: "schemas", name: "B", value: doc.Get("/schemas/B")},
		{section: "parameters", name: "P", value: doc.Get("/parameters/P")},
	}, got)
	assert.Nil(t, sectionEntries(nil))
}

func TestPureRef(t *testing.T) {
	t.Parallel()

	doc, err := oasdoc.Parse([]byte("a: {$ref: x.yaml}\nb: {$ref: x.yaml, description: d}\nc: {$ref: {}}\nd: {type: string}\ne: [1]\n"), "spec.yaml")
	require.NoError(t, err)

	assert.Equal(t, "x.yaml", pureRef(doc.Get("/a")))
	for _, ptr := range []string{"/b", "/c", "/d", "/e"} {
		assert.Empty(t, pureRef(doc.Get(ptr)), ptr)
	}
}

func TestNameOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		t    target
		want string
	}{
		{name: "Last pointer token", t: target{loc: "common.yaml", frag: "/components/schemas/Error"}, want: "Error"},
		{name: "Escaped token", t: target{loc: "common.yaml", frag: "/defs/a~1b"}, want: "a/b"},
		{name: "Whole file", t: target{loc: "schemas/Pet.yaml"}, want: "Pet"},
		{name: "Empty last token", t: target{loc: "schemas/Pet.yaml", frag: "/"}, want: "Pet"},
		{name: "Bad percent escape", t: target{loc: "schemas/Pet.yaml", frag: "/%zz"}, want: "Pet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, nameOf(tt.t))
		})
	}
}

func TestIsFileName(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{"cat.yaml": true, "cat.yml": true, "cat.json": true, "Cat": false, "cat.txt": false}
	for name, want := range tests {
		assert.Equal(t, want, isFileName(name), name)
	}
}
