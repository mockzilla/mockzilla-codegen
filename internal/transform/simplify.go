// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package transform

import (
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

const nullType = "null"

// mergedKeys are what a collapsed inline union takes from its variant.
var mergedKeys = []string{"type", "format", "enum", "required", "properties", "items", "additionalProperties"}

type simplifier struct {
	cfg       config.Simplify
	isChanged bool
}

// Simplify rewrites the top-level schemas of components, parameters, bodies and responses so they
// are simpler to generate from, and strips x- keys from every schema under them.
func Simplify(doc *oasdoc.Doc, s config.Simplify) bool {
	sm := &simplifier{cfg: s}
	oasdoc.Visit(doc.Root(), "", oasdoc.KindDocument, func(ptr string, n *yaml.Node, k oasdoc.Kind) bool {
		if k != oasdoc.KindSchema {
			return true
		}
		sm.schema(ptr, n)
		return false
	})
	return sm.isChanged
}

func (sm *simplifier) schema(ptr string, n *yaml.Node) {
	oasdoc.Visit(n, ptr, oasdoc.KindSchema, func(_ string, s *yaml.Node, _ oasdoc.Kind) bool {
		sm.stripExtensions(s)
		return true
	})

	if sm.cfg.Unions {
		sm.collapse(n)
		sm.unionProperties(n)
	}
	if sm.cfg.OptionalProperties != nil {
		sm.capOptional(ptr, n)
	}
}

func (sm *simplifier) stripExtensions(n *yaml.Node) {
	if oasdoc.DeleteChildren(n, func(key string, _ *yaml.Node) bool { return strings.HasPrefix(key, "x-") }) {
		sm.isChanged = true
	}
}

// collapse keeps one variant of each oneOf and anyOf: the first $ref as a one-element allOf, else
// the first variant that is not null, merged into n. A 3.1 type list keeps its first type.
func (sm *simplifier) collapse(n *yaml.Node) {
	for _, key := range []string{"oneOf", "anyOf"} {
		variants := oasdoc.Child(n, key)
		if variants == nil || variants.Kind != yaml.SequenceNode || len(variants.Content) == 0 {
			continue
		}

		oasdoc.DeleteChild(n, key)
		oasdoc.DeleteChild(n, "discriminator")
		sm.isChanged = true
		if i := slices.IndexFunc(variants.Content, func(v *yaml.Node) bool { return oasdoc.Child(v, "$ref") != nil }); i >= 0 {
			all := oasdoc.Child(n, "allOf")
			if all == nil {
				all = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
				oasdoc.SetChild(n, "allOf", all)
			}
			all.Content = append(all.Content, variants.Content[i])
			continue
		}
		merge(n, firstNonNull(variants.Content))
	}

	if t := oasdoc.Child(n, "type"); t != nil && t.Kind == yaml.SequenceNode {
		types, isNullable := typesOf(n)
		if len(types) > 1 {
			setTypes(n, types[:1], isNullable)
			sm.isChanged = true
		}
	}
}

// unionProperties drops optional properties whose schema is a union and collapses required ones.
func (sm *simplifier) unionProperties(n *yaml.Node) {
	props := oasdoc.Child(n, "properties")
	if props == nil || props.Kind != yaml.MappingNode {
		return
	}

	required := scalars(oasdoc.Child(n, "required"))
	isRemoved := oasdoc.DeleteChildren(props, func(name string, p *yaml.Node) bool {
		if !isUnion(p) {
			return false
		}
		if slices.Contains(required, name) {
			sm.collapse(p)
			return false
		}
		return true
	})
	sm.isChanged = sm.isChanged || isRemoved
}

// capOptional keeps the required properties and the alphabetically first optional ones.
func (sm *simplifier) capOptional(ptr string, n *yaml.Node) {
	props := oasdoc.Child(n, "properties")
	if props == nil || props.Kind != yaml.MappingNode {
		return
	}

	required := scalars(oasdoc.Child(n, "required"))
	var optional []string
	for i := 0; i+1 < len(props.Content); i += 2 {
		if name := props.Content[i].Value; !slices.Contains(required, name) {
			optional = append(optional, name)
		}
	}
	slices.Sort(optional)

	keep := sm.optionalCount(ptr)
	if len(optional) <= keep {
		return
	}
	for _, name := range optional[keep:] {
		oasdoc.DeleteChild(props, name)
	}
	sm.isChanged = true
}

// optionalCount is max when min equals max, else a number in [min, max] seeded by the config seed
// and the schema's pointer, so one schema's count never depends on the others.
func (sm *simplifier) optionalCount(ptr string) int {
	lo, hi := sm.cfg.OptionalProperties.Min, sm.cfg.OptionalProperties.Max
	if lo >= hi {
		return hi
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(ptr))
	r := rand.New(rand.NewPCG(uint64(sm.cfg.OptionalProperties.Seed), h.Sum64())) //nolint:gosec // output must repeat for a seed
	return lo + r.IntN(hi-lo+1)
}

// isUnion is true for a oneOf or anyOf, and for a 3.1 type list with more than one type besides null.
func isUnion(n *yaml.Node) bool {
	for _, key := range []string{"oneOf", "anyOf"} {
		if v := oasdoc.Child(n, key); v != nil && v.Kind == yaml.SequenceNode && len(v.Content) > 0 {
			return true
		}
	}
	types, _ := typesOf(n)
	return len(types) > 1
}

// firstNonNull skips variants whose type is only null.
func firstNonNull(variants []*yaml.Node) *yaml.Node {
	for _, v := range variants {
		if types, _ := typesOf(v); len(types) > 0 || oasdoc.Child(v, "type") == nil {
			return v
		}
	}
	return variants[0]
}

// merge copies what n lacks from v; required and properties are joined.
func merge(n, v *yaml.Node) {
	for _, key := range mergedKeys {
		from := oasdoc.Child(v, key)
		if from == nil {
			continue
		}
		to := oasdoc.Child(n, key)
		switch {
		case to == nil:
			oasdoc.SetChild(n, key, oasdoc.Clone(from))
		case key == "required" && to.Kind == yaml.SequenceNode && from.Kind == yaml.SequenceNode:
			for _, c := range from.Content {
				if !slices.Contains(scalars(to), c.Value) {
					to.Content = append(to.Content, oasdoc.Clone(c))
				}
			}
		case key == "properties" && to.Kind == yaml.MappingNode && from.Kind == yaml.MappingNode:
			for i := 0; i+1 < len(from.Content); i += 2 {
				if oasdoc.Child(to, from.Content[i].Value) == nil {
					oasdoc.SetChild(to, from.Content[i].Value, oasdoc.Clone(from.Content[i+1]))
				}
			}
		}
	}
}

// typesOf reads a schema's types without null, and whether null is allowed: 3.0 says so with
// nullable, 3.1 with a "null" entry in a type list.
func typesOf(n *yaml.Node) ([]string, bool) {
	t := oasdoc.Child(n, "type")
	isNullable := oasdoc.Child(n, "nullable") != nil && oasdoc.Child(n, "nullable").Value == "true"
	var all []string
	switch {
	case t == nil:
	case t.Kind == yaml.SequenceNode:
		all = scalars(t)
	default:
		all = []string{t.Value}
	}

	var types []string
	for _, v := range all {
		if v == nullType {
			isNullable = true
			continue
		}
		types = append(types, v)
	}
	return types, isNullable
}

// setTypes writes a 3.1 type list: one type alone, or with null.
func setTypes(n *yaml.Node, types []string, isNullable bool) {
	if isNullable {
		types = append(types, nullType)
	}
	if len(types) == 1 {
		oasdoc.SetChild(n, "type", oasdoc.NewString(types[0]))
		return
	}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, t := range types {
		list.Content = append(list.Content, oasdoc.NewString(t))
	}
	oasdoc.SetChild(n, "type", list)
}
