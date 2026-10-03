// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Reading and changing YAML mappings: get, set and delete a key, new nodes, deep copies.

package oasdoc

import (
	"slices"

	"go.yaml.in/yaml/v4"
)

// Child returns the value under key when n is a mapping, following an alias, or nil.
func Child(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	i := keyIndex(n, key)
	if i < 0 {
		return nil
	}
	if v := n.Content[i+1]; v.Kind == yaml.AliasNode {
		return v.Alias
	}
	return n.Content[i+1]
}

// SetChild puts value under key in mapping n, appending the key when it is missing.
func SetChild(n *yaml.Node, key string, value *yaml.Node) {
	if i := keyIndex(n, key); i >= 0 {
		n.Content[i+1] = value
		return
	}
	n.Content = append(n.Content, NewString(key), value)
}

// DeleteChild removes key from mapping n and reports whether it was there.
func DeleteChild(n *yaml.Node, key string) bool {
	i := keyIndex(n, key)
	if i < 0 {
		return false
	}
	n.Content = slices.Delete(n.Content, i, i+2)
	return true
}

// DeleteChildren removes, in one pass, every entry of mapping n that drop picks, and reports
// whether it removed any.
func DeleteChildren(n *yaml.Node, drop func(key string, value *yaml.Node) bool) bool {
	kept := n.Content[:0]
	for i := 0; i+1 < len(n.Content); i += 2 {
		if !drop(n.Content[i].Value, n.Content[i+1]) {
			kept = append(kept, n.Content[i], n.Content[i+1])
		}
	}
	isRemoved := len(kept) < len(n.Content)
	clear(n.Content[len(kept):])
	n.Content = kept
	return isRemoved
}

func NewString(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: strTag, Value: value}
}

func NewMapping() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

// Clone copies n deeply, keeping lines and columns. An alias becomes a copy of its target.
func Clone(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode {
		return Clone(n.Alias)
	}

	out := *n
	out.Anchor = ""
	out.Content = make([]*yaml.Node, len(n.Content))
	for i, c := range n.Content {
		out.Content[i] = Clone(c)
	}
	return &out
}
