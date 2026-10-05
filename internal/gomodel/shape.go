// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The kind of Go type a schema becomes, read from its keywords.

package gomodel

import (
	"math/bits"
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
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

// classify reads a flattened schema.
func classify(s *spec.Schema) shape {
	t := s.Types &^ spec.TypeNull
	isObject := t == spec.TypeObject || t == 0 && (len(s.Properties) > 0 || s.AdditionalProperties.Mode != spec.AdditionalUnset)
	switch {
	case isUnion(s):
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

// isUnion reports a oneOf or anyOf with more than one member that is not null, if with both then
// and else, or a 3.1 type list with several types.
func isUnion(s *spec.Schema) bool {
	return len(nonNull(s.OneOf))+len(nonNull(s.AnyOf)) > 1 || s.Then != nil && s.Else != nil ||
		bits.OnesCount8(uint8(s.Types&^spec.TypeNull)) > 1
}

// hasShape reports keywords that change the Go type. allOf and a union's sole member are left to
// the caller.
func hasShape(s *spec.Schema) bool {
	return len(s.Properties) > 0 || s.AdditionalProperties.Mode != spec.AdditionalUnset || s.Items != nil ||
		len(s.PrefixItems) > 0 || len(s.Enum) > 0 || isUnion(s) || s.Then != nil || s.Else != nil || hasGoType(s)
}

func hasGoType(s *spec.Schema) bool {
	return slices.ContainsFunc(s.Extensions, func(e spec.Extension) bool { return e.Name == extension.GoType })
}

// members are the schemas a schema is made of besides its own keywords: its allOf members and
// the sole member of a oneOf or anyOf whose other members are null.
func members(s *spec.Schema) []*spec.Schema {
	if m := soleMember(s); m != nil {
		return append(slices.Clip(s.AllOf), m)
	}
	return s.AllOf
}

// soleMember returns the one member of a oneOf or anyOf that is not null, as in
// oneOf: [$ref, {type: null}]. The schema is then that member, nullable.
func soleMember(s *spec.Schema) *spec.Schema {
	list := slices.Concat(nonNull(s.OneOf), nonNull(s.AnyOf))
	if len(list) != 1 {
		return nil
	}
	return list[0]
}

func nonNull(list []*spec.Schema) []*spec.Schema {
	var out []*spec.Schema
	for _, s := range list {
		if !isNull(s) {
			out = append(out, s)
		}
	}
	return out
}

// hasNullMember reports a oneOf or anyOf member that allows only null.
func hasNullMember(s *spec.Schema) bool {
	return slices.ContainsFunc(s.OneOf, isNull) || slices.ContainsFunc(s.AnyOf, isNull)
}

// isNull reports a schema that allows only null: type null, or a null const or enum.
func isNull(s *spec.Schema) bool {
	isNullValue := func(v spec.Value) bool { return v.Kind == spec.KindNull }
	return s.Ref == nil && (s.Types == spec.TypeNull || s.Const != nil && isNullValue(*s.Const) ||
		len(s.Enum) > 0 && !slices.ContainsFunc(s.Enum, func(v spec.Value) bool { return !isNullValue(v) }))
}

// refOf returns the ref a schema stands for: a $ref with only docs and flags next to it, or an
// allOf of one such schema plus members that only add docs and flags.
func refOf(s *spec.Schema) *spec.Ref {
	switch {
	case hasShape(s):
		return nil
	case s.Ref != nil:
		if len(members(s)) == 0 {
			return s.Ref
		}
		return nil
	}

	var ref *spec.Ref
	for _, m := range members(s) {
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

// isBare reports a schema with nothing but docs: no limits, pattern or flags either.
func isBare(s *spec.Schema) bool {
	return isDocOnly(s) && s.Limits == spec.Limits{} && s.Pattern == "" && !s.Nullable && !s.ReadOnly && !s.WriteOnly &&
		!s.Deprecated && s.Default == nil && s.Not == nil && s.PropertyNames == nil && s.ContentEncoding == "" && s.ContentMediaType == ""
}

// isDocOnly reports a schema that adds nothing to a type: a description, flags or limits.
func isDocOnly(s *spec.Schema) bool {
	return s.Ref == nil && len(members(s)) == 0 && !hasShape(s) && s.Types == 0 && s.Format == "" && s.Const == nil
}
