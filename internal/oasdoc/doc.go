// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package oasdoc edits an OpenAPI document as a YAML node tree, addressed by JSON pointers.
package oasdoc

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

const (
	defaultIndent = 2
	maxIndent     = 9
	refKey        = "$ref"
)

// Doc is a parsed spec. JSON input is read as YAML and written back as block YAML.
type Doc struct {
	root      *yaml.Node
	file      string
	files     map[*yaml.Node]string
	indent    int
	isCompact bool
}

// Ref is one $ref in the document. Owner is the pointer of the object that holds it.
type Ref struct {
	Owner string
	Value string
}

// cursor is where walk stands: the node's pointer, its mapping key and the file it came from.
type cursor struct {
	ptr  string
	key  *yaml.Node
	file string
}

// Parse reads a spec. file is only used in errors and positions.
func Parse(data []byte, file string) (*Doc, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrParse, file, err)
	}
	if len(n.Content) == 0 || n.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: %s", ErrNotObject, file)
	}

	root := n.Content[0]
	d := &Doc{root: &n, file: file, indent: defaultIndent}
	d.detectLayout(root)
	if root.Style&yaml.FlowStyle != 0 {
		blockStyle(root)
	}
	return d, nil
}

func (d *Doc) Root() *yaml.Node {
	return d.root.Content[0]
}

func (d *Doc) File() string {
	return d.file
}

// Version reads the openapi field. Swagger 2.0 and anything outside 3.0 to 3.2 is unsupported.
func (d *Doc) Version() (spec.Version, error) {
	if n := d.Get("/swagger"); n != nil {
		return 0, fmt.Errorf("%w: swagger %s", ErrUnsupportedVersion, n.Value)
	}

	n := d.Get("/openapi")
	if n == nil {
		return 0, fmt.Errorf("%w: no openapi field", ErrUnsupportedVersion)
	}
	for _, v := range []spec.Version{spec.V30, spec.V31, spec.V32} {
		if prefix := v.String(); n.Value == prefix || strings.HasPrefix(n.Value, prefix+".") {
			return v, nil
		}
	}
	return 0, fmt.Errorf("%w: %s", ErrUnsupportedVersion, n.Value)
}

// Marshal keeps the source indentation; the encoder rewrites scalar tags and quotes in place.
func (d *Doc) Marshal() ([]byte, error) {
	out, err := yaml.Dump(d.root,
		yaml.WithV3Defaults(),
		yaml.WithIndent(d.indent),
		yaml.WithCompactSeqIndent(d.isCompact),
		yaml.WithLineWidth(-1),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMarshal, err)
	}
	return out, nil
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
		SetChild(parent, last, value)
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
		return DeleteChild(parent, last)
	case parent.Kind == yaml.SequenceNode:
		if i, ok := seqIndex(parent, last); ok {
			parent.Content = slices.Delete(parent.Content, i, i+1)
			return true
		}
	}
	return false
}

// Walk visits nodes depth-first with their pointers; false skips children, aliases are not followed.
func (d *Doc) Walk(fn func(ptr string, n *yaml.Node) bool) {
	d.walk(d.Root(), cursor{file: d.file}, func(at cursor, n *yaml.Node) bool { return fn(at.ptr, n) })
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

// Positions maps the pointer of every node to where it starts. A mapping entry starts at its key,
// unless its value came from another file. Nodes made in memory have no line and are left out, so
// lookups fall back to an ancestor.
func (d *Doc) Positions() map[string]diag.Origin {
	out := map[string]diag.Origin{}
	d.walk(d.Root(), cursor{file: d.file}, func(at cursor, n *yaml.Node) bool {
		pos := n
		if _, isGrafted := d.files[n]; !isGrafted && at.key != nil && at.key.Line > 0 {
			pos = at.key
		}
		if pos.Line > 0 {
			out[at.ptr] = diag.Origin{File: at.file, Line: pos.Line, Col: pos.Column}
		}
		return true
	})
	return out
}

// CopyLayout makes d marshal with the indentation and sequence style of from.
func (d *Doc) CopyLayout(from *Doc) {
	d.indent, d.isCompact = from.indent, from.isCompact
}

// SetFile records that n and everything under it came from file, for Positions.
func (d *Doc) SetFile(n *yaml.Node, file string) {
	if d.files == nil {
		d.files = map[*yaml.Node]string{}
	}
	d.files[n] = file
}

func (d *Doc) detectLayout(root *yaml.Node) {
	var hasIndent, hasSeq bool
	for i := 0; i+1 < len(root.Content) && (!hasIndent || !hasSeq); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if len(value.Content) == 0 || value.Content[0].Line == key.Line {
			continue
		}

		switch {
		case value.Kind == yaml.MappingNode && !hasIndent:
			if step := value.Content[0].Column - key.Column; step >= defaultIndent && step <= maxIndent {
				d.indent, hasIndent = step, true
			}
		case value.Kind == yaml.SequenceNode && value.Style&yaml.FlowStyle == 0 && !hasSeq:
			// A block item's column is after its "- ", so a compact item sits two columns right of its key.
			d.isCompact, hasSeq = value.Content[0].Column-2 == key.Column, true
		}
	}
}

func (d *Doc) walk(n *yaml.Node, at cursor, fn func(at cursor, n *yaml.Node) bool) {
	if file, ok := d.files[n]; ok {
		at.file = file
	}
	if !fn(at, n) {
		return
	}

	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			d.walk(n.Content[i+1], cursor{ptr: at.ptr + "/" + Escape(n.Content[i].Value), key: n.Content[i], file: at.file}, fn)
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			d.walk(c, cursor{ptr: at.ptr + "/" + strconv.Itoa(i), file: at.file}, fn)
		}
	default:
	}
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

func blockStyle(n *yaml.Node) {
	n.Style = 0
	for _, c := range n.Content {
		blockStyle(c)
	}
}
