// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package render

import (
	"strconv"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestCheckFuncs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		funcs   template.FuncMap
		wantMsg string
	}{
		{name: "No funcs"},
		{name: "Funcs a template takes", funcs: template.FuncMap{"shout": strings.ToUpper, "atoi": strconv.Atoi, "quote": strconv.Quote}},
		{name: "Name that is no identifier", funcs: template.FuncMap{"to-upper": strings.ToUpper}, wantMsg: `template func: function name "to-upper" is not a valid identifier`},
		{name: "Value that is no func", funcs: template.FuncMap{"shout": "loud"}, wantMsg: "template func: value for shout not a function"},
		{name: "Func without a result", funcs: template.FuncMap{"shout": func(string) {}}, wantMsg: "template func: function shout has 0 return values; should be 1 or 2"},
		{
			name:    "Func whose second result is no error",
			funcs:   template.FuncMap{"cut": func(s string) (string, bool) { return s, true }},
			wantMsg: "template func: invalid function signature for cut: second return value should be error; is bool",
		},
		{
			name:    "First of several by name",
			funcs:   template.FuncMap{"zip": 1, "shout": strings.ToUpper, "cut": 2, "walk": 3},
			wantMsg: "template func: value for cut not a function",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := CheckFuncs(tc.funcs)

			if tc.wantMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrFunc)
			assert.EqualError(t, err, tc.wantMsg)
		})
	}
}

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
