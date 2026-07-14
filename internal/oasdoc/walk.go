// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"strconv"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/diag"
)

const refKey = "$ref"

// Ref is one $ref in the document. Owner is the pointer of the object that holds it.
type Ref struct {
	Owner string
	Value string
}

// Walk visits nodes depth-first with their pointers; false skips children, aliases are not followed.
func (d *Doc) Walk(fn func(ptr string, n *yaml.Node) bool) {
	walk(d.Root(), "", nil, func(ptr string, _, n *yaml.Node) bool { return fn(ptr, n) })
}

// Refs lists every $ref with a string value, in document order.
func (d *Doc) Refs() []Ref {
	var refs []Ref
	d.Walk(func(ptr string, n *yaml.Node) bool {
		if n.Kind != yaml.MappingNode {
			return true
		}
		if i := keyIndex(n, refKey); i >= 0 && n.Content[i+1].Kind == yaml.ScalarNode {
			refs = append(refs, Ref{Owner: ptr, Value: n.Content[i+1].Value})
		}
		return true
	})
	return refs
}

// Positions maps the pointer of every node to where it starts. A mapping entry starts at its key.
func (d *Doc) Positions() map[string]diag.Origin {
	out := map[string]diag.Origin{}
	walk(d.Root(), "", nil, func(ptr string, key, n *yaml.Node) bool {
		at := n
		if key != nil {
			at = key
		}
		out[ptr] = diag.Origin{File: d.file, Line: at.Line, Col: at.Column}
		return true
	})
	return out
}

func walk(n *yaml.Node, ptr string, key *yaml.Node, fn func(ptr string, key, n *yaml.Node) bool) {
	if !fn(ptr, key, n) {
		return
	}

	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			walk(n.Content[i+1], ptr+"/"+Escape(n.Content[i].Value), n.Content[i], fn)
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			walk(c, ptr+"/"+strconv.Itoa(i), nil, fn)
		}
	default:
	}
}
