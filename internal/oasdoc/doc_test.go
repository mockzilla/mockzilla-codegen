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

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/spec"
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

const walkDoc = `paths:
  /a:
    get:
      responses:
        '200': {$ref: '#/components/responses/Ok'}
components:
  schemas:
    Pet:
      properties:
        $ref: {type: string}
        owner: {$ref: 'models.yaml#/Owner'}
      example: &ex {name: x}
    Copy: *ex
`

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantErr error
	}{
		{name: "YAML mapping", src: "openapi: 3.1.0\n"},
		{name: "JSON object", src: `{"openapi": "3.1.0"}`},
		{name: "Invalid YAML", src: "a: [b\n", wantErr: ErrParse},
		{name: "Empty input", src: "", wantErr: ErrNotObject},
		{name: "Scalar root", src: "hello\n", wantErr: ErrNotObject},
		{name: "Sequence root", src: "- a\n", wantErr: ErrNotObject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := Parse([]byte(tt.src), "spec.yaml")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.ErrorContains(t, err, "spec.yaml")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "spec.yaml", d.File())
			assert.Equal(t, yaml.MappingNode, d.Root().Kind)
		})
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		want    spec.Version
		wantErr string
	}{
		{name: "3.0 patch release", src: "openapi: 3.0.3\n", want: spec.V30},
		{name: "3.1", src: "openapi: 3.1.0\n", want: spec.V31},
		{name: "3.2", src: "openapi: 3.2.0\n", want: spec.V32},
		{name: "Bare minor version", src: "openapi: '3.1'\n", want: spec.V31},
		{name: "Swagger 2.0", src: "swagger: '2.0'\n", wantErr: "unsupported OpenAPI version: swagger 2.0"},
		{name: "No version field", src: "info: {}\n", wantErr: "unsupported OpenAPI version: no openapi field"},
		{name: "Unknown version", src: "openapi: 3.10.0\n", wantErr: "unsupported OpenAPI version: 3.10.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := Parse([]byte(tt.src), "spec.yaml")
			require.NoError(t, err)

			got, err := d.Version()
			if tt.wantErr != "" {
				require.ErrorIs(t, err, ErrUnsupportedVersion)
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "Two-space indentation and indented sequences",
			src:  "openapi: 3.1.0\ninfo:\n  title: t\ntags:\n  - a\n",
		},
		{
			name: "Four-space indentation",
			src:  "openapi: 3.1.0\ninfo:\n    title: t\n    contact:\n        name: n\n",
		},
		{
			name: "Compact sequences",
			src:  "tags:\n- a\n- b\ninfo:\n  title: t\n",
		},
		{
			name: "Long lines are not folded",
			src:  "info:\n  description: aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq\n",
		},
		{
			name: "Flow mapping on one line is kept and not used as an indentation sample",
			src:  "info: {title: t}\npaths:\n    /a: {}\n",
		},
		{
			name: "Indentation beyond the supported range falls back to two",
			src:  "info:\n           title: t\n",
			want: "info:\n  title: t\n",
		},
		{
			name: "JSON input becomes block YAML with its indentation",
			src:  "{\n    \"openapi\": \"3.1.0\",\n    \"info\": {\n        \"title\": \"123\"\n    },\n    \"tags\": [\"a\"]\n}\n",
			want: "openapi: 3.1.0\ninfo:\n    title: \"123\"\ntags:\n    - a\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := Parse([]byte(tt.src), "spec.yaml")
			require.NoError(t, err)

			out, err := d.Marshal()
			require.NoError(t, err)
			want := tt.want
			if want == "" {
				want = tt.src
			}
			assert.Equal(t, want, string(out))
		})
	}
}

func TestMarshalError(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte("a: 1\n"), "spec.yaml")
	require.NoError(t, err)
	require.NoError(t, d.Set("/b", &yaml.Node{Kind: 99}))

	_, err = d.Marshal()
	assert.ErrorIs(t, err, ErrMarshal)
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

func TestWalk(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte(walkDoc), "spec.yaml")
	require.NoError(t, err)

	var got []string
	d.Walk(func(ptr string, _ *yaml.Node) bool {
		got = append(got, ptr)
		return ptr != "/paths/~1a/get"
	})
	assert.Equal(t, []string{
		"",
		"/paths",
		"/paths/~1a",
		"/paths/~1a/get",
		"/components",
		"/components/schemas",
		"/components/schemas/Pet",
		"/components/schemas/Pet/properties",
		"/components/schemas/Pet/properties/$ref",
		"/components/schemas/Pet/properties/$ref/type",
		"/components/schemas/Pet/properties/owner",
		"/components/schemas/Pet/properties/owner/$ref",
		"/components/schemas/Pet/example",
		"/components/schemas/Pet/example/name",
		"/components/schemas/Copy",
	}, got)
}

func TestRefs(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte(walkDoc), "spec.yaml")
	require.NoError(t, err)

	assert.Equal(t, []Ref{
		{Owner: "/paths/~1a/get/responses/200", Value: "#/components/responses/Ok"},
		{Owner: "/components/schemas/Pet/properties/owner", Value: "models.yaml#/Owner"},
	}, d.Refs())
}

func TestPositions(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte("openapi: 3.1.0\npaths:\n  /a:\n    get: {}\ntags:\n  - name: x\n"), "spec.yaml")
	require.NoError(t, err)

	assert.Equal(t, map[string]diag.Origin{
		"":               {File: "spec.yaml", Line: 1, Col: 1},
		"/openapi":       {File: "spec.yaml", Line: 1, Col: 1},
		"/paths":         {File: "spec.yaml", Line: 2, Col: 1},
		"/paths/~1a":     {File: "spec.yaml", Line: 3, Col: 3},
		"/paths/~1a/get": {File: "spec.yaml", Line: 4, Col: 5},
		"/tags":          {File: "spec.yaml", Line: 5, Col: 1},
		"/tags/0":        {File: "spec.yaml", Line: 6, Col: 5},
		"/tags/0/name":   {File: "spec.yaml", Line: 6, Col: 5},
	}, d.Positions())
}
