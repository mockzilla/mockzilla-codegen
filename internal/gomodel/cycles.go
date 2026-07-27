// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mockzilla/codegen/internal/diag"
)

// tarjan finds strongly connected components in one depth-first pass.
type tarjan struct {
	adj     [][]int
	index   []int
	low     []int
	onStack []bool
	stack   []int
	comp    []int
	next    int
	count   int
}

func (t *tarjan) visit(v int) {
	t.index[v], t.low[v] = t.next, t.next
	t.next++
	t.stack = append(t.stack, v)
	t.onStack[v] = true

	for _, w := range t.adj[v] {
		switch {
		case t.index[w] < 0:
			t.visit(w)
			t.low[v] = min(t.low[v], t.low[w])
		case t.onStack[w]:
			t.low[v] = min(t.low[v], t.index[w])
		}
	}
	if t.low[v] != t.index[v] {
		return
	}

	for {
		w := t.stack[len(t.stack)-1]
		t.stack = t.stack[:len(t.stack)-1]
		t.onStack[w] = false
		t.comp[w] = t.count
		if w == v {
			break
		}
	}
	t.count++
}

// components returns the strongly connected component of every node of a graph.
func components(adj [][]int) []int {
	n := len(adj)
	t := &tarjan{adj: adj, index: make([]int, n), low: make([]int, n), onStack: make([]bool, n), comp: make([]int, n)}
	for i := range t.index {
		t.index[i] = -1
	}
	for v := range adj {
		if t.index[v] < 0 {
			t.visit(v)
		}
	}
	return t.comp
}

// breakAliasCycles points an alias that leads back to itself at any: Go rejects alias cycles.
func breakAliasCycles(decls []*Decl, c *diag.Collector) {
	done := map[*Decl]bool{}
	for _, d := range decls {
		var chain []*Decl
		for cur := d; cur != nil && !done[cur]; cur = aliasTarget(cur) {
			if i := slices.Index(chain, cur); i >= 0 {
				c.Append(aliasCycle(chain[i:]))
				cur.Target = anyType
				break
			}
			chain = append(chain, cur)
		}
		for _, x := range chain {
			done[x] = true
		}
	}
}

func aliasTarget(d *Decl) *Decl {
	if r, ok := d.Target.(DeclRef); ok && d.Kind == KindAlias {
		return r.Decl
	}
	return nil
}

func aliasCycle(cycle []*Decl) diag.Diagnostic {
	names := make([]string, 0, len(cycle)+1)
	for _, d := range cycle {
		names = append(names, d.Name)
	}
	names = append(names, cycle[0].Name)

	return diag.Diagnostic{
		Severity: diag.Warning,
		Code:     diag.CodeAliasCycle,
		Pointer:  cycle[0].ID,
		Origin:   cycle[0].Origin,
		Message:  fmt.Sprintf("types refer to each other in a loop (%s); %s becomes any", strings.Join(names, " = "), cycle[0].Name),
	}
}
