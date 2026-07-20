// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package transform filters, simplifies and prunes a spec as a YAML node tree.
package transform

import (
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/oasdoc"
)

const additionalOperations = "additionalOperations"

var methods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

// operation is one method of a path item; isAdditional marks a 3.2 additionalOperations entry.
type operation struct {
	method       string
	node         *yaml.Node
	isAdditional bool
}

// HasOperations reports whether any path or webhook has an operation.
func HasOperations(doc *oasdoc.Doc) bool {
	for _, m := range []*yaml.Node{doc.Get("/paths"), doc.Get("/webhooks")} {
		for i := 0; m != nil && i+1 < len(m.Content); i += 2 {
			if len(operationsOf(resolveLocal(doc, m.Content[i+1]))) > 0 {
				return true
			}
		}
	}
	return false
}

func operationsOf(item *yaml.Node) []operation {
	var out []operation
	for _, m := range methods {
		if n := oasdoc.Child(item, m); n != nil {
			out = append(out, operation{method: m, node: n})
		}
	}
	extra := oasdoc.Child(item, additionalOperations)
	for i := 0; extra != nil && i+1 < len(extra.Content); i += 2 {
		out = append(out, operation{method: extra.Content[i].Value, node: extra.Content[i+1], isAdditional: true})
	}
	return out
}

// resolveLocal follows a local $ref, as a 3.1 path item to components/pathItems has.
func resolveLocal(doc *oasdoc.Doc, n *yaml.Node) *yaml.Node {
	ref := oasdoc.Child(n, "$ref")
	if ref == nil || !strings.HasPrefix(ref.Value, "#") {
		return n
	}
	if target := doc.Get(ref.Value); target != nil {
		return target
	}
	return n
}

func scalars(n *yaml.Node) []string {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]string, 0, len(n.Content))
	for _, c := range n.Content {
		out = append(out, c.Value)
	}
	return out
}
