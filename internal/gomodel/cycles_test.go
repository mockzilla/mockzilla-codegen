// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
)

func TestComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		adj  [][]int
		want []int
	}{
		{name: "No nodes", adj: [][]int{}, want: []int{}},
		{name: "No edges", adj: [][]int{nil, nil}, want: []int{0, 1}},
		{name: "Self loop", adj: [][]int{{0}}, want: []int{0}},
		{name: "Chain", adj: [][]int{{1}, {2}, nil}, want: []int{2, 1, 0}},
		{name: "Two nodes in a loop", adj: [][]int{{1}, {0}}, want: []int{0, 0}},
		{name: "Loop and a tail", adj: [][]int{{1}, {2}, {1, 3}, nil}, want: []int{2, 1, 1, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, components(tt.adj))
		})
	}
}

func TestBreakAliasCycles(t *testing.T) {
	t.Parallel()

	a := &Decl{ID: "/a", Name: "A", Kind: KindAlias}
	b := &Decl{ID: "/b", Name: "B", Kind: KindAlias, Target: DeclRef{Decl: a}}
	a.Target = DeclRef{Decl: b}
	c := &Decl{ID: "/c", Name: "C", Kind: KindAlias, Target: DeclRef{Decl: a}}
	s := &Decl{ID: "/s", Name: "S", Kind: KindStruct}
	d := &Decl{ID: "/d", Name: "D", Kind: KindAlias, Target: DeclRef{Decl: s}}

	var col diag.Collector
	breakAliasCycles([]*Decl{c, a, b, d, s}, &col)

	assert.Equal(t, anyType, a.Target)
	assert.Equal(t, DeclRef{Decl: a}, b.Target)
	assert.Equal(t, DeclRef{Decl: s}, d.Target)
	assert.Equal(t, []diag.Diagnostic{{
		Severity: diag.Warning,
		Code:     diag.CodeAliasCycle,
		Pointer:  "/a",
		Message:  "types refer to each other in a loop (A = B = A); A becomes any",
	}}, col.List())
}

func TestRecursion(t *testing.T) {
	t.Parallel()
	checkGolden(t, "recursion", "recursion", testOptions())
}
