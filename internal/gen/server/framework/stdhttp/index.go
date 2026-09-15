// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package stdhttp

import (
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
)

// index holds the patterns of the routes kept so far, keyed by what a pattern can conflict with:
// the patterns of its segment count, and those that take the rest of the path.
type index struct {
	patterns []pattern
	bySize   map[int][]int
	multi    []int
}

func newIndex() *index {
	return &index{bySize: map[int][]int{}}
}

// add keeps p, the pattern of the next kept route.
func (x *index) add(p pattern) {
	i := len(x.patterns)
	x.patterns = append(x.patterns, p)
	if p.isMulti() {
		x.multi = append(x.multi, i)
		return
	}
	x.bySize[len(p.segments)] = append(x.bySize[len(p.segments)], i)
}

// conflict is why p cannot be registered next to the earliest of the kept routes it conflicts
// with, or empty when it can.
func (x *index) conflict(p pattern, kept []framework.Route) string {
	for _, i := range x.candidates(p) {
		rel := p.compare(x.patterns[i])
		if rel == equivalent {
			return "matches the same requests as " + kept[i].Operation + " at " + kept[i].Path
		}
		if rel == overlaps {
			return "overlaps with " + kept[i].Operation + " at " + kept[i].Path + ", and neither is more specific"
		}
	}
	return ""
}

// candidates are the kept patterns p can conflict with, earliest first: every one for a pattern
// that takes the rest of the path, else those of its segment count and those that take the rest.
func (x *index) candidates(p pattern) []int {
	if p.isMulti() {
		all := make([]int, len(x.patterns))
		for i := range all {
			all[i] = i
		}
		return all
	}
	c := slices.Concat(x.bySize[len(p.segments)], x.multi)
	slices.Sort(c)
	return c
}
