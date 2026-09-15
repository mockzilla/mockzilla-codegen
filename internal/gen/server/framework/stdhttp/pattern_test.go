// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package stdhttp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		route string
		want  pattern
	}{
		{name: "Literals and wildcards", route: "GET /pets/{id}", want: pattern{method: "GET", segments: []segment{{literal: "pets"}, {literal: "id", isWild: true}}}},
		{name: "The root", route: "GET /{$}", want: pattern{method: "GET", segments: []segment{{literal: "/"}}}},
		{name: "The rest of the path", route: "DELETE /files/{path...}", want: pattern{method: "DELETE", segments: []segment{{literal: "files"}, {literal: "path", isWild: true, isMulti: true}}}},
		{name: "An escaped literal", route: "GET /caf%C3%A9", want: pattern{method: "GET", segments: []segment{{literal: "café"}}}},
		{name: "An invalid escape stays", route: "GET /100%", want: pattern{method: "GET", segments: []segment{{literal: "100%"}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, parse(tc.route))
		})
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p, q string
		want relation
	}{
		{name: "The same route", p: "GET /pets/{id}", q: "GET /pets/{petId}", want: equivalent},
		{name: "Another method", p: "POST /pets", q: "GET /pets", want: disjoint},
		{name: "GET takes HEAD requests too", p: "GET /pets", q: "HEAD /pets", want: moreGeneral},
		{name: "HEAD is a part of GET", p: "HEAD /pets", q: "GET /pets", want: moreSpecific},
		{name: "Another segment count", p: "GET /pets", q: "GET /pets/{id}", want: disjoint},
		{name: "Another literal", p: "GET /pets/{id}", q: "GET /cats/{id}", want: disjoint},
		{name: "A wildcard over a literal", p: "GET /pets/{id}", q: "GET /pets/mine", want: moreGeneral},
		{name: "A literal under a wildcard", p: "GET /pets/mine", q: "GET /pets/{id}", want: moreSpecific},
		{name: "A wildcard does not match the trailing slash", p: "GET /pets/{id}", q: "GET /pets/{$}", want: disjoint},
		{name: "The trailing slash is no wildcard", p: "GET /pets/{$}", q: "GET /pets/{id}", want: disjoint},
		{name: "The rest over the rest", p: "GET /pets/{a...}", q: "GET /pets/{b...}", want: equivalent},
		{name: "The rest over a wildcard", p: "GET /pets/{rest...}", q: "GET /pets/{id}", want: moreGeneral},
		{name: "A wildcard under the rest", p: "GET /pets/{id}", q: "GET /pets/{rest...}", want: moreSpecific},
		{name: "A shorter route that takes the rest", p: "GET /{rest...}", q: "GET /pets/{id}", want: moreGeneral},
		{name: "A longer route under the rest", p: "GET /pets/{id}/photos", q: "GET /pets/{rest...}", want: moreSpecific},
		{name: "A shorter route without the rest", p: "GET /pets", q: "GET /pets/{rest...}", want: disjoint},
		{name: "Neither more specific", p: "GET /a/{x}", q: "GET /{y}/b", want: overlaps},
		{name: "Neither more specific, then equal", p: "GET /a/{x}/c", q: "GET /{y}/b/c", want: overlaps},
		{name: "Neither more specific in method and path", p: "GET /a/{x}", q: "HEAD /{y}/b", want: overlaps},
		{name: "More general, then equal", p: "GET /{y}/b", q: "GET /a/b", want: moreGeneral},
		{name: "More general, then another literal", p: "GET /{y}/b", q: "GET /a/c", want: disjoint},
		{name: "More general in method, then another literal", p: "GET /a", q: "HEAD /b", want: disjoint},
		{name: "More general twice", p: "GET /{y}/{z}", q: "GET /a/b", want: moreGeneral},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, parse(tc.p).compare(parse(tc.q)))
		})
	}
}
