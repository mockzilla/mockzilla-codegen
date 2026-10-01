// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

func TestTemplateUnmarshalYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		want    Template
		wantMsg string
	}{
		{name: "Plain text", src: "t: Tenant string\n", want: Template{Text: "Tenant string"}},
		{name: "Text that ends like a file name", src: "t: // see header.tmpl\n", want: Template{Text: "// see header.tmpl"}},
		{name: "Quoted text keeps its line breaks", src: "t: \"\\n\\tTenant string\"\n", want: Template{Text: "\n\tTenant string"}},
		{name: "Block of text", src: "t: |\n  Tenant string\n  Region string\n", want: Template{Text: "Tenant string\nRegion string\n"}},
		{name: "Number is text", src: "t: 5\n", want: Template{Text: "5"}},
		{name: "File", src: "t: {file: ./header.tmpl}\n", want: Template{File: "./header.tmpl"}},
		{name: "File through an alias", src: "base: &b {file: ./header.txt}\nt: *b\n", want: Template{File: "./header.txt"}},
		{name: "No value", src: "t:\n"},
		{name: "Mapping without a file", src: "t: {}\n"},
		{
			name:    "Mapping with another key",
			src:     "t: {text: Tenant string}\n",
			wantMsg: "yaml: construct errors: line 1: field text not found in type config.template",
		},
		{
			name:    "List",
			src:     "t: [Tenant string]\n",
			wantMsg: "yaml: construct errors: line 1: cannot construct !!seq into config.template",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got struct {
				Base any      `yaml:"base"`
				T    Template `yaml:"t"`
			}
			err := yaml.Load([]byte(tc.src), &got)

			if tc.wantMsg != "" {
				require.EqualError(t, err, tc.wantMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.T)
		})
	}
}
