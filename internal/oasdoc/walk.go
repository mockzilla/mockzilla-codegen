// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"strconv"

	"go.yaml.in/yaml/v4"
)

const refKey = "$ref"

// Ref is one $ref in the document. Owner is the pointer of the object that holds it.
type Ref struct {
	Owner string
	Value string
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
