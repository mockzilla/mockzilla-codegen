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

func TestFormat(t *testing.T) {
	t.Parallel()

	got, err := Format([]byte("package api\ntype Pet struct{Name string `json:\"name\"`\nAge int}\n"))

	require.NoError(t, err)
	assert.Equal(t, "package api\n\ntype Pet struct {\n\tName string `json:\"name\"`\n\tAge  int\n}\n", string(got))
}

func TestFormatError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "Problem in the middle of the file",
			src:  "package api\n\n// 3\n// 4\n// 5\ntype Pet struct {\n\tName string string\n}\n// 9\n// 10\n// 11\n",
			want: "format generated code: 7:14: expected ';', found string (and 1 more errors)\n" +
				"     4 | // 4\n" +
				"     5 | // 5\n" +
				"     6 | type Pet struct {\n" +
				">    7 | \tName string string\n" +
				"     8 | }\n" +
				"     9 | // 9\n" +
				"    10 | // 10\n",
		},
		{
			name: "Problem below a line directive is counted in the file all the same",
			src:  "package api\n\n//line pets.tmpl:40\ntype Pet struct {\n\tName string string\n}\n",
			want: "format generated code: 5:14: expected ';', found string (and 1 more errors)\n" +
				"     2 | \n" +
				"     3 | //line pets.tmpl:40\n" +
				"     4 | type Pet struct {\n" +
				">    5 | \tName string string\n" +
				"     6 | }\n" +
				"     7 | \n",
		},
		{
			name: "Problem at the end of text without a package clause",
			src:  "var limit =",
			want: "format generated code: 1:12: expected operand, found 'EOF'\n" +
				">    1 | var limit =\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Format([]byte(tc.src))

			require.ErrorIs(t, err, ErrFormat)
			assert.Equal(t, tc.src, string(got))
			assert.EqualError(t, err, tc.want)
		})
	}
}

func TestLocateWithoutSyntaxErrors(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, locate(ErrFormat, "", []byte("package api\n"), 0))
}

func TestExcerptWithoutLine(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "     1 | a\n     2 | b\n", excerpt("a\nb", 0))
}
