// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"math/bits"

	"github.com/mockzilla/codegen/internal/spec"
)

// shape is the kind of Go type a schema becomes. Enums, structs and unions always get a name.
type shape int

const (
	shapeAny shape = iota
	shapePrimitive
	shapeEnum
	shapeStruct
	shapeMap
	shapeArray
	shapeUnion
)

// classify reads a flattened schema. A 3.1 type list with several types is a union.
func classify(s *spec.Schema) shape {
	t := s.Types &^ spec.TypeNull
	isObject := t == spec.TypeObject || t == 0 && (len(s.Properties) > 0 || s.AdditionalProperties.Mode != spec.AdditionalUnset)
	switch {
	case len(s.OneOf) > 0 || len(s.AnyOf) > 0 || bits.OnesCount8(uint8(t)) > 1:
		return shapeUnion
	case len(enumValues(enumKindOf(s), s.Enum)) > 0:
		return shapeEnum
	case isObject && (len(s.Properties) > 0 || s.AdditionalProperties.Mode == spec.AdditionalDenied):
		return shapeStruct
	case isObject:
		return shapeMap
	case t == spec.TypeArray || t == 0 && s.Items != nil:
		return shapeArray
	case t != 0 || s.Format != "" || s.Const != nil:
		return shapePrimitive
	}
	return shapeAny
}

// hasShape reports keywords that change the Go type. allOf is left to the caller.
func hasShape(s *spec.Schema) bool {
	return len(s.Properties) > 0 || s.AdditionalProperties.Mode != spec.AdditionalUnset || s.Items != nil ||
		len(s.PrefixItems) > 0 || len(s.Enum) > 0 || len(s.OneOf) > 0 || len(s.AnyOf) > 0
}

// refOf returns the ref a schema stands for: a $ref with only docs and flags next to it, or an
// allOf of one such schema plus members that only add docs and flags.
func refOf(s *spec.Schema) *spec.Ref {
	switch {
	case hasShape(s):
		return nil
	case s.Ref != nil:
		if len(s.AllOf) == 0 {
			return s.Ref
		}
		return nil
	}

	var ref *spec.Ref
	for _, m := range s.AllOf {
		switch r := refOf(m); {
		case isDocOnly(m):
		case r != nil && ref == nil:
			ref = r
		default:
			return nil
		}
	}
	return ref
}

// siblings are s and its allOf members that only add docs and flags, as in allOf: [$ref, {description}].
func siblings(s *spec.Schema) []*spec.Schema {
	out := []*spec.Schema{s}
	for _, m := range s.AllOf {
		if isDocOnly(m) {
			out = append(out, m)
		}
	}
	return out
}

func description(s *spec.Schema) string {
	for _, x := range siblings(s) {
		if x.Description != "" {
			return x.Description
		}
	}
	return ""
}

// isDocOnly reports a schema that adds nothing to a type: a description, flags or limits.
func isDocOnly(s *spec.Schema) bool {
	return s.Ref == nil && len(s.AllOf) == 0 && !hasShape(s) && s.Types == 0 && s.Format == "" && s.Const == nil
}
