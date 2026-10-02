// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDecls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{name: "No text"},
		{name: "Comment alone", src: "// Nothing is declared here.\n"},
		{name: "Declarations of every kind", src: "\nimport \"embed\"\n\n// Files holds the pages.\nvar Files embed.FS\n\nconst Limit = 8\n\ntype Page struct{}\n\nfunc (Page) Size() int { return Limit }\n"},
		{name: "Declaration that ends without a line break", src: "var Limit = 8 // The most pages."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.NoError(t, CheckDecls("plugin.sample.pages", []byte(tc.src)))
		})
	}
}

func TestCheckDeclsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "Problem on the first line",
			src:  "func Register( {",
			want: "parse generated code: plugin.sample.register:1:16: expected ')', found '{'\n" +
				">    1 | func Register( {\n",
		},
		{
			name: "Problem further down is counted from the first line of the text",
			src:  "\n// 2\n// 3\n// 4\ntype Pet struct {\n\tName string string\n}\n// 8\n// 9\n// 10\n",
			want: "parse generated code: plugin.sample.register:6:14: expected ';', found string (and 1 more errors)\n" +
				"     3 | // 3\n" +
				"     4 | // 4\n" +
				"     5 | type Pet struct {\n" +
				">    6 | \tName string string\n" +
				"     7 | }\n" +
				"     8 | // 8\n" +
				"     9 | // 9\n",
		},
		{
			name: "Problem below a line directive is counted in the text all the same",
			src:  "var Limit = 8\n//line pets.tmpl:40\nfunc Register( {\n}\n",
			want: "parse generated code: plugin.sample.register:3:16: expected ')', found '{' (and 1 more errors)\n" +
				"     1 | var Limit = 8\n" +
				"     2 | //line pets.tmpl:40\n" +
				">    3 | func Register( {\n" +
				"     4 | }\n" +
				"     5 | \n",
		},
		{
			name: "Declaration that is left open",
			src:  "\nfunc Register() {\n",
			want: "parse generated code: plugin.sample.register:2:19: expected '}', found 'EOF'\n" +
				"     1 | \n" +
				">    2 | func Register() {\n" +
				"     3 | \n",
		},
		{
			name: "Statement",
			src:  "routes := 4\n",
			want: "parse generated code: plugin.sample.register:1:1: expected declaration, found routes\n" +
				">    1 | routes := 4\n" +
				"     2 | \n",
		},
		{
			name: "Package clause",
			src:  "package api\n",
			want: "parse generated code: plugin.sample.register:1:1: expected declaration, found 'package'\n" +
				">    1 | package api\n" +
				"     2 | \n",
		},
		{
			name: "Import below a declaration",
			src:  "var Limit = 8\nimport \"embed\"\n",
			want: "parse generated code: plugin.sample.register:2:1: imports must appear before other declarations\n" +
				"     1 | var Limit = 8\n" +
				">    2 | import \"embed\"\n" +
				"     3 | \n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := CheckDecls("plugin.sample.register", []byte(tc.src))

			require.ErrorIs(t, err, ErrParse)
			assert.EqualError(t, err, tc.want)
		})
	}
}

func TestIsFieldType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{name: "Identifier", expr: "int", want: true},
		{name: "Qualified identifier", expr: "trace.Span", want: true},
		{name: "Pointer, slice, array, map and channels", expr: "*[]map[Kind][4]chan<- <-chan Pet", want: true},
		{name: "Func", expr: "func(ctx context.Context, names ...string) (any, error)", want: true},
		{name: "Struct over several lines", expr: "struct {\n\tName string `json:\"name\"`\n}", want: true},
		{name: "Interface", expr: "interface{ Close() error }", want: true},
		{name: "Generic type", expr: "Pair[string, []Pet]", want: true},
		{name: "Type in parentheses", expr: "(int)", want: true},
		{name: "No text"},
		{name: "Func that is left open", expr: "func( any"},
		{name: "Array of an open length", expr: "[...]Pet"},
		{name: "Value", expr: "1 + 2"},
		{name: "Call", expr: "newPet()"},
		{name: "Space before the type", expr: " int"},
		{name: "Space after the type", expr: "int "},
		{name: "Line break before the type", expr: "\nint"},
		{name: "Comment after the type", expr: "int // The count."},
		{name: "Tag after the type", expr: "int `json:\"count\"`"},
		{name: "Second name before the type", expr: ", Other int"},
		{name: "Field after the type", expr: "int\nOther string"},
		{name: "Declaration after the struct is closed", expr: "int\n}\n\ntype Other struct {\n\tName string"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, IsFieldType(tc.expr))
		})
	}
}
