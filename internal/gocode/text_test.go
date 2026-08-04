// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestComment(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("word ", 30)
	url := "https://example.com/" + strings.Repeat("x", 100)
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "Blank text gives nothing", text: " \n\t\n", want: ""},
		{name: "One line", text: "A pet.", want: "// A pet."},
		{
			name: "Line breaks stay and blank runs become one",
			text: "\nFirst line.\nSecond line.\n\n\n\nNext paragraph.  \n\n",
			want: "// First line.\n// Second line.\n//\n// Next paragraph.",
		},
		{
			name: "Long lines wrap at 100 columns",
			text: long,
			want: "// " + strings.TrimSpace(strings.Repeat("word ", 19)) + "\n// " + strings.TrimSpace(strings.Repeat("word ", 11)),
		},
		{
			name: "Wrapped lines keep their indentation",
			text: "- item\n  " + long,
			want: "// - item\n//   " + strings.TrimSpace(strings.Repeat("word ", 19)) + "\n//   " + strings.TrimSpace(strings.Repeat("word ", 11)),
		},
		{name: "A word longer than the width gets its own line", text: "see " + url + " now", want: "// see\n// " + url + "\n// now"},
		{name: "Characters Go source cannot hold are dropped", text: "a\x00b\r\nc\ufeffd\xffe\x7f", want: "// ab\n// cde"},
		{name: "Tabs stay", text: "\tcode", want: "// \tcode"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Comment(tc.text))
		})
	}
}

func TestQuote(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `"a\"b\n"`, Quote("a\"b\n"))
}

func TestTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []gomodel.Tag
		want string
	}{
		{name: "No tags", want: ""},
		{
			name: "Raw literal",
			tags: []gomodel.Tag{{Key: "json", Value: "name,omitempty"}, {Key: "yaml", Value: "name"}},
			want: "`json:\"name,omitempty\" yaml:\"name\"`",
		},
		{name: "Quotes in a value are escaped", tags: []gomodel.Tag{{Key: "json", Value: `a"b`}}, want: "`json:\"a\\\"b\"`"},
		{name: "Backquote needs an interpreted literal", tags: []gomodel.Tag{{Key: "json", Value: "a`b"}}, want: `"json:\"a` + "`" + `b\""`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Tag(tc.tags))
		})
	}
}

func TestLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value spec.Value
		want  string
	}{
		{name: "String", value: spec.Value{Kind: spec.KindString, Str: "in \"progress\""}, want: `"in \"progress\""`},
		{name: "Number keeps its form", value: spec.Value{Kind: spec.KindNumber, Num: json.Number("1.0")}, want: "1.0"},
		{name: "Bool", value: spec.Value{Kind: spec.KindBool, Bool: true}, want: "true"},
		{name: "Null", value: spec.Value{Kind: spec.KindNull}, want: "nil"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Literal(tc.value))
		})
	}
}
