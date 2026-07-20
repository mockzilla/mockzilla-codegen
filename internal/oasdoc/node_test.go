// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

func TestChild(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte("a: &x {b: 1}\nc: *x\nd: [1]\n"), "spec.yaml")
	require.NoError(t, err)

	tests := []struct {
		name string
		n    *yaml.Node
		key  string
		want *yaml.Node
	}{
		{name: "Key in a mapping", n: d.Root(), key: "a", want: d.Get("/a")},
		{name: "Alias gives its target", n: d.Root(), key: "c", want: d.Get("/a")},
		{name: "Missing key", n: d.Root(), key: "z"},
		{name: "Not a mapping", n: d.Get("/d"), key: "0"},
		{name: "Nil node", key: "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Same(t, tt.want, Child(tt.n, tt.key))
		})
	}
}

func TestSetAndDeleteChild(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte("a: 1\nb: 2\n"), "spec.yaml")
	require.NoError(t, err)

	SetChild(d.Root(), "a", NewString("one"))
	SetChild(d.Root(), "c", NewMapping())
	assert.True(t, DeleteChild(d.Root(), "b"))
	assert.False(t, DeleteChild(d.Root(), "b"))

	out, err := d.Marshal()
	require.NoError(t, err)
	assert.Equal(t, "a: one\nc: {}\n", string(out))
}

func TestClone(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte("base: &b {type: object}\ncopy: *b\n"), "spec.yaml")
	require.NoError(t, err)

	got := Clone(d.Root())
	require.NotSame(t, d.Root(), got)
	assert.NotSame(t, d.Get("/base"), got.Content[1])
	assert.Empty(t, got.Content[1].Anchor)
	assert.Equal(t, yaml.MappingNode, got.Content[3].Kind)
	assert.Equal(t, d.Get("/base").Line, got.Content[1].Line)

	c := &Doc{root: &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{got}}, indent: defaultIndent}
	out, err := c.Marshal()
	require.NoError(t, err)
	assert.Equal(t, "base: {type: object}\ncopy: {type: object}\n", string(out))
}

func TestDeleteChildren(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		drop        []string
		want        string
		wantRemoved bool
	}{
		{name: "Some entries", drop: []string{"a", "c"}, want: "b: 2\n", wantRemoved: true},
		{name: "No entries", drop: []string{"z"}, want: "a: 1\nb: 2\nc: 3\n"},
		{name: "Every entry", drop: []string{"a", "b", "c"}, want: "{}\n", wantRemoved: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := Parse([]byte("a: 1\nb: 2\nc: 3\n"), "spec.yaml")
			require.NoError(t, err)

			isRemoved := DeleteChildren(d.Root(), func(key string, _ *yaml.Node) bool { return slices.Contains(tt.drop, key) })
			out, err := d.Marshal()
			require.NoError(t, err)
			assert.Equal(t, tt.wantRemoved, isRemoved)
			assert.Equal(t, tt.want, string(out))
		})
	}
}
