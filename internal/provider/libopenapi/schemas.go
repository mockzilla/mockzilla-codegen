// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// boundSource is one side of a numeric range, with the nodes holding the numbers as written.
type boundSource struct {
	value         *float64
	valueNode     *yaml.Node
	exclusive     *base.DynamicValue[bool, float64]
	exclusiveNode *yaml.Node
}

func (b boundSource) resolve(isLower bool) *spec.Bound {
	var out *spec.Bound
	if b.value != nil {
		out = &spec.Bound{Value: number(b.valueNode, *b.value)}
	}

	switch ex := b.exclusive; {
	case ex == nil:
	case ex.IsA():
		if out != nil {
			out.Exclusive = ex.A
		}
	case out == nil || (isLower && ex.B >= *b.value) || (!isLower && ex.B <= *b.value):
		out = &spec.Bound{Value: number(b.exclusiveNode, ex.B), Exclusive: true}
	}
	return out
}

func typeOf(name string) spec.TypeSet {
	switch name {
	case "string":
		return spec.TypeString
	case "number":
		return spec.TypeNumber
	case "integer":
		return spec.TypeInteger
	case "boolean":
		return spec.TypeBoolean
	case "object":
		return spec.TypeObject
	case "array":
		return spec.TypeArray
	case "null":
		return spec.TypeNull
	default:
		return 0
	}
}

// limits reads numbers from their nodes so they keep the precision and form they were written in.
func limits(h *base.Schema) spec.Limits {
	low := h.GoLow()
	out := spec.Limits{
		Minimum: boundSource{
			value:         h.Minimum,
			valueNode:     low.Minimum.ValueNode,
			exclusive:     h.ExclusiveMinimum,
			exclusiveNode: low.ExclusiveMinimum.ValueNode,
		}.resolve(true),
		Maximum: boundSource{
			value:         h.Maximum,
			valueNode:     low.Maximum.ValueNode,
			exclusive:     h.ExclusiveMaximum,
			exclusiveNode: low.ExclusiveMaximum.ValueNode,
		}.resolve(false),
		MinLength:     clonePtr(h.MinLength),
		MaxLength:     clonePtr(h.MaxLength),
		MinItems:      clonePtr(h.MinItems),
		MaxItems:      clonePtr(h.MaxItems),
		MinProperties: clonePtr(h.MinProperties),
		MaxProperties: clonePtr(h.MaxProperties),
		UniqueItems:   h.UniqueItems != nil && *h.UniqueItems,
	}
	if h.MultipleOf != nil {
		out.MultipleOf = new(number(low.MultipleOf.ValueNode, *h.MultipleOf))
	}
	return out
}

// examples prefers the 3.1 examples list and falls back to the single example.
func examples(h *base.Schema) []spec.Value {
	if len(h.Examples) > 0 {
		return values(h.Examples)
	}
	if h.Example != nil {
		return []spec.Value{value(h.Example)}
	}
	return nil
}

func number(n *yaml.Node, f float64) json.Number {
	if n != nil && isJSONNumber(n.Value) {
		return json.Number(n.Value)
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 64))
}

func isFileName(s string) bool {
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	return new(*p)
}
