// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package stdhttp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	t.Parallel()

	x := newIndex()
	kept := routesOf("GET /a/{x...}", "GET /pets", "GET /pets/{id}", "GET /{rest...}", "POST /pets/{id}")
	var patterns []pattern
	for _, r := range kept {
		patterns = append(patterns, parse(r.Pattern))
		x.add(parse(r.Pattern))
	}

	assert.Equal(t, &index{patterns: patterns, bySize: map[int][]int{1: {1}, 2: {2, 4}}, multi: []int{0, 3}}, x)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, x.candidates(parse("GET /b/{y...}")), "the rest of the path can conflict with anything")
	assert.Equal(t, []int{0, 2, 3, 4}, x.candidates(parse("GET /{y}/b")), "a route of two segments, earliest first")
	assert.Equal(t, []int{0, 3}, x.candidates(parse("GET /a/b/c")), "a route of three segments")
	assert.Equal(t, "overlaps with Op1 at /a/{x...}, and neither is more specific", x.conflict(parse("GET /{y}/b"), kept))
	assert.Equal(t, "matches the same requests as Op5 at /pets/{id}", x.conflict(parse("POST /pets/{petId}"), kept))
	assert.Empty(t, x.conflict(parse("GET /pets/mine"), kept))
}
