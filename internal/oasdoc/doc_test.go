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

	"github.com/mockzilla/codegen/internal/spec"
)

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
