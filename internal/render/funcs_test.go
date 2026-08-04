// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package render

import (
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestFuncs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		data any
		want string
	}{
		{name: "Comment writes a Go comment", text: `{{comment .}}`, data: "A pet.", want: "// A pet."},
		{name: "ToGoComment is comment under its old name", text: `{{toGoComment .}}`, data: "A pet.", want: "// A pet."},
		{name: "Quote writes a string literal", text: `{{quote .}}`, data: `say "hi"`, want: `"say \"hi\""`},
		{name: "EscapeGoString escapes without quotes", text: `{{escapeGoString .}}`, data: "say \"hi\"\n", want: `say \"hi\"\n`},
		{name: "Tag writes a struct tag", text: `{{tag .}}`, data: []gomodel.Tag{{Key: "json", Value: "id"}}, want: "`json:\"id\"`"},
		{name: "Lower lowers every letter", text: `{{lower .}}`, data: "PetID", want: "petid"},
		{name: "UcFirst raises the first letter", text: `{{ucFirst .}}`, data: "élan", want: "Élan"},
		{name: "UcFirst keeps empty text", text: `{{ucFirst .}}`, data: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpl, err := template.New("").Funcs(funcs()).Parse(tc.text)
			require.NoError(t, err)

			var b strings.Builder
			require.NoError(t, tmpl.Execute(&b, tc.data))
			assert.Equal(t, tc.want, b.String())
		})
	}
}
