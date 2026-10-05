// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package rules

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

func validItem() Item {
	return Item{Name: "Lamp", Code: "LMP", Tags: []string{"home"}}
}

func TestItemValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(i *Item)
		want string
	}{
		{name: "Valid item"},
		{name: "Name too short", edit: func(i *Item) { i.Name = "L" }, want: "name: must be at least 2 characters long"},
		{name: "Name too long", edit: func(i *Item) { i.Name = "Lamp shade xl" }, want: "name: must be at most 10 characters long"},
		{name: "Pattern from a referenced type", edit: func(i *Item) { i.Code = "lmp" }, want: "code: must match ^[A-Z]{3}$"},
		{name: "UUID format", edit: func(i *Item) { i.ID = new("42") }, want: "id: must be a valid uuid"},
		{name: "URI format", edit: func(i *Item) { i.Site = new("/shop") }, want: "site: must be a valid uri"},
		{name: "Host name format", edit: func(i *Item) { i.Host = new("-shop") }, want: "host: must be a valid hostname"},
		{name: "Email", edit: func(i *Item) { i.Email = new(runtime.Email("shop")) }, want: "email: invalid email address"},
		{name: "Minimum", edit: func(i *Item) { i.Price = new(-1.0) }, want: "price: must be at least 0"},
		{name: "Exclusive maximum", edit: func(i *Item) { i.Price = new(1000.0) }, want: "price: must be less than 1000"},
		{name: "Multiple of", edit: func(i *Item) { i.Price = new(1.005) }, want: "price: must be a multiple of 0.01"},
		{name: "Exclusive minimum", edit: func(i *Item) { i.Count = new(0) }, want: "count: must be greater than 0"},
		{name: "Maximum", edit: func(i *Item) { i.Count = new(6) }, want: "count: must be at most 5"},
		{name: "Required slice", edit: func(i *Item) { i.Tags = nil }, want: "tags: is required"},
		{name: "Min items", edit: func(i *Item) { i.Tags = []string{} }, want: "tags: must have at least 1 items"},
		{name: "Max items", edit: func(i *Item) { i.Tags = []string{"a", "b", "c", "d"} }, want: "tags: must have at most 3 items"},
		{name: "Unique items", edit: func(i *Item) { i.Tags = []string{"a", "a"} }, want: "tags: must have unique items"},
		{name: "Item rule", edit: func(i *Item) { i.Tags = []string{"a", "kitchen"} }, want: "tags[1]: must be at most 5 characters long"},
		{name: "Max properties", edit: func(i *Item) { i.Labels = map[string]string{"a": "1", "b": "2", "c": "3"} }, want: "labels: must have at most 2 properties"},
		{name: "Map value rule", edit: func(i *Item) { i.Labels = map[string]string{"a": "1", "b": ""} }, want: `labels["b"]: must be at least 1 characters long`},
		{name: "Const", edit: func(i *Item) { i.Kind = new("thing") }, want: "kind: must be item"},
		{name: "Enum", edit: func(i *Item) { i.Size = new(Size("xl")) }, want: "size: must be one of s, m, l"},
		{name: "A no-break space is a space", edit: func(i *Item) { i.Word = new("a\U000000A0b") }, want: `word: must match ^\S+$`},
		{name: "A dot is no line end", edit: func(i *Item) { i.Line = new("a\rb") }, want: "line: must match ^.*$"},
		{name: "Bytes count their base64 text", edit: func(i *Item) { i.Token = []byte("hello!!") }, want: "token: must be at most 8 characters long"},
		{name: "The stricter allOf minimum", edit: func(i *Item) { i.Stock = new(5) }, want: "stock: must be at least 10"},
		{name: "Every allOf pattern", edit: func(i *Item) { i.Ref = new("A") }, want: "ref: must match [0-9]$"},
		{
			name: "Several errors",
			edit: func(i *Item) { i.Name, i.Code = "L", "x" },
			want: "name: must be at least 2 characters long; code: must match ^[A-Z]{3}$",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			item := validItem()
			if tc.edit != nil {
				tc.edit(&item)
			}

			err := item.Validate()

			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.want)
			var errs runtime.ValidationErrors
			assert.True(t, errors.As(err, &errs))
		})
	}
}
