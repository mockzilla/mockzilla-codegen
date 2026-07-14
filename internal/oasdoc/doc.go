// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package oasdoc edits an OpenAPI document as a YAML node tree, addressed by JSON pointers.
package oasdoc

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/spec"
)

const (
	defaultIndent = 2
	maxIndent     = 9
)

// Doc is a parsed spec. JSON input is read as YAML and written back as block YAML.
type Doc struct {
	root      *yaml.Node
	file      string
	indent    int
	isCompact bool
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

func blockStyle(n *yaml.Node) {
	n.Style = 0
	for _, c := range n.Content {
		blockStyle(c)
	}
}
