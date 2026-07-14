// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

const strTag = "!!str"

// Escape encodes one reference token: ~ becomes ~0 and / becomes ~1.
func Escape(token string) string {
	if !strings.ContainsAny(token, "~/") {
		return token
	}
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

func Unescape(token string) string {
	if !strings.Contains(token, "~") {
		return token
	}
	return strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
}

// Split turns a pointer into unescaped tokens; a # prefix marks a $ref fragment, also percent-decoded.
func Split(ptr string) ([]string, error) {
	isFragment := strings.HasPrefix(ptr, "#")
	raw := strings.TrimPrefix(ptr, "#")
	if raw == "" {
		return nil, nil
	}
	if raw[0] != '/' {
		return nil, fmt.Errorf("%w: %q", ErrPointer, ptr)
	}

	tokens := strings.Split(raw[1:], "/")
	for i, t := range tokens {
		if isFragment {
			decoded, err := url.PathUnescape(t)
			if err != nil {
				return nil, fmt.Errorf("%w: %q: %w", ErrPointer, ptr, err)
			}
			t = decoded
		}
		tokens[i] = Unescape(t)
	}
	return tokens, nil
}

// Get returns the node at ptr, or nil when there is none or ptr is invalid.
func (d *Doc) Get(ptr string) *yaml.Node {
	tokens, err := Split(ptr)
	if err != nil {
		return nil
	}
	return d.lookup(tokens)
}

// Set puts value at ptr. A missing last key is added to its mapping; "-" appends to a sequence.
func (d *Doc) Set(ptr string, value *yaml.Node) error {
	tokens, err := Split(ptr)
	if err != nil {
		return err
	}
	if len(tokens) == 0 {
		if value.Kind != yaml.MappingNode {
			return fmt.Errorf("%w: root must be a mapping", ErrNotObject)
		}
		d.root.Content[0] = value
		return nil
	}

	parent, last := d.lookup(tokens[:len(tokens)-1]), tokens[len(tokens)-1]
	switch {
	case parent == nil:
	case parent.Kind == yaml.MappingNode:
		if i := keyIndex(parent, last); i >= 0 {
			parent.Content[i+1] = value
			return nil
		}
		parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: strTag, Value: last}, value)
		return nil
	case parent.Kind == yaml.SequenceNode && last == "-":
		parent.Content = append(parent.Content, value)
		return nil
	case parent.Kind == yaml.SequenceNode:
		if i, ok := seqIndex(parent, last); ok {
			parent.Content[i] = value
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, ptr)
}

// Delete removes the node at ptr and reports whether there was one. The root cannot be deleted.
func (d *Doc) Delete(ptr string) bool {
	tokens, err := Split(ptr)
	if err != nil || len(tokens) == 0 {
		return false
	}

	parent, last := d.lookup(tokens[:len(tokens)-1]), tokens[len(tokens)-1]
	switch {
	case parent == nil:
	case parent.Kind == yaml.MappingNode:
		if i := keyIndex(parent, last); i >= 0 {
			parent.Content = slices.Delete(parent.Content, i, i+2)
			return true
		}
	case parent.Kind == yaml.SequenceNode:
		if i, ok := seqIndex(parent, last); ok {
			parent.Content = slices.Delete(parent.Content, i, i+1)
			return true
		}
	}
	return false
}

func (d *Doc) lookup(tokens []string) *yaml.Node {
	n := d.Root()
	for _, t := range tokens {
		switch n.Kind {
		case yaml.MappingNode:
			i := keyIndex(n, t)
			if i < 0 {
				return nil
			}
			n = n.Content[i+1]
		case yaml.SequenceNode:
			i, ok := seqIndex(n, t)
			if !ok {
				return nil
			}
			n = n.Content[i]
		default:
			return nil
		}
		if n.Kind == yaml.AliasNode {
			n = n.Alias
		}
	}
	return n
}

func keyIndex(n *yaml.Node, key string) int {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func seqIndex(n *yaml.Node, token string) (int, bool) {
	i, err := strconv.Atoi(token)
	if err != nil || i >= len(n.Content) || token != strconv.Itoa(i) || i < 0 {
		return 0, false
	}
	return i, true
}
