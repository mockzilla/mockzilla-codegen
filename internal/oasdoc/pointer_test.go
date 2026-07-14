// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

const pointerDoc = `paths:
  /pets/{id}:
    get:
      tags: [a, b]
components:
  schemas:
    a~b: {type: string}
    base: &base {type: object}
    alias: *base
`

func TestEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		token   string
		escaped string
	}{
		{name: "Plain token is unchanged", token: "Pet", escaped: "Pet"},
		{name: "Slash", token: "/pets/{id}", escaped: "~1pets~1{id}"},
		{name: "Tilde goes first so ~1 in the input survives", token: "a~1/b", escaped: "a~01~1b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.escaped, Escape(tt.token))
			assert.Equal(t, tt.token, Unescape(tt.escaped))
		})
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ptr     string
		want    []string
		wantErr bool
	}{
		{name: "Empty pointer is the root", ptr: ""},
		{name: "Bare fragment is the root", ptr: "#"},
		{name: "Tokens are unescaped", ptr: "/paths/~1pets~1{id}/get", want: []string{"paths", "/pets/{id}", "get"}},
		{name: "Fragment tokens are percent-decoded", ptr: "#/paths/~1pets~1%7Bid%7D", want: []string{"paths", "/pets/{id}"}},
		{name: "Plain pointers are not percent-decoded", ptr: "/a%20b", want: []string{"a%20b"}},
		{name: "Empty token", ptr: "/", want: []string{""}},
		{name: "Missing leading slash", ptr: "paths", wantErr: true},
		{name: "Bad percent escape", ptr: "#/a%zz", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Split(tt.ptr)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrPointer)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ptr  string
		want string
	}{
		{name: "Root", ptr: "", want: "paths"},
		{name: "Escaped path key", ptr: "/paths/~1pets~1{id}/get/tags/1", want: "b"},
		{name: "Fragment form", ptr: "#/paths/~1pets~1%7Bid%7D/get/tags/0", want: "a"},
		{name: "Escaped tilde", ptr: "/components/schemas/a~0b/type", want: "string"},
		{name: "Alias is followed", ptr: "/components/schemas/alias/type", want: "object"},
		{name: "Missing key", ptr: "/components/responses"},
		{name: "Index out of range", ptr: "/paths/~1pets~1{id}/get/tags/2"},
		{name: "Negative index", ptr: "/paths/~1pets~1{id}/get/tags/-1"},
		{name: "Index with a leading zero", ptr: "/paths/~1pets~1{id}/get/tags/01"},
		{name: "Index that is not a number", ptr: "/paths/~1pets~1{id}/get/tags/x"},
		{name: "Step into a scalar", ptr: "/components/schemas/a~0b/type/x"},
		{name: "Invalid pointer", ptr: "components"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := Parse([]byte(pointerDoc), "spec.yaml")
			require.NoError(t, err)

			n := d.Get(tt.ptr)
			if tt.want == "" {
				assert.Nil(t, n)
				return
			}
			require.NotNil(t, n)
			if n.Kind == yaml.MappingNode {
				n = n.Content[0]
			}
			assert.Equal(t, tt.want, n.Value)
		})
	}
}

func TestSet(t *testing.T) {
	t.Parallel()

	scalar := func() *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "new"} }
	mapping := func() *yaml.Node {
		return &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "openapi"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "3.1.0"},
		}}
	}
	tests := []struct {
		name    string
		ptr     string
		value   func() *yaml.Node
		want    string
		wantErr error
	}{
		{
			name:  "Replace a mapping value",
			ptr:   "/info/title",
			value: scalar,
			want:  "info:\n  title: new\ntags:\n  - a\n",
		},
		{
			name:  "Add a missing key",
			ptr:   "/info/version",
			value: scalar,
			want:  "info:\n  title: t\n  version: new\ntags:\n  - a\n",
		},
		{
			name:  "Replace a sequence item",
			ptr:   "/tags/0",
			value: scalar,
			want:  "info:\n  title: t\ntags:\n  - new\n",
		},
		{
			name:  "Append to a sequence",
			ptr:   "/tags/-",
			value: scalar,
			want:  "info:\n  title: t\ntags:\n  - a\n  - new\n",
		},
		{
			name:  "Replace the root",
			ptr:   "",
			value: mapping,
			want:  "openapi: 3.1.0\n",
		},
		{name: "Root must be a mapping", ptr: "", value: scalar, wantErr: ErrNotObject},
		{name: "Missing parent", ptr: "/paths/x", value: scalar, wantErr: ErrNotFound},
		{name: "Index out of range", ptr: "/tags/5", value: scalar, wantErr: ErrNotFound},
		{name: "Parent is a scalar", ptr: "/info/title/x", value: scalar, wantErr: ErrNotFound},
		{name: "Invalid pointer", ptr: "info", value: scalar, wantErr: ErrPointer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := Parse([]byte("info:\n  title: t\ntags:\n  - a\n"), "spec.yaml")
			require.NoError(t, err)

			err = d.Set(tt.ptr, tt.value())
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			out, err := d.Marshal()
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))
		})
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ptr       string
		isDeleted bool
		want      string
	}{
		{name: "Mapping key", ptr: "/info/title", isDeleted: true, want: "info: {}\ntags:\n  - a\n  - b\n"},
		{name: "Sequence item", ptr: "/tags/0", isDeleted: true, want: "info:\n  title: t\ntags:\n  - b\n"},
		{name: "Missing key", ptr: "/info/version"},
		{name: "Index out of range", ptr: "/tags/2"},
		{name: "Missing parent", ptr: "/paths/x"},
		{name: "Parent is a scalar", ptr: "/info/title/x"},
		{name: "Root cannot be deleted", ptr: ""},
		{name: "Invalid pointer", ptr: "info"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const src = "info:\n  title: t\ntags:\n  - a\n  - b\n"
			d, err := Parse([]byte(src), "spec.yaml")
			require.NoError(t, err)

			assert.Equal(t, tt.isDeleted, d.Delete(tt.ptr))
			out, err := d.Marshal()
			require.NoError(t, err)
			want := tt.want
			if want == "" {
				want = src
			}
			assert.Equal(t, want, string(out))
		})
	}
}
